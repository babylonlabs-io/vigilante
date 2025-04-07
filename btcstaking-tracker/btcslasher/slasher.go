package btcslasher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	bbn "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"go.uber.org/zap"
)

type BTCSlasher struct {
	logger *zap.SugaredLogger

	// connect to BTC node
	BTCClient btcclient.BTCClient
	// BBNQuerier queries epoch info from Babylon
	BBNQuerier BabylonQueryClient

	// parameters
	netParams              *chaincfg.Params
	btcFinalizationTimeout uint32
	retrySleepTime         time.Duration
	maxRetrySleepTime      time.Duration
	maxRetryTimes          uint
	// channel for SKs of slashed finality providers
	slashedFPSKChan chan *btcec.PrivateKey
	// channel for receiving the slash result of each BTC delegation
	slashResultChan chan *SlashResult

	maxSlashingConcurrency int64

	metrics *metrics.SlasherMetrics

	startOnce             sync.Once
	stopOnce              sync.Once
	wg                    sync.WaitGroup
	quit                  chan struct{}
	mu                    sync.Mutex
	height                uint64
	evidenceFetchInterval time.Duration
}

func New(
	parentLogger *zap.Logger,
	btcClient btcclient.BTCClient,
	bbnQuerier BabylonQueryClient,
	netParams *chaincfg.Params,
	retrySleepTime time.Duration,
	maxRetrySleepTime time.Duration,
	maxRetryTimes uint,
	maxSlashingConcurrency uint8,
	slashedFPSKChan chan *btcec.PrivateKey,
	metrics *metrics.SlasherMetrics,
	evidenceFetchInterval time.Duration,
) (*BTCSlasher, error) {
	logger := parentLogger.With(zap.String("module", "slasher")).Sugar()

	return &BTCSlasher{
		logger:                 logger,
		BTCClient:              btcClient,
		BBNQuerier:             bbnQuerier,
		netParams:              netParams,
		retrySleepTime:         retrySleepTime,
		maxRetrySleepTime:      maxRetrySleepTime,
		maxRetryTimes:          maxRetryTimes,
		maxSlashingConcurrency: int64(maxSlashingConcurrency),
		slashedFPSKChan:        slashedFPSKChan,
		slashResultChan:        make(chan *SlashResult, 1000),
		quit:                   make(chan struct{}),
		metrics:                metrics,
		evidenceFetchInterval:  evidenceFetchInterval,
	}, nil
}

