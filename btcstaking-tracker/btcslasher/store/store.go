package store

import (
	"encoding/binary"
	"errors"

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
		_, err := tx.CreateTopLevelBucket(processedEvidenceHeightBucket)
		if err != nil {
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

func uint64ToBytes(n uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, n)

	return buf
}

func bytesToUint64(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}
