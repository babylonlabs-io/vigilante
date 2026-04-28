package store

import (
	"encoding/binary"
	"errors"

	ftypes "github.com/babylonlabs-io/babylon/v4/x/finality/types"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb"
)

type SlasherStore struct {
	db kvdb.Backend
}

var (
	// ErrCorruptedDB For some reason, db on disk representation have changed
	ErrCorruptedDB = errors.New("db is corrupted")
	// ErrNotFound Value not found
	ErrNotFound = errors.New("not found")
)

var (
	processedEvidenceHeightBucket = []byte("processedevidenceheight")
	processedEvidenceHeightKey    = []byte("lastevidenceheight")

	pendingEvidencesBucket = []byte("pendingevidences")
)

func NewSlasherStore(backend kvdb.Backend) (*SlasherStore, error) {
	store := &SlasherStore{db: backend}
	if err := store.createBucket(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *SlasherStore) createBucket() error {
	if err := s.db.Update(func(tx kvdb.RwTx) error {
		if _, err := tx.CreateTopLevelBucket(processedEvidenceHeightBucket); err != nil {
			return err
		}
		if _, err := tx.CreateTopLevelBucket(pendingEvidencesBucket); err != nil {
			return err
		}

		return nil
	}, func() {}); err != nil {
		return err
	}

	return nil
}

func (s *SlasherStore) get(key, bucketName []byte) ([]byte, bool, error) {
	var returnVal []byte

	err := s.db.View(func(tx walletdb.ReadTx) error {
		b := tx.ReadBucket(bucketName)
		if b == nil {
			return ErrCorruptedDB
		}

		byteVal := b.Get(key)
		if byteVal == nil {
			return ErrNotFound
		}

		returnVal = byteVal

		return nil
	}, func() {})

	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, false, nil
		}

		return nil, false, err
	}

	return returnVal, true, nil
}

func (s *SlasherStore) put(key []byte, val []byte, bucketName []byte) error {
	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		bucket := tx.ReadWriteBucket(bucketName)
		if bucket == nil {
			return ErrCorruptedDB
		}

		return bucket.Put(key, val)
	})
}

func (s *SlasherStore) delete(key []byte, bucketName []byte) error {
	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		bucket := tx.ReadWriteBucket(bucketName)
		if bucket == nil {
			return ErrCorruptedDB
		}

		return bucket.Delete(key)
	})
}

func (s *SlasherStore) LastProcessedHeight() (uint64, bool, error) {
	data, f, err := s.get(processedEvidenceHeightKey, processedEvidenceHeightBucket)
	if err != nil {
		return 0, false, err
	} else if !f {
		return 0, false, nil
	}

	return bytesToUint64(data), true, err
}

func (s *SlasherStore) PutHeight(height uint64) error {
	return s.put(processedEvidenceHeightKey, uint64ToBytes(height), processedEvidenceHeightBucket)
}

// PutPendingEvidence persists an evidence that the slasher has dispatched but
// has not yet confirmed durably-completed (i.e., BTC tx K-deep or all paths
// non-slashable). Key is composed of big-endian block height followed by the
// FP BTC PK hex bytes, so iteration is sorted by height.
func (s *SlasherStore) PutPendingEvidence(ev *ftypes.EvidenceResponse) error {
	if ev == nil {
		return errors.New("nil evidence")
	}
	key := pendingEvidenceKey(ev.BlockHeight, ev.FpBtcPkHex)
	val, err := ev.Marshal()
	if err != nil {
		return err
	}

	return s.put(key, val, pendingEvidencesBucket)
}

// DeletePendingEvidence removes a pending evidence after all per-delegation
// goroutines for that FP at that height have reached a terminal state.
// Deleting a missing key is a no-op.
func (s *SlasherStore) DeletePendingEvidence(blockHeight uint64, fpBtcPkHex string) error {
	return s.delete(pendingEvidenceKey(blockHeight, fpBtcPkHex), pendingEvidencesBucket)
}

// ListPendingEvidences returns all pending evidences in ascending block-height
// order (and lexicographic by fp pk within the same height).
func (s *SlasherStore) ListPendingEvidences() ([]*ftypes.EvidenceResponse, error) {
	var out []*ftypes.EvidenceResponse
	err := s.db.View(func(tx walletdb.ReadTx) error {
		b := tx.ReadBucket(pendingEvidencesBucket)
		if b == nil {
			return ErrCorruptedDB
		}

		return b.ForEach(func(_, v []byte) error {
			var ev ftypes.EvidenceResponse
			if err := ev.Unmarshal(v); err != nil {
				return err
			}
			out = append(out, &ev)

			return nil
		})
	}, func() {})
	if err != nil {
		return nil, err
	}

	return out, nil
}

func pendingEvidenceKey(blockHeight uint64, fpBtcPkHex string) []byte {
	// 8 bytes BE height || fp pk hex bytes — BE ensures ascending iteration by height
	key := make([]byte, 0, 8+len(fpBtcPkHex))
	heightBE := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBE, blockHeight)
	key = append(key, heightBE...)
	key = append(key, []byte(fpBtcPkHex)...)

	return key
}

func uint64ToBytes(n uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, n)

	return buf
}

func bytesToUint64(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}
