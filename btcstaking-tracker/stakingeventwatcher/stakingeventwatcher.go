package stakingeventwatcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/types"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/utils"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	notifier "github.com/lightningnetwork/lnd/chainntnfs"
	"go.uber.org/zap"
)

var (
	fixedDelyTypeWithJitter = retry.DelayType(retry.CombineDelay(retry.FixedDelay, retry.RandomDelay))
	retryForever            = retry.Attempts(0)
)

func (uw *StakingEventWatcher) quitContext() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	uw.wg.Add(1)
	go func() {
		defer cancel()
		defer uw.wg.Done()

		select {
		case <-uw.quit:

		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

type newDelegation struct {
	stakingTxHash         chainhash.Hash
	stakingTx             *wire.MsgTx
	stakingOutputIdx      uint32
	delegationStartHeight uint64
	unbondingOutput       *wire.TxOut
}

type delegationInactive struct {
	stakingTxHash chainhash.Hash
}

type StakingEventWatcher struct {
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup
	quit      chan struct{}
	cfg       *config.BTCStakingTrackerConfig
	logger    *zap.SugaredLogger

	btcClient   btcclient.BTCClient
	btcNotifier notifier.ChainNotifier
	metrics     *metrics.UnbondingWatcherMetrics
	// TODO: Ultimately all requests to babylon should go through some kind of semaphore
	// to avoid spamming babylon with requests
	babylonNodeAdapter BabylonNodeAdapter
	unbondingTracker   *TrackedDelegations
	pendingTracker     *TrackedDelegations

	unbondingDelegationChan chan *newDelegation

	unbondingRemovalChan   chan *delegationInactive
	currentBestBlockHeight atomic.Uint32
}

func NewStakingEventWatcher(
	btcNotifier notifier.ChainNotifier,
	btcClient btcclient.BTCClient,
	babylonNodeAdapter BabylonNodeAdapter,
	cfg *config.BTCStakingTrackerConfig,
	parentLogger *zap.Logger,
	metrics *metrics.UnbondingWatcherMetrics,
) *StakingEventWatcher {
	return &StakingEventWatcher{
		quit:                    make(chan struct{}),
		cfg:                     cfg,
		logger:                  parentLogger.With(zap.String("module", "staking_event_watcher")).Sugar(),
		btcNotifier:             btcNotifier,
		btcClient:               btcClient,
		babylonNodeAdapter:      babylonNodeAdapter,
		metrics:                 metrics,
		unbondingTracker:        NewTrackedDelegations(),
		pendingTracker:          NewTrackedDelegations(),
		unbondingDelegationChan: make(chan *newDelegation),
		unbondingRemovalChan:    make(chan *delegationInactive),
	}
}

func (uw *StakingEventWatcher) Start() error {
	var startErr error
	uw.startOnce.Do(func() {
		uw.logger.Info("starting staking event watcher")

		blockEventNotifier, err := uw.btcNotifier.RegisterBlockEpochNtfn(nil)
		if err != nil {
			startErr = err
			return
		}

		// we registered for notifications with `nil` so we should receive best block immediately
		select {
		case block := <-blockEventNotifier.Epochs:
			uw.currentBestBlockHeight.Store(uint32(block.Height))
		case <-uw.quit:
			startErr = errors.New("watcher quit before finishing start")
			return
		}

		uw.logger.Infof("Initial btc best block height is: %d", uw.currentBestBlockHeight.Load())

		uw.wg.Add(4)
		go uw.handleNewBlocks(blockEventNotifier)
		go uw.handleDelegations()
		go uw.fetchDelegations()
		go uw.handlerVerifiedDelegations()

		uw.logger.Info("staking event watcher started")
	})
	return startErr
}

func (uw *StakingEventWatcher) Stop() error {
	var stopErr error
	uw.stopOnce.Do(func() {
		uw.logger.Info("stopping staking event watcher")
		close(uw.quit)
		uw.wg.Wait()
		uw.logger.Info("stopped staking event watcher")
	})
	return stopErr
}

func (uw *StakingEventWatcher) handleNewBlocks(blockNotifier *notifier.BlockEpochEvent) {
	defer uw.wg.Done()
	defer blockNotifier.Cancel()
	for {
		select {
		case block, ok := <-blockNotifier.Epochs:
			if !ok {
				return
			}
			uw.currentBestBlockHeight.Store(uint32(block.Height))
			uw.logger.Debugf("Received new best btc block: %d", block.Height)
		case <-uw.quit:
			return
		}
	}
}

// checkBabylonDelegations iterates over all active babylon delegations, and reports not already
// tracked delegations to the unbondingDelegationChan
func (uw *StakingEventWatcher) checkBabylonDelegations() error {
	var i = uint64(0)
	for {
		delegations, err := uw.babylonNodeAdapter.BtcDelegations(i, uw.cfg.NewDelegationsBatchSize)

		if err != nil {
			return fmt.Errorf("error fetching active delegations from babylon: %v", err)
		}

		uw.logger.Debugf("fetched %d delegations from babylon", len(delegations))

		for _, delegation := range delegations {
			stakingTxHash := delegation.StakingTx.TxHash()

			del := &newDelegation{
				stakingTxHash:         stakingTxHash,
				stakingTx:             delegation.StakingTx,
				stakingOutputIdx:      delegation.StakingOutputIdx,
				delegationStartHeight: delegation.DelegationStartHeight,
				unbondingOutput:       delegation.UnbondingOutput,
			}

			// if we already have this delegation, skip it
			if uw.unbondingTracker.GetDelegation(stakingTxHash) == nil && delegation.HasProof {
				utils.PushOrQuit(uw.unbondingDelegationChan, del, uw.quit)
			}

			if uw.pendingTracker.GetDelegation(stakingTxHash) == nil && !delegation.HasProof {
				_, _ = uw.pendingTracker.AddDelegation(
					del.stakingTx,
					del.stakingOutputIdx,
					del.unbondingOutput,
				)
			}
		}

		if len(delegations) < int(uw.cfg.NewDelegationsBatchSize) {
			// we received less delegations than we asked for, it means went through all of them
			return nil
		}

		i += uw.cfg.NewDelegationsBatchSize
	}
}

func (uw *StakingEventWatcher) fetchDelegations() {
	defer uw.wg.Done()
	ticker := time.NewTicker(uw.cfg.CheckDelegationsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			uw.logger.Debug("Querying babylon for new delegations")
			btcLightClientTipHeight, err := uw.babylonNodeAdapter.BtcClientTipHeight()

			if err != nil {
				uw.logger.Errorf("error fetching babylon tip height: %v", err)
				continue
			}

			currentBtcNodeHeight := uw.currentBestBlockHeight.Load()

			// Our local node is out of sync with the babylon btc light client. If we would
			// query for delegation we might receive delegations from blocks that we cannot check.
			// Log this and give node chance to catch up i.e do not check current delegations
			if currentBtcNodeHeight < btcLightClientTipHeight {
				uw.logger.Debugf("btc light client tip height is %d, connected node best block height is %d. Waiting for node to catch up", btcLightClientTipHeight, uw.currentBestBlockHeight.Load())
				continue
			}

			if err = uw.checkBabylonDelegations(); err != nil {
				uw.logger.Errorf("error checking babylon delegations: %v", err)
				continue
			}

		case <-uw.quit:
			uw.logger.Debug("fetch delegations loop quit")
			return
		}
	}
}

func getStakingTxInputIdx(tx *wire.MsgTx, td *TrackedDelegation) (int, error) {
	stakingTxHash := td.StakingTx.TxHash()

	for i, txIn := range tx.TxIn {
		if txIn.PreviousOutPoint.Hash == stakingTxHash && txIn.PreviousOutPoint.Index == td.StakingOutputIdx {
			return i, nil
		}
	}

	return -1, fmt.Errorf("transaction does not point to staking output. expected hash:%s, expected outoputidx: %d", stakingTxHash, td.StakingOutputIdx)
}

// tryParseStakerSignatureFromSpentTx tries to parse staker signature from unbonding tx.
// If provided tx is not unbonding tx it returns error.
func tryParseStakerSignatureFromSpentTx(tx *wire.MsgTx, td *TrackedDelegation) (*schnorr.Signature, error) {
	if len(tx.TxOut) != 1 {
		return nil, fmt.Errorf("unbonding tx must have exactly one output. Priovided tx has %d outputs", len(tx.TxOut))
	}

	if tx.TxOut[0].Value != td.UnbondingOutput.Value || !bytes.Equal(tx.TxOut[0].PkScript, td.UnbondingOutput.PkScript) {
		return nil, fmt.Errorf("unbonding tx must have ouput which matches unbonding output of retrieved from Babylon")
	}

	stakingTxInputIdx, err := getStakingTxInputIdx(tx, td)

	if err != nil {
		return nil, fmt.Errorf("unbonding tx does not spend staking output: %v", err)
	}

	stakingTxInput := tx.TxIn[stakingTxInputIdx]
	witnessLen := len(stakingTxInput.Witness)
	// minimal witness size for staking tx input is 4:
	// covenant_signature, staker_signature, script, control_block
	// If that is not the case, something weird is going on and we should investigate.
	if witnessLen < 4 {
		panic(fmt.Errorf("staking tx input witness has less than 4 elements for unbonding tx %s", tx.TxHash()))
	}

	// staker signature is 3rd element from the end
	stakerSignature := stakingTxInput.Witness[witnessLen-3]

	return schnorr.ParseSignature(stakerSignature)
}

// waitForDelegationToStopBeingActive polls babylon until delegation is no longer active.
func (uw *StakingEventWatcher) waitForDelegationToStopBeingActive(
	ctx context.Context,
	stakingTxHash chainhash.Hash,
) {
	_ = retry.Do(func() error {
		active, err := uw.babylonNodeAdapter.IsDelegationActive(stakingTxHash)

		if err != nil {
			return fmt.Errorf("error checking if delegation is active: %v", err)
		}

		if !active {
			return nil
		}

		return fmt.Errorf("delegation for staking tx %s is still active", stakingTxHash)
	},
		retry.Context(ctx),
		retryForever,
		fixedDelyTypeWithJitter,
		retry.MaxDelay(uw.cfg.CheckDelegationActiveInterval),
		retry.MaxJitter(uw.cfg.RetryJitter),
		retry.OnRetry(func(n uint, err error) {
			uw.logger.Debugf("retrying checking if delegation is active for staking tx %s. Attempt: %d. Err: %v", stakingTxHash, n, err)
		}),
	)
}

func (uw *StakingEventWatcher) reportUnbondingToBabylon(
	ctx context.Context,
	stakingTxHash chainhash.Hash,
	unbondingSignature *schnorr.Signature,
) {
	_ = retry.Do(func() error {
		active, err := uw.babylonNodeAdapter.IsDelegationActive(stakingTxHash)

		if err != nil {
			return fmt.Errorf("error checking if delegation is active: %v", err)
		}

		if !active {
			//
			uw.logger.Debugf("cannot report unbonding. delegation for staking tx %s is no longer active", stakingTxHash)
			return nil
		}

		err = uw.babylonNodeAdapter.ReportUnbonding(ctx, stakingTxHash, unbondingSignature)

		if err != nil {
			uw.metrics.FailedReportedUnbondingTransactions.Inc()
			return fmt.Errorf("error reporting unbonding tx %s to babylon: %v", stakingTxHash, err)
		}

		uw.metrics.ReportedUnbondingTransactionsCounter.Inc()
		return nil
	},
		retry.Context(ctx),
		retryForever,
		fixedDelyTypeWithJitter,
		retry.MaxDelay(uw.cfg.RetrySubmitUnbondingTxInterval),
		retry.MaxJitter(uw.cfg.RetryJitter),
		retry.OnRetry(func(n uint, err error) {
			uw.logger.Debugf("retrying submitting unbodning tx, for staking tx: %s. Attempt: %d. Err: %v", stakingTxHash, n, err)
		}),
	)
}

func (uw *StakingEventWatcher) watchForSpend(spendEvent *notifier.SpendEvent, td *TrackedDelegation) {
	defer uw.wg.Done()
	quitCtx, cancel := uw.quitContext()
	defer cancel()

	var spendingTx *wire.MsgTx = nil
	select {
	case spendDetail := <-spendEvent.Spend:
		spendingTx = spendDetail.SpendingTx
	case <-uw.quit:
		return
	}

	schnorrSignature, err := tryParseStakerSignatureFromSpentTx(spendingTx, td)
	delegationId := td.StakingTx.TxHash()
	spendingTxHash := spendingTx.TxHash()

	if err != nil {
		uw.metrics.DetectedNonUnbondingTransactionsCounter.Inc()
		// Error means that this is not unbonding tx. At this point, it means that it is
		// either withdrawal transaction or slashing transaction spending staking staking output.
		// As we only care about unbonding transactions, we do not need to take additional actions.
		// We start polling babylon for delegation to stop being active, and then delete it from unbondingTracker.
		uw.logger.Debugf("Spending tx %s for staking tx %s is not unbonding tx. Info: %v", spendingTxHash, delegationId, err)
		uw.waitForDelegationToStopBeingActive(quitCtx, delegationId)
	} else {
		uw.metrics.DetectedUnbondingTransactionsCounter.Inc()
		// We found valid unbonding tx. We need to try to report it to babylon.
		// We stop reporting if delegation is no longer active or we succeed.
		uw.logger.Debugf("found unbonding tx %s for staking tx %s", spendingTxHash, delegationId)
		uw.reportUnbondingToBabylon(quitCtx, delegationId, schnorrSignature)
		uw.logger.Debugf("unbonding tx %s for staking tx %s reported to babylon", spendingTxHash, delegationId)
	}

	utils.PushOrQuit[*delegationInactive](
		uw.unbondingRemovalChan,
		&delegationInactive{stakingTxHash: delegationId},
		uw.quit,
	)
}

func (uw *StakingEventWatcher) handleDelegations() {
	defer uw.wg.Done()
	for {
		select {
		case activeDel := <-uw.unbondingDelegationChan:
			uw.logger.Debugf("Received new delegation to watch for staking transaction with hash %s", activeDel.stakingTxHash)

			del, err := uw.unbondingTracker.AddDelegation(
				activeDel.stakingTx,
				activeDel.stakingOutputIdx,
				activeDel.unbondingOutput,
			)

			if err != nil {
				uw.logger.Errorf("error adding delegation to unbondingTracker: %v", err)
				continue
			}

			uw.metrics.NumberOfTrackedActiveDelegations.Inc()

			stakingOutpoint := wire.OutPoint{
				Hash:  activeDel.stakingTxHash,
				Index: activeDel.stakingOutputIdx,
			}

			spendEv, err := uw.btcNotifier.RegisterSpendNtfn(
				&stakingOutpoint,
				activeDel.stakingTx.TxOut[activeDel.stakingOutputIdx].PkScript,
				uint32(activeDel.delegationStartHeight),
			)

			if err != nil {
				uw.logger.Errorf("error registering spend ntfn for staking tx %s: %v", activeDel.stakingTxHash, err)
				continue
			}

			uw.wg.Add(1)
			go uw.watchForSpend(spendEv, del)
		case in := <-uw.unbondingRemovalChan:
			uw.logger.Debugf("Delegation for staking transaction with hash %s stopped being active", in.stakingTxHash)
			// remove delegation from unbondingTracker
			uw.unbondingTracker.RemoveDelegation(in.stakingTxHash)

			uw.metrics.NumberOfTrackedActiveDelegations.Dec()

		case <-uw.quit:
			uw.logger.Debug("handle delegations loop quit")
			return
		}
	}
}

func (uw *StakingEventWatcher) handlerVerifiedDelegations() {
	defer uw.wg.Done()
	ticker := time.NewTicker(uw.cfg.CheckDelegationsInterval) // todo(lazar): use different interval in config
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			uw.logger.Debug("Checking for new staking txs")

			if err := uw.checkBtcForStakingTx(); err != nil {
				uw.logger.Errorf("error checking verified delegations: %v", err)
				continue
			}

		case <-uw.quit:
			uw.logger.Debug("verified delegations loop quit")
			return
		}
	}
}

