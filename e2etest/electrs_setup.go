package e2etest

import (
	"fmt"
	"github.com/babylonlabs-io/vigilante/e2etest/container"
	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"
)

type ElectrsTestHandler struct {
	t *testing.T
	m *container.Manager
}

func NewElectrsHandler(t *testing.T, manager *container.Manager) *ElectrsTestHandler {
	return &ElectrsTestHandler{
		t: t,
		m: manager,
	}
}

func (h *ElectrsTestHandler) Start(t *testing.T, bitcoindPath, btcRpcAddr string) (*dockertest.Resource, error) {
	resource, err := h.m.RunElectrsResource(t, t.TempDir(), bitcoindPath, btcRpcAddr)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://localhost:%s", resource.GetPort("3000/tcp"))

	require.Eventually(h.t, func() bool {
		_, err := fetch(fmt.Sprintf("%s/blocks/tip/height", url))

		return err == nil
	}, startTimeout, 500*time.Millisecond, "electrs did not start")

	return resource, nil
}

func (h *ElectrsTestHandler) GetTipHeight(url string) (int, error) {
	body, err := fetch(fmt.Sprintf("%s/blocks/tip/height", url))
	if err != nil {
		return 0, err
	}

	height, err := strconv.Atoi(body)
	if err != nil {
		return 0, err
	}

	return height, nil
}

func (h *ElectrsTestHandler) Remove(name string) error {
	return h.m.RemoveContainer(name)
}

func fetch(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
