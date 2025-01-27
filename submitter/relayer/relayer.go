package relayer

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/babylonlabs-io/vigilante/submitter/store"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lntypes"
	"math"
	"strconv"
	"time"

	"github.com/babylonlabs-io/babylon/btctxformatter"
	ckpttypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/types"
)

const (
	changePosition                = 1
	dustThreshold  btcutil.Amount = 546
)

type GetLatestCheckpointFunc func() (*store.StoredCheckpoint, bool, error)
type GetRawTransactionFunc func(txHash *chainhash.Hash) (*btcutil.Tx, error)
type SendTransactionFunc func(tx *wire.MsgTx) (*chainhash.Hash, error)

type Relayer struct {
	chainfee.Estimator
	btcclient.BTCWallet
	store                   *store.SubmitterStore
	lastSubmittedCheckpoint *types.CheckpointInfo
	tag                     btctxformatter.BabylonTag
	version                 btctxformatter.FormatVersion
	submitterAddress        sdk.AccAddress
	metrics                 *metrics.RelayerMetrics
	config                  *config.SubmitterConfig
	logger                  *zap.SugaredLogger
	walletName              string
}

func New(
	wallet btcclient.BTCWallet,
	walletName string,
	tag btctxformatter.BabylonTag,
	version btctxformatter.FormatVersion,
	submitterAddress sdk.AccAddress,
	metrics *metrics.RelayerMetrics,
	est chainfee.Estimator,
	config *config.SubmitterConfig,
	parentLogger *zap.Logger,
	db kvdb.Backend,
) *Relayer {
	subStore, err := store.NewSubmitterStore(db)
	if err != nil {
		panic(fmt.Errorf("error setting up store: %w", err))
	}

	metrics.ResendIntervalSecondsGauge.Set(float64(config.ResendIntervalSeconds))

	return &Relayer{
		Estimator:               est,
		BTCWallet:               wallet,
		walletName:              walletName,
		store:                   subStore,
		tag:                     tag,
		version:                 version,
		submitterAddress:        submitterAddress,
		metrics:                 metrics,
		config:                  config,
		lastSubmittedCheckpoint: &types.CheckpointInfo{},
		logger:                  parentLogger.With(zap.String("module", "relayer")).Sugar(),
	}
}

// SendCheckpointToBTC converts the checkpoint into two transactions and send them to BTC
// if the checkpoint has been sent but the status is still Sealed, we will bump the fee
// of the second tx of the checkpoint and resend the tx
// Note: we only consider bumping the second tx of a submitted checkpoint because
// it is as effective as bumping the two but simpler
func (rl *Relayer) SendCheckpointToBTC(ckpt *ckpttypes.RawCheckpointWithMetaResponse) error {
	ckptEpoch := ckpt.Ckpt.EpochNum
	if ckpt.Status != ckpttypes.Sealed {
		rl.logger.Errorf("The checkpoint for epoch %v is not sealed", ckptEpoch)
		rl.metrics.InvalidCheckpointCounter.Inc()
		// we do not consider this case as a failed submission but a software bug
		return nil
	}

	storeCkptFunc := func(tx1, tx2 *wire.MsgTx, epochNum uint64) error {
		storedCkpt := store.NewStoredCheckpoint(tx1, tx2, epochNum)

		return rl.store.PutCheckpoint(storedCkpt)
	}

	if rl.shouldSendCompleteCkpt(ckptEpoch) || rl.shouldSendTx2(ckptEpoch) {
		hasBeenProcessed, err := maybeResendFromStore(
			ckptEpoch,
			rl.store.LatestCheckpoint,
			rl.GetRawTransaction,
			rl.sendTxToBTC,
		)
		if err != nil {
			return err
		}

		if hasBeenProcessed {
			return nil
		}
	}

	if rl.shouldSendCompleteCkpt(ckptEpoch) {
		rl.logger.Infof("Submitting a raw checkpoint for epoch %v", ckptEpoch)

		submittedCkpt, err := rl.convertCkptToTwoTxAndSubmit(ckpt.Ckpt)
		if err != nil {
			return err
		}

		rl.lastSubmittedCheckpoint = submittedCkpt

		err = storeCkptFunc(submittedCkpt.Tx1.Tx, submittedCkpt.Tx2.Tx, submittedCkpt.Epoch)
		if err != nil {
			return err
		}

		return nil
	} else if rl.shouldSendTx2(ckptEpoch) {
		rl.logger.Infof("Retrying to send tx2 for epoch %v, tx1 %s", ckptEpoch, rl.lastSubmittedCheckpoint.Tx1.TxID)
		submittedCkpt, err := rl.retrySendTx2(ckpt.Ckpt)
		if err != nil {
			return err
		}

		rl.lastSubmittedCheckpoint = submittedCkpt

		err = storeCkptFunc(submittedCkpt.Tx1.Tx, submittedCkpt.Tx2.Tx, submittedCkpt.Epoch)
		if err != nil {
			return err
		}

		return nil
	}

	return nil
}

