package relayer

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/btcjson"
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

type Relayer struct {
	chainfee.Estimator
	btcclient.BTCWallet
	lastSubmittedCheckpoint *types.CheckpointInfo
	tag                     btctxformatter.BabylonTag
	version                 btctxformatter.FormatVersion
	submitterAddress        sdk.AccAddress
	metrics                 *metrics.RelayerMetrics
	config                  *config.SubmitterConfig
	logger                  *zap.SugaredLogger
}

func New(
	wallet btcclient.BTCWallet,
	tag btctxformatter.BabylonTag,
	version btctxformatter.FormatVersion,
	submitterAddress sdk.AccAddress,
	metrics *metrics.RelayerMetrics,
	est chainfee.Estimator,
	config *config.SubmitterConfig,
	parentLogger *zap.Logger,
) *Relayer {
	metrics.ResendIntervalSecondsGauge.Set(float64(config.ResendIntervalSeconds))
	return &Relayer{
		Estimator:        est,
		BTCWallet:        wallet,
		tag:              tag,
		version:          version,
		submitterAddress: submitterAddress,
		metrics:          metrics,
		config:           config,
		logger:           parentLogger.With(zap.String("module", "relayer")).Sugar(),
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

	if rl.lastSubmittedCheckpoint == nil ||
		rl.lastSubmittedCheckpoint.Tx1 == nil ||
		rl.lastSubmittedCheckpoint.Epoch < ckptEpoch {
		rl.logger.Infof("Submitting a raw checkpoint for epoch %v for the first time", ckptEpoch)

		if rl.lastSubmittedCheckpoint == nil {
			rl.lastSubmittedCheckpoint = &types.CheckpointInfo{}
		}

		submittedCheckpoint, err := rl.convertCkptToTwoTxAndSubmit(ckpt.Ckpt)
		if err != nil {
			return err
		}

		rl.lastSubmittedCheckpoint = submittedCheckpoint

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

	// now that the checkpoint has been sent, we should try to resend it
	// if the resend interval has passed
	durSeconds := uint(time.Since(rl.lastSubmittedCheckpoint.Ts).Seconds())
	if durSeconds >= rl.config.ResendIntervalSeconds {
		rl.logger.Debugf("The checkpoint for epoch %v was sent more than %v seconds ago but not included on BTC",
			ckptEpoch, rl.config.ResendIntervalSeconds)

		bumpedFee := rl.calculateBumpedFee(rl.lastSubmittedCheckpoint)

		// make sure the bumped fee is effective
		if !rl.shouldResendCheckpoint(rl.lastSubmittedCheckpoint, bumpedFee) {
			return nil
		}

		rl.logger.Debugf("Resending the second tx of the checkpoint %v, old fee of the second tx: %v Satoshis, txid: %s",
			ckptEpoch, rl.lastSubmittedCheckpoint.Tx2.Fee, rl.lastSubmittedCheckpoint.Tx2.TxId.String())

		resubmittedTx2, err := rl.resendSecondTxOfCheckpointToBTC(rl.lastSubmittedCheckpoint.Tx2, bumpedFee)
		if err != nil {
			rl.metrics.FailedResentCheckpointsCounter.Inc()
			return fmt.Errorf("failed to re-send the second tx of the checkpoint %v: %w", rl.lastSubmittedCheckpoint.Epoch, err)
		}

		// record the metrics of the resent tx2
		rl.metrics.NewSubmittedCheckpointSegmentGaugeVec.WithLabelValues(
			strconv.Itoa(int(ckptEpoch)),
			"1",
			resubmittedTx2.TxId.String(),
			strconv.Itoa(int(resubmittedTx2.Fee)),
		).SetToCurrentTime()
		rl.metrics.ResentCheckpointsCounter.Inc()

		rl.logger.Infof("Successfully re-sent the second tx of the checkpoint %v, txid: %s, bumped fee: %v Satoshis",
			rl.lastSubmittedCheckpoint.Epoch, resubmittedTx2.TxId.String(), resubmittedTx2.Fee)

		// update the second tx of the last submitted checkpoint as it is replaced
		rl.lastSubmittedCheckpoint.Tx2 = resubmittedTx2
	}

	return nil
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

// resendSecondTxOfCheckpointToBTC resends the second tx of the checkpoint with bumpedFee
func (rl *Relayer) resendSecondTxOfCheckpointToBTC(tx2 *types.BtcTxInfo, bumpedFee btcutil.Amount) (*types.BtcTxInfo, error) {
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
	tx2.TxId = txID

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

	minRelayFee := minRelayFeeRate.FeeForVSize(txVirtualSize)

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

func (rl *Relayer) convertCkptToTwoTxAndSubmit(ckpt *ckpttypes.RawCheckpointResponse) (*types.CheckpointInfo, error) {
	rawCkpt, err := ckpt.ToRawCheckpoint()
	if err != nil {
		return nil, err
	}
	btcCkpt, err := ckpttypes.FromRawCkptToBTCCkpt(rawCkpt, rl.submitterAddress)
	if err != nil {
		return nil, err
	}
	data1, data2, err := btctxformatter.EncodeCheckpointData(
		rl.tag,
		rl.version,
		btcCkpt,
	)
	if err != nil {
		return nil, err
	}

	tx1, tx2, err := rl.ChainTwoTxAndSend(data1, data2)
	if err != nil {
		return nil, err
	}

	// this is to wait for btcwallet to update utxo database so that
	// the tx that tx1 consumes will not appear in the next unspent txs lit
	time.Sleep(1 * time.Second)

	rl.logger.Infof("Sent two txs to BTC for checkpointing epoch %v, first txid: %s, second txid: %s",
		ckpt.EpochNum, tx1.Tx.TxHash().String(), tx2.Tx.TxHash().String())

	// record metrics of the two transactions
	rl.metrics.NewSubmittedCheckpointSegmentGaugeVec.WithLabelValues(
		strconv.Itoa(int(ckpt.EpochNum)),
		"0",
		tx1.Tx.TxHash().String(),
		strconv.Itoa(int(tx1.Fee)),
	).SetToCurrentTime()
	rl.metrics.NewSubmittedCheckpointSegmentGaugeVec.WithLabelValues(
		strconv.Itoa(int(ckpt.EpochNum)),
		"1",
		tx2.Tx.TxHash().String(),
		strconv.Itoa(int(tx2.Fee)),
	).SetToCurrentTime()

	return &types.CheckpointInfo{
		Epoch: ckpt.EpochNum,
		Ts:    time.Now(),
		Tx1:   tx1,
		Tx2:   tx2,
	}, nil
}

// ChainTwoTxAndSend builds two chaining txs with the given data:
// the second tx consumes the output of the first tx
func (rl *Relayer) ChainTwoTxAndSend(data1 []byte, data2 []byte) (*types.BtcTxInfo, *types.BtcTxInfo, error) {
	// recipient is a change address that all the
	// remaining balance of the utxo is sent to
	tx1 := rl.lastSubmittedCheckpoint.Tx1
	// prevent resending tx1 if it was successful
	if rl.lastSubmittedCheckpoint.Tx1 == nil {
		var err error
		tx1, err = rl.buildTxWithData(data1, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to add data to tx1: %w", err)
		}

		tx1.TxId, err = rl.sendTxToBTC(tx1.Tx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to send tx1 to BTC: %w", err)
		}
		// cache a successful tx1 submission
		rl.lastSubmittedCheckpoint.Tx1 = tx1
	}

	// the second tx consumes the second output (index 1)
	// of the first tx, as the output at index 0 is OP_RETURN
	tx2, err := rl.buildTxWithData(data2, tx1.Tx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add data to tx2: %w", err)
	}

	tx2.TxId, err = rl.sendTxToBTC(tx2.Tx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send tx2 to BTC: %w", err)
	}

	return tx1, tx2, nil
}

// buildTxWithData constructs a Bitcoin transaction with custom data inserted as an OP_RETURN output.
// If `firstTx` is provided, it uses its transaction ID and a predefined output index (`changePosition`)
// to create an input for the new transaction. The OP_RETURN output is added as the first output (index 0).
//
// This function also ensures that the transaction fee is sufficient and signs the transaction before returning it.
// If the UTXO value is insufficient to cover the fee or if the change amount falls below the dust threshold,
// an error is returned.
//
// Parameters:
//   - data: The custom data to be inserted into the transaction as an OP_RETURN output.
//   - firstTx: An optional transaction used to create an input for the new transaction.
func (rl *Relayer) buildTxWithData(data []byte, firstTx *wire.MsgTx) (*types.BtcTxInfo, error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	if firstTx != nil {
		txID := firstTx.TxHash()
		outPoint := wire.NewOutPoint(&txID, changePosition)
		txIn := wire.NewTxIn(outPoint, nil, nil)
		// Enable replace-by-fee, see https://river.com/learn/terms/r/replace-by-fee-rbf
		txIn.Sequence = math.MaxUint32 - 2
		tx.AddTxIn(txIn)
	}

	// build txOut for data
	builder := txscript.NewScriptBuilder()
	dataScript, err := builder.AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		return nil, err
	}
	tx.AddTxOut(wire.NewTxOut(0, dataScript))

	changePosition := 1 // must declare here as you cannot take address of const needed bellow
	feeRate := btcutil.Amount(rl.getFeeRate()).ToBTC()
	rawTxResult, err := rl.BTCWallet.FundRawTransaction(tx, btcjson.FundRawTransactionOpts{
		FeeRate:        &feeRate,
		ChangePosition: &changePosition,
	}, nil)
	if err != nil {
		return nil, err
	}

	rl.logger.Debugf("Building a BTC tx using %s with data %x", rawTxResult.Transaction.TxID(), data)

	_, addresses, _, err := txscript.ExtractPkScriptAddrs(
		rawTxResult.Transaction.TxOut[changePosition].PkScript,
		rl.GetNetParams(),
	)

	if err != nil {
		return nil, err
	}

	if len(addresses) == 0 {
		return nil, errors.New("no change address found")
	}

	changeAddr := addresses[0]
	rl.logger.Debugf("Got a change address %v", changeAddr.String())

	txSize, err := calculateTxVirtualSize(rawTxResult.Transaction)
	if err != nil {
		return nil, err
	}

	changeAmount := btcutil.Amount(rawTxResult.Transaction.TxOut[changePosition].Value)
	minRelayFee := rl.calcMinRelayFee(txSize)

	if changeAmount < minRelayFee {
		return nil, fmt.Errorf("the value of the utxo is not sufficient for relaying the tx. Require: %v. Have: %v", minRelayFee, changeAmount)
	}

	txFee := rawTxResult.Fee
	// ensuring the tx fee is not lower than the minimum relay fee
	if txFee < minRelayFee {
		txFee = minRelayFee
	}
	// ensuring the tx fee is not higher than the utxo value
	if changeAmount < txFee {
		return nil, fmt.Errorf("the value of the utxo is not sufficient for paying the calculated fee of the tx. Calculated: %v. Have: %v", txFee, changeAmount)
	}

	// sign tx
	tx, err = rl.signTx(rawTxResult.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx: %w", err)
	}

	// serialization
	var signedTxBytes bytes.Buffer
	if err := tx.Serialize(&signedTxBytes); err != nil {
		return nil, err
	}

	change := changeAmount - txFee

	if change < dustThreshold {
		return nil, fmt.Errorf("change amount is %v less then dust treshold %v", change, dustThreshold)
	}

	rl.logger.Debugf("Successfully composed a BTC tx: tx fee: %v, output value: %v, tx size: %v, hex: %v",
		txFee, changeAmount, txSize, hex.EncodeToString(signedTxBytes.Bytes()))

	return &types.BtcTxInfo{
		Tx:            tx,
		ChangeAddress: changeAddr,
		Size:          txSize,
		Fee:           txFee,
	}, nil
}

// getFeeRate returns the estimated fee rate, ensuring it within [tx-fee-max, tx-fee-min]
func (rl *Relayer) getFeeRate() chainfee.SatPerKVByte {
	fee, err := rl.EstimateFeePerKW(uint32(rl.GetBTCConfig().TargetBlockNum))
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
