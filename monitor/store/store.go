package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb"
)

type MonitorStore struct {
	db kvdb.Backend
}

var (
	// storing last submitted epoch
	epochsBucketName = []byte("epoch")
	heightBucketName = []byte("height")
	latestEpochKey   = []byte("ltsepoch")
	latestHeightKey  = []byte("ltsheight")
)

var (
	// ErrCorruptedDb For some reason, db on disk representation have changed
	ErrCorruptedDb = errors.New("db is corrupted")
	ErrNotFound    = errors.New("not found")
)

func NewMonitorStore(backend kvdb.Backend) (*MonitorStore, error) {
	store := &MonitorStore{db: backend}
	if err := store.createBuckets(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *MonitorStore) createBuckets() error {
	buckets := [][]byte{epochsBucketName, heightBucketName}
	for _, bucket := range buckets {
		if err := kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
			_, err := tx.CreateTopLevelBucket(bucket)
			if err != nil {
				return err
			}

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func (s *MonitorStore) LatestEpoch() (uint64, error) {
	var latestEpoch uint64

	if err := s.db.View(func(tx walletdb.ReadTx) error {
		epochsBucket := tx.ReadBucket(epochsBucketName)
		if epochsBucket == nil {
			return ErrCorruptedDb
		}

		value := epochsBucket.Get(latestEpochKey)

		if value == nil {
			latestEpoch = 0
			return nil
		}

		epoch, err := uint64FromBytes(value)
		if err != nil {
			return err
		}
		latestEpoch = epoch

		return nil
	}, func() {
	}); err != nil {
		return 0, err
	}

	return latestEpoch, nil
}

func (s *MonitorStore) PutLatestEpoch(epoch uint64) error {
	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		bucket := tx.ReadWriteBucket(epochsBucketName)
		if bucket == nil {
			return ErrCorruptedDb
		}

		return bucket.Put(latestEpochKey, uint64ToBytes(epoch))
	})
}

// Converts a byte slice to a uint64 value.
func uint64FromBytes(b []byte) (uint64, error) {
	if len(b) != 8 {
		return 0, fmt.Errorf("invalid byte slice length: expected 8, got %d", len(b))
	}
	return binary.BigEndian.Uint64(b), nil
}

// Converts a uint64 value to a byte slice.
func uint64ToBytes(v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	return buf[:]
}
