package types

import (
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"sync"
)

var (
	ErrEmptyCache        = errors.New("empty cache")
	ErrInvalidStopHeight = errors.New("invalid stop height")
)

type BTCCache struct {
	blocks     []*IndexedBlock
	maxEntries uint64

	sync.RWMutex
}

func NewBTCCache(maxEntries uint64) *BTCCache {
	return &BTCCache{
		blocks:     make([]*IndexedBlock, 0, maxEntries),
		maxEntries: maxEntries,
	}
}

func (b *BTCCache) Init(ibs []*IndexedBlock) error {
	b.Lock()
	defer b.Unlock()

	if b.maxEntries != 0 && len(ibs) > int(b.maxEntries) {
		return fmt.Errorf("the number of blocks is more than maxEntries")
	}
	for _, ib := range ibs {
		b.Add(ib)
	}
	b.reverse()
	return nil
}

func (b *BTCCache) Add(ib *IndexedBlock) {
	b.Lock()
	defer b.Unlock()

	if b.maxEntries == 0 {
		return
	}

	if uint64(len(b.blocks)) == b.maxEntries {
		b.blocks = b.blocks[1:]
	}

	b.blocks = append(b.blocks, ib)
}

func (b *BTCCache) Tip() *IndexedBlock {
	b.RLock()
	defer b.RUnlock()

	if b.maxEntries == 0 {
		return nil
	}

	return b.blocks[len(b.blocks)-1]
}

// Delete deletes the block at the given height from cache
func (b *BTCCache) Delete(blockHeight uint64, blockHash chainhash.Hash) {
	b.Lock()
	defer b.Unlock()

	for i := len(b.blocks) - 1; i >= 0; i-- {
		// block not found
		if b.blocks[i].Height < int32(blockHeight) {
			return
		}

		// block found
		if b.blocks[i].Height == int32(blockHeight) && b.blocks[i].BlockHash().String() == blockHash.String() {
			b.blocks = append(b.blocks[:i], b.blocks[i+1:]...)
			return
		}
	}
}

func (b *BTCCache) Rebuild(stopHeight uint64, lastBtcBlocks []*IndexedBlock) error {
	b.Lock()
	defer b.Unlock()

	if b.Size() == 0 || b.maxEntries == 0 {
		return ErrEmptyCache
	}

	if stopHeight > uint64(b.Tip().Height) {
		return ErrInvalidStopHeight
	}

	var j int
	for i := len(b.blocks) - 1; i >= 0; i-- {
		if b.blocks[i].Height == int32(stopHeight) {
			j = i
			break
		}
	}

	b.blocks = b.blocks[:j+1]
	b.blocks = append(b.blocks, lastBtcBlocks...)
	return nil
}

func (b *BTCCache) Size() uint64 {
	b.RLock()
	defer b.RUnlock()

	return uint64(len(b.blocks))
}

func (b *BTCCache) reverse() {
	b.Lock()
	defer b.Unlock()

	for i, j := 0, len(b.blocks)-1; i < j; i, j = i+1, j-1 {
		b.blocks[i], b.blocks[j] = b.blocks[j], b.blocks[i]
	}
}

// GetLastBlocks returns list of blocks between the given stopHeight and the tip of the chain in cache
func (b *BTCCache) GetLastBlocks(stopHeight uint64) ([]*IndexedBlock, error) {
	b.RLock()
	defer b.RUnlock()

	firstHeight := b.blocks[0].Height
	lastHeight := b.blocks[len(b.blocks)-1].Height
	if int32(stopHeight) < firstHeight || lastHeight < int32(stopHeight) {
		return []*IndexedBlock{}, fmt.Errorf("the given stopHeight %d is out of range [%d, %d] of BTC cache", stopHeight, firstHeight, lastHeight)
	}

	var j int
	for i := len(b.blocks) - 1; i >= 0; i-- {
		if b.blocks[i].Height == int32(stopHeight) {
			j = i
			break
		}
	}

	return b.blocks[j:], nil
}

// FindBlock finds block at the given height in cache
func (b *BTCCache) FindBlock(blockHeight uint64) *IndexedBlock {
	b.RLock()
	defer b.RUnlock()

	firstHeight := b.blocks[0].Height
	lastHeight := b.blocks[len(b.blocks)-1].Height
	if int32(blockHeight) < firstHeight || lastHeight < int32(blockHeight) {
		return nil
	}

	for i := len(b.blocks) - 1; i >= 0; i-- {
		if b.blocks[i].Height == int32(blockHeight) {
			return b.blocks[i]
		}
	}

	return nil
}

// TrimToSized trims BTCCache `b` to only keep the latest `maxEntries` blocks, and set `maxEntries` to be the cache size
// If `b` contains no more than `maxEntries` blocks, then assign all blocks to the new cache
func (b *BTCCache) TrimToSized(maxEntries uint64) *BTCCache {
	b.RLock()
	defer b.RUnlock()

	newCache := NewBTCCache(maxEntries)
	if maxEntries < b.Size() {
		newCache.blocks = b.blocks[b.Size()-maxEntries:]
	} else {
		newCache.blocks = b.blocks
	}
	return newCache
}