// MaybeResubmitSecondCheckpointTx based on the resend interval attempts to resubmit 2nd ckpt tx with a bumped fee
func (rl *Relayer) MaybeResubmitSecondCheckpointTx(ckpt *ckpttypes.RawCheckpointWithMetaResponse) error {
	ckptEpoch := ckpt.Ckpt.EpochNum
	if ckpt.Status != ckpttypes.Sealed {
		rl.logger.Errorf("The checkpoint for epoch %v is not sealed", ckptEpoch)
		rl.metrics.InvalidCheckpointCounter.Inc()

		return nil
	}

	lastSubmittedEpoch := rl.lastSubmittedCheckpoint.Epoch
	if ckptEpoch < lastSubmittedEpoch {
		rl.logger.Errorf("The checkpoint for epoch %v is lower than the last submission for epoch %v",
			ckptEpoch, lastSubmittedEpoch)
		rl.metrics.InvalidCheckpointCounter.Inc()
		// we do not consider this case as a failed submission but a software bug

		return nil
	}

	durSeconds := uint(time.Since(rl.lastSubmittedCheckpoint.TS).Seconds())
	if durSeconds < rl.config.ResendIntervalSeconds {
		return nil
	}

	rl.logger.Debugf("The checkpoint for epoch %v was sent more than %v seconds ago but not included on BTC",
		ckptEpoch, rl.config.ResendIntervalSeconds)

	bumpedFee := rl.calculateBumpedFee(rl.lastSubmittedCheckpoint)

	// make sure the bumped fee is effective
	if !rl.shouldResendCheckpoint(rl.lastSubmittedCheckpoint, bumpedFee) {
		return nil
	}

	rl.logger.Debugf("Maybe resending the second tx of the checkpoint %v, old fee of the second tx: %v Satoshis, txid: %s",
		ckptEpoch, rl.lastSubmittedCheckpoint.Tx2.Fee, rl.lastSubmittedCheckpoint.Tx2.TxID.String())

	resubmittedTx2, err := rl.maybeResendSecondTxOfCheckpointToBTC(rl.lastSubmittedCheckpoint.Tx2, bumpedFee)
	if err != nil {
		rl.metrics.FailedResentCheckpointsCounter.Inc()

		return fmt.Errorf("failed to re-send the second tx of the checkpoint %v: %w", rl.lastSubmittedCheckpoint.Epoch, err)
	}

	if resubmittedTx2 == nil {
		return nil
	}

	// record the metrics of the resent tx2
	rl.metrics.NewSubmittedCheckpointSegmentGaugeVec.WithLabelValues(
		strconv.FormatUint(ckptEpoch, 10),
		"1",
		resubmittedTx2.TxID.String(),
		strconv.Itoa(int(resubmittedTx2.Fee)),
	).SetToCurrentTime()
	rl.metrics.ResentCheckpointsCounter.Inc()

	rl.logger.Infof("Successfully re-sent the second tx of the checkpoint %v, txid: %s, bumped fee: %v Satoshis",
		rl.lastSubmittedCheckpoint.Epoch, resubmittedTx2.TxID.String(), resubmittedTx2.Fee)

	// update the second tx of the last submitted checkpoint as it is replaced
	rl.lastSubmittedCheckpoint.Tx2 = resubmittedTx2
	rl.lastSubmittedCheckpoint.TS = time.Now()

	storedCkpt := store.NewStoredCheckpoint(
		rl.lastSubmittedCheckpoint.Tx1.Tx,
		rl.lastSubmittedCheckpoint.Tx2.Tx,
		rl.lastSubmittedCheckpoint.Epoch,
	)

	return rl.store.PutCheckpoint(storedCkpt)
}

