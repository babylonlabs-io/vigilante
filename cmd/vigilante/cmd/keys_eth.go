package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/babylonlabs-io/vigilante/ethkeystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)


const (
	flagKeystoreDir     = "keystore-dir"
	flagPrivateKey      = "private-key"
	flagAddress         = "address"
	flagDerivationPath  = "derivation-path"
	flagPasswordFromEnv = "password-from-env"
)

// GetKeysEthCmd returns the keys eth command group
func GetKeysEthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eth",
		Short: "Ethereum keystore management commands",
		Long:  "Commands for managing Ethereum keystores (Web3 Secret Storage format)",
	}

	cmd.AddCommand(
		getKeysEthNewCmd(),
		getKeysEthImportCmd(),
		getKeysEthImportMnemonicCmd(),
		getKeysEthListCmd(),
		getKeysEthExportCmd(),
	)

	return cmd
}

// getKeysEthNewCmd creates a new keystore account
func getKeysEthNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new Ethereum account",
		Long:  "Generate a new random private key and store it in an encrypted keystore file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			keystoreDir, _ := cmd.Flags().GetString(flagKeystoreDir)
			passwordFromEnv, _ := cmd.Flags().GetBool(flagPasswordFromEnv)

			// Load or create keystore
			ks, err := ethkeystore.LoadKeystore(keystoreDir)
			if err != nil {
				return fmt.Errorf("failed to load keystore: %w", err)
			}

			// Get password
			password, err := getPassword(passwordFromEnv, true)
			if err != nil {
				return err
			}

			// Create new account
			account, err := ethkeystore.NewAccount(ks, password)
			if err != nil {
				return err
			}

			cmd.Printf("New account created:\n")
			cmd.Printf("  Address:  %s\n", account.Address.Hex())
			cmd.Printf("  Keystore: %s\n", account.URL.Path)

			return nil
		},
	}

	cmd.Flags().String(flagKeystoreDir, "", "Directory to store keystore files (required)")
	cmd.Flags().Bool(flagPasswordFromEnv, false, "Read password from VIGILANTE_ETH_KEYSTORE_PASSWORD env var instead of prompting")
	_ = cmd.MarkFlagRequired(flagKeystoreDir)

	return cmd
}

// getKeysEthImportCmd imports a hex private key
func getKeysEthImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import an existing private key",
		Long: `Import a hex-encoded private key into the keystore.

The private key can be provided via:
  1. --private-key flag (not recommended for security)
  2. stdin: echo "0x..." | vigilante keys eth import --keystore-dir /path`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			keystoreDir, _ := cmd.Flags().GetString(flagKeystoreDir)
			privateKeyFlag, _ := cmd.Flags().GetString(flagPrivateKey)
			passwordFromEnv, _ := cmd.Flags().GetBool(flagPasswordFromEnv)

			// Load or create keystore
			ks, err := ethkeystore.LoadKeystore(keystoreDir)
			if err != nil {
				return fmt.Errorf("failed to load keystore: %w", err)
			}

			// Get private key from flag or stdin
			privateKey := privateKeyFlag
			if privateKey == "" {
				// Try reading from stdin
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & os.ModeCharDevice) == 0 {
					// Data is being piped
					reader := bufio.NewReader(os.Stdin)
					line, err := reader.ReadString('\n')
					if err != nil && line == "" {
						return fmt.Errorf("failed to read private key from stdin: %w", err)
					}
					privateKey = strings.TrimSpace(line)
				}
			}

			if privateKey == "" {
				return fmt.Errorf("private key required: use --private-key flag or pipe via stdin")
			}

			// Get password
			password, err := getPassword(passwordFromEnv, true)
			if err != nil {
				return err
			}

			// Import key
			account, err := ethkeystore.ImportPrivateKey(ks, privateKey, password)
			if err != nil {
				return err
			}

			cmd.Printf("Account imported:\n")
			cmd.Printf("  Address:  %s\n", account.Address.Hex())
			cmd.Printf("  Keystore: %s\n", account.URL.Path)

			return nil
		},
	}

	cmd.Flags().String(flagKeystoreDir, "", "Directory to store keystore files (required)")
	cmd.Flags().String(flagPrivateKey, "", "Private key in hex format (with or without 0x prefix)")
	cmd.Flags().Bool(flagPasswordFromEnv, false, "Read password from VIGILANTE_ETH_KEYSTORE_PASSWORD env var instead of prompting")
	_ = cmd.MarkFlagRequired(flagKeystoreDir)

	return cmd
}

