package stakingeventwatcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/babylonlabs-io/babylon/v4/client/babylonclient"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	btcctypes "github.com/babylonlabs-io/babylon/v4/x/btccheckpoint/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v4/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/types"
	"golang.org/x/sync/semaphore"

	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/utils"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcjson"
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
	indexer     SpendChecker
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

	unbondingDelegationChan       chan *newDelegation
	unbondingRemovalChan          chan *delegationInactive
	currentBestBlockHeight        atomic.Uint32
	currentCometTipHeight         atomic.Int64
	delegationRetrievalInProgress atomic.Bool
	activationLimiter             *semaphore.Weighted
	babylonConfirmationTimeBlocks uint32
}

func NewStakingEventWatcher(
	btcNotifier notifier.ChainNotifier,
	btcClient btcclient.BTCClient,
	indexer SpendChecker,
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
		indexer:                         indexer,
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

		blockEventNotifier.Cancel()

		sew.logger.Infof("Initial btc best block height is: %d", sew.currentBestBlockHeight.Load())

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		latestHeight, err := sew.babylonNodeAdapter.CometBFTTipHeight(ctx)
		cancel()
		if err != nil {
			startErr = fmt.Errorf("error getting comet tip height: %w", err)

			return
		}

		params, err := sew.babylonNodeAdapter.Params()
		if err != nil {
			startErr = fmt.Errorf("error getting tx params: %s", err.Error())

			return
		}
		sew.babylonConfirmationTimeBlocks = params.ConfirmationTimeBlocks

		sew.currentCometTipHeight.Store(latestHeight)
		sew.delegationRetrievalInProgress.Store(true) // avoid potential race condition with fetchDelegations

		sew.wg.Add(5)
		go sew.handleNewBlocks()
		go sew.handleUnbondedDelegations()
		go sew.fetchDelegations()
		go sew.handlerVerifiedDelegations()
		go sew.fetchCometBftBlockForever()

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

func (sew *StakingEventWatcher) handleNewBlocks() {
	defer sew.wg.Done()

	for {
		select {
		case <-sew.quit:
			return
		default:
			if err := sew.runBlockNotifier(); err != nil {
				sew.logger.Errorf("Block notifier error: %v, reconnecting in 5s", err)
				select {
				case <-time.After(time.Second * 5):
				case <-sew.quit:
					return
				}
			}
		}
	}
}
func (sew *StakingEventWatcher) runBlockNotifier() error {
	sew.logger.Info("Starting RegisterBlockEpochNtfn")

	blockNotifier, err := sew.btcNotifier.RegisterBlockEpochNtfn(nil)
	if err != nil {
		return fmt.Errorf("failed to register block notifier: %w", err)
	}
	defer blockNotifier.Cancel()

	// Timeout as a fail-safe mechanism, as we should get !ok from blockNotifier.Epochs
	timeoutTimer := time.NewTimer(sew.cfg.ReconnectBTCNodeInterval)
	defer timeoutTimer.Stop()

	for {
		select {
		case block, ok := <-blockNotifier.Epochs:
			if !ok {
				return fmt.Errorf("block notifier channel closed")
			}

			// Reset timeout timer on each block received
			timeoutTimer.Stop()
			timeoutTimer = time.NewTimer(sew.cfg.ReconnectBTCNodeInterval)

			if block.Height < 0 {
				panic(fmt.Errorf("received negative block height: %d", block.Height))
			}

			sew.currentBestBlockHeight.Store(uint32(block.Height))
			sew.metrics.BestBlockHeight.Set(float64(block.Height))
			sew.logger.Debugf("Received new best btc block: %d", block.Height)

			if err := sew.checkSpend(); err != nil {
				sew.logger.Errorf("error checking spend: %v", err)
			}

		case <-timeoutTimer.C:
			return fmt.Errorf("no block received from notifier for %v, connection may be stale", sew.cfg.ReconnectBTCNodeInterval)

		case <-sew.quit:
			sew.logger.Info("Block notifier shutting down")

			return nil
		}
	}
}

// checkBabylonDelegations iterates over all babylon delegations and adds them to unbondingTracker or pendingTracker
func (sew *StakingEventWatcher) checkBabylonDelegations() error {
	status := btcstakingtypes.BTCDelegationStatus_ANY
	defer sew.latency(fmt.Sprintf("checkBabylonDelegations: %s", status))()

	cursor := []byte(nil)
	for {
		delegations, nextCursor, err := sew.babylonNodeAdapter.DelegationsByStatus(status, cursor, sew.cfg.NewDelegationsBatchSize)
		if err != nil {
			return fmt.Errorf("error fetching delegations: %w", err)
		}

		sew.logger.Debugf("fetched %d delegations from babylon by status %s", len(delegations), status)

		for _, delegation := range delegations {
			switch delegation.Status {
			case btcstakingtypes.BTCDelegationStatus_ACTIVE.String():
				sew.addToUnbondingFunc(delegation)
			case btcstakingtypes.BTCDelegationStatus_VERIFIED.String():
				sew.addToPendingFunc(delegation)
				sew.addToUnbondingFunc(delegation)
			}
		}

		if nextCursor == nil {
			break
		}
		cursor = nextCursor
	}

	return nil
}

// fetchDelegations - fetches all babylon delegations, used for bootstrap
func (sew *StakingEventWatcher) fetchDelegations() {
	defer sew.wg.Done()

	sew.delegationRetrievalInProgress.Store(true)

	if err := sew.checkBabylonDelegations(); err != nil {
		sew.logger.Errorf("error checking babylon delegations: %v", err)
	}

	sew.delegationRetrievalInProgress.Store(false)
}

// syncedWithBabylon - indicates whether the current btc tip is same as babylon btc light client tip
func (sew *StakingEventWatcher) syncedWithBabylon() (bool, error) {
	btcLightClientTipHeight, err := sew.babylonNodeAdapter.BtcClientTipHeight()
	if err != nil {
		sew.logger.Errorf("error fetching babylon tip height: %v", err)

		return false, err
	}

	sew.metrics.BtcLightClientHeight.Set(float64(btcLightClientTipHeight))

	currentBtcNodeHeight := sew.currentBestBlockHeight.Load()
	if currentBtcNodeHeight < btcLightClientTipHeight {
		sew.logger.Debugf("btc light client tip height is %d, connected node best block height is %d. Waiting for node to catch up",
			btcLightClientTipHeight, currentBtcNodeHeight)

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
	// minimal witness size for staking tx input is at least 4:
	// covenant_signatures, staker_signatures, script, control_block
	// If that is not the case, something weird is going on and we should investigate.
	if witnessLen < 4 {
		panic(fmt.Errorf("staking tx input witness has less than 4 elements for unbonding tx %s", tx.TxHash()))
	}

	// staker signatures started from 3rd element from the end
	// TODO: there are more than one stakerSignature if it's multisig btc delegation, so we need to handle this case properly
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

		fundingTxs, err := sew.getFundingTxs(stakeSpendingTx)
		if err != nil {
			return fmt.Errorf("error getting funding txs: %w", err)
		}

		if err = sew.babylonNodeAdapter.ReportUnbonding(ctx, stakingTxHash, stakeSpendingTx, proof, fundingTxs); err != nil {
			if !strings.Contains(err.Error(), "cannot unbond an unbonded BTC delegation") {
				del, err := sew.babylonNodeAdapter.BTCDelegation(stakingTxHash.String())
				if err != nil {
					return fmt.Errorf("error fetching delegation from babylon: %w", err)
				}
				// delegation still not unbonded, some other vigilante didn't manage to do it, err
				if del.Status != btcstakingtypes.BTCDelegationStatus_UNBONDED.String() {
					sew.metrics.FailedReportedUnbondingTransactions.Inc()
				}
			}

			if errors.Is(err, babylonclient.ErrTimeoutAfterWaitingForTxBroadcast) {
				sew.metrics.UnbondingCensorshipGaugeVec.WithLabelValues(stakingTxHash.String()).Inc()
			}

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

func (sew *StakingEventWatcher) handleSpend(ctx context.Context, spendingTx *wire.MsgTx, td *TrackedDelegation) {
	delegationID := td.StakingTx.TxHash()
	spendingTxHash := spendingTx.TxHash()

	// check if the spending tx is a valid unbonding tx
	_, errParseStkSig := tryParseStakerSignatureFromSpentTx(spendingTx, td)
	nonUnbondingTx := errParseStkSig != nil

	// if the spending tx is a stake expansion it should wait to be k-deep
	stakeExpansion, err := sew.babylonNodeAdapter.BTCDelegation(spendingTxHash.String())
	isStakeExpansion := err == nil && stakeExpansion != nil

	switch {
	case isStakeExpansion:
		sew.metrics.DetectedUnbondedStakeExpansionCounter.Inc()
		sew.logger.Debugf("found stake expansion tx %s spending the previous staking tx %s", spendingTxHash, delegationID)

		var (
			txResult *btcjson.TxRawResult
			err      error
		)

		if err := retry.Do(func() error {
			txResult, err = sew.btcClient.GetRawTransactionVerbose(&spendingTxHash)
			if err != nil {
				return err
			}

			return nil
		},
			retry.Context(ctx),
			retry.Attempts(5),
			fixedDelyTypeWithJitter,
			retry.Delay(5*time.Second),
			retry.MaxJitter(5*time.Second),
			retry.LastErrorOnly(true),
		); err != nil {
			sew.logger.Warnf("error on getting stake expansion tx result from BTC client: tx %s for staking tx %s. Error: %w", spendingTxHash, delegationID, err)

			return // check what to do with err with lazar
		}

		blkHashInclusion, err := chainhash.NewHashFromStr(txResult.BlockHash)
		if err != nil {
			sew.logger.Errorf("error parsing block hash from tx result for staking tx %s: %v", delegationID, err)

			return
		}
		// wait stk expansion to be k-deep
		if err := sew.waitForRequiredDepth(ctx, blkHashInclusion, sew.babylonConfirmationTimeBlocks); err != nil {
			sew.logger.Warnf("exceeded waiting for required depth for stake expansion tx: %s, will try later. Err: %v", spendingTxHash.String(), err)

			return
		}

	case nonUnbondingTx:
		sew.metrics.DetectedNonUnbondingTransactionsCounter.Inc()
		// Error means that this is not unbonding tx. At this point, it means that it is
		// either withdrawal transaction or slashing transaction spending staking staking output.
		// As we only care about unbonding transactions, we do not need to take additional actions.
		// We start polling babylon for delegation to stop being active, and then delete it from unbondingTracker.
		sew.logger.Debugf("Spending tx %s for staking tx %s is not unbonding tx. Info: %v", spendingTxHash, delegationID, err)

	default: // spending tx is the expected unbonding tx
		sew.metrics.DetectedUnbondingTransactionsCounter.Inc()
		// We found valid unbonding tx. We need to try to report it to babylon.
		// We stop reporting if delegation is no longer active or we succeed.
	}

	sew.logger.Debugf("before check if stake spending tx %s is in chain for staking tx %s", spendingTxHash, delegationID)
	proof := sew.waitForStakeSpendInclusionProof(ctx, spendingTx)
	if proof == nil {
		sew.logger.Errorf("unbonding tx %s for staking tx %s proof not built", spendingTxHash, delegationID)

		return
	}
	sew.logger.Debugf("found unbonding tx %s for staking tx %s", spendingTxHash, delegationID)
	sew.reportUnbondingToBabylon(ctx, delegationID, spendingTx, proof)
	sew.logger.Debugf("unbonding tx %s for staking tx %s reported to babylon", spendingTxHash, delegationID)

	utils.PushOrQuit[*delegationInactive](
		sew.unbondingRemovalChan,
		&delegationInactive{stakingTxHash: delegationID},
		sew.quit,
	)
}

func (sew *StakingEventWatcher) checkSpend() error {
	for del := range sew.unbondingTracker.DelegationsIter(1000) {
		if del.InProgress {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		response, err := sew.indexer.GetOutspend(ctx, del.StakingTx.TxHash().String(), del.StakingOutputIdx)
		cancel()

		if err != nil {
			sew.logger.Errorf("error getting outspend for staking tx %s: %v", del.StakingTx.TxHash(), err)

			continue
		}

		if !response.Spent {
			continue
		}

		if err := sew.unbondingTracker.UpdateActivation(del.StakingTx.TxHash(), true); err != nil {
			return err
		}

		// nolint:contextcheck
		go func() {
			defer func() {
				if err := sew.unbondingTracker.UpdateActivation(del.StakingTx.TxHash(), false); err != nil {
					sew.logger.Warnf("error updating activation Status for staking tx %s: %v", del.StakingTx.TxHash(), err)
				}
			}()
			innerCtx, innerCancel := sew.quitContext()
			defer innerCancel()

			txHash, err := chainhash.NewHashFromStr(response.TxID)
			if err != nil {
				sew.logger.Errorf("error parsing tx hash %s: %v", response.TxID, err)

				return
			}
			spendingTx, err := sew.btcClient.GetRawTransaction(txHash)
			if err != nil {
				sew.logger.Errorf("error getting spending tx %s: %v", response.TxID, err)

				return
			}

			sew.handleSpend(innerCtx, spendingTx.MsgTx(), del)
		}()
	}

	return nil
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

			_, err := sew.unbondingTracker.AddDelegation(
				activeDel.stakingTx,
				activeDel.stakingOutputIdx,
				activeDel.unbondingOutput,
				activeDel.delegationStartHeight,
				true,
			)

			if err != nil {
				sew.logger.Errorf("error adding delegation to unbondingTracker for staking tx %s: %v", activeDel.stakingTxHash, err)

				continue
			}

			sew.metrics.NumberOfTrackedActiveDelegations.Inc()
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
	for del := range sew.pendingTracker.DelegationsIter(1000) {
		if del.InProgress {
			continue
		}
		txHash := del.StakingTx.TxHash()

		details, status, err := sew.btcClient.TxDetails(&txHash, del.StakingTx.TxOut[del.StakingOutputIdx].PkScript)
		if err != nil {
			sew.logger.Errorf("error getting tx: %v, err: %v", txHash, err)

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
			sew.logger.Warnf("error making spv proof for tx %s: %v", txHash, err)

			continue
		}

		if err := sew.activationLimiter.Acquire(context.Background(), 1); err != nil {
			sew.logger.Warnf("error acquiring activation semaphore for tx %s: %v", txHash, err)

			continue
		}

		if err := sew.pendingTracker.UpdateActivation(txHash, true); err != nil {
			sew.logger.Warnf("error updating activation in pending tracker tx: %v", txHash)
			sew.activationLimiter.Release(1) // in probable edge case, insure we release the sem

			continue
		}

		go func() {
			defer sew.activationLimiter.Release(1)
			sew.activateBtcDelegation(txHash, proof, details.Block.BlockHash(), sew.babylonConfirmationTimeBlocks)
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
			sew.logger.Warnf("err updating activation in pending tracker tx: %v", stakingTxHash)
		}
	}()

	if err := sew.waitForRequiredDepth(ctx, &inclusionBlockHash, requiredDepth); err != nil {
		sew.logger.Warnf("exceeded waiting for required depth for tx: %s, will try later. Err: %v", stakingTxHash.String(), err)

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
		del, err := sew.babylonNodeAdapter.BTCDelegation(stakingTxHash.String())
		if err != nil {
			return fmt.Errorf("error checking if delegation is active: %w", err)
		}

		if del.Status != btcstakingtypes.BTCDelegationStatus_VERIFIED.String() {
			sew.logger.Debugf("skipping tx %s is not in verified Status", stakingTxHash)
			sew.pendingTracker.RemoveDelegation(stakingTxHash)
			sew.verifiedSufficientConfTracker.RemoveDelegation(stakingTxHash)
			sew.metrics.NumberOfVerifiedDelegations.Dec()

			return nil
		}

		if err := sew.babylonNodeAdapter.ActivateDelegation(ctx, stakingTxHash, proof); err != nil {
			if !strings.Contains(err.Error(), "already has inclusion proof") {
				verified, err := sew.babylonNodeAdapter.IsDelegationVerified(stakingTxHash)
				if err != nil {
					return fmt.Errorf("error checking if delegation is active: %w", err)
				}
				// delegation still not activated, increase the err metric
				if verified {
					sew.metrics.FailedReportedActivateDelegations.Inc()
				}
			}

			if errors.Is(err, babylonclient.ErrTimeoutAfterWaitingForTxBroadcast) {
				sew.metrics.InclusionProofCensorshipGaugeVec.WithLabelValues(stakingTxHash.String()).Inc()
			}

			return fmt.Errorf("error reporting activate delegation tx %s to babylon: %w", stakingTxHash, err)
		}

		sew.metrics.ReportedActivateDelegationsCounter.Inc()
		sew.pendingTracker.RemoveDelegation(stakingTxHash)
		sew.metrics.NumberOfVerifiedDelegations.Dec()

		sew.logger.Infof("staking tx activated %s", stakingTxHash.String())

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
	)
}

func (sew *StakingEventWatcher) latency(method string) func() {
	startTime := time.Now()

	return func() {
		duration := time.Since(startTime)
		sew.metrics.MethodExecutionLatency.WithLabelValues(method).Observe(duration.Seconds())
	}
}

func (sew *StakingEventWatcher) getFundingTxs(tx *wire.MsgTx) ([][]byte, error) {
	if len(tx.TxIn) == 0 {
		return nil, fmt.Errorf("no inputs in tx %s", tx.TxHash())
	}

	var fundingTxs [][]byte
	for _, txIn := range tx.TxIn {
		rawTransaction, err := sew.btcClient.GetRawTransaction(&txIn.PreviousOutPoint.Hash)
		if err != nil {
			return nil, fmt.Errorf("error getting rawTransaction %s: %w", txIn.PreviousOutPoint.Hash, err)
		}

		serializedTx, err := bbn.SerializeBTCTx(rawTransaction.MsgTx())
		if err != nil {
			return nil, fmt.Errorf("error serializing rawTransaction %s: %w", txIn.PreviousOutPoint.Hash, err)
		}

		fundingTxs = append(fundingTxs, serializedTx)
	}

	if len(fundingTxs) == 0 {
		return nil, fmt.Errorf("no funding txs found for tx %s", tx.TxHash())
	}

	return fundingTxs, nil
}

func (sew *StakingEventWatcher) fetchCometBftBlockForever() {
	defer sew.wg.Done()

	ticker := time.NewTicker(sew.cfg.FetchCometBlockInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if sew.delegationRetrievalInProgress.Load() {
				sew.logger.Debug("waiting for delegation retrieval to finish")

				continue
			}
			nodeSynced, err := sew.syncedWithBabylon()
			if err != nil || !nodeSynced {
				// Log message and continue if there's an error or node isn't synced
				sew.logger.Debugf("err or node not synced with babylon, err: %v, synced: %v", err, nodeSynced)

				continue
			}
			if err := sew.fetchCometBftBlockOnce(); err != nil {
				sew.logger.Errorf("error fetching comet bft block: %v", err)
			}
		case <-sew.quit:
			sew.logger.Debug("fetch babylon block loop quit")

			return
		}
	}
}

func (sew *StakingEventWatcher) fetchCometBftBlockOnce() error {
	sew.logger.Debug("Querying comet bft for new blocks")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	latestHeight, err := sew.babylonNodeAdapter.CometBFTTipHeight(ctx)
	cancel()
	if err != nil {
		return fmt.Errorf("error querying comet bft for new blocks: %w", err)
	}

	currentHeight := sew.currentCometTipHeight.Load()

	// Enforce monotonic block height processing
	if latestHeight < currentHeight {
		return fmt.Errorf("non-monotonic block height detected: latest height %d is less than current height %d",
			latestHeight, currentHeight)
	}

	if latestHeight == currentHeight {
		sew.logger.Debugf("no new comet bft blocks, current height: %d", currentHeight)

		return nil
	}

	if err := sew.fetchDelegationsByEvents(currentHeight, latestHeight); err != nil {
		return fmt.Errorf("error fetching delegations by events: %w", err)
	}

	sew.currentCometTipHeight.Store(latestHeight + 1)

	return nil
}

func (sew *StakingEventWatcher) fetchDelegationsByEvents(startHeight, endHeight int64) error {
	const (
		covQuorumEvent           = `babylon.btcstaking.v1.EventCovenantQuorumReached`
		inclusionProofReceived   = `babylon.btcstaking.v1.EventBTCDelegationInclusionProofReceived`
		btcDelegationStateUpdate = `babylon.btcstaking.v1.EventBTCDelegationStateUpdate`
	)
	var (
		stakingTxHashes []string
	)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	events := []string{covQuorumEvent, inclusionProofReceived, btcDelegationStateUpdate}

	for i := startHeight; i <= endHeight; i++ {
		hashes, err := sew.fetchDelegationsModifiedByEvents(ctx, i, events)
		if err != nil {
			return fmt.Errorf("error fetching modified delegations by events: %w", err)
		}

		stakingTxHashes = append(stakingTxHashes, hashes...)
	}

	stakingTxHashes = deduplicateStrings(stakingTxHashes)

	for _, stakingTxHash := range stakingTxHashes {
		delegation, err := sew.babylonNodeAdapter.BTCDelegation(stakingTxHash)
		if err != nil {
			return fmt.Errorf("error getting delegation %s: %w", stakingTxHash, err)
		}

		switch delegation.Status {
		case btcstakingtypes.BTCDelegationStatus_VERIFIED.String():
			sew.addToPendingFunc(*delegation)
			sew.addToUnbondingFunc(*delegation)
		case btcstakingtypes.BTCDelegationStatus_ACTIVE.String():
			sew.addToUnbondingFunc(*delegation)
		}
	}

	return nil
}

func (sew *StakingEventWatcher) fetchDelegationsModifiedByEvents(
	ctx context.Context,
	height int64,
	eventTypes []string,
) ([]string, error) {
	sew.latency("fetchDelegationsModifiedByEvents")()
	var stakingTxHashes []string

	err := retry.Do(func() error {
		stkTxs, err := sew.babylonNodeAdapter.DelegationsModifiedInBlock(ctx, height, eventTypes)

		if err != nil {
			return fmt.Errorf("error fetching staking tx hashes by event from babylon: %w", err)
		}

		stakingTxHashes = append(stakingTxHashes, stkTxs...)

		return nil
	},
		retry.Context(ctx),
		retry.Attempts(5),
		fixedDelyTypeWithJitter,
		retry.Delay(5*time.Second),
		retry.MaxJitter(5*time.Second),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return nil, err
	}

	return stakingTxHashes, nil
}

func (sew *StakingEventWatcher) addToUnbondingFunc(delegation Delegation) {
	del := &newDelegation{
		stakingTxHash:         delegation.StakingTx.TxHash(),
		stakingTx:             delegation.StakingTx,
		stakingOutputIdx:      delegation.StakingOutputIdx,
		delegationStartHeight: delegation.DelegationStartHeight,
		unbondingOutput:       delegation.UnbondingOutput,
	}

	// if we already have this delegation, we still want to check if it has changed,
	// we should track both verified and active Status for unbonding
	changed, exists := sew.unbondingTracker.HasDelegationChanged(delegation.StakingTx.TxHash(), del)
	if exists && changed {
		// The Delegation exists and has changed, push the update.
		utils.PushOrQuit(sew.unbondingDelegationChan, del, sew.quit)
	} else if !exists {
		// The Delegation doesn't exist, push the new delegation.
		utils.PushOrQuit(sew.unbondingDelegationChan, del, sew.quit)
	}
}

// addToPendingFunc adds a delegation to the pending tracker if it is not already present
func (sew *StakingEventWatcher) addToPendingFunc(delegation Delegation) {
	// if is stake expansion, we should not activate it,
	// the inclusion proof is reported in a MsgBTCUndelegate of the expanded delegation
	stkTxHash := delegation.StakingTx.TxHash()
	if delegation.IsStakeExpansion {
		sew.logger.Debugf("skipping stake expansion tx adding to activation tracker %s", stkTxHash)

		return
	}

	_, exists := sew.pendingTracker.GetDelegation(stkTxHash)
	if !exists && !delegation.HasProof {
		_, _ = sew.pendingTracker.AddDelegation(
			delegation.StakingTx,
			delegation.StakingOutputIdx,
			delegation.UnbondingOutput,
			delegation.DelegationStartHeight,
			false,
		)
		sew.logger.Debugf("Received new verified delegation to watch: %s", stkTxHash)
		sew.metrics.NumberOfVerifiedDelegations.Inc()
	}
}

func deduplicateStrings(slice []string) []string {
	seen := make(map[string]struct{})
	var deduped []string

	for _, s := range slice {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			deduped = append(deduped, s)
		}
	}

	return deduped
}
