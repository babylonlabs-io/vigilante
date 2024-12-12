package types

import (
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"sync"
)

// PrivateKeyWithMutex wraps a btcec.PrivateKey with a mutex to ensure thread-safe access.
type PrivateKeyWithMutex struct {
	mu  sync.Mutex
	key *secp256k1.PrivateKey
}

// NewPrivateKeyWithMutex creates a new PrivateKeyWithMutex.
func NewPrivateKeyWithMutex(key *secp256k1.PrivateKey) *PrivateKeyWithMutex {
	return &PrivateKeyWithMutex{
		key: key,
	}
}

// GetKey safely retrieves the private key.
func (p *PrivateKeyWithMutex) GetKey() *secp256k1.PrivateKey {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.key
}

// UseKey performs an operation with the private key in a thread-safe manner.
func (p *PrivateKeyWithMutex) UseKey(operation func(key *secp256k1.PrivateKey)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	operation(p.key)
}
