package store

import (
	"errors"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/kvdb"
)

type SubmitterStore struct {
	db kvdb.Backend
}

var (
	// storing submitted txns
	storedCheckpointBucketName = []byte("storedckpt")
	lastSubmittedCkptKey       = []byte("lastsubckpt")
)

var (
	// ErrCorruptedDb For some reason, db on disk representation have changed
	ErrCorruptedDb = errors.New("db is corrupted")
	// ErrNotFound Value not found
	ErrNotFound = errors.New("not found")
)

type StoredTx struct {
	TxId *chainhash.Hash // we store this to check against the mempool
	Tx   *wire.MsgTx     // we store this in case we need to resubmit
}

type StoredCheckpoint struct {
	Tx1   StoredTx
	Tx2   StoredTx
	Epoch uint64
}

func NewSubmitterStore(backend kvdb.Backend) (*SubmitterStore, error) {
	store := &SubmitterStore{db: backend}
	if err := store.createBuckets(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *SubmitterStore) createBuckets() error {
	buckets := [][]byte{storedCheckpointBucketName}
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
