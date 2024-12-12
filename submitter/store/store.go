package store

import (
	"errors"
	"github.com/babylonlabs-io/vigilante/proto"
	"github.com/babylonlabs-io/vigilante/utils"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb"
	pm "google.golang.org/protobuf/proto"
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
	// ErrCorruptedDB For some reason, db on disk representation have changed
	ErrCorruptedDB = errors.New("db is corrupted")
	// ErrNotFound Value not found
	ErrNotFound = errors.New("not found")
)

type StoredCheckpoint struct {
	Tx1   *wire.MsgTx
	Tx2   *wire.MsgTx
	Epoch uint64
}

func NewStoredCheckpoint(tx1, tx2 *wire.MsgTx, epoch uint64) *StoredCheckpoint {
	return &StoredCheckpoint{
		Tx1:   tx1,
		Tx2:   tx2,
		Epoch: epoch,
	}
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

func (s *SubmitterStore) LatestCheckpoint() (*StoredCheckpoint, bool, error) {
	data, f, err := s.get(lastSubmittedCkptKey, storedCheckpointBucketName)
	if err != nil {
		return nil, false, err
	} else if !f {
		return nil, false, nil
	}

	protoCkpt := &proto.StoredCheckpoint{}
	if err := pm.Unmarshal(data, protoCkpt); err != nil {
		return nil, false, err
	}

	var storedCkpt StoredCheckpoint
	if err := storedCkpt.FromProto(protoCkpt); err != nil {
		return nil, false, err
	}

	return &storedCkpt, true, err
}

func (s *SubmitterStore) PutCheckpoint(ckpt *StoredCheckpoint) error {
	protoCkpt, err := ckpt.ToProto()
	if err != nil {
		return err
	}

	data, err := pm.Marshal(protoCkpt)
	if err != nil {
		return err
	}

	return s.put(lastSubmittedCkptKey, data, storedCheckpointBucketName)
}

func (s *SubmitterStore) get(key, bucketName []byte) ([]byte, bool, error) {
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

func (s *SubmitterStore) put(key []byte, val []byte, bucketName []byte) error {
	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		bucket := tx.ReadWriteBucket(bucketName)
		if bucket == nil {
			return ErrCorruptedDB
		}

		return bucket.Put(key, val)
	})
}

// ToProto converts StoredTx to its Protobuf equivalent
func (s *StoredCheckpoint) ToProto() (*proto.StoredCheckpoint, error) {
	bufTx1, err := utils.SerializeMsgTx(s.Tx1)
	if err != nil {
		return nil, err
	}

	bufTx2, err := utils.SerializeMsgTx(s.Tx2)
	if err != nil {
		return nil, err
	}

	return &proto.StoredCheckpoint{
		Tx1:   bufTx1,
		Tx2:   bufTx2,
		Epoch: s.Epoch,
	}, nil
}

// FromProto converts a Protobuf StoredTx to the Go struct
func (s *StoredCheckpoint) FromProto(protoTx *proto.StoredCheckpoint) error {
	var err error
	tx1, err := utils.DeserializeMsgTx(protoTx.Tx1)
	if err != nil {
		return err
	}

	tx2, err := utils.DeserializeMsgTx(protoTx.Tx2)
	if err != nil {
		return err
	}

	s.Tx1 = tx1
	s.Tx2 = tx2
	s.Epoch = protoTx.Epoch

	return nil
}
