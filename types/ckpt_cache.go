package types // nolint:revive

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/babylonlabs-io/babylon/v4/btctxformatter"
)

type CheckpointCache struct {
	mu      sync.Mutex
	Tag     btctxformatter.BabylonTag
	Version btctxformatter.FormatVersion

	// list that contains matched checkpoints
	Checkpoints []*Ckpt

	// map that contains checkpoint segments
	// first key: index of the segment in the checkpoint (0 or 1)
	// second key: hash of the OP_RETURN data in this ckpt segment
	Segments map[uint8]map[string]*CkptSegment

	// attemptedPairs tracks (hash(part0) | hash(part1)) keys for matched pairs
	// that have already been submitted to Babylon and rejected with a
	// non-transient error. Match() skips these so the same poisoned proof
	// is not resubmitted on every cycle. Entries are pruned by the cleanup
	// routine using the same TTL applied to segments.
	attemptedPairs map[string]time.Time
}

func NewCheckpointCache(tag btctxformatter.BabylonTag, version btctxformatter.FormatVersion) *CheckpointCache {
	segMap := map[uint8]map[string]*CkptSegment{}
	for i := uint8(0); i < btctxformatter.NumberOfParts; i++ {
		segMap[i] = map[string]*CkptSegment{}
	}

	return &CheckpointCache{
		Tag:            tag,
		Version:        version,
		Checkpoints:    []*Ckpt{},
		Segments:       segMap,
		attemptedPairs: map[string]time.Time{},
	}
}

func (c *CheckpointCache) AddSegment(ckptSeg *CkptSegment) error {
	if ckptSeg.Index >= btctxformatter.NumberOfParts {
		return fmt.Errorf("the index of the ckpt segment in block %v is out of scope: got %d, at most %d", ckptSeg.AssocBlock.BlockHash(), ckptSeg.Index, btctxformatter.NumberOfParts-1)
	}
	hash := sha256.Sum256(ckptSeg.Data)
	c.mu.Lock()
	ckptSeg.Timestamp = time.Now() // Store insertion time, for TTL
	c.Segments[ckptSeg.Index][string(hash[:])] = ckptSeg
	c.mu.Unlock()

	return nil
}

func (c *CheckpointCache) AddCheckpoint(ckpt *Ckpt) {
	c.Checkpoints = append(c.Checkpoints, ckpt)
}

func (c *CheckpointCache) sortCheckpoints() {
	// Sort the matched pairs by epoch, since they have to be submitted in order
	// TODO: find smarter way for sorting
	sort.Slice(c.Checkpoints, func(i, j int) bool {
		return c.Checkpoints[i].Epoch < c.Checkpoints[j].Epoch
	})
}

// pairKey returns a stable key identifying the (part0, part1) pair by the
// sha256 of each segment's OP_RETURN data. This matches the key shape used
// for c.Segments so the same hash bytes are reused.
func pairKey(seg0, seg1 *CkptSegment) string {
	h0 := sha256.Sum256(seg0.Data)
	h1 := sha256.Sum256(seg1.Data)

	return string(h0[:]) + "|" + string(h1[:])
}

// TODO: generalise to NumExpectedProofs > 2
// TODO: optimise the complexity by hashmap
//
// Match scans cached part0/part1 segments and queues every (part0, part1)
// pair that connects and decodes into a valid raw checkpoint. Pairs that
// were previously submitted and rejected with a non-transient error are
// skipped via attemptedPairs. Segments are NOT deleted here, so the same
// part0 can later pair with a different part1 if the first attempt is
// rejected by Babylon (see VIG-02 fix).
func (c *CheckpointCache) Match() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, ckptSeg1 := range c.Segments[uint8(0)] {
		for _, ckptSeg2 := range c.Segments[uint8(1)] {
			if _, ok := c.attemptedPairs[pairKey(ckptSeg1, ckptSeg2)]; ok {
				// this pair was submitted before and rejected; skip it
				// to avoid resubmitting the same proof
				continue
			}
			connected, err := btctxformatter.ConnectParts(c.Version, ckptSeg1.Data, ckptSeg2.Data)
			if err != nil {
				continue
			}
			// found a pair, check if it is a valid checkpoint
			rawCheckpoint, err := btctxformatter.DecodeRawCheckpoint(c.Version, connected)
			if err != nil {
				continue
			}
			// queue the matched checkpoint candidate. Segments stay in the
			// cache until submission succeeds (RemoveSegments) or the pair
			// is rejected by Babylon (MarkAttempted).
			ckpt := NewCkpt(ckptSeg1, ckptSeg2, rawCheckpoint.Epoch)
			c.AddCheckpoint(ckpt)
		}
	}

	// this ensures that checkpoints in the cache is always in order
	c.sortCheckpoints()
}

func (c *CheckpointCache) PopEarliestCheckpoint() *Ckpt {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.Checkpoints) > 0 {
		ckpt := c.Checkpoints[0]
		c.Checkpoints = c.Checkpoints[1:]

		return ckpt
	}

	return nil
}

// MarkAttempted records that the (part0, part1) pair backing the given
// matched checkpoint was submitted to Babylon and rejected with a
// non-transient error. Subsequent Match() calls will skip this pair so
// the same poisoned proof is not resubmitted. Segments are intentionally
// kept in the cache so part0 can still pair with a different part1
// (e.g. the legitimate one) on a later Match() call.
func (c *CheckpointCache) MarkAttempted(ckpt *Ckpt) {
	if ckpt == nil || len(ckpt.Segments) != int(btctxformatter.NumberOfParts) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attemptedPairs[pairKey(ckpt.Segments[0], ckpt.Segments[1])] = time.Now()
}

// RemoveSegments removes the segments composing the given matched checkpoint
// from the cache. Called after the proof is accepted by Babylon (or returns
// an expected duplicate/finalized response) so the pair is not re-matched
// on the next Match() call. Also clears any attemptedPairs entry for the
// pair, since the pair has now been resolved.
func (c *CheckpointCache) RemoveSegments(ckpt *Ckpt) {
	if ckpt == nil || len(ckpt.Segments) != int(btctxformatter.NumberOfParts) {
		return
	}
	h0 := sha256.Sum256(ckpt.Segments[0].Data)
	h1 := sha256.Sum256(ckpt.Segments[1].Data)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Segments[uint8(0)], string(h0[:]))
	delete(c.Segments[uint8(1)], string(h1[:]))
	delete(c.attemptedPairs, string(h0[:])+"|"+string(h1[:]))
}

func (c *CheckpointCache) NumSegments() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := 0
	for _, segMap := range c.Segments {
		size += len(segMap)
	}

	return size
}

func (c *CheckpointCache) NumCheckpoints() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.Checkpoints)
}

// NumAttemptedPairs reports how many (part0, part1) pairs are currently
// blacklisted from re-matching due to prior rejection by Babylon. Intended
// for tests and observability.
func (c *CheckpointCache) NumAttemptedPairs() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.attemptedPairs)
}

func (c *CheckpointCache) StartCleanupRoutine(stopChan chan struct{}, cleanupInterval time.Duration, segmentTTL time.Duration) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			now := time.Now()
			c.mu.Lock()
			for _, segMap := range c.Segments {
				for hash, seg := range segMap {
					if now.Sub(seg.Timestamp) > segmentTTL {
						delete(segMap, hash)
					}
				}
			}
			// prune attemptedPairs entries that have outlived the segment
			// TTL so the set does not grow unbounded
			for key, ts := range c.attemptedPairs {
				if now.Sub(ts) > segmentTTL {
					delete(c.attemptedPairs, key)
				}
			}
			c.mu.Unlock()
		}
	}
}
