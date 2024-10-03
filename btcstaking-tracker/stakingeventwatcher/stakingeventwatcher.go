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

func (sew *StakingEventWatcher) quitContext() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	sew.wg.Add(1)
	go func() {
		defer cancel()
		defer sew.wg.Done()

		select {
		case <-sew.quit:

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

func (sew *StakingEventWatcher) Start() error {
	var startErr error
	sew.startOnce.Do(func() {
		sew.logger.Info("starting staking event watcher")

		blockEventNotifier, err := sew.btcNotifier.RegisterBlockEpochNtfn(nil)
		if err != nil {
			startErr = err
			return
		}

		// we registered for notifications with `nil` so we should receive best block immediately
		select {
		case block := <-blockEventNotifier.Epochs:
			sew.currentBestBlockHeight.Store(uint32(block.Height))
		case <-sew.quit:
			startErr = errors.New("watcher quit before finishing start")
			return
		}

		sew.logger.Infof("Initial btc best block height is: %d", sew.currentBestBlockHeight.Load())

		sew.wg.Add(4)
		go sew.handleNewBlocks(blockEventNotifier)
		go sew.handleUnbondedDelegations()
		go sew.fetchDelegations()
		go sew.handlerVerifiedDelegations()

		sew.logger.Info("staking event watcher started")
	})
	return startErr
}

func (sew *StakingEventWatcher) Stop() error {
	var stopErr error
	sew.stopOnce.Do(func() {
		sew.logger.Info("stopping staking event watcher")
		close(sew.quit)
		sew.wg.Wait()
		sew.logger.Info("stopped staking event watcher")
	})
	return stopErr
}

func (sew *StakingEventWatcher) handleNewBlocks(blockNotifier *notifier.BlockEpochEvent) {
	defer sew.wg.Done()
	defer blockNotifier.Cancel()
	for {
		select {
		case block, ok := <-blockNotifier.Epochs:
			if !ok {
				return
			}
			sew.currentBestBlockHeight.Store(uint32(block.Height))
			sew.logger.Debugf("Received new best btc block: %d", block.Height)
		case <-sew.quit:
			return
		}
	}
}

// checkBabylonDelegations iterates over all active babylon delegations, and reports not already
// tracked delegations to the unbondingDelegationChan
func (sew *StakingEventWatcher) checkBabylonDelegations() error {
	var i = uint64(0)
	for {
		delegations, err := sew.babylonNodeAdapter.BtcDelegations(i, sew.cfg.NewDelegationsBatchSize)

		if err != nil {
			return fmt.Errorf("error fetching active delegations from babylon: %v", err)
		}

		sew.logger.Debugf("fetched %d delegations from babylon", len(delegations))

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
			// we should track both verified and active status for unbonding
			if sew.unbondingTracker.GetDelegation(stakingTxHash) == nil {
				utils.PushOrQuit(sew.unbondingDelegationChan, del, sew.quit)
			}

			if sew.pendingTracker.GetDelegation(stakingTxHash) == nil && !delegation.HasProof {
				_, _ = sew.pendingTracker.AddDelegation(
					del.stakingTx,
					del.stakingOutputIdx,
					del.unbondingOutput,
				)
			}
		}

		if len(delegations) < int(sew.cfg.NewDelegationsBatchSize) {
			// we received less delegations than we asked for, it means went through all of them
			return nil
		}

		i += sew.cfg.NewDelegationsBatchSize
	}
}

func (sew *StakingEventWatcher) fetchDelegations() {
	defer sew.wg.Done()
	ticker := time.NewTicker(sew.cfg.CheckDelegationsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sew.logger.Debug("Querying babylon for new delegations")
			btcLightClientTipHeight, err := sew.babylonNodeAdapter.BtcClientTipHeight()

			if err != nil {
				sew.logger.Errorf("error fetching babylon tip height: %v", err)
				continue
			}

			currentBtcNodeHeight := sew.currentBestBlockHeight.Load()

			// Our local node is out of sync with the babylon btc light client. If we would
			// query for delegation we might receive delegations from blocks that we cannot check.
			// Log this and give node chance to catch up i.e do not check current delegations
			if currentBtcNodeHeight < btcLightClientTipHeight {
				sew.logger.Debugf("btc light client tip height is %d, connected node best block height is %d. Waiting for node to catch up", btcLightClientTipHeight, sew.currentBestBlockHeight.Load())
				continue
			}

			if err = sew.checkBabylonDelegations(); err != nil {
				sew.logger.Errorf("error checking babylon delegations: %v", err)
				continue
			}

		case <-sew.quit:
			sew.logger.Debug("fetch delegations loop quit")
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
func (sew *StakingEventWatcher) waitForDelegationToStopBeingActive(
	ctx context.Context,
	stakingTxHash chainhash.Hash,
) {
	_ = retry.Do(func() error {
		active, err := sew.babylonNodeAdapter.IsDelegationActive(stakingTxHash)

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
		retry.MaxDelay(sew.cfg.CheckDelegationActiveInterval),
		retry.MaxJitter(sew.cfg.RetryJitter),
		retry.OnRetry(func(n uint, err error) {
			sew.logger.Debugf("retrying checking if delegation is active for staking tx %s. Attempt: %d. Err: %v", stakingTxHash, n, err)
		}),
	)
}

func (sew *StakingEventWatcher) reportUnbondingToBabylon(
	ctx context.Context,
	stakingTxHash chainhash.Hash,
	unbondingSignature *schnorr.Signature,
) {
	_ = retry.Do(func() error {
		active, err := sew.babylonNodeAdapter.IsDelegationActive(stakingTxHash)

		if err != nil {
			return fmt.Errorf("error checking if delegation is active: %v", err)
		}

		verified, err := sew.babylonNodeAdapter.IsDelegationVerified(stakingTxHash)

		if err != nil {
			return fmt.Errorf("error checking if delegation is verified: %v", err)
		}

		if !active && !verified {
			sew.logger.Debugf("cannot report unbonding. delegation for staking tx %s is no longer active", stakingTxHash)
			return nil
		}

		if err = sew.babylonNodeAdapter.ReportUnbonding(ctx, stakingTxHash, unbondingSignature); err != nil {
			sew.metrics.FailedReportedUnbondingTransactions.Inc()
			return fmt.Errorf("error reporting unbonding tx %s to babylon: %v", stakingTxHash, err)
		}

		sew.metrics.ReportedUnbondingTransactionsCounter.Inc()
		return nil
	},
		retry.Context(ctx),
		retryForever,
		fixedDelyTypeWithJitter,
		retry.MaxDelay(sew.cfg.RetrySubmitUnbondingTxInterval),
		retry.MaxJitter(sew.cfg.RetryJitter),
		retry.OnRetry(func(n uint, err error) {
			sew.logger.Debugf("retrying submitting unbodning tx, for staking tx: %s. Attempt: %d. Err: %v", stakingTxHash, n, err)
		}),
	)
}

func (sew *StakingEventWatcher) watchForSpend(spendEvent *notifier.SpendEvent, td *TrackedDelegation) {
	defer sew.wg.Done()
	quitCtx, cancel := sew.quitContext()
	defer cancel()

	var spendingTx *wire.MsgTx = nil
	select {
	case spendDetail := <-spendEvent.Spend:
		spendingTx = spendDetail.SpendingTx
	case <-sew.quit:
		return
	}

	schnorrSignature, err := tryParseStakerSignatureFromSpentTx(spendingTx, td)
	delegationId := td.StakingTx.TxHash()
	spendingTxHash := spendingTx.TxHash()

	if err != nil {
		sew.metrics.DetectedNonUnbondingTransactionsCounter.Inc()
		// Error means that this is not unbonding tx. At this point, it means that it is
		// either withdrawal transaction or slashing transaction spending staking staking output.
		// As we only care about unbonding transactions, we do not need to take additional actions.
		// We start polling babylon for delegation to stop being active, and then delete it from unbondingTracker.
		sew.logger.Debugf("Spending tx %s for staking tx %s is not unbonding tx. Info: %v", spendingTxHash, delegationId, err)
		sew.waitForDelegationToStopBeingActive(quitCtx, delegationId)
	} else {
		sew.metrics.DetectedUnbondingTransactionsCounter.Inc()
		// We found valid unbonding tx. We need to try to report it to babylon.
		// We stop reporting if delegation is no longer active or we succeed.
		sew.logger.Debugf("found unbonding tx %s for staking tx %s", spendingTxHash, delegationId)
		sew.reportUnbondingToBabylon(quitCtx, delegationId, schnorrSignature)
		sew.logger.Debugf("unbonding tx %s for staking tx %s reported to babylon", spendingTxHash, delegationId)
	}

	utils.PushOrQuit[*delegationInactive](
		sew.unbondingRemovalChan,
		&delegationInactive{stakingTxHash: delegationId},
		sew.quit,
	)
}

func (sew *StakingEventWatcher) handleUnbondedDelegations() {
	defer sew.wg.Done()
	for {
		select {
		case activeDel := <-sew.unbondingDelegationChan:
			sew.logger.Debugf("Received new delegation to watch for staking transaction with hash %s", activeDel.stakingTxHash)

			del, err := sew.unbondingTracker.AddDelegation(
				activeDel.stakingTx,
				activeDel.stakingOutputIdx,
				activeDel.unbondingOutput,
			)

			if err != nil {
				sew.logger.Errorf("error adding delegation to unbondingTracker: %v", err)
				continue
			}

			sew.metrics.NumberOfTrackedActiveDelegations.Inc()

			stakingOutpoint := wire.OutPoint{
				Hash:  activeDel.stakingTxHash,
				Index: activeDel.stakingOutputIdx,
			}

			spendEv, err := sew.btcNotifier.RegisterSpendNtfn(
				&stakingOutpoint,
				activeDel.stakingTx.TxOut[activeDel.stakingOutputIdx].PkScript,
				uint32(activeDel.delegationStartHeight),
			)

			if err != nil {
				sew.logger.Errorf("error registering spend ntfn for staking tx %s: %v", activeDel.stakingTxHash, err)
				continue
			}

			sew.wg.Add(1)
			go sew.watchForSpend(spendEv, del)
		case in := <-sew.unbondingRemovalChan:
			sew.logger.Debugf("Delegation for staking transaction with hash %s stopped being active", in.stakingTxHash)
			// remove delegation from unbondingTracker
			sew.unbondingTracker.RemoveDelegation(in.stakingTxHash)

			sew.metrics.NumberOfTrackedActiveDelegations.Dec()

		case <-sew.quit:
			sew.logger.Debug("handle delegations loop quit")
			return
		}
	}
}

func (sew *StakingEventWatcher) handlerVerifiedDelegations() {
	defer sew.wg.Done()
	ticker := time.NewTicker(sew.cfg.CheckDelegationsInterval) // todo(lazar): use different interval in config
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sew.logger.Debug("Checking for new staking txs")

			if err := sew.checkBtcForStakingTx(); err != nil {
				sew.logger.Errorf("error checking verified delegations: %v", err)
				continue
			}

		case <-sew.quit:
			sew.logger.Debug("verified delegations loop quit")
			return
		}
	}
}

// checkBtcForStakingTx gets a snapshot of current Delegations in cache
// checks if staking tx is in BTC, generates a proof and invokes sending of MsgAddBTCDelegationInclusionProof
func (sew *StakingEventWatcher) checkBtcForStakingTx() error {
	delegations := sew.pendingTracker.GetDelegations()
	if delegations == nil {
		return nil
	}

	for _, del := range delegations {
		txHash := del.StakingTx.TxHash()
		details, status, err := sew.btcClient.TxDetails(&txHash, del.StakingTx.TxOut[del.StakingOutputIdx].PkScript)
		if err != nil {
			sew.logger.Debugf("error getting tx %v", txHash)
			continue
		}

		if status != btcclient.TxInChain {
			continue
		}

		btcTxs := types.GetWrappedTxs(details.Block)
		ib := types.NewIndexedBlock(int32(details.BlockHeight), &details.Block.Header, btcTxs)

		proof, err := ib.GenSPVProof(int(details.TxIndex))
		if err != nil {
			sew.logger.Debugf("error making spv proof %s", err)
			continue
		}

		go sew.activateBtcDelegation(txHash, proof)
	}

	return nil
}

// activateBtcDelegation invokes bbn client and send MsgAddBTCDelegationInclusionProof
func (sew *StakingEventWatcher) activateBtcDelegation(
	stakingTxHash chainhash.Hash, proof *btcctypes.BTCSpvProof) {
	ctx, cancel := sew.quitContext()
	defer cancel()

	_ = retry.Do(func() error {
		verified, err := sew.babylonNodeAdapter.IsDelegationVerified(stakingTxHash)
		if err != nil {
			return fmt.Errorf("error checking if delegation is active: %v", err)
		}

		if !verified {
			sew.logger.Debugf("skipping tx %s is not in verified status", stakingTxHash)
			return nil
		}

		if err := sew.babylonNodeAdapter.ActivateDelegation(ctx, stakingTxHash, proof); err != nil {
			sew.metrics.FailedReportedActivateDelegations.Inc()
			return fmt.Errorf("error reporting activate delegation tx %s to babylon: %v", stakingTxHash, err)
		}

		sew.metrics.ReportedActivateDelegationsCounter.Inc()

		sew.pendingTracker.RemoveDelegation(stakingTxHash)

		return nil
	},
		retry.Context(ctx),
		retry.Attempts(5), // todo(Lazar): add this to config
		fixedDelyTypeWithJitter,
		retry.MaxDelay(sew.cfg.RetrySubmitUnbondingTxInterval),
		retry.MaxJitter(sew.cfg.RetryJitter),
		retry.OnRetry(func(n uint, err error) {
			sew.logger.Debugf("retrying to submit activation tx, for staking tx: %s. Attempt: %d. Err: %v", stakingTxHash, n, err)
		}),
	)
}
