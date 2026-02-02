package reporter

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/contracts/btcprism"
	"github.com/babylonlabs-io/vigilante/ethkeystore"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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

	// Load keystore
	ks, err := ethkeystore.LoadKeystore(cfg.KeystoreDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load keystore: %w", err)
	}

	// Find account
	if !common.IsHexAddress(cfg.AccountAddress) {
		return nil, fmt.Errorf("invalid account address: %s", cfg.AccountAddress)
	}
	accountAddress := common.HexToAddress(cfg.AccountAddress)

	account, err := ethkeystore.FindAccount(ks, accountAddress)
	if err != nil {
		return nil, err
	}

	// Resolve password from env var or file
	password, err := ethkeystore.ResolvePassword(cfg.PasswordFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get keystore password: %w", err)
	}

	// Unlock account in keystore
	if err := ks.Unlock(account, password); err != nil {
		return nil, fmt.Errorf("failed to unlock account: %w", err)
	}

	// Create transaction signer using keystore
	// #nosec G115 -- Chain IDs are small values (mainnet=1, testnets in thousands), no overflow risk
	chainID := big.NewInt(int64(cfg.ChainID))
	auth, err := bind.NewKeyStoreTransactorWithChainID(ks, account, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %w", err)
	}

	// Set gas limit if specified
	if cfg.GasLimit > 0 {
		auth.GasLimit = cfg.GasLimit
	}

	// Set max gas price if specified
	if cfg.MaxGasPrice > 0 {
		// #nosec G115 -- Gas prices in Gwei are reasonable values, no overflow risk
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

// ContainsBlock checks if the Ethereum contract has the given BTC block hash at the specified height.
// This uses O(1) height-based lookup instead of searching through blocks.
func (e *EthereumBackend) ContainsBlock(ctx context.Context, hash *chainhash.Hash, height uint32) (bool, error) {
	// Get contract's latest height to check if our block could exist
	latestHeight, err := e.contract.GetLatestBlockHeight(&bind.CallOpts{Context: ctx})
	if err != nil {
		return false, fmt.Errorf("failed to get latest block height: %w", err)
	}

	// If the block height is beyond the contract's tip, it's definitely not there
	if uint64(height) > latestHeight.Uint64() {
		e.logger.Debugw("Block height beyond contract tip",
			"block_height", height,
			"contract_tip", latestHeight.Uint64(),
		)

		return false, nil
	}

	// Look up the hash stored at this height in the contract
	// #nosec G115 -- Bitcoin block heights are well below int64 max
	storedHash, err := e.contract.GetBlockHash(&bind.CallOpts{Context: ctx}, big.NewInt(int64(height)))
	if err != nil {
		return false, fmt.Errorf("failed to get block hash at height %d: %w", height, err)
	}

	// Convert our Bitcoin hash to Ethereum format for comparison
	targetHashBytes := hash.CloneBytes()
	reverseBytes(targetHashBytes) // Bitcoin hashes are little-endian, Ethereum expects big-endian

	if common.BytesToHash(targetHashBytes) == storedHash {
		e.logger.Debugw("Block found in contract", "hash", hash.String(), "height", height)

		return true, nil
	}

	// Hash at this height doesn't match - could be a fork situation
	e.logger.Debugw("Block not found at height (hash mismatch)",
		"height", height,
		"expected_hash", hash.String(),
		"stored_hash", common.BytesToHash(storedHash[:]).Hex(),
	)

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

	// #nosec G115 -- Bitcoin block heights are well below uint32 max (4.2B), current height ~800k
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
		// #nosec G115 -- Converting block heights for logging gap, values are safe
		e.logger.Infow("Contract state before submit",
			"contract_latest_height", latestHeight.Uint64(),
			"submitting_start_height", startHeight,
			"gap", int64(startHeight)-int64(latestHeight.Uint64()),
		)
	}

	// Check if we're trying to submit blocks that don't connect
	if latestHeight != nil && startHeight > latestHeight.Uint64()+1 {
		e.logger.Warnw("Gap detected: cannot submit blocks that don't connect to contract tip",
			"contract_tip", latestHeight.Uint64(),
			"submitting_start", startHeight,
			"missing_blocks", startHeight-latestHeight.Uint64()-1,
		)
		return fmt.Errorf("%w: contract_tip=%d, submitting_start=%d, missing=%d",
			ErrGapInHeaderChain,
			latestHeight.Uint64(),
			startHeight,
			startHeight-latestHeight.Uint64()-1,
		)
	}

	// Submit transaction
	// #nosec G115 -- Bitcoin block heights are well below int64 max, safe conversion
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
	return e.waitForBlockConfirmation(ctx, tx, "safe", e.getSafeBlockNumber)
}

// waitForFinalizedBlock waits for transaction to be in a "finalized" block (slowest, most secure)
func (e *EthereumBackend) waitForFinalizedBlock(ctx context.Context, tx *types.Transaction) error {
	return e.waitForBlockConfirmation(ctx, tx, "finalized", e.getFinalizedBlockNumber)
}

// waitForBlockConfirmation is a generic helper that waits for a transaction to reach a specific confirmation level
func (e *EthereumBackend) waitForBlockConfirmation(
	ctx context.Context,
	tx *types.Transaction,
	blockTag string,
	getBlockNumber func(context.Context) (uint64, error),
) error {
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

	// Poll for confirmed block
	ticker := time.NewTicker(6 * time.Second) // Ethereum block time ~12s, check every 6s
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s confirmation", blockTag)
		case <-ticker.C:
			// Get the target block using raw RPC call with block tag
			// This requires go-ethereum v1.10.0+ and a PoS Ethereum node
			confirmedBlockNumber, err := getBlockNumber(ctx)
			if err != nil {
				e.logger.Warnw("Failed to get block", "block_tag", blockTag, "error", err)

				continue
			}

			if confirmedBlockNumber >= txBlockNumber {
				e.logger.Debugw("Transaction confirmed",
					"tx_hash", tx.Hash().Hex(),
					"tx_block", txBlockNumber,
					"confirmed_block", confirmedBlockNumber,
					"block_tag", blockTag,
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

// GetConfig returns the Ethereum configuration
func (e *EthereumBackend) GetConfig() *config.EthereumConfig {
	return e.cfg
}

// reverseBytes reverses a byte slice in place (for endianness conversion)
func reverseBytes(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}
