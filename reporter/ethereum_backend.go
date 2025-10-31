package reporter

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/contracts/btcprism"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

// EthereumBackend submits BTC headers to an Ethereum smart contract (BtcPrism.sol)
type EthereumBackend struct {
	client          *ethclient.Client
	contract        *btcprism.Btcprism
	auth            *bind.TransactOpts
	cfg             *config.EthereumConfig
	contractAddress common.Address
	logger          *zap.SugaredLogger
}

// NewEthereumBackend creates a new Ethereum backend for submitting BTC headers
func NewEthereumBackend(cfg *config.EthereumConfig, parentLogger *zap.Logger) (Backend, error) {
	logger := parentLogger.With(zap.String("module", "ethereum-backend")).Sugar()

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ethereum config: %w", err)
	}

	// Connect to Ethereum RPC
	client, err := ethclient.Dial(cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum RPC: %w", err)
	}

	// Parse contract address
	if !common.IsHexAddress(cfg.ContractAddress) {
		return nil, fmt.Errorf("invalid contract address: %s", cfg.ContractAddress)
	}
	contractAddress := common.HexToAddress(cfg.ContractAddress)

	// Load contract binding
	contract, err := btcprism.NewBtcprism(contractAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to load BtcPrism contract: %w", err)
	}

	// Parse private key
	privateKeyStr := strings.TrimPrefix(cfg.PrivateKey, "0x")
	privateKey, err := crypto.HexToECDSA(privateKeyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Create transaction signer
	chainID := big.NewInt(int64(cfg.ChainID))
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %w", err)
	}

	// Set gas limit if specified
	if cfg.GasLimit > 0 {
		auth.GasLimit = cfg.GasLimit
	}

	// Set max gas price if specified
	if cfg.MaxGasPrice > 0 {
		maxGasPriceWei := new(big.Int).Mul(
			big.NewInt(int64(cfg.MaxGasPrice)),
			big.NewInt(1e9), // Convert Gwei to Wei
		)
		auth.GasPrice = maxGasPriceWei
	}

	logger.Infow("Ethereum backend initialized",
		"rpc_url", cfg.RPCURL,
		"contract_address", contractAddress.Hex(),
		"chain_id", cfg.ChainID,
		"confirmation_mode", cfg.ConfirmationMode,
	)

	return &EthereumBackend{
		client:          client,
		contract:        contract,
		auth:            auth,
		cfg:             cfg,
		contractAddress: contractAddress,
		logger:          logger,
	}, nil
}

// ContainsBlock checks if the Ethereum contract has the given BTC block hash
// This queries the contract to check if the block exists at its expected height
func (e *EthereumBackend) ContainsBlock(ctx context.Context, hash *chainhash.Hash) (bool, error) {
	// The BtcPrism contract stores blocks by height, not hash
	// We need the BTCCache to provide height information
	// For now, we query the latest height and search backwards

	latestHeight, err := e.contract.GetLatestBlockHeight(&bind.CallOpts{Context: ctx})
	if err != nil {
		return false, fmt.Errorf("failed to get latest block height: %w", err)
	}

	// Search backwards through recent blocks (last 1000 blocks max, as per contract MAX_ALLOWED_REORG)
	// This is not ideal but works for deduplication
	searchDepth := uint64(1000)
	if latestHeight.Uint64() < searchDepth {
		searchDepth = latestHeight.Uint64()
	}

	targetHashBytes := hash.CloneBytes()
	reverseBytes(targetHashBytes) // Bitcoin hashes are little-endian, Ethereum expects big-endian

	for i := uint64(0); i < searchDepth; i++ {
		height := new(big.Int).Sub(latestHeight, big.NewInt(int64(i)))
		blockHash, err := e.contract.GetBlockHash(&bind.CallOpts{Context: ctx}, height)
		if err != nil {
			return false, fmt.Errorf("failed to get block hash at height %d: %w", height, err)
		}

		if common.BytesToHash(targetHashBytes) == blockHash {
			e.logger.Debugw("Block found in contract", "hash", hash.String(), "height", height)

			return true, nil
		}
	}

	return false, nil
}

// GetTip returns the Ethereum contract's current tip block height and hash.
func (e *EthereumBackend) GetTip(ctx context.Context) (uint32, *chainhash.Hash, error) {
	// Get latest height from contract
	latestHeight, err := e.contract.GetLatestBlockHeight(&bind.CallOpts{Context: ctx})
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get latest block height: %w", err)
	}

	// Get hash at latest height
	blockHash, err := e.contract.GetBlockHash(&bind.CallOpts{Context: ctx}, latestHeight)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get block hash at height %d: %w", latestHeight, err)
	}

	// Convert from Ethereum's big-endian [32]byte to Bitcoin's little-endian chainhash.Hash
	hashBytes := blockHash[:]
	reverseBytes(hashBytes)
	hash, err := chainhash.NewHash(hashBytes)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse block hash: %w", err)
	}

	return uint32(latestHeight.Uint64()), hash, nil
}