func (bs *BTCSlasher) quitContext() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	bs.wg.Add(1)
	go func() {
		defer cancel()
		defer bs.wg.Done()

		select {
		case <-bs.quit:

		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

func (bs *BTCSlasher) LoadParams() error {
	if bs.btcFinalizationTimeout != 0 {
		// already loaded, skip
		return nil
	}

	btccParamsResp, err := bs.BBNQuerier.BTCCheckpointParams()
	if err != nil {
		return err
	}
	bs.btcFinalizationTimeout = btccParamsResp.Params.CheckpointFinalizationTimeout

	return nil
}

func (bs *BTCSlasher) Start() error {
	var startErr error
	bs.startOnce.Do(func() {
		// load module parameters
		if err := bs.LoadParams(); err != nil {
			startErr = err

			return
		}

		// start slasher
		bs.wg.Add(2)
		go bs.fetchEvidences()
		go bs.slashingEnforcer()

		bs.logger.Info("the BTC slasher has started")
	})

	return startErr
}

// slashingEnforcer is a routine that keeps receiving finality providers
// to be slashed and slashes their BTC delegations on Bitcoin
func (bs *BTCSlasher) slashingEnforcer() {
	defer bs.wg.Done()

	bs.logger.Info("slashing enforcer has started")

	// start handling incoming slashing events
	for {
		select {
		case <-bs.quit:
			bs.logger.Debug("handle delegations loop quit")

			return
		case fpBTCSK, ok := <-bs.slashedFPSKChan:
			if !ok {
				// slasher receives the channel from outside, so its lifecycle
				// is out of slasher's control. So we need to ensure the channel
				// is not closed yet
				bs.logger.Debug("slashedFKSK channel is already closed, terminating the slashing enforcer")

				return
			}
			// slash all the BTC delegations of this finality provider
			fpBTCPKHex := bbn.NewBIP340PubKeyFromBTCPK(fpBTCSK.PubKey()).MarshalHex()
			bs.logger.Infof("slashing finality provider %s", fpBTCPKHex)

			if err := bs.SlashFinalityProvider(fpBTCSK); err != nil {
				bs.logger.Errorf("failed to slash finality provider %s: %v", fpBTCPKHex, err)
			}
		case slashRes := <-bs.slashResultChan:
			if slashRes.Err != nil {
				bs.logger.Errorf(
					"failed to slash BTC delegation with staking tx hash %s under finality provider %s: %v",
					slashRes.Del.StakingTxHex,
					slashRes.Del.FpBtcPkList[0].MarshalHex(), // TODO: work with restaking
					slashRes.Err,
				)
			} else {
				bs.logger.Infof(
					"successfully slash BTC delegation with staking tx hash %s under finality provider %s",
					slashRes.Del.StakingTxHex,
					slashRes.Del.FpBtcPkList[0].MarshalHex(), // TODO: work with restaking
				)

				// record the metrics of the slashed delegation
				bs.metrics.RecordSlashedDelegation(slashRes.Del)
			}
		}
	}
}

// SlashFinalityProvider slashes all BTC delegations under a given finality provider
// the checkBTC option indicates whether to check the slashing tx's input is still spendable
// on Bitcoin (including mempool txs).
func (bs *BTCSlasher) SlashFinalityProvider(extractedFpBTCSK *btcec.PrivateKey) error {
	fpBTCPK := bbn.NewBIP340PubKeyFromBTCPK(extractedFpBTCSK.PubKey())
	bs.logger.Infof("start slashing finality provider %s", fpBTCPK.MarshalHex())

	// get all active and unbonded BTC delegations at the current BTC height
	// Some BTC delegations could be expired in Babylon's view but not expired in
	// Bitcoin's view. We will not slash such BTC delegations since they don't have
	// voting power (thus don't affect consensus) in Babylon
	activeBTCDels, unbondedBTCDels, err := bs.getAllActiveAndUnbondedBTCDelegations(fpBTCPK)
	if err != nil {
		return fmt.Errorf("failed to get BTC delegations under finality provider %s: %w", fpBTCPK.MarshalHex(), err)
	}

	// Initialize a semaphore to control the number of concurrent operations
	sem := semaphore.NewWeighted(bs.maxSlashingConcurrency)
	activeBTCDels = append(activeBTCDels, unbondedBTCDels...)
	delegations := activeBTCDels

	// try to slash both staking and unbonding txs for each BTC delegation
	// sign and submit slashing tx for each active and unbonded delegation

	bs.logger.Infof("slashing %d BTC delegations under finality provider %s", len(delegations), fpBTCPK.MarshalHex())
	bs.logger.Debugf("these delegation's will be slashed")
	for _, del := range delegations {
		bs.logger.Debugf("BTC delegation %s, under fp %s ", del.StakingTxHex, fpBTCPK.MarshalHex())
	}

	var wg sync.WaitGroup
	for _, del := range delegations {
		wg.Add(1)
		bs.logger.Debugf("IN LOOP: slashing BTC delegation %s, under fp %s ", del.StakingTxHex, fpBTCPK.MarshalHex())

		go func(d *bstypes.BTCDelegationResponse) {
			defer wg.Done()
			ctx, cancel := bs.quitContext()
			defer cancel()

			// Acquire the semaphore before interacting with the BTC node
			if err := sem.Acquire(ctx, 1); err != nil {
				bs.logger.Errorf("failed to acquire semaphore: %v", err)

				return
			}
			defer sem.Release(1)

			bs.logger.Debugf("IN routine: slashing BTC delegation %s, under fp %s ", del.StakingTxHex, fpBTCPK.MarshalHex())

			bs.slashBTCDelegation(fpBTCPK, extractedFpBTCSK, d)
		}(del)
	}

	wg.Wait()

	bs.metrics.SlashedFinalityProvidersCounter.Inc()

	return nil
}

func (bs *BTCSlasher) WaitForShutdown() {
	bs.wg.Wait()
}

func (bs *BTCSlasher) Stop() error {
	var stopErr error
	bs.stopOnce.Do(func() {
		bs.logger.Info("stopping slasher")
		// notify all subroutines
		close(bs.quit)
		bs.wg.Wait()

		bs.logger.Info("stopped slasher")
	})

	return stopErr
}

func (bs *BTCSlasher) fetchEvidences() {
	defer bs.wg.Done()
	ticker := time.NewTicker(bs.evidenceFetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lastSlashedHeight, err := bs.processEvidencesFromHeight(bs.height)
			if err != nil {
				bs.logger.Errorf("error processing evidence from height: %d, err: %v", bs.height, err)

				continue
			}

			// Only update height if we actually processed evidence
			if lastSlashedHeight > 0 {
				bs.mu.Lock()
				bs.height = lastSlashedHeight + 1
				bs.mu.Unlock()
			}
		case <-bs.quit:
			bs.logger.Debug("fetch evidence loop quit")

			return
		}
	}
}
