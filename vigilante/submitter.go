package vigilante

import (
	"errors"
	"sync"

	"github.com/babylonchain/vigilante/babylonclient"
	"github.com/babylonchain/vigilante/btcclient"
	"github.com/babylonchain/vigilante/config"
)

type Submitter struct {
	btcClient         *btcclient.Client
	btcClientLock     sync.Mutex
	babylonClient     *babylonclient.Client
	babylonClientLock sync.Mutex
	// TODO: add wallet client

	// TODO: add Babylon parameters
	wg sync.WaitGroup

	started bool
	quit    chan struct{}
	quitMu  sync.Mutex
}

func NewSubmitter(cfg *config.SubmitterConfig, btcClient *btcclient.Client, babylonClient *babylonclient.Client) (*Submitter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Submitter{
		btcClient:     btcClient,
		babylonClient: babylonClient,
		quit:          make(chan struct{}),
	}, nil
}

// Start starts the goroutines necessary to manage a vigilante.
func (s *Submitter) Start() {
	s.quitMu.Lock()
	select {
	case <-s.quit:
		// Restart the vigilante goroutines after shutdown finishes.
		s.WaitForShutdown()
		s.quit = make(chan struct{})
	default:
		// Ignore when the vigilante is still running.
		if s.started {
			s.quitMu.Unlock()
			return
		}
		s.started = true
	}
	s.quitMu.Unlock()

	log.Infof("Successfully created the vigilant submitter")

	// s.wg.Add(2)
	// go s.txCreator()
	// go s.walletLocker()
}

// SynchronizeRPC associates the vigilante with the consensus RPC client,
// synchronizes the vigilante with the latest changes to the blockchain, and
// continuously updates the vigilante through RPC notifications.
//
// This method is unstable and will be removed when all syncing logic is moved
// outside of the vigilante package.
func (s *Submitter) SynchronizeRPC(btcClient *btcclient.Client) {
	s.quitMu.Lock()
	select {
	case <-s.quit:
		s.quitMu.Unlock()
		return
	default:
	}
	s.quitMu.Unlock()

	// TODO: Ignoring the new client when one is already set breaks callers
	// who are replacing the client, perhaps after a disconnect.
	s.btcClientLock.Lock()
	if s.btcClient != nil {
		s.btcClientLock.Unlock()
		return
	}
	s.btcClient = btcClient
	s.btcClientLock.Unlock()

	// TODO: add internal logic of submitter
	// TODO: It would be preferable to either run these goroutines
	// separately from the vigilante (use vigilante mutator functions to
	// make changes from the RPC client) and not have to stop and
	// restart them each time the client disconnects and reconnets.
	// s.wg.Add(4)
	// go s.handleChainNotifications()
	// go s.rescanBatchHandler()
	// go s.rescanProgressHandler()
	// go s.rescanRPCHandler()
}

// requirebtcClient marks that a vigilante method can only be completed when the
// consensus RPC server is set.  This function and all functions that call it
// are unstable and will need to be moved when the syncing code is moved out of
// the vigilante.
func (s *Submitter) requireGetBtcClient() (*btcclient.Client, error) {
	s.btcClientLock.Lock()
	btcClient := s.btcClient
	s.btcClientLock.Unlock()
	if btcClient == nil {
		return nil, errors.New("blockchain RPC is inactive")
	}
	return btcClient, nil
}

// btcClient returns the optional consensus RPC client associated with the
// vigilante.
//
// This function is unstable and will be removed once sync logic is moved out of
// the vigilante.
func (s *Submitter) getBtcClient() *btcclient.Client {
	s.btcClientLock.Lock()
	btcClient := s.btcClient
	s.btcClientLock.Unlock()
	return btcClient
}

func (s *Submitter) requireGetBabylonClient() (*babylonclient.Client, error) {
	s.babylonClientLock.Lock()
	client := s.babylonClient
	s.babylonClientLock.Unlock()
	if client == nil {
		return nil, errors.New("Babylon client is inactive")
	}
	return client, nil
}

func (s *Submitter) getBabylonClient() *babylonclient.Client {
	s.babylonClientLock.Lock()
	client := s.babylonClient
	s.babylonClientLock.Unlock()
	return client
}

// quitChan atomically reads the quit channel.
func (s *Submitter) quitChan() <-chan struct{} {
	s.quitMu.Lock()
	c := s.quit
	s.quitMu.Unlock()
	return c
}

// Stop signals all vigilante goroutines to shutdown.
func (s *Submitter) Stop() {
	s.quitMu.Lock()
	quit := s.quit
	s.quitMu.Unlock()

	select {
	case <-quit:
	default:
		close(quit)
		// shutdown BTC client
		s.btcClientLock.Lock()
		if s.btcClient != nil {
			s.btcClient.Stop()
			s.btcClient = nil
		}
		s.btcClientLock.Unlock()
		// shutdown Babylon client
		s.babylonClientLock.Lock()
		if s.babylonClient != nil {
			s.babylonClient.Stop()
			s.babylonClient = nil
		}
		s.babylonClientLock.Unlock()
	}
}

// ShuttingDown returns whether the vigilante is currently in the process of
// shutting down or not.
func (s *Submitter) ShuttingDown() bool {
	select {
	case <-s.quitChan():
		return true
	default:
		return false
	}
}

// WaitForShutdown blocks until all vigilante goroutines have finished executing.
func (s *Submitter) WaitForShutdown() {
	s.btcClientLock.Lock()
	if s.btcClient != nil {
		s.btcClient.WaitForShutdown()
	}
	s.btcClientLock.Unlock()
	s.wg.Wait()
}