// checkBtcForStakingTx
func (uw *StakingEventWatcher) checkBtcForStakingTx() error {
	delegations := uw.pendingTracker.GetDelegations()
	if delegations == nil {
		return nil
	}

	for _, del := range delegations {
		txHash := del.StakingTx.TxHash()
		// todo (lazar): check if this is valid, of we need to getRawTx
		details, status, err := uw.btcClient.TxDetails(&txHash, del.StakingTx.TxOut[del.StakingOutputIdx].PkScript)
		if err != nil {
			uw.logger.Debugf("error getting tx %v", txHash)
			continue
		}

		if status != btcclient.TxInChain {
			continue
		}

		btcTxs := types.GetWrappedTxs(details.Block)
		ib := types.NewIndexedBlock(int32(details.BlockHeight), &details.Block.Header, btcTxs)

		proof, err := ib.GenSPVProof(int(details.TxIndex))
		if err != nil {
			uw.logger.Debugf("error making spv proof %w", err)
			continue
		}

		go uw.activateBtcDelegation(txHash, proof)
	}

	return nil
}

func (uw *StakingEventWatcher) activateBtcDelegation(
	stakingTxHash chainhash.Hash, proof *btcctypes.BTCSpvProof) {
	ctx, cancel := uw.quitContext()
	defer cancel()

	_ = retry.Do(func() error {
		verified, err := uw.babylonNodeAdapter.IsDelegationVerified(stakingTxHash)
		if err != nil {
			return fmt.Errorf("error checking if delegation is active: %v", err)
		}

		if !verified {
			uw.logger.Debugf("skipping tx %s is not in verified status", stakingTxHash)
			return nil
		}

		if err := uw.babylonNodeAdapter.ActivateDelegation(ctx, stakingTxHash, proof); err != nil {
			uw.metrics.FailedReportedActivateDelegations.Inc()
			return fmt.Errorf("error reporting activate delegation tx %s to babylon: %v", stakingTxHash, err)
		}

		uw.metrics.ReportedActivateDelegationsCounter.Inc()

		uw.pendingTracker.RemoveDelegation(stakingTxHash)

		return nil
	},
		retry.Context(ctx),
		retry.Attempts(5), // todo(Lazar): add this to config, we prob don't want to retry forever here
		fixedDelyTypeWithJitter,
		retry.MaxDelay(uw.cfg.RetrySubmitUnbondingTxInterval),
		retry.MaxJitter(uw.cfg.RetryJitter),
		retry.OnRetry(func(n uint, err error) {
			uw.logger.Debugf("retrying to submit activation tx, for staking tx: %s. Attempt: %d. Err: %v", stakingTxHash, n, err)
		}),
	)
}
