package ethkeystore

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	hdwallet "github.com/miguelmota/go-ethereum-hdwallet"
)

const (
	// DefaultDerivationPath is the standard Ethereum derivation path (BIP-44)
	DefaultDerivationPath = "m/44'/60'/0'/0/0"

	// PasswordEnvVar is the environment variable for keystore password
	PasswordEnvVar = "VIGILANTE_ETH_KEYSTORE_PASSWORD"
)

// LoadKeystore creates a new keystore instance from a directory.
// If the directory doesn't exist, it will be created.
func LoadKeystore(dir string) (*keystore.KeyStore, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create keystore directory: %w", err)
	}

	// Use standard scrypt parameters for production security
	ks := keystore.NewKeyStore(dir, keystore.StandardScryptN, keystore.StandardScryptP)

	return ks, nil
}

// FindAccount finds an account by address in the keystore.
func FindAccount(ks *keystore.KeyStore, address common.Address) (accounts.Account, error) {
	account, err := ks.Find(accounts.Account{Address: address})
	if err != nil {
		return accounts.Account{}, fmt.Errorf("account %s not found in keystore: %w", address.Hex(), err)
	}

	return account, nil
}

// UnlockAccount unlocks an account and returns its private key.
func UnlockAccount(ks *keystore.KeyStore, address common.Address, password string) (*ecdsa.PrivateKey, error) {
	account, err := FindAccount(ks, address)
	if err != nil {
		return nil, err
	}

	// Read the key file
	keyJSON, err := os.ReadFile(account.URL.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read keystore file: %w", err)
	}

	// Decrypt the key
	key, err := keystore.DecryptKey(keyJSON, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keystore: %w", err)
	}

	return key.PrivateKey, nil
}

// ImportPrivateKey imports a hex-encoded private key into the keystore.
func ImportPrivateKey(ks *keystore.KeyStore, hexKey, password string) (accounts.Account, error) {
	// Remove 0x prefix if present
	hexKey = strings.TrimPrefix(hexKey, "0x")

	// Parse private key
	privateKey, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("invalid private key: %w", err)
	}

	// Import into keystore
	account, err := ks.ImportECDSA(privateKey, password)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("failed to import key: %w", err)
	}

	return account, nil
}

// ImportMnemonic derives a private key from a BIP-39 mnemonic and imports it.
func ImportMnemonic(ks *keystore.KeyStore, mnemonic, derivationPath, password string) (accounts.Account, error) {
	if derivationPath == "" {
		derivationPath = DefaultDerivationPath
	}

	// Create HD wallet from mnemonic
	wallet, err := hdwallet.NewFromMnemonic(mnemonic)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("invalid mnemonic: %w", err)
	}

	// Parse derivation path
	path, err := hdwallet.ParseDerivationPath(derivationPath)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("invalid derivation path: %w", err)
	}

	// Derive account
	hdAccount, err := wallet.Derive(path, false)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("failed to derive account: %w", err)
	}

	// Get private key
	privateKey, err := wallet.PrivateKey(hdAccount)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("failed to get private key: %w", err)
	}

	// Import into keystore
	account, err := ks.ImportECDSA(privateKey, password)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("failed to import key: %w", err)
	}

	return account, nil
}

// ExportPrivateKey exports a private key from the keystore as hex string.
func ExportPrivateKey(ks *keystore.KeyStore, address common.Address, password string) (string, error) {
	privateKey, err := UnlockAccount(ks, address, password)
	if err != nil {
		return "", err
	}

	return "0x" + common.Bytes2Hex(crypto.FromECDSA(privateKey)), nil
}

// NewAccount creates a new account with a random private key.
func NewAccount(ks *keystore.KeyStore, password string) (accounts.Account, error) {
	account, err := ks.NewAccount(password)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("failed to create account: %w", err)
	}

	return account, nil
}

// ResolvePassword resolves the keystore password from environment variable or file.
func ResolvePassword(passwordFile string) (string, error) {
	// First try environment variable (empty password is valid)
	if password, exists := os.LookupEnv(PasswordEnvVar); exists {
		return password, nil
	}

	// Then try password file
	if passwordFile != "" {
		data, err := os.ReadFile(passwordFile)
		if err != nil {
			return "", fmt.Errorf("failed to read password file: %w", err)
		}
		// Trim whitespace/newlines
		return strings.TrimSpace(string(data)), nil
	}

	return "", fmt.Errorf("keystore password not found: set %s environment variable or specify password-file in config", PasswordEnvVar)
}

// ListAccounts returns all accounts in the keystore with their file paths.
func ListAccounts(ks *keystore.KeyStore) []accounts.Account {
	return ks.Accounts()
}