// SubmitHeaders submits a batch of BTC headers to the Ethereum contract
func (e *EthereumBackend) SubmitHeaders(ctx context.Context, startHeight uint64, headers [][]byte) error {
	if len(headers) == 0 {
		return fmt.Errorf("no headers to submit")
	}

	// Concatenate all 80-byte headers into single bytes array
	// The contract expects: function submit(uint256 blockHeight, bytes calldata blockHeaders)
	concatenated := make([]byte, 0, len(headers)*80)
	for i, header := range headers {
		if len(header) != 80 {
			return fmt.Errorf("invalid header length at index %d: got %d, want 80", i, len(header))
		}
		concatenated = append(concatenated, header...)
	}

	e.logger.Infow("Submitting headers to Ethereum",
		"start_height", startHeight,
		"num_headers", len(headers),
		"total_bytes", len(concatenated),
		"first_header_hex", fmt.Sprintf("%x", headers[0][:20]), // First 20 bytes for debug
	)

	// Check contract state before submitting
	latestHeight, latestHeightErr := e.contract.GetLatestBlockHeight(&bind.CallOpts{Context: ctx})
	if latestHeightErr != nil {
		e.logger.Warnw("Failed to get contract latest height (non-fatal)", "error", latestHeightErr)
	} else {
		e.logger.Infow("Contract state before submit",
			"contract_latest_height", latestHeight.Uint64(),
			"submitting_start_height", startHeight,
			"gap", int64(startHeight)-int64(latestHeight.Uint64()),
		)
	}

	// Check if we're trying to submit blocks that don't connect
	if latestHeight != nil && startHeight > latestHeight.Uint64()+1 {
		e.logger.Warnw("Gap detected: trying to submit blocks that don't connect to contract tip",
			"contract_tip", latestHeight.Uint64(),
			"submitting_start", startHeight,
			"missing_blocks", startHeight-latestHeight.Uint64()-1,
		)
	}

	// Submit transaction
	tx, err := e.contract.Submit(e.auth, big.NewInt(int64(startHeight)), concatenated)
	if err != nil {
		e.logger.Errorw("Failed to submit headers to contract",
			"start_height", startHeight,
			"num_headers", len(headers),
			"contract_latest_height", latestHeight,
			"error_type", fmt.Sprintf("%T", err),
			"error_full", fmt.Sprintf("%+v", err),
		)

		return parseContractError(err)
	}

	e.logger.Infow("Transaction sent",
		"tx_hash", tx.Hash().Hex(),
		"nonce", tx.Nonce(),
	)

	// Wait for confirmation based on configured mode
	if err := e.waitForConfirmation(ctx, tx); err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	e.logger.Infow("Headers submitted successfully",
		"tx_hash", tx.Hash().Hex(),
		"start_height", startHeight,
		"num_headers", len(headers),
	)

	return nil
}

// waitForConfirmation waits for transaction confirmation based on configured mode
func (e *EthereumBackend) waitForConfirmation(ctx context.Context, tx *types.Transaction) error {
	switch e.cfg.ConfirmationMode {
	case "receipt":
		return e.waitForReceipt(ctx, tx)
	case "safe":
		return e.waitForSafeBlock(ctx, tx)
	case "finalized":
		return e.waitForFinalizedBlock(ctx, tx)
	default:
		return e.waitForReceipt(ctx, tx)
	}
}

