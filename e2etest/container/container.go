package container

import (
	"bytes"
	"context"
	"fmt"
	bbn "github.com/babylonlabs-io/babylon/types"
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/btcsuite/btcd/btcec/v2"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
)

const (
	bitcoindContainerName = "bitcoind"
	babylondContainerName = "babylond"
)

var (
	_, covenantPK = btcec.PrivKeyFromBytes(
		[]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
	)
)

var errRegex = regexp.MustCompile(`(E|e)rror`)

// Manager is a wrapper around all Docker instances, and the Docker API.
// It provides utilities to run and interact with all Docker containers used within e2e testing.
type Manager struct {
	cfg       ImageConfig
	pool      *dockertest.Pool
	resources map[string]*dockertest.Resource
}

// NewManager creates a new Manager instance and initializes
// all Docker specific utilities. Returns an error if initialization fails.
func NewManager(t *testing.T) (docker *Manager, err error) {
	docker = &Manager{
		cfg:       NewImageConfig(t),
		resources: make(map[string]*dockertest.Resource),
	}
	docker.pool, err = dockertest.NewPool("")
	if err != nil {
		return nil, err
	}

	return docker, nil
}

func (m *Manager) ExecBitcoindCliCmd(t *testing.T, command []string) (bytes.Buffer, bytes.Buffer, error) {
	// this is currently hardcoded, as it will be the same for all tests
	cmd := []string{"bitcoin-cli", "-chain=regtest", "-rpcuser=user", "-rpcpassword=pass"}
	cmd = append(cmd, command...)
	return m.ExecCmd(t, bitcoindContainerName, cmd)
}

// ExecCmd executes command by running it on the given container.
// It word for word `error` in output to discern between error and regular output.
// It retures stdout and stderr as bytes.Buffer and an error if the command fails.
func (m *Manager) ExecCmd(t *testing.T, containerName string, command []string) (bytes.Buffer, bytes.Buffer, error) {
	if _, ok := m.resources[containerName]; !ok {
		return bytes.Buffer{}, bytes.Buffer{}, fmt.Errorf("no resource %s found", containerName)
	}
	containerId := m.resources[containerName].Container.ID

	var (
		outBuf bytes.Buffer
		errBuf bytes.Buffer
	)

	timeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	t.Logf("\n\nRunning: \"%s\"", command)

	// We use the `require.Eventually` function because it is only allowed to do one transaction per block without
	// sequence numbers. For simplicity, we avoid keeping track of the sequence number and just use the `require.Eventually`.
	require.Eventually(
		t,
		func() bool {
			exec, err := m.pool.Client.CreateExec(docker.CreateExecOptions{
				Context:      ctx,
				AttachStdout: true,
				AttachStderr: true,
				Container:    containerId,
				User:         "root",
				Cmd:          command,
			})

			if err != nil {
				t.Logf("failed to create exec: %v", err)
				return false
			}

			err = m.pool.Client.StartExec(exec.ID, docker.StartExecOptions{
				Context:      ctx,
				Detach:       false,
				OutputStream: &outBuf,
				ErrorStream:  &errBuf,
			})
			if err != nil {
				t.Logf("failed to start exec: %v", err)
				return false
			}

			errBufString := errBuf.String()
			// Note that this does not match all errors.
			// This only works if CLI outputs "Error" or "error"
			// to stderr.
			if errRegex.MatchString(errBufString) {
				t.Log("\nstderr:")
				t.Log(errBufString)

				t.Log("\nstdout:")
				t.Log(outBuf.String())
				return false
			}

			return true
		},
		timeout,
		500*time.Millisecond,
		"command failed",
	)

	return outBuf, errBuf, nil
}