func (rl *Relayer) shouldSendCompleteCkpt(ckptEpoch uint64) bool {
	return rl.lastSubmittedCheckpoint.Tx1 == nil || rl.lastSubmittedCheckpoint.Epoch < ckptEpoch
}

// shouldSendTx2 - we want to avoid resending tx1 if only tx2 submission has failed
func (rl *Relayer) shouldSendTx2(ckptEpoch uint64) bool {
	return (rl.lastSubmittedCheckpoint.Tx1 != nil || rl.lastSubmittedCheckpoint.Epoch < ckptEpoch) &&
		rl.lastSubmittedCheckpoint.Tx2 == nil
}

// shouldResendCheckpoint checks whether the bumpedFee is effective for replacement
func (rl *Relayer) shouldResendCheckpoint(ckptInfo *types.CheckpointInfo, bumpedFee btcutil.Amount) bool {
	// if the bumped fee is less than the fee of the previous second tx plus the minimum required bumping fee
	// then the bumping would not be effective
	requiredBumpingFee := ckptInfo.Tx2.Fee + rl.calcMinRelayFee(ckptInfo.Tx2.Size)

	rl.logger.Debugf("the bumped fee: %v Satoshis, the required fee: %v Satoshis",
		bumpedFee, requiredBumpingFee)

	return bumpedFee >= requiredBumpingFee
}

// calculateBumpedFee calculates the bumped fees of the second tx of the checkpoint
// based on the current BTC load, considering both tx sizes
// the result is multiplied by ResubmitFeeMultiplier set in config
func (rl *Relayer) calculateBumpedFee(ckptInfo *types.CheckpointInfo) btcutil.Amount {
	return ckptInfo.Tx2.Fee.MulF64(rl.config.ResubmitFeeMultiplier)
}

// maybeResendSecondTxOfCheckpointToBTC resends the second tx of the checkpoint with bumpedFee
func (rl *Relayer) maybeResendSecondTxOfCheckpointToBTC(tx2 *types.BtcTxInfo, bumpedFee btcutil.Amount) (*types.BtcTxInfo, error) {
	_, status, err := rl.TxDetails(tx2.TxID, tx2.Tx.TxOut[changePosition].PkScript)
	if err != nil {
		return nil, err
	}

	// No need to resend, transaction already confirmed
	if status == btcclient.TxInChain {
		rl.logger.Debugf("Transaction %v is already confirmed", tx2.TxID)

		return nil, nil
	}

	// set output value of the second tx to be the balance minus the bumped fee
	// if the bumped fee is higher than the balance, then set the bumped fee to
	// be equal to the balance to ensure the output value is not negative
	balance := btcutil.Amount(tx2.Tx.TxOut[changePosition].Value)

	// todo: revise this as this means we will end up with output with value 0 that will be rejected by bitcoind as dust output.
	if bumpedFee > balance {
		rl.logger.Debugf("the bumped fee %v Satoshis for the second tx is more than UTXO amount %v Satoshis",
			bumpedFee, balance)
		bumpedFee = balance
	}

	tx2.Tx.TxOut[changePosition].Value = int64(balance - bumpedFee)

	// resign the tx as the output is changed
	tx, err := rl.signTx(tx2.Tx)
	if err != nil {
		return nil, err
	}

	txID, err := rl.sendTxToBTC(tx)
	if err != nil {
		return nil, err
	}

	// update tx info
	tx2.Fee = bumpedFee
	tx2.TxID = txID

	return tx2, nil
}