// waitForReceipt waits for transaction receipt (fastest, ~1 confirmation)
func (e *EthereumBackend) waitForReceipt(ctx context.Context, tx *types.Transaction) error {
	ctx, cancel := context.WithTimeout(ctx, e.cfg.ConfirmationTimeout)
	defer cancel()

	receipt, err := bind.WaitMined(ctx, e.client, tx)
	if err != nil {
		return fmt.Errorf("failed waiting for receipt: %w", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		return fmt.Errorf("transaction failed on-chain: %s", tx.Hash().Hex())
	}

	e.logger.Debugw("Transaction mined",
		"tx_hash", tx.Hash().Hex(),
		"block_number", receipt.BlockNumber,
		"gas_used", receipt.GasUsed,
	)

	return nil
}

// waitForSafeBlock waits for transaction to be in a "safe" block (recommended for Ethereum PoS)
func (e *EthereumBackend) waitForSafeBlock(ctx context.Context, tx *types.Transaction) error {
	// First wait for receipt
	if err := e.waitForReceipt(ctx, tx); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, e.cfg.ConfirmationTimeout)
	defer cancel()

	// Get transaction receipt to find block number
	receipt, err := e.client.TransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to get receipt: %w", err)
	}

	txBlockNumber := receipt.BlockNumber.Uint64()

	// Poll for safe block
	ticker := time.NewTicker(6 * time.Second) // Ethereum block time ~12s, check every 6s
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for safe confirmation")
		case <-ticker.C:
			// Get the "safe" block using raw RPC call with block tag
			// This requires go-ethereum v1.10.0+ and a PoS Ethereum node
			safeBlockNumber, err := e.getSafeBlockNumber(ctx)
			if err != nil {
				e.logger.Warnw("Failed to get safe block", "error", err)

				continue
			}

			if safeBlockNumber >= txBlockNumber {
				e.logger.Debugw("Transaction confirmed in safe block",
					"tx_hash", tx.Hash().Hex(),
					"tx_block", txBlockNumber,
					"safe_block", safeBlockNumber,
				)

				return nil
			}
		}
	}
}

// waitForFinalizedBlock waits for transaction to be in a "finalized" block (slowest, most secure)
func (e *EthereumBackend) waitForFinalizedBlock(ctx context.Context, tx *types.Transaction) error {
	// First wait for receipt
	if err := e.waitForReceipt(ctx, tx); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, e.cfg.ConfirmationTimeout)
	defer cancel()

	// Get transaction receipt to find block number
	receipt, err := e.client.TransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to get receipt: %w", err)
	}

	txBlockNumber := receipt.BlockNumber.Uint64()

	// Poll for finalized block
	ticker := time.NewTicker(6 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for finalized confirmation")
		case <-ticker.C:
			// Get the "finalized" block using raw RPC call with block tag
			// This requires go-ethereum v1.10.0+ and a PoS Ethereum node
			finalizedBlockNumber, err := e.getFinalizedBlockNumber(ctx)
			if err != nil {
				e.logger.Warnw("Failed to get finalized block", "error", err)

				continue
			}

			if finalizedBlockNumber >= txBlockNumber {
				e.logger.Debugw("Transaction finalized",
					"tx_hash", tx.Hash().Hex(),
					"tx_block", txBlockNumber,
					"finalized_block", finalizedBlockNumber,
				)

				return nil
			}
		}
	}
}

// getSafeBlockNumber gets the block number of the "safe" block using raw RPC
func (e *EthereumBackend) getSafeBlockNumber(ctx context.Context) (uint64, error) {
	type rpcBlock struct {
		Number string `json:"number"`
	}

	var result rpcBlock
	err := e.client.Client().CallContext(ctx, &result, "eth_getBlockByNumber", "safe", false)
	if err != nil {
		return 0, fmt.Errorf("failed to get safe block: %w", err)
	}

	// Parse hex string to uint64
	blockNumber := new(big.Int)
	if _, ok := blockNumber.SetString(result.Number[2:], 16); !ok {
		return 0, fmt.Errorf("failed to parse safe block number: %s", result.Number)
	}

	return blockNumber.Uint64(), nil
}

// getFinalizedBlockNumber gets the block number of the "finalized" block using raw RPC
func (e *EthereumBackend) getFinalizedBlockNumber(ctx context.Context) (uint64, error) {
	type rpcBlock struct {
		Number string `json:"number"`
	}

	var result rpcBlock
	err := e.client.Client().CallContext(ctx, &result, "eth_getBlockByNumber", "finalized", false)
	if err != nil {
		return 0, fmt.Errorf("failed to get finalized block: %w", err)
	}

	// Parse hex string to uint64
	blockNumber := new(big.Int)
	if _, ok := blockNumber.SetString(result.Number[2:], 16); !ok {
		return 0, fmt.Errorf("failed to parse finalized block number: %s", result.Number)
	}

	return blockNumber.Uint64(), nil
}

// Stop closes the Ethereum client connection
func (e *EthereumBackend) Stop() error {
	e.client.Close()
	e.logger.Info("Ethereum backend stopped")

	return nil
}

// reverseBytes reverses a byte slice in place (for endianness conversion)
func reverseBytes(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}

// GetPublicKeyFromPrivate derives the public key from a private key
func GetPublicKeyFromPrivate(privateKey *ecdsa.PrivateKey) *ecdsa.PublicKey {
	return &privateKey.PublicKey
}