// RunBitcoindResource starts a bitcoind docker container
func (m *Manager) RunBitcoindResource(
	t *testing.T,
	bitcoindCfgPath string,
) (*dockertest.Resource, error) {
	bitcoindResource, err := m.pool.RunWithOptions(
		&dockertest.RunOptions{
			Name:       fmt.Sprintf("%s-%s", bitcoindContainerName, t.Name()),
			Repository: m.cfg.BitcoindRepository,
			Tag:        m.cfg.BitcoindVersion,
			User:       "root:root",
			Labels: map[string]string{
				"e2e": "bitcoind",
			},
			Mounts: []string{
				fmt.Sprintf("%s/:/data/.bitcoin", bitcoindCfgPath),
			},
			ExposedPorts: []string{
				"18443/tcp",
			},
			Cmd: []string{
				"-regtest",
				"-txindex",
				"-rpcuser=user",
				"-rpcpassword=pass",
				"-rpcallowip=0.0.0.0/0",
				"-rpcbind=0.0.0.0",
				"-zmqpubsequence=tcp://0.0.0.0:28333",
				"-fallbackfee=0.0002",
			},
		},
		func(config *docker.HostConfig) {
			config.PortBindings = map[docker.Port][]docker.PortBinding{
				"18443/tcp": {{HostIP: "", HostPort: strconv.Itoa(testutil.AllocateUniquePort(t))}}, // only expose what we need
			}
			config.PublishAllPorts = false // because in dockerfile they already expose them
		},
		noRestart,
	)
	if err != nil {
		return nil, err
	}
	m.resources[bitcoindContainerName] = bitcoindResource
	return bitcoindResource, nil
}

// RunBabylondResource starts a babylond container
func (m *Manager) RunBabylondResource(
	t *testing.T,
	mounthPath string,
	baseHeaderHex string,
	slashingPkScript string,
	epochInterval uint,
) (*dockertest.Resource, error) {
	cmd := []string{
		"sh", "-c", fmt.Sprintf(
			"babylond testnet --v=1 --output-dir=/home --starting-ip-address=192.168.10.2 "+
				"--keyring-backend=test --chain-id=chain-test --btc-finalization-timeout=4 "+
				"--btc-confirmation-depth=2 --additional-sender-account --btc-network=regtest "+
				"--min-staking-time-blocks=200 --min-staking-amount-sat=10000 "+
				"--epoch-interval=%d --slashing-pk-script=%s --btc-base-header=%s "+
				"--covenant-quorum=1 --covenant-pks=%s && chmod -R 777 /home && babylond start --home=/home/node0/babylond",
			epochInterval, slashingPkScript, baseHeaderHex, bbn.NewBIP340PubKeyFromBTCPK(covenantPK).MarshalHex()),
	}

	resource, err := m.pool.RunWithOptions(
		&dockertest.RunOptions{
			Name:       fmt.Sprintf("%s-%s", babylondContainerName, t.Name()),
			Repository: m.cfg.BabylonRepository,
			Tag:        m.cfg.BabylonVersion,
			Labels: map[string]string{
				"e2e": "babylond",
			},
			User: "root:root",
			Mounts: []string{
				fmt.Sprintf("%s/:/home/", mounthPath),
			},
			ExposedPorts: []string{
				"9090/tcp", // only expose what we need
				"26657/tcp",
			},
			Cmd: cmd,
		},
		func(config *docker.HostConfig) {
			config.PortBindings = map[docker.Port][]docker.PortBinding{
				"9090/tcp":  {{HostIP: "", HostPort: strconv.Itoa(testutil.AllocateUniquePort(t))}},
				"26657/tcp": {{HostIP: "", HostPort: strconv.Itoa(testutil.AllocateUniquePort(t))}},
			}
		},
		noRestart,
	)
	if err != nil {
		return nil, err
	}

	m.resources[babylondContainerName] = resource

	return resource, nil
}

// ClearResources removes all outstanding Docker resources created by the Manager.
func (m *Manager) ClearResources() error {
	for _, resource := range m.resources {
		if err := m.pool.Purge(resource); err != nil {
			return err
		}
	}

	return nil
}

func noRestart(config *docker.HostConfig) {
	// in this case, we don't want the nodes to restart on failure
	config.RestartPolicy = docker.RestartPolicy{
		Name: "no",
	}
}