// calcMinRelayFee returns the minimum transaction fee required for a
// transaction with the passed serialized size to be accepted into the memory
// pool and relayed.
// Adapted from https://github.com/btcsuite/btcd/blob/f9cbff0d819c951d20b85714cf34d7f7cc0a44b7/mempool/policy.go#L61
func (rl *Relayer) calcMinRelayFee(txVirtualSize int64) btcutil.Amount {
	// Calculate the minimum fee for a transaction to be allowed into the
	// mempool and relayed by scaling the base fee (which is the minimum
	// free transaction relay fee).
	minRelayFeeRate := rl.RelayFeePerKW().FeePerKVByte()

	rl.logger.Debugf("current minimum relay fee rate is %v", minRelayFeeRate)

	// #nosec G115 - Ignored G115 because the application ensures txVirtualSize is always non-negative.
	minRelayFee := minRelayFeeRate.FeeForVSize(lntypes.VByte(txVirtualSize))

	// Set the minimum fee to the maximum possible value if the calculated
	// fee is not in the valid range for monetary amounts.
	if minRelayFee > btcutil.MaxSatoshi {
		minRelayFee = btcutil.MaxSatoshi
	}

	return minRelayFee
}

func (rl *Relayer) signTx(tx *wire.MsgTx) (*wire.MsgTx, error) {
	// unlock the wallet
	if err := rl.WalletPassphrase(rl.GetWalletPass(), rl.GetWalletLockTime()); err != nil {
		return nil, err
	}

	signedTx, allSigned, err := rl.BTCWallet.SignRawTransactionWithWallet(tx)
	if err != nil {
		return nil, err
	}

	if !allSigned {
		return nil, errors.New("transaction is only partially signed")
	}

	return signedTx, nil
}

func (rl *Relayer) encodeCheckpointData(ckpt *ckpttypes.RawCheckpointResponse) ([]byte, []byte, error) {
	// Convert to raw checkpoint
	rawCkpt, err := ckpt.ToRawCheckpoint()
	if err != nil {
		return nil, nil, err
	}

	// Convert raw checkpoint to BTC checkpoint
	btcCkpt, err := ckpttypes.FromRawCkptToBTCCkpt(rawCkpt, rl.submitterAddress)
	if err != nil {
		return nil, nil, err
	}

	// Encode checkpoint data
	data1, data2, err := btctxformatter.EncodeCheckpointData(
		rl.tag,
		rl.version,
		btcCkpt,
	)
	if err != nil {
		return nil, nil, err
	}

	// Return the encoded data
	return data1, data2, nil
}

func (rl *Relayer) logAndRecordCheckpointMetrics(tx1, tx2 *types.BtcTxInfo, epochNum uint64) {
	// Log the transactions sent for checkpointing
	rl.logger.Infof("Sent two txs to BTC for checkpointing epoch %v, first txid: %s, second txid: %s",
		epochNum, tx1.Tx.TxHash().String(), tx2.Tx.TxHash().String())

	// Record metrics for the first transaction
	rl.metrics.NewSubmittedCheckpointSegmentGaugeVec.WithLabelValues(
		strconv.FormatUint(epochNum, 10),
		"0",
		tx1.Tx.TxHash().String(),
		strconv.Itoa(int(tx1.Fee)),
	).SetToCurrentTime()

	// Record metrics for the second transaction
	rl.metrics.NewSubmittedCheckpointSegmentGaugeVec.WithLabelValues(
		strconv.FormatUint(epochNum, 10),
		"1",
		tx2.Tx.TxHash().String(),
		strconv.Itoa(int(tx2.Fee)),
	).SetToCurrentTime()
}

