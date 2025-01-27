package stakingeventwatcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/semaphore"
	"sync"
	"sync/atomic"
	"time"

	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/types"

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
	fixedDelyTypeWithJitter  = retry.DelayType(retry.CombineDelay(retry.FixedDelay, retry.RandomDelay))
	retryForever             = retry.Attempts(0)
	maxConcurrentActivations = int64(1500)
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
	delegationStartHeight uint32
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
	// keeps track of unbonding delegations, used as a cache to avoid registering ntfn twice
	unbondingTracker *TrackedDelegations
	// keeps track of verified delegations to be activated, periodically iterate over them and try to activate them
	pendingTracker *TrackedDelegations
	// used for metrics purposes, keeps track of verified delegations that are not in consumer chain
	verifiedNotInChainTracker *TrackedDelegations
	// used for metrics purposes, keeps track of verified delegations in chain but without sufficient confirmations
	verifiedInsufficientConfTracker *TrackedDelegations
	// used for metrics purposes, keeps track of verified delegations that are currently trying to be activated
	verifiedSufficientConfTracker *TrackedDelegations

	unbondingDelegationChan chan *newDelegation
	unbondingRemovalChan    chan *delegationInactive
	currentBestBlockHeight  atomic.Uint32
	activationLimiter       *semaphore.Weighted
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
		quit:                            make(chan struct{}),
		cfg:                             cfg,
		logger:                          parentLogger.With(zap.String("module", "staking_event_watcher")).Sugar(),
		btcNotifier:                     btcNotifier,
		btcClient:                       btcClient,
		babylonNodeAdapter:              babylonNodeAdapter,
		metrics:                         metrics,
		unbondingTracker:                NewTrackedDelegations(),
		pendingTracker:                  NewTrackedDelegations(),
		verifiedInsufficientConfTracker: NewTrackedDelegations(),
		verifiedNotInChainTracker:       NewTrackedDelegations(),
		verifiedSufficientConfTracker:   NewTrackedDelegations(),
		unbondingDelegationChan:         make(chan *newDelegation),
		unbondingRemovalChan:            make(chan *delegationInactive),
		activationLimiter:               semaphore.NewWeighted(maxConcurrentActivations), // todo(lazar): this should be in config
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
			if block.Height < 0 {
				panic(fmt.Errorf("received negative block height: %d", block.Height))
			}
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
			if block.Height < 0 {
				panic(fmt.Errorf("received negative block height: %d", block.Height))
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
func (sew *StakingEventWatcher) checkBabylonDelegations(status btcstakingtypes.BTCDelegationStatus, addDel func(del Delegation)) error {
	defer sew.latency(fmt.Sprintf("checkBabylonDelegations: %s", status))()

	var i = uint64(0)
	for {
		delegations, err := sew.babylonNodeAdapter.DelegationsByStatus(status, i, sew.cfg.NewDelegationsBatchSize)

		if err != nil {
			return fmt.Errorf("error fetching active delegations from babylon: %w", err)
		}

		sew.logger.Debugf("fetched %d delegations from babylon by status %s", len(delegations), status)

		for _, delegation := range delegations {
			addDel(delegation)
		}

		if uint64(len(delegations)) < sew.cfg.NewDelegationsBatchSize {
			// we received fewer delegations than we asked for; it means went through all of them
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

			nodeSynced, err := sew.syncedWithBabylon()
			if err != nil || !nodeSynced {
				// Log message and continue if there's an error or node isn't synced
				continue
			}

			addToUnbondingFunc := func(delegation Delegation) {
				del := &newDelegation{
					stakingTxHash:         delegation.StakingTx.TxHash(),
					stakingTx:             delegation.StakingTx,
					stakingOutputIdx:      delegation.StakingOutputIdx,
					delegationStartHeight: delegation.DelegationStartHeight,
					unbondingOutput:       delegation.UnbondingOutput,
				}

				// if we already have this delegation, we still want to check if it has changed,
				// we should track both verified and active status for unbonding
				changed, exists := sew.unbondingTracker.HasDelegationChanged(delegation.StakingTx.TxHash(), del)
				if exists && changed {
					// The Delegation exists and has changed, push the update.
					utils.PushOrQuit(sew.unbondingDelegationChan, del, sew.quit)
				} else if !exists {
					// The Delegation doesn't exist, push the new delegation.
					utils.PushOrQuit(sew.unbondingDelegationChan, del, sew.quit)
				}
			}

			addToPendingFunc := func(delegation Delegation) {
				del := &newDelegation{
					stakingTxHash:         delegation.StakingTx.TxHash(),
					stakingTx:             delegation.StakingTx,
					stakingOutputIdx:      delegation.StakingOutputIdx,
					delegationStartHeight: delegation.DelegationStartHeight,
					unbondingOutput:       delegation.UnbondingOutput,
				}
				_, exists := sew.pendingTracker.GetDelegation(delegation.StakingTx.TxHash())
				if !exists && !delegation.HasProof {
					_, _ = sew.pendingTracker.AddDelegation(
						del.stakingTx,
						del.stakingOutputIdx,
						del.unbondingOutput,
						del.delegationStartHeight,
						false,
					)
					sew.metrics.NumberOfVerifiedDelegations.Inc()
				}
			}

			var wg sync.WaitGroup
			wg.Add(3)

			go func() {
				defer wg.Done()
				if err = sew.checkBabylonDelegations(btcstakingtypes.BTCDelegationStatus_ACTIVE, addToUnbondingFunc); err != nil {
					sew.logger.Errorf("error checking babylon delegations: %v", err)
				}
			}()

			go func() {
				defer wg.Done()
				if err = sew.checkBabylonDelegations(btcstakingtypes.BTCDelegationStatus_VERIFIED, addToUnbondingFunc); err != nil {
					sew.logger.Errorf("error checking babylon delegations: %v", err)
				}
			}()

			go func() {
				defer wg.Done()
				if err = sew.checkBabylonDelegations(btcstakingtypes.BTCDelegationStatus_VERIFIED, addToPendingFunc); err != nil {
					sew.logger.Errorf("error checking babylon delegations: %v", err)
				}
			}()

			wg.Wait()
		case <-sew.quit:
			sew.logger.Debug("fetch delegations loop quit")

			return
		}
	}
}

func (sew *StakingEventWatcher) syncedWithBabylon() (bool, error) {
	btcLightClientTipHeight, err := sew.babylonNodeAdapter.BtcClientTipHeight()
	if err != nil {
		sew.logger.Errorf("error fetching babylon tip height: %v", err)

		return false, err
	}

	currentBtcNodeHeight := sew.currentBestBlockHeight.Load()

	if currentBtcNodeHeight < btcLightClientTipHeight {
		sew.logger.Debugf("btc light client tip height is %d, connected node best block height is %d. Waiting for node to catch up", btcLightClientTipHeight, currentBtcNodeHeight)

		return false, nil
	}

	return true, nil
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
		return nil, fmt.Errorf("unbonding tx must have output which matches unbonding output of retrieved from Babylon")
	}

	stakingTxInputIdx, err := getStakingTxInputIdx(tx, td)

	if err != nil {
		return nil, fmt.Errorf("unbonding tx does not spend staking output: %w", err)
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

func (sew *StakingEventWatcher) reportUnbondingToBabylon(
	ctx context.Context,
	stakingTxHash chainhash.Hash,
	stakeSpendingTx *wire.MsgTx,
	proof *btcstakingtypes.InclusionProof,
) {
	_ = retry.Do(func() error {
		active, err := sew.babylonNodeAdapter.IsDelegationActive(stakingTxHash)

		if err != nil {
			return fmt.Errorf("error checking if delegation is active: %w", err)
		}

		verified, err := sew.babylonNodeAdapter.IsDelegationVerified(stakingTxHash)

		if err != nil {
			return fmt.Errorf("error checking if delegation is verified: %w", err)
		}

		if !active && !verified {
			sew.logger.Debugf("cannot report unbonding. delegation for staking tx %s is no longer active", stakingTxHash)

			return nil
		}

		if err = sew.babylonNodeAdapter.ReportUnbonding(ctx, stakingTxHash, stakeSpendingTx, proof); err != nil {
			sew.metrics.FailedReportedUnbondingTransactions.Inc()

			return fmt.Errorf("error reporting unbonding tx %s to babylon: %w", stakingTxHash, err)
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

	var spendingTx *wire.MsgTx
	select {
	case spendDetail := <-spendEvent.Spend:
		spendingTx = spendDetail.SpendingTx
	case <-sew.quit:
		return
	}

	_, err := tryParseStakerSignatureFromSpentTx(spendingTx, td)
	delegationID := td.StakingTx.TxHash()
	spendingTxHash := spendingTx.TxHash()

	if err != nil {
		sew.metrics.DetectedNonUnbondingTransactionsCounter.Inc()
		// Error means that this is not unbonding tx. At this point, it means that it is
		// either withdrawal transaction or slashing transaction spending staking staking output.
		// As we only care about unbonding transactions, we do not need to take additional actions.
		// We start polling babylon for delegation to stop being active, and then delete it from unbondingTracker.
		sew.logger.Debugf("Spending tx %s for staking tx %s is not unbonding tx. Info: %v", spendingTxHash, delegationID, err)
		proof := sew.waitForStakeSpendInclusionProof(quitCtx, spendingTx)
		if proof == nil {
			sew.logger.Errorf("unbonding tx %s for staking tx %s proof not built", spendingTxHash, delegationID)

			return
		}
		sew.reportUnbondingToBabylon(quitCtx, delegationID, spendingTx, proof)
	} else {
		sew.metrics.DetectedUnbondingTransactionsCounter.Inc()
		// We found valid unbonding tx. We need to try to report it to babylon.
		// We stop reporting if delegation is no longer active or we succeed.
		proof := sew.waitForStakeSpendInclusionProof(quitCtx, spendingTx)
		if proof == nil {
			sew.logger.Errorf("unbonding tx %s for staking tx %s proof not built", spendingTxHash, delegationID)

			return
		}
		sew.logger.Debugf("found unbonding tx %s for staking tx %s", spendingTxHash, delegationID)
		sew.reportUnbondingToBabylon(quitCtx, delegationID, spendingTx, proof)
		sew.logger.Debugf("unbonding tx %s for staking tx %s reported to babylon", spendingTxHash, delegationID)
	}

	utils.PushOrQuit[*delegationInactive](
		sew.unbondingRemovalChan,
		&delegationInactive{stakingTxHash: delegationID},
		sew.quit,
	)
}

func (sew *StakingEventWatcher) buildSpendingTxProof(spendingTx *wire.MsgTx) (*btcstakingtypes.InclusionProof, error) {
	txHash := spendingTx.TxHash()
	if len(spendingTx.TxOut) == 0 {
		panic(fmt.Errorf("stake spending tx has no outputs %s", spendingTx.TxHash().String())) // this is a software error
	}
	details, status, err := sew.btcClient.TxDetails(&txHash, spendingTx.TxOut[0].PkScript)
	if err != nil {
		return nil, err
	}

	if status != btcclient.TxInChain {
		return nil, nil
	}

	btcTxs := types.GetWrappedTxs(details.Block)
	ib := types.NewIndexedBlock(details.BlockHeight, &details.Block.Header, btcTxs)

	proof, err := ib.GenSPVProof(int(details.TxIndex))
	if err != nil {
		return nil, err
	}

	return btcstakingtypes.NewInclusionProofFromSpvProof(proof), nil
}

// waitForStakeSpendInclusionProof polls btc until stake spend tx has inclusion proof built
func (sew *StakingEventWatcher) waitForStakeSpendInclusionProof(
	ctx context.Context,
	spendingTx *wire.MsgTx,
) *btcstakingtypes.InclusionProof {
	var (
		proof *btcstakingtypes.InclusionProof
		err   error
	)
	_ = retry.Do(func() error {
		proof, err = sew.buildSpendingTxProof(spendingTx)
		if err != nil {
			return err
		}

		if proof == nil {
			return fmt.Errorf("proof not yet built")
		}

		return nil
	},
		retry.Context(ctx),
		retryForever,
		fixedDelyTypeWithJitter,
		retry.MaxDelay(sew.cfg.CheckDelegationActiveInterval),
		retry.MaxJitter(sew.cfg.RetryJitter),
		retry.OnRetry(func(n uint, err error) {
			sew.logger.Debugf("retrying checking if stake spending tx is in chain %s. Attempt: %d. Err: %v", spendingTx.TxHash(), n, err)
		}),
	)

	return proof
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
				activeDel.delegationStartHeight,
				true,
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
				activeDel.delegationStartHeight,
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
			sew.checkBtcForStakingTx()
		case <-sew.quit:
			sew.logger.Debug("verified delegations loop quit")

			return
		}
	}
}

// checkBtcForStakingTx gets a snapshot of current Delegations in cache
// checks if staking tx is in BTC, generates a proof and invokes sending of MsgAddBTCDelegationInclusionProof
func (sew *StakingEventWatcher) checkBtcForStakingTx() {
	params, err := sew.babylonNodeAdapter.Params()
	if err != nil {
		sew.logger.Errorf("error getting tx params %v", err)

		return
	}

	for del := range sew.pendingTracker.DelegationsIter(1000) {
		if del.ActivationInProgress {
			continue
		}
		txHash := del.StakingTx.TxHash()

		details, status, err := sew.btcClient.TxDetails(&txHash, del.StakingTx.TxOut[del.StakingOutputIdx].PkScript)
		if err != nil {
			sew.logger.Debugf("error getting tx %v", txHash)

			continue
		}

		if status != btcclient.TxInChain {
			if err := sew.verifiedNotInChainTracker.AddEmptyDelegation(txHash); err == nil {
				sew.metrics.NumberOfVerifiedNotInChainDelegations.Set(float64(sew.verifiedNotInChainTracker.Count()))
			}

			continue
		}

		if _, exists := sew.verifiedNotInChainTracker.GetDelegation(txHash); exists {
			sew.verifiedNotInChainTracker.RemoveDelegation(txHash)
			sew.metrics.NumberOfVerifiedNotInChainDelegations.Set(float64(sew.verifiedNotInChainTracker.Count()))
		}

		btcTxs := types.GetWrappedTxs(details.Block)
		ib := types.NewIndexedBlock(details.BlockHeight, &details.Block.Header, btcTxs)

		proof, err := ib.GenSPVProof(int(details.TxIndex))
		if err != nil {
			sew.logger.Debugf("error making spv proof %s", err)

			continue
		}

		if err := sew.activationLimiter.Acquire(context.Background(), 1); err != nil {
			sew.logger.Warnf("error acquiring a activation semaphore %s", err)

			continue
		}

		if err := sew.pendingTracker.UpdateActivation(txHash, true); err != nil {
			sew.logger.Debugf("error updating activation in pending tracker tx: %v", txHash)
			sew.activationLimiter.Release(1) // in probable edge case, insure we release the sem

			continue
		}

		go func() {
			defer sew.activationLimiter.Release(1)
			sew.activateBtcDelegation(txHash, proof, details.Block.BlockHash(), params.ConfirmationTimeBlocks)
		}()
	}
}

// activateBtcDelegation invokes bbn client and send MsgAddBTCDelegationInclusionProof
func (sew *StakingEventWatcher) activateBtcDelegation(
	stakingTxHash chainhash.Hash,
	proof *btcctypes.BTCSpvProof,
	inclusionBlockHash chainhash.Hash,
	requiredDepth uint32,
) {
	sew.metrics.NumberOfActivationInProgress.Inc()
	defer sew.metrics.NumberOfActivationInProgress.Dec()

	ctx, cancel := sew.quitContext()
	defer cancel()

	defer sew.latency("activateBtcDelegation")()
	defer func() {
		if err := sew.pendingTracker.UpdateActivation(stakingTxHash, false); err != nil {
			sew.logger.Debugf("err updating activation in pending tracker tx: %v", stakingTxHash)
		}
	}()

	if err := sew.waitForRequiredDepth(ctx, stakingTxHash, &inclusionBlockHash, requiredDepth); err != nil {
		sew.logger.Warnf("exceeded waiting for required depth for tx: %s, will try later: err %v", stakingTxHash.String(), err)

		if err := sew.verifiedInsufficientConfTracker.AddEmptyDelegation(stakingTxHash); err == nil {
			sew.metrics.NumberOfVerifiedInsufficientConfDelegations.Set(float64(sew.verifiedInsufficientConfTracker.Count()))
		}

		return
	}

	if _, exists := sew.verifiedInsufficientConfTracker.GetDelegation(stakingTxHash); exists {
		sew.verifiedInsufficientConfTracker.RemoveDelegation(stakingTxHash)
		sew.metrics.NumberOfVerifiedInsufficientConfDelegations.Set(float64(sew.verifiedInsufficientConfTracker.Count()))
	}

	defer sew.latency("activateDelegationRPC")()

	if err := sew.verifiedSufficientConfTracker.AddEmptyDelegation(stakingTxHash); err == nil {
		sew.metrics.NumberOfVerifiedSufficientConfDelegations.Set(float64(sew.verifiedSufficientConfTracker.Count()))
	}

	_ = retry.Do(func() error {
		verified, err := sew.babylonNodeAdapter.IsDelegationVerified(stakingTxHash)
		if err != nil {
			return fmt.Errorf("error checking if delegation is active: %w", err)
		}

		if !verified {
			sew.logger.Debugf("skipping tx %s is not in verified status", stakingTxHash)

			return nil
		}

		if err := sew.babylonNodeAdapter.ActivateDelegation(ctx, stakingTxHash, proof); err != nil {
			sew.metrics.FailedReportedActivateDelegations.Inc()

			return fmt.Errorf("error reporting activate delegation tx %s to babylon: %w", stakingTxHash, err)
		}

		sew.metrics.ReportedActivateDelegationsCounter.Inc()
		sew.pendingTracker.RemoveDelegation(stakingTxHash)
		sew.metrics.NumberOfVerifiedDelegations.Dec()

		sew.logger.Debugf("staking tx activated %s", stakingTxHash.String())

		if _, exists := sew.verifiedSufficientConfTracker.GetDelegation(stakingTxHash); exists {
			sew.verifiedSufficientConfTracker.RemoveDelegation(stakingTxHash)
			sew.metrics.NumberOfVerifiedInsufficientConfDelegations.Set(float64(sew.verifiedSufficientConfTracker.Count()))
		}

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

func (sew *StakingEventWatcher) waitForRequiredDepth(
	ctx context.Context,
	stakingTxHash chainhash.Hash,
	inclusionBlockHash *chainhash.Hash,
	requiredDepth uint32,
) error {
	defer sew.latency("waitForRequiredDepth")()

	var depth uint32

	return retry.Do(func() error {
		var err error
		depth, err = sew.babylonNodeAdapter.QueryHeaderDepth(inclusionBlockHash)
		if err != nil {
			// If the header is not known to babylon, or it is on LCFork, then most probably
			// lc is not up to date, we should retry sending delegation after some time.
			if errors.Is(err, ErrHeaderNotKnownToBabylon) || errors.Is(err, ErrHeaderOnBabylonLCFork) {
				return fmt.Errorf("btc light client error %s: %w", err.Error(), ErrBabylonBtcLightClientNotReady)
			}

			return fmt.Errorf("error while getting delegation data: %w", err)
		}

		if depth < requiredDepth {
			return fmt.Errorf("btc lc not ready, required depth: %d, current depth: %d: %w",
				requiredDepth, depth, ErrBabylonBtcLightClientNotReady)
		}

		return nil
	},
		retry.Context(ctx),
		retry.Attempts(20),
		fixedDelyTypeWithJitter,
		retry.MaxDelay(sew.cfg.RetrySubmitUnbondingTxInterval),
		retry.MaxJitter(sew.cfg.RetryJitter),
		retry.LastErrorOnly(true), // let's avoid high log spam
		retry.OnRetry(func(n uint, err error) {
			sew.logger.Debugf("waiting for staking tx: %s to be k-deep. Current[%d], required[%d]. "+
				"Attempt: %d. Err: %v", stakingTxHash, depth, requiredDepth, n, err)
		}),
	)
}

func (sew *StakingEventWatcher) latency(method string) func() {
	startTime := time.Now()

	return func() {
		duration := time.Since(startTime)
		sew.logger.Debugf("execution time for method: %s, duration: %s", method, duration.String())
		sew.metrics.MethodExecutionLatency.WithLabelValues(method).Observe(duration.Seconds())
	}
}
