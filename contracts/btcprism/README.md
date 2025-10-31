# BTC Prism Smart Contract Bindings

This directory contains the BtcPrism.sol smart contract and its Go bindings.

## Files

- `BtcPrism.sol` - Main Bitcoin light client contract
- `Endian.sol` - Endianness conversion utilities
- `IBtcPrism.sol` - Contract interface
- `BtcPrism.abi` - Contract ABI (JSON, committed)
- `BtcPrism.bin` - Compiled bytecode (committed)
- `btcprism.go` - Generated Go bindings (committed)

## Regenerating Bindings

If the Solidity contracts change, regenerate the bindings:

```bash
# From vigilante root directory
make generate-contracts
```

Or manually:

```bash
# Compile contracts
solc --abi --bin --overwrite --base-path . \
  contracts/btcprism/BtcPrism.sol \
  -o contracts/btcprism/

# Generate Go bindings
abigen --abi=contracts/btcprism/BtcPrism.abi \
       --bin=contracts/btcprism/BtcPrism.bin \
       --pkg=btcprism \
       --out=contracts/btcprism/btcprism.go
```

## Contract Source

Original contract from: https://github.com/babylonlabs-io/vault-contracts

## Usage

```go
import "github.com/babylonlabs-io/vigilante/contracts/btcprism"

// Connect to contract
contract, err := btcprism.NewBtcprism(contractAddress, ethClient)

// Submit headers
tx, err := contract.Submit(auth, startHeight, concatenatedHeaders)
```