func (rl *Relayer) convertCkptToTwoTxAndSubmit(ckpt *ckpttypes.RawCheckpointResponse) (*types.CheckpointInfo, error) {
	data1, data2, err := rl.encodeCheckpointData(ckpt)
	if err != nil {
		return nil, err
	}

	tx1, tx2, err := rl.ChainTwoTxAndSend(data1, data2)
	if err != nil {
		return nil, err
	}

	rl.logAndRecordCheckpointMetrics(tx1, tx2, ckpt.EpochNum)

	return &types.CheckpointInfo{
		Epoch: ckpt.EpochNum,
		TS:    time.Now(),
		Tx1:   tx1,
		Tx2:   tx2,
	}, nil
}

// retrySendTx2 - rebuilds the tx2 and sends it, expects that tx1 has been sent and
// lastSubmittedCheckpoint.Tx1 is not nil
func (rl *Relayer) retrySendTx2(ckpt *ckpttypes.RawCheckpointResponse) (*types.CheckpointInfo, error) {
	_, data2, err := rl.encodeCheckpointData(ckpt)
	if err != nil {
		return nil, err
	}

	tx1 := rl.lastSubmittedCheckpoint.Tx1
	if tx1 == nil {
		return nil, fmt.Errorf("tx1 is nil") // shouldn't happen, sanity check
	}

	tx2, err := rl.buildAndSendTx(func() (*types.BtcTxInfo, error) {
		return rl.buildChainedDataTx(data2, tx1.Tx)
	})
	if err != nil {
		return nil, err
	}

	rl.logAndRecordCheckpointMetrics(tx1, tx2, ckpt.EpochNum)

	return &types.CheckpointInfo{
		Epoch: ckpt.EpochNum,
		TS:    time.Now(),
		Tx1:   tx1,
		Tx2:   tx2,
	}, nil
}

// buildAndSendTx helper function to build and send a transaction
func (rl *Relayer) buildAndSendTx(builderFunc func() (*types.BtcTxInfo, error)) (*types.BtcTxInfo, error) {
	tx, err := builderFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to add data to tx: %w", err)
	}

	tx.TxID, err = rl.sendTxToBTC(tx.Tx)
	if err != nil {
		return nil, fmt.Errorf("failed to send tx to BTC: %w", err)
	}

	return tx, nil
}

// ChainTwoTxAndSend builds two chaining txs with the given data:
// the second tx consumes the output of the first tx
func (rl *Relayer) ChainTwoTxAndSend(data1 []byte, data2 []byte) (*types.BtcTxInfo, *types.BtcTxInfo, error) {
	// recipient is a change address that all the
	// remaining balance of the utxo is sent to

	tx1, err := rl.buildAndSendTx(func() (*types.BtcTxInfo, error) {
		return rl.buildDataTx(data1)
	})
	if err != nil {
		return nil, nil, err
	}

	// cache the success of tx1, we need it if we fail with tx2 send
	rl.lastSubmittedCheckpoint.Tx1 = tx1

	// Build and send tx2, using tx1 as the parent
	tx2, err := rl.buildAndSendTx(func() (*types.BtcTxInfo, error) {
		return rl.buildChainedDataTx(data2, tx1.Tx)
	})
	if err != nil {
		return nil, nil, err
	}

	return tx1, tx2, nil
}

