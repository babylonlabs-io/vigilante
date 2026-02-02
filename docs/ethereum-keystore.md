# Ethereum Keystore Setup

This document explains how to set up and manage Ethereum keystores for the
Vigilante ETH Reporter. The keystore uses the Web3 Secret Storage format,
which is compatible with geth, MetaMask, and other Ethereum tools.

## Table of Contents

1. [Overview](#1-overview)
2. [Creating a New Keystore](#2-creating-a-new-keystore)
3. [Importing an Existing Key](#3-importing-an-existing-key)
4. [Configuring the Vigilante](#4-configuring-the-vigilante)
5. [Password Management](#5-password-management)
6. [Key Management Commands](#6-key-management-commands)
7. [Security Best Practices](#7-security-best-practices)

## 1. Overview

The Ethereum keystore provides secure storage for private keys used to sign
transactions when submitting BTC headers to the Ethereum smart contract.
Keys are encrypted using scrypt KDF and stored in JSON files.

### Benefits

- **Encrypted storage**: Private keys are never stored in plain text
- **Standard format**: Compatible with geth, MetaMask, and other tools
- **Portable**: Keystore files can be backed up and migrated easily

## 2. Creating a New Keystore

To create a new Ethereum account with a randomly generated private key:

```shell
vigilante keys eth new --keystore-dir /path/to/keystore
```

You will be prompted to enter and confirm a password. The output will show:

```
New account created:
  Address:  0x1234567890abcdef1234567890abcdef12345678
  Keystore: /path/to/keystore/UTC--2024-01-01T00-00-00.000000000Z--1234...
```

Save the address for use in your configuration file.

### Non-Interactive Mode

For automated deployments, you can provide the password via environment variable:

```shell
export VIGILANTE_ETH_KEYSTORE_PASSWORD="your-secure-password"
vigilante keys eth new --keystore-dir /path/to/keystore --password-from-env
```

## 3. Importing an Existing Key

### From Hex Private Key

If you have an existing private key (e.g., from a previous setup), import it:

```shell
# Via stdin (recommended - key won't appear in shell history)
echo "0xYOUR_PRIVATE_KEY" | vigilante keys eth import --keystore-dir /path/to/keystore

# Or via flag (less secure - appears in shell history)
vigilante keys eth import --keystore-dir /path/to/keystore --private-key 0xYOUR_PRIVATE_KEY
```

### From Mnemonic Seed Phrase

To import from a BIP-39 mnemonic (12 or 24 word seed phrase):

```shell
vigilante keys eth import-mnemonic --keystore-dir /path/to/keystore
```

You will be prompted to enter the mnemonic (input is hidden for security).

The default derivation path is `m/44'/60'/0'/0/0` (standard Ethereum path).
To use a different derivation path:

```shell
vigilante keys eth import-mnemonic --keystore-dir /path/to/keystore --derivation-path "m/44'/60'/0'/0/1"
```

## 4. Configuring the Vigilante

Update your `vigilante.yml` configuration file with the keystore settings:

```yaml
ethereum:
  # Ethereum RPC endpoint
  rpc-url: "https://mainnet.infura.io/v3/YOUR_PROJECT_ID"

  # Keystore configuration
  keystore-dir: "/path/to/keystore"
  account-address: "0x1234567890abcdef1234567890abcdef12345678"
  password-file: ""  # Optional: path to file containing password

  # Contract and chain settings
  contract-address: "0xCONTRACT_ADDRESS"
  chain-id: 1  # 1=Mainnet, 11155111=Sepolia
```

### Configuration Parameters

| Parameter | Description |
|-----------|-------------|
| `keystore-dir` | Directory containing the encrypted keystore JSON files |
| `account-address` | Ethereum address to use for signing (must exist in keystore) |
| `password-file` | Optional path to a file containing the keystore password |

## 5. Password Management

The keystore password can be provided in two ways:

### Option 1: Environment Variable (Recommended for Production)

Set the `VIGILANTE_ETH_KEYSTORE_PASSWORD` environment variable:

```shell
export VIGILANTE_ETH_KEYSTORE_PASSWORD="your-secure-password"
vigilante reporter --config vigilante.yml
```

For systemd services, add to your service file:

```ini
[Service]
Environment="VIGILANTE_ETH_KEYSTORE_PASSWORD=your-secure-password"
```

### Option 2: Password File

Create a file containing only the password (no newline):

```shell
echo -n "your-secure-password" > /path/to/password.txt
chmod 600 /path/to/password.txt
```

Then reference it in your configuration:

```yaml
ethereum:
  password-file: "/path/to/password.txt"
```

### Priority Order

1. `VIGILANTE_ETH_KEYSTORE_PASSWORD` environment variable (checked first)
2. Password file specified in `password-file` config

## 6. Key Management Commands

### List Accounts

View all accounts in a keystore directory:

```shell
vigilante keys eth list --keystore-dir /path/to/keystore
```

Output:

```
Accounts in /path/to/keystore:

  1. 0x1234567890abcdef1234567890abcdef12345678
     File: UTC--2024-01-01T00-00-00.000000000Z--1234...
```

### Export Private Key

Export a private key for backup or migration:

```shell
vigilante keys eth export --keystore-dir /path/to/keystore --address 0x1234...
```

**Warning**: This displays the private key in plain text. Ensure:
- No one is watching your screen
- Clear your terminal history afterward
- Store the exported key securely

### Command Reference

| Command | Description |
|---------|-------------|
| `vigilante keys eth new` | Create a new account with random key |
| `vigilante keys eth import` | Import a hex private key |
| `vigilante keys eth import-mnemonic` | Import from BIP-39 mnemonic |
| `vigilante keys eth list` | List all accounts in keystore |
| `vigilante keys eth export` | Export private key from keystore |

## 7. Security Best Practices

### Keystore Directory

- Set restrictive permissions: `chmod 700 /path/to/keystore`
- Store on encrypted filesystem when possible
- Regular backups to secure, offline storage

### Password Security

- Use a strong, unique password (16+ characters)
- Never commit passwords to version control
- Use environment variables or password files with restricted permissions
- Rotate passwords periodically

### Operational Security

- Keep keystore files separate from configuration files
- Monitor the account for unexpected transactions
- Maintain sufficient ETH balance for gas fees
- Consider using a hardware wallet for high-value operations

### Backup Recommendations

1. **Keystore files**: Back up the entire keystore directory
2. **Password**: Store separately from keystore backup
3. **Test recovery**: Periodically verify backups can be restored

### Migration from Plain Text Keys

If you previously used a plain text private key in configuration:

```shell
# Import your existing key
echo "0xYOUR_OLD_PRIVATE_KEY" | vigilante keys eth import --keystore-dir /path/to/keystore

# Update your config to use keystore
# Then securely delete any files containing the plain text key
```
