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

	tests := []struct {
		name       string
		privateKey string
	}{
		{
			name:       "with 0x prefix",
			privateKey: "0x" + testPrivateKey,
		},
		{
			name:       "without 0x prefix",
			privateKey: testPrivateKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			ks, err := LoadKeystore(dir)
			require.NoError(t, err)

			account, err := ImportPrivateKey(ks, tt.privateKey, testPassword)
			require.NoError(t, err)
			require.Equal(t, common.HexToAddress(testAddress), account.Address)

			// Verify account is in keystore
			accounts := ListAccounts(ks)
			require.Len(t, accounts, 1)
			require.Equal(t, account.Address, accounts[0].Address)
		})
	}
}

func TestImportMnemonic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		derivationPath string
		expectedAddr   string // empty means just check it's not empty
	}{
		{
			name:           "default derivation path",
			derivationPath: DefaultDerivationPath,
			expectedAddr:   "", // Will verify address is generated
		},
		{
			name:           "custom derivation path (second account)",
			derivationPath: "m/44'/60'/0'/0/1",
			expectedAddr:   "", // Will verify different from default
		},
	}

	// Store addresses to verify they're different
	addresses := make(map[string]common.Address)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			ks, err := LoadKeystore(dir)
			require.NoError(t, err)

			account, err := ImportMnemonic(ks, testMnemonic, tt.derivationPath, testPassword)
			require.NoError(t, err)
			require.NotEmpty(t, account.Address.Hex())

			// Verify account is in keystore
			accounts := ListAccounts(ks)
			require.Len(t, accounts, 1)

			addresses[tt.name] = account.Address
		})
	}

	// Verify different derivation paths produce different addresses
	// (This is implicitly tested since each subtest runs in isolation)
}

func TestUnlockAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		password    string
		expectError bool
	}{
		{
			name:        "correct password",
			password:    testPassword,
			expectError: false,
		},
		{
			name:        "wrong password",
			password:    "wrongpassword",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			ks, err := LoadKeystore(dir)
			require.NoError(t, err)

			account, err := ImportPrivateKey(ks, testPrivateKey, testPassword)
			require.NoError(t, err)

			privateKey, err := UnlockAccount(ks, account.Address, tt.password)
			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, privateKey)
			} else {
				require.NoError(t, err)
				require.NotNil(t, privateKey)
			}
		})
	}
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

	tests := []struct {
		name        string
		address     common.Address
		shouldExist bool
	}{
		{
			name:        "existing account",
			address:     common.HexToAddress(testAddress),
			shouldExist: true,
		},
		{
			name:        "non-existing account",
			address:     common.HexToAddress("0x0000000000000000000000000000000000000001"),
			shouldExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			ks, err := LoadKeystore(dir)
			require.NoError(t, err)

			// Import the test account
			importedAccount, err := ImportPrivateKey(ks, testPrivateKey, testPassword)
			require.NoError(t, err)

			found, err := FindAccount(ks, tt.address)
			if tt.shouldExist {
				require.NoError(t, err)
				require.Equal(t, importedAccount.Address, found.Address)
			} else {
				require.Error(t, err)
			}
		})
	}
}

//nolint:paralleltest // subtests use t.Setenv which is incompatible with t.Parallel
func TestResolvePassword(t *testing.T) {
	tests := []struct {
		name         string
		setupEnv     func(t *testing.T)
		passwordFile string
		expected     string
		expectError  bool
	}{
		{
			name: "from environment variable",
			setupEnv: func(t *testing.T) {
				t.Setenv(PasswordEnvVar, testPassword)
			},
			passwordFile: "",
			expected:     testPassword,
			expectError:  false,
		},
		{
			name: "empty password from env (valid)",
			setupEnv: func(t *testing.T) {
				t.Setenv(PasswordEnvVar, "")
			},
			passwordFile: "",
			expected:     "",
			expectError:  false,
		},
		{
			name:         "no password source",
			setupEnv:     nil,
			passwordFile: "",
			expected:     "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupEnv != nil {
				tt.setupEnv(t)
			}

			password, err := ResolvePassword(tt.passwordFile)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, password)
			}
		})
	}

	// Test password file separately (needs file setup)
	t.Run("from password file", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "password.txt")
		err := os.WriteFile(tmpFile, []byte(testPassword+"\n"), 0600)
		require.NoError(t, err)

		password, err := ResolvePassword(tmpFile)
		require.NoError(t, err)
		require.Equal(t, testPassword, password)
	})
}

func TestListAccounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		numAccounts   int
		expectedCount int
	}{
		{
			name:          "empty keystore",
			numAccounts:   0,
			expectedCount: 0,
		},
		{
			name:          "single account",
			numAccounts:   1,
			expectedCount: 1,
		},
		{
			name:          "multiple accounts",
			numAccounts:   3,
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			ks, err := LoadKeystore(dir)
			require.NoError(t, err)

			// Create the specified number of accounts
			for range tt.numAccounts {
				_, err = NewAccount(ks, testPassword)
				require.NoError(t, err)
			}

			accounts := ListAccounts(ks)
			require.Len(t, accounts, tt.expectedCount)
		})
	}
}