// buildDataTx constructs a new Bitcoin transaction with custom data inserted as an OP_RETURN output.
//
// Parameters:
//   - data: The custom data to be inserted into the transaction as an OP_RETURN output.
func (rl *Relayer) buildDataTx(data []byte) (*types.BtcTxInfo, error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	// Build txOut for data
	builder := txscript.NewScriptBuilder()
	dataScript, err := builder.AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		return nil, fmt.Errorf("failed to build data script: %w", err)
	}
	tx.AddTxOut(wire.NewTxOut(0, dataScript))

	// Fund the transaction
	changePosition := 1
	feeRate := btcutil.Amount(rl.getFeeRate()).ToBTC()
	rawTxResult, err := rl.BTCWallet.FundRawTransaction(tx, btcjson.FundRawTransactionOpts{
		FeeRate:        &feeRate,
		ChangePosition: &changePosition,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fund raw tx in buildDataTx: %w", err)
	}

	// Ensure there's a change output with at least dust amount
	hasChange := len(rawTxResult.Transaction.TxOut) > changePosition
	if !hasChange {
		rl.logger.Debugf("no change, adding change address manually, tx ref: %s", tx.TxHash())
		changeAddr, err := rl.BTCWallet.GetNewAddress("")
		if err != nil {
			return nil, fmt.Errorf("err getting raw change address %w", err)
		}

		changePkScript, err := txscript.PayToAddrScript(changeAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create script for change address: %s err %w", changeAddr, err)
		}

		changeOutput := wire.NewTxOut(int64(dustThreshold), changePkScript)
		rawTxResult.Transaction.AddTxOut(changeOutput)
	}

	rl.logger.Debugf("Building a BTC tx using %s with data %x", tx.TxHash(), data)

	return rl.finalizeTransaction(rawTxResult.Transaction)
}

// buildChainedDataTx constructs a Bitcoin transaction that spends from a previous transaction.
// It uses the change output from the previous transaction as its input.
//
// Parameters:
//   - data: The custom data to be inserted into the transaction as an OP_RETURN output.
//   - prevTx: The previous transaction to use as input.
func (rl *Relayer) buildChainedDataTx(data []byte, prevTx *wire.MsgTx) (*types.BtcTxInfo, error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add input from previous transaction
	txID := prevTx.TxHash()
	outPoint := wire.NewOutPoint(&txID, changePosition)
	txIn := wire.NewTxIn(outPoint, nil, nil)
	// Enable replace-by-fee, see https://river.com/learn/terms/r/replace-by-fee-rbf
	txIn.Sequence = math.MaxUint32 - 2
	tx.AddTxIn(txIn)

	// Build txOut for data
	builder := txscript.NewScriptBuilder()
	dataScript, err := builder.AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		return nil, fmt.Errorf("failed to build data script: %w", err)
	}
	tx.AddTxOut(wire.NewTxOut(0, dataScript))

	// Fund the transaction
	changePosition := 1
	feeRate := btcutil.Amount(rl.getFeeRate()).ToBTC()
	rawTxResult, err := rl.BTCWallet.FundRawTransaction(tx, btcjson.FundRawTransactionOpts{
		FeeRate:        &feeRate,
		ChangePosition: &changePosition,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fund raw tx in buildChainedDataTx: %w", err)
	}

	rl.logger.Debugf("Building a BTC tx using %s with data %x", tx.TxHash(), data)

	return rl.finalizeTransaction(rawTxResult.Transaction)
}

// finalizeTransaction handles the common logic for validating and finalizing a transaction,
// including fee calculation, change verification, and signing.
func (rl *Relayer) finalizeTransaction(tx *wire.MsgTx) (*types.BtcTxInfo, error) {
	hasChange := len(tx.TxOut) > changePosition
	var changeAmount btcutil.Amount

	if hasChange {
		changeAmount = btcutil.Amount(tx.TxOut[changePosition].Value)
		_, addresses, _, err := txscript.ExtractPkScriptAddrs(
			tx.TxOut[changePosition].PkScript,
			rl.GetNetParams(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to ExtractPkScriptAddrs: %w", err)
		}
		if len(addresses) == 0 {
			return nil, errors.New("no change address found")
		}
		rl.logger.Debugf("Got a change address %v", addresses[0].String())
	}

	txSize, err := calculateTxVirtualSize(tx)
	if err != nil {
		return nil, err
	}

	txFee := rl.calcMinRelayFee(txSize)
	if hasChange && changeAmount < txFee {
		return nil, fmt.Errorf("the value of the utxo is not sufficient for relaying the tx. Require: %v. Have: %v", txFee, changeAmount)
	}

	// Sign tx
	signedTx, err := rl.signTx(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx: %w", err)
	}

	// Serialization
	var signedTxBytes bytes.Buffer
	if err := signedTx.Serialize(&signedTxBytes); err != nil {
		return nil, fmt.Errorf("failed to serialize signedTx: %w", err)
	}

	change := changeAmount - txFee
	if hasChange && change < dustThreshold {
		return nil, fmt.Errorf("change amount is %v less than dust threshold %v", change, dustThreshold)
	}

	rl.logger.Debugf("Successfully composed a BTC tx. Tx fee: %v, output value: %v, tx size: %v, hex: %v",
		txFee, changeAmount, txSize, hex.EncodeToString(signedTxBytes.Bytes()))

	return &types.BtcTxInfo{
		Tx:   signedTx,
		Size: txSize,
		Fee:  txFee,
	}, nil
}

// getFeeRate returns the estimated fee rate, ensuring it within [tx-fee-max, tx-fee-min]
func (rl *Relayer) getFeeRate() chainfee.SatPerKVByte {
	targetBlockNum := rl.GetBTCConfig().TargetBlockNum

	// check we are within the uint32 range
	if targetBlockNum < 0 || targetBlockNum > int64(^uint32(0)) {
		panic(fmt.Errorf("targetBlockNum (%d) is out of uint32 range", targetBlockNum)) // software bug, panic
	}
	fee, err := rl.EstimateFeePerKW(uint32(targetBlockNum))
	if err != nil {
		defaultFee := rl.GetBTCConfig().DefaultFee
		rl.logger.Errorf("failed to estimate transaction fee. Using default fee %v: %s", defaultFee, err.Error())

		return defaultFee
	}

	feePerKVByte := fee.FeePerKVByte()

	rl.logger.Debugf("current tx fee rate is %v", feePerKVByte)

	cfg := rl.GetBTCConfig()
	if feePerKVByte > cfg.TxFeeMax {
		rl.logger.Debugf("current tx fee rate is higher than the maximum tx fee rate %v, using the max", cfg.TxFeeMax)
		feePerKVByte = cfg.TxFeeMax
	}
	if feePerKVByte < cfg.TxFeeMin {
		rl.logger.Debugf("current tx fee rate is lower than the minimum tx fee rate %v, using the min", cfg.TxFeeMin)
		feePerKVByte = cfg.TxFeeMin
	}

	return feePerKVByte
}

func (rl *Relayer) sendTxToBTC(tx *wire.MsgTx) (*chainhash.Hash, error) {
	rl.logger.Debugf("Sending tx %v to BTC", tx.TxHash().String())

	ha, err := rl.SendRawTransaction(tx, true)
	if err != nil {
		return nil, err
	}
	rl.logger.Debugf("Successfully sent tx %v to BTC", tx.TxHash().String())

	return ha, nil
}

// maybeResendFromStore - checks if we need to resubmit txns from a store
// in case "submitter" service was restarted, we want to ensure that we don't send txns again for a checkpoint
// that has already been processed.
// Returns true if the first transactions are in the mempool (no resubmission needed),
// and false if any transaction was re-sent from the store.
func maybeResendFromStore(
	epoch uint64,
	getLatestStoreCheckpoint GetLatestCheckpointFunc,
	getRawTransaction GetRawTransactionFunc,
	sendTransaction SendTransactionFunc,
) (bool, error) {
	storedCkpt, exists, err := getLatestStoreCheckpoint()
	if err != nil {
		return false, err
	} else if !exists {
		return false, nil
	}

	if storedCkpt.Epoch != epoch {
		return false, nil
	}

	maybeResendFunc := func(tx *wire.MsgTx) error {
		txID := tx.TxHash()
		_, err = getRawTransaction(&txID) // todo(lazar): check for specific not found err
		if err != nil {
			_, err := sendTransaction(tx)
			if err != nil {
				return err
			}

			// we know about this tx, but we needed to resend it from already constructed tx from db
			return nil
		}

		// tx exists in mempool and is known to us
		return nil
	}

	if err := maybeResendFunc(storedCkpt.Tx1); err != nil {
		return false, err
	}

	if err := maybeResendFunc(storedCkpt.Tx2); err != nil {
		return false, err
	}

	return true, nil
}
