package e2etest

import (
	"encoding/json"
	"fmt"
	"github.com/babylonlabs-io/vigilante/e2etest/container"
	"github.com/stretchr/testify/require"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	startTimeout = 30 * time.Second
)

type CreateWalletResponse struct {
	Name    string `json:"name"`
	Warning string `json:"warning"`
}

type GenerateBlockResponse struct {
	// address of the recipient of rewards
	Address string `json:"address"`
	// blocks generated
	Blocks []string `json:"blocks"`
}

type BitcoindTestHandler struct {
	t *testing.T
	m *container.Manager
}

func NewBitcoindHandler(t *testing.T) *BitcoindTestHandler {
	manager, err := container.NewManager()
	require.NoError(t, err)

	return &BitcoindTestHandler{
		t: t,
		m: manager,
	}
}

func (h *BitcoindTestHandler) Start() {
	tempPath, err := os.MkdirTemp("", "vigilante-test-*")
	require.NoError(h.t, err)

	h.t.Cleanup(func() {
		_ = os.RemoveAll(tempPath)
	})

	_, err = h.m.RunBitcoindResource(tempPath)
	require.NoError(h.t, err)

	h.t.Cleanup(func() {
		_ = h.m.ClearResources()
	})

	require.Eventually(h.t, func() bool {
		_, err := h.GetBlockCount()
		if err != nil {
			h.t.Logf("failed to get block count: %v", err)
		}
		return err == nil
	}, startTimeout, 500*time.Millisecond, "bitcoind did not start")
}

func (h *BitcoindTestHandler) GetBlockCount() (int, error) {
	buff, _, err := h.m.ExecBitcoindCliCmd(h.t, []string{"getblockcount"})
	if err != nil {
		return 0, err
	}

	parsedBuffStr := strings.TrimSuffix(buff.String(), "\n")

	return strconv.Atoi(parsedBuffStr)
}

func (h *BitcoindTestHandler) GenerateBlocks(count int) *GenerateBlockResponse {
	buff, _, err := h.m.ExecBitcoindCliCmd(h.t, []string{"-generate", fmt.Sprintf("%d", count)})
	require.NoError(h.t, err)

	var response GenerateBlockResponse
	err = json.Unmarshal(buff.Bytes(), &response)
	require.NoError(h.t, err)

	return &response
}

func (h *BitcoindTestHandler) CreateWallet(walletName string, passphrase string) *CreateWalletResponse {
	// last false on the list will create legacy wallet. This is needed, as currently
	// we are signing all taproot transactions by dumping the private key and signing it
	// on app level. Descriptor wallets do not allow dumping private keys.
	buff, _, err := h.m.ExecBitcoindCliCmd(h.t, []string{"createwallet", walletName, "false", "false", passphrase, "false", "true"})
	require.NoError(h.t, err)

	var response CreateWalletResponse
	err = json.Unmarshal(buff.Bytes(), &response)
	require.NoError(h.t, err)

	return &response
}

// InvalidateBlock invalidates blocks starting from specified block hash
func (h *BitcoindTestHandler) InvalidateBlock(blockHash string) {
	_, _, err := h.m.ExecBitcoindCliCmd(h.t, []string{"invalidateblock", blockHash})
	require.NoError(h.t, err)
}

// ImportDescriptors - todo
func (h *BitcoindTestHandler) ImportDescriptors(descriptor string) {
	_, _, err := h.m.ExecBitcoindCliCmd(h.t, []string{"importdescriptors", descriptor})
	require.NoError(h.t, err)
}
