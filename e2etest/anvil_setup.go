package e2etest

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/contracts/btcprism"
	"github.com/babylonlabs-io/vigilante/e2etest/container"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
)

const (
	// Anvil default test account #0
	anvilPrivateKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	anvilAddress    = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	anvilChainID    = 31337
)

type AnvilTestHandler struct {
	t            *testing.T
	m            *container.Manager
	client       *ethclient.Client
	contractAddr common.Address
	rpcURL       string
}

func NewAnvilHandler(t *testing.T, manager *container.Manager) *AnvilTestHandler {
	return &AnvilTestHandler{
		t: t,
		m: manager,
	}
}

// Start starts an Anvil container and waits for it to be ready
func (h *AnvilTestHandler) Start(t *testing.T) (*dockertest.Resource, error) {
	anvilResource, err := h.m.RunAnvilResource(t)
	if err != nil {
		return nil, err
	}

	h.t.Cleanup(func() {
		if h.client != nil {
			h.client.Close()
		}
	})

	// Get the RPC URL
	hostPort := anvilResource.GetPort("8545/tcp")
	h.rpcURL = fmt.Sprintf("http://localhost:%s", hostPort)

	// Wait for Anvil to be ready
	require.Eventually(h.t, func() bool {
		client, err := ethclient.Dial(h.rpcURL)
		if err != nil {
			h.t.Logf("failed to connect to anvil: %v", err)
			return false
		}
		defer client.Close()

		_, err = client.BlockNumber(context.Background())
		if err != nil {
			h.t.Logf("failed to get block number: %v", err)
			return false
		}
		return true
	}, startTimeout, 500*time.Millisecond, "anvil did not start")

	// Connect client for future use
	h.client, err = ethclient.Dial(h.rpcURL)
	require.NoError(h.t, err)

	return anvilResource, nil
}

// DeployBtcPrismContract deploys the BtcPrism contract and returns the contract address
func (h *AnvilTestHandler) DeployBtcPrismContract(t *testing.T, blockHeight uint64, blockHash [32]byte, blockTime uint32, expectedTarget *big.Int, isTestnet bool) common.Address {
	require.NotNil(t, h.client, "anvil client not initialized")

	// Parse private key
	privateKey, err := crypto.HexToECDSA(anvilPrivateKey)
	require.NoError(t, err)

	// Create transaction signer
	chainID := big.NewInt(anvilChainID)
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)

	// Convert parameters to big.Int
	blockHeightBig := new(big.Int).SetUint64(blockHeight)
	blockTimeBig := new(big.Int).SetUint64(uint64(blockTime))

	// Deploy contract
	contractAddr, tx, _, err := btcprism.DeployBtcprism(
		auth,
		h.client,
		blockHeightBig,
		blockHash,
		blockTimeBig,
		expectedTarget,
		isTestnet,
	)
	require.NoError(t, err)

	// Wait for deployment transaction to be mined
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	receipt, err := bind.WaitMined(ctx, h.client, tx)
	require.NoError(t, err)
	require.Equal(t, uint64(1), receipt.Status, "contract deployment failed")

	h.contractAddr = contractAddr
	t.Logf("BtcPrism contract deployed at: %s", contractAddr.Hex())

	return contractAddr
}

// GetRPCURL returns the Anvil RPC URL
func (h *AnvilTestHandler) GetRPCURL() string {
	return h.rpcURL
}

// GetContractAddress returns the deployed contract address
func (h *AnvilTestHandler) GetContractAddress() common.Address {
	return h.contractAddr
}

// GetEthereumConfig returns a config.EthereumConfig for testing
func (h *AnvilTestHandler) GetEthereumConfig() *config.EthereumConfig {
	return &config.EthereumConfig{
		RPCURL:                h.rpcURL,
		PrivateKey:            anvilPrivateKey,
		ContractAddress:       h.contractAddr.Hex(),
		ChainID:               anvilChainID,
		GasLimit:              0, // auto-estimate
		MaxGasPrice:           0, // no limit
		ConfirmationMode:      "receipt",
		ConfirmationTimeout:   30 * time.Second,
		MaxConfirmationBlocks: 100,
	}
}

// GetBlockNumber returns the current block number
func (h *AnvilTestHandler) GetBlockNumber() (uint64, error) {
	return h.client.BlockNumber(context.Background())
}

// GetClient returns the Ethereum client
func (h *AnvilTestHandler) GetClient() *ethclient.Client {
	return h.client
}

// Remove removes the anvil container
func (h *AnvilTestHandler) Remove(name string) error {
	return h.m.RemoveContainer(name)
}
