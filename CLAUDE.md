# Vigilante - Babylon Bitcoin Bridge

## Overview

Vigilante is a collection of daemon programs that act as a bridge between the Babylon blockchain and Bitcoin, facilitating both the Bitcoin timestamping protocol and the Bitcoin staking protocol. This is a Go-based project that provides essential infrastructure for Babylon's Bitcoin integration.

## Project Structure

The repository is organized into four main vigilante programs:

### 1. **Submitter** (`./submitter/`)
- **Purpose**: Submits Babylon checkpoints to Bitcoin
- **Role**: Ensures Babylon's state is timestamped on the Bitcoin blockchain
- **Adapted from**: btcsuite/btcwallet

### 2. **Reporter** (`./reporter/`)
- **Purpose**: Reports Bitcoin headers and checkpoints to Babylon
- **Key Functions**:
  - Syncs latest BTC blocks with Bitcoin node
  - Extracts headers and checkpoints from BTC blocks
  - Forwards headers and checkpoints to Babylon
  - Detects inconsistencies between Bitcoin and Babylon BTCLightclient
  - Reports stalling attacks (w-deep checkpoints not included in Babylon)
- **Adapted from**: btcsuite/btcwallet

### 3. **Monitor** (`./monitor/`)
- **Purpose**: BTC timestamping monitor
- **Role**: Monitors censorship of Babylon checkpoints in Babylon

### 4. **BTC Staking Tracker** (`./btcstaking-tracker/`)
- **Purpose**: Monitors Bitcoin staking protocol operations
- **Key Components**:
  - **Unbonding Watcher**: Detects early unbonding transactions and reports to Babylon
  - **BTC Slasher**: Slashes adversarial finality providers for equivocation or selective slashing
  - **Atomic Slasher**: Enforces atomic slashing using adaptor signatures to prevent selective slashing

## Architecture

The vigilante programs connect to:
- **Bitcoin Node**: For querying blocks, transactions, and receiving notifications
- **Babylon Node**: For submitting transactions and querying state
- **Electrs Indexer**: Required for BTC staking tracker operations

## Key Features

- **Bitcoin Timestamping**: Ensures Babylon checkpoints are recorded on Bitcoin
- **Bitcoin Staking Protocol**: Enables Bitcoin to be staked in Babylon ecosystem
- **Slashing Protection**: Prevents malicious finality provider behavior
- **Real-time Monitoring**: Continuously watches for relevant events on both chains

## Dependencies

- Go 1.23+
- Bitcoin Core (bitcoind)
- libzmq package
- Electrs indexer (for staking tracker)

## Configuration

The project uses YAML configuration files (`vigilante.yml`) and provides Docker deployment options with sample configurations.

## Development

- **Language**: Go
- **Module**: `github.com/babylonlabs-io/vigilante`
- **Key Dependencies**: Babylon, btcsuite, Cosmos SDK, Lightning Network components
- **Testing**: Includes E2E tests and unit tests
- **Metrics**: Prometheus metrics integration

## Build & Run

```bash
make build
```

The vigilante can be run locally or via Docker containers with different configurations for mainnet, testnet, or regtest environments.

This project is essential infrastructure for Babylon's Bitcoin integration, providing the critical bridge between the two blockchains for both timestamping and staking operations.