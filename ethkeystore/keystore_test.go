package ethkeystore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const (
	// Test private key (DO NOT use in production)
	testPrivateKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	testAddress    = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	testPassword   = "testpassword123"
	// Valid BIP-39 test mnemonic (DO NOT use in production)
	testMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
)

func TestLoadKeystore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ks, err := LoadKeystore(dir)
	require.NoError(t, err)
	require.NotNil(t, ks)

	// Verify directory was created
	_, err = os.Stat(dir)
	require.NoError(t, err)
}

func TestNewAccount(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ks, err := LoadKeystore(dir)
	require.NoError(t, err)

	account, err := NewAccount(ks, testPassword)
	require.NoError(t, err)
	require.NotEmpty(t, account.Address.Hex())

	// Verify account is in keystore
	accounts := ListAccounts(ks)
	require.Len(t, accounts, 1)
	require.Equal(t, account.Address, accounts[0].Address)
}

func TestImportPrivateKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ks, err := LoadKeystore(dir)
	require.NoError(t, err)

	// Import with 0x prefix
	account, err := ImportPrivateKey(ks, "0x"+testPrivateKey, testPassword)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(testAddress), account.Address)

	// Verify account is in keystore
	accounts := ListAccounts(ks)
	require.Len(t, accounts, 1)
	require.Equal(t, account.Address, accounts[0].Address)
}

func TestImportPrivateKeyWithoutPrefix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ks, err := LoadKeystore(dir)
	require.NoError(t, err)

	// Import without 0x prefix
	account, err := ImportPrivateKey(ks, testPrivateKey, testPassword)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(testAddress), account.Address)
}

func TestImportMnemonic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ks, err := LoadKeystore(dir)
	require.NoError(t, err)

	account, err := ImportMnemonic(ks, testMnemonic, DefaultDerivationPath, testPassword)
	require.NoError(t, err)
	require.NotEmpty(t, account.Address.Hex())

	// Verify account is in keystore
	accounts := ListAccounts(ks)
	require.Len(t, accounts, 1)
}

func TestImportMnemonicCustomPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ks, err := LoadKeystore(dir)
	require.NoError(t, err)

	// Use a different derivation path (second account)
	customPath := "m/44'/60'/0'/0/1"
	account1, err := ImportMnemonic(ks, testMnemonic, DefaultDerivationPath, testPassword)
	require.NoError(t, err)

	// Need a new keystore for second account (can't import same address twice)
	dir2 := t.TempDir()
	ks2, err := LoadKeystore(dir2)
	require.NoError(t, err)

	account2, err := ImportMnemonic(ks2, testMnemonic, customPath, testPassword)
	require.NoError(t, err)

	// Different derivation paths should produce different addresses
	require.NotEqual(t, account1.Address, account2.Address)
}

func TestUnlockAccount(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ks, err := LoadKeystore(dir)
	require.NoError(t, err)

	account, err := ImportPrivateKey(ks, testPrivateKey, testPassword)
	require.NoError(t, err)

	// Unlock with correct password
	privateKey, err := UnlockAccount(ks, account.Address, testPassword)
	require.NoError(t, err)
	require.NotNil(t, privateKey)

	// Unlock with wrong password should fail
	_, err = UnlockAccount(ks, account.Address, "wrongpassword")
	require.Error(t, err)
}

func TestExportPrivateKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ks, err := LoadKeystore(dir)
	require.NoError(t, err)

	_, err = ImportPrivateKey(ks, testPrivateKey, testPassword)
	require.NoError(t, err)

	// Export the key
	exportedKey, err := ExportPrivateKey(ks, common.HexToAddress(testAddress), testPassword)
	require.NoError(t, err)
	require.Equal(t, "0x"+testPrivateKey, exportedKey)
}

func TestFindAccount(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ks, err := LoadKeystore(dir)
	require.NoError(t, err)

	account, err := ImportPrivateKey(ks, testPrivateKey, testPassword)
	require.NoError(t, err)

	// Find existing account
	found, err := FindAccount(ks, account.Address)
	require.NoError(t, err)
	require.Equal(t, account.Address, found.Address)

	// Find non-existing account
	nonExistent := common.HexToAddress("0x0000000000000000000000000000000000000001")
	_, err = FindAccount(ks, nonExistent)
	require.Error(t, err)
}

//nolint:paralleltest // subtests use t.Setenv which is incompatible with t.Parallel
func TestResolvePassword(t *testing.T) {
	// Test with environment variable
	t.Run("from_env", func(t *testing.T) {
		t.Setenv(PasswordEnvVar, testPassword)

		password, err := ResolvePassword("")
		require.NoError(t, err)
		require.Equal(t, testPassword, password)
	})

	// Test with empty password (valid for testing)
	t.Run("from_env_empty", func(t *testing.T) {
		t.Setenv(PasswordEnvVar, "")

		password, err := ResolvePassword("")
		require.NoError(t, err)
		require.Equal(t, "", password)
	})

	// Test with password file
	t.Run("from_file", func(t *testing.T) {
		// Create temp password file in test's temp dir
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "password.txt")
		err := os.WriteFile(tmpFile, []byte(testPassword+"\n"), 0600)
		require.NoError(t, err)

		password, err := ResolvePassword(tmpFile)
		require.NoError(t, err)
		require.Equal(t, testPassword, password)
	})

	// Test with no password source
	t.Run("no_source", func(t *testing.T) {
		// Ensure env var is not set
		_, err := ResolvePassword("")
		require.Error(t, err)
	})
}

func TestListAccounts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ks, err := LoadKeystore(dir)
	require.NoError(t, err)

	// Empty keystore
	accounts := ListAccounts(ks)
	require.Len(t, accounts, 0)

	// Add accounts
	_, err = NewAccount(ks, testPassword)
	require.NoError(t, err)
	_, err = NewAccount(ks, testPassword)
	require.NoError(t, err)

	accounts = ListAccounts(ks)
	require.Len(t, accounts, 2)
}