// getKeysEthImportMnemonicCmd imports from a mnemonic
func getKeysEthImportMnemonicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import-mnemonic",
		Short: "Import from BIP-39 mnemonic seed phrase",
		Long: `Import a private key derived from a BIP-39 mnemonic seed phrase.

The mnemonic will be read interactively (not echoed to screen for security).
Default derivation path: m/44'/60'/0'/0/0 (standard Ethereum path)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			keystoreDir, _ := cmd.Flags().GetString(flagKeystoreDir)
			derivationPath, _ := cmd.Flags().GetString(flagDerivationPath)
			passwordFromEnv, _ := cmd.Flags().GetBool(flagPasswordFromEnv)

			// Load or create keystore
			ks, err := ethkeystore.LoadKeystore(keystoreDir)
			if err != nil {
				return fmt.Errorf("failed to load keystore: %w", err)
			}

			// Read mnemonic interactively
			mnemonic, err := readMnemonic()
			if err != nil {
				return err
			}

			// Get password
			password, err := getPassword(passwordFromEnv, true)
			if err != nil {
				return err
			}

			// Import from mnemonic
			account, err := ethkeystore.ImportMnemonic(ks, mnemonic, derivationPath, password)
			if err != nil {
				return err
			}

			cmd.Printf("Account imported from mnemonic:\n")
			cmd.Printf("  Address:         %s\n", account.Address.Hex())
			cmd.Printf("  Derivation path: %s\n", derivationPath)
			cmd.Printf("  Keystore:        %s\n", account.URL.Path)

			return nil
		},
	}

	cmd.Flags().String(flagKeystoreDir, "", "Directory to store keystore files (required)")
	cmd.Flags().String(flagDerivationPath, ethkeystore.DefaultDerivationPath, "BIP-44 derivation path")
	cmd.Flags().Bool(flagPasswordFromEnv, false, "Read password from VIGILANTE_ETH_KEYSTORE_PASSWORD env var instead of prompting")
	_ = cmd.MarkFlagRequired(flagKeystoreDir)

	return cmd
}

// getKeysEthListCmd lists all accounts
func getKeysEthListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all accounts in keystore",
		Long:  "Display all Ethereum addresses and their keystore file paths",
		RunE: func(cmd *cobra.Command, _ []string) error {
			keystoreDir, _ := cmd.Flags().GetString(flagKeystoreDir)

			// Load keystore
			ks, err := ethkeystore.LoadKeystore(keystoreDir)
			if err != nil {
				return fmt.Errorf("failed to load keystore: %w", err)
			}

			accounts := ethkeystore.ListAccounts(ks)
			if len(accounts) == 0 {
				cmd.Printf("No accounts found in %s\n", keystoreDir)

				return nil
			}

			cmd.Printf("Accounts in %s:\n\n", keystoreDir)
			for i, acc := range accounts {
				cmd.Printf("  %d. %s\n", i+1, acc.Address.Hex())
				cmd.Printf("     File: %s\n\n", filepath.Base(acc.URL.Path))
			}

			return nil
		},
	}

	cmd.Flags().String(flagKeystoreDir, "", "Directory containing keystore files (required)")
	_ = cmd.MarkFlagRequired(flagKeystoreDir)

	return cmd
}

// getKeysEthExportCmd exports a private key
func getKeysEthExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export private key from keystore",
		Long: `Export the private key for an account (for backup or migration).

WARNING: The private key will be displayed in plain text.
Ensure no one is watching your screen and clear your terminal history afterward.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			keystoreDir, _ := cmd.Flags().GetString(flagKeystoreDir)
			addressStr, _ := cmd.Flags().GetString(flagAddress)
			passwordFromEnv, _ := cmd.Flags().GetBool(flagPasswordFromEnv)

			if !common.IsHexAddress(addressStr) {
				return fmt.Errorf("invalid Ethereum address: %s", addressStr)
			}
			address := common.HexToAddress(addressStr)

			// Load keystore
			ks, err := ethkeystore.LoadKeystore(keystoreDir)
			if err != nil {
				return fmt.Errorf("failed to load keystore: %w", err)
			}

			// Get password
			password, err := getPassword(passwordFromEnv, false)
			if err != nil {
				return err
			}

			// Export private key
			privateKey, err := ethkeystore.ExportPrivateKey(ks, address, password)
			if err != nil {
				return err
			}

			cmd.Printf("Private key for %s:\n", address.Hex())
			cmd.Printf("%s\n", privateKey)
			cmd.Printf("\nWARNING: Store this securely and clear your terminal history!\n")

			return nil
		},
	}

	cmd.Flags().String(flagKeystoreDir, "", "Directory containing keystore files (required)")
	cmd.Flags().String(flagAddress, "", "Ethereum address to export (required)")
	cmd.Flags().Bool(flagPasswordFromEnv, false, "Read password from VIGILANTE_ETH_KEYSTORE_PASSWORD env var instead of prompting")
	_ = cmd.MarkFlagRequired(flagKeystoreDir)
	_ = cmd.MarkFlagRequired(flagAddress)

	return cmd
}

// getPassword reads password from env var or prompts interactively
func getPassword(fromEnv bool, confirm bool) (string, error) {
	if fromEnv {
		password := os.Getenv(ethkeystore.PasswordEnvVar)
		if password == "" {
			return "", fmt.Errorf("%s environment variable not set", ethkeystore.PasswordEnvVar)
		}

		return password, nil
	}

	// Interactive prompt
	fmt.Print("Enter keystore password: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}
	password := string(passwordBytes)

	if confirm {
		fmt.Print("Confirm password: ")
		confirmBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("failed to read password confirmation: %w", err)
		}

		if password != string(confirmBytes) {
			return "", fmt.Errorf("passwords do not match")
		}
	}

	return password, nil
}

// readMnemonic reads a mnemonic phrase interactively
func readMnemonic() (string, error) {
	fmt.Print("Enter mnemonic seed phrase: ")
	mnemonicBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read mnemonic: %w", err)
	}

	mnemonic := strings.TrimSpace(string(mnemonicBytes))
	if mnemonic == "" {
		return "", fmt.Errorf("mnemonic cannot be empty")
	}

	return mnemonic, nil
}
