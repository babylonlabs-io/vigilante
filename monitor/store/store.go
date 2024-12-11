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
	// ErrCorruptedDB For some reason, db on disk representation have changed
	ErrCorruptedDB = errors.New("db is corrupted")
	// ErrNotFound Value not found
	ErrNotFound = errors.New("not found")
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
		if err := s.db.Update(func(tx kvdb.RwTx) error {
			_, err := tx.CreateTopLevelBucket(bucket)
			if err != nil {
				return err
			}

			return nil
		}, func() {}); err != nil {
			return err
		}
	}

	return nil
}

func (s *MonitorStore) LatestEpoch() (uint64, bool, error) {
	return s.get(latestEpochKey, epochsBucketName)
}

func (s *MonitorStore) LatestHeight() (uint64, bool, error) {
	return s.get(latestHeightKey, heightBucketName)
}

func (s *MonitorStore) PutLatestEpoch(epoch uint64) error {
	return s.put(latestEpochKey, epoch, epochsBucketName)
}

func (s *MonitorStore) PutLatestHeight(height uint64) error {
	return s.put(latestHeightKey, height, heightBucketName)
}

func (s *MonitorStore) get(key, bucketName []byte) (uint64, bool, error) {
	var returnVal uint64

	err := s.db.View(func(tx walletdb.ReadTx) error {
		b := tx.ReadBucket(bucketName)
		if b == nil {
			return ErrCorruptedDB
		}

		byteVal := b.Get(key)
		if byteVal == nil {
			return ErrNotFound
		}

		val, err := uint64FromBytes(byteVal)
		if err != nil {
			return err
		}
		returnVal = val

		return nil
	}, func() {})

	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return 0, false, nil
		}

		return 0, false, err
	}

	return returnVal, true, nil
}

func (s *MonitorStore) put(key []byte, val uint64, bucketName []byte) error {
	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		bucket := tx.ReadWriteBucket(bucketName)
		if bucket == nil {
			return ErrCorruptedDB
		}

		return bucket.Put(key, uint64ToBytes(val))
	})
}

// Converts a byte slice to a uint64 value.
func uint64FromBytes(b []byte) (uint64, error) {
	if len(b) != 8 {
		return 0, fmt.Errorf("invalid byte slice length: expected 8, got %d", len(b))
	}

	return binary.BigEndian.Uint64(b), nil
}

// Converts an uint64 value to a byte slice.
func uint64ToBytes(v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)

	return buf[:]
}
