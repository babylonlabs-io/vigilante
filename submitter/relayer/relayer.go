package relayer

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/babylonchain/babylon/btctxformatter"
	ckpttypes "github.com/babylonchain/babylon/x/checkpointing/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/jinzhu/copier"

	"github.com/babylonchain/vigilante/btcclient"
	"github.com/babylonchain/vigilante/log"
	"github.com/babylonchain/vigilante/types"
)

type Relayer struct {
	btcclient.BTCWallet
	lastSubmittedCheckpoint *types.CheckpointInfo
	tag                     btctxformatter.BabylonTag
	version                 btctxformatter.FormatVersion
	submitterAddress        sdk.AccAddress
	resendIntervalSeconds   uint
}

func New(
	wallet btcclient.BTCWallet,
	tag btctxformatter.BabylonTag,
	version btctxformatter.FormatVersion,
	submitterAddress sdk.AccAddress,
	resendIntervalSeconds uint,
) *Relayer {
	return &Relayer{
		BTCWallet:             wallet,
		tag:                   tag,
		version:               version,
		submitterAddress:      submitterAddress,
		resendIntervalSeconds: resendIntervalSeconds,
	}
}

// SendCheckpointToBTC converts the checkpoint into two transactions and send them to BTC
func (rl *Relayer) SendCheckpointToBTC(ckpt *ckpttypes.RawCheckpointWithMeta) error {
	ckptEpoch := ckpt.Ckpt.EpochNum
	if ckpt.Status != ckpttypes.Sealed {
		log.Logger.Warnf("The checkpoint for epoch %v is not sealed", ckptEpoch)
		// we do not consider this case as a failed submission but a software bug
		return nil
	}

	lastSubmittedEpoch := rl.lastSubmittedCheckpoint.Epoch
	if ckptEpoch < lastSubmittedEpoch {
		log.Logger.Warnf("The checkpoint for epoch %v is lower than the last submission for epoch %v",
			ckptEpoch, lastSubmittedEpoch)
		// we do not consider this case as a failed submission but a software bug
		return nil
	}

	if ckptEpoch > lastSubmittedEpoch {
		log.Logger.Debugf("Submitting a raw checkpoint for epoch %v for the first time", ckptEpoch)

		err := rl.convertCkptToTwoTxAndSubmit(ckpt)
		if err != nil {
			return err
		}

		return nil
	}

	// should resend if the checkpoint epoch matches the last submission epoch and
	// the resend interval has passed
	durSeconds := uint(time.Since(rl.lastSubmittedCheckpoint.Ts).Seconds())
	if durSeconds >= rl.resendIntervalSeconds {
		log.Logger.Debugf("The checkpoint for epoch %v was sent more than %v seconds ago but not included on BTC, resending the checkpoint",
			ckptEpoch, rl.resendIntervalSeconds)

		err := rl.resendCheckpointToBTC(rl.lastSubmittedCheckpoint)
		if err != nil {
			return err
		}
	}

	return nil
}

// resendCheckpointToBTC resends the BTC txs of the checkpoint with re-calculated tx fee
func (rl *Relayer) resendCheckpointToBTC(ckptInfo *types.CheckpointInfo) error {
	// resend tx1 of the checkpoint
	tx1Fee := rl.GetTxFee(ckptInfo.Tx1.Size)
	tx1 := ckptInfo.Tx1
	tx1.Tx.TxOut[1].Value = int64(ckptInfo.Tx1.UtxoAmount - tx1Fee)
	txid1, err := rl.sendTxToBTC(tx1.Tx)
	if err != nil {
		return fmt.Errorf("failed to re-send tx1 of the checkpoint %v: %w", ckptInfo.Epoch, err)
	}
	log.Logger.Debugf("Successfully re-sent tx1 of the checkpoint %v with new tx fee of %v, txid: %s",
		ckptInfo.Epoch, tx1Fee, txid1.String())

	// resend tx2 of the checkpoint
	tx2Fee := rl.GetTxFee(ckptInfo.Tx2.Size)
	tx2 := ckptInfo.Tx2
	tx2.Tx.TxOut[1].Value = int64(ckptInfo.Tx2.UtxoAmount - tx2Fee)
	txid2, err := rl.sendTxToBTC(tx2.Tx)
	if err != nil {
		return fmt.Errorf("failed to re-send tx2 of the checkpoint %v: %w", ckptInfo.Epoch, err)
	}
	log.Logger.Debugf("Successfully re-sent tx2 of the checkpoint %v with new tx fee of %v, txid: %s",
		ckptInfo.Epoch, tx2Fee, txid2.String())

	// update the checkpoint info
	rl.lastSubmittedCheckpoint = &types.CheckpointInfo{
		Epoch: ckptInfo.Epoch,
		Ts:    time.Now(),
		Tx1:   tx1,
		Tx2:   tx2,
	}

	return nil
}

func (rl *Relayer) convertCkptToTwoTxAndSubmit(ckpt *ckpttypes.RawCheckpointWithMeta) error {
	btcCkpt, err := ckpttypes.FromRawCkptToBTCCkpt(ckpt.Ckpt, rl.submitterAddress)
	if err != nil {
		return err
	}
	data1, data2, err := btctxformatter.EncodeCheckpointData(
		rl.tag,
		rl.version,
		btcCkpt,
	)
	if err != nil {
		return err
	}

	utxo, err := rl.PickHighUTXO()
	if err != nil {
		return err
	}

	log.Logger.Debugf("Found one unspent tx with sufficient amount: %v", utxo.TxID)

	tx1, tx2, err := rl.ChainTwoTxAndSend(
		utxo,
		data1,
		data2,
	)
	if err != nil {
		return err
	}

	rl.lastSubmittedCheckpoint = &types.CheckpointInfo{
		Epoch: ckpt.Ckpt.EpochNum,
		Ts:    time.Now(),
		Tx1:   tx1,
		Tx2:   tx2,
	}

	// this is to wait for btcwallet to update utxo database so that
	// the tx that tx1 consumes will not appear in the next unspent txs lit
	time.Sleep(1 * time.Second)

	log.Logger.Infof("Sent two txs to BTC for checkpointing epoch %v, first txid: %s, second txid: %s",
		ckpt.Ckpt.EpochNum, tx1.Tx.TxHash().String(), tx2.Tx.TxHash().String())

	return nil
}

// ChainTwoTxAndSend consumes one utxo and build two chaining txs:
// the second tx consumes the output of the first tx
func (rl *Relayer) ChainTwoTxAndSend(
	utxo *types.UTXO,
	data1 []byte,
	data2 []byte,
) (*types.BtcTxInfo, *types.BtcTxInfo, error) {

	// recipient is a change address that all the
	// remaining balance of the utxo is sent to
	tx1, err := rl.buildTxWithData(
		utxo,
		data1,
	)
	if err != nil {
		return nil, nil, err
	}

	txid1, err := rl.sendTxToBTC(tx1.Tx)
	if err != nil {
		return nil, nil, err
	}

	changeUtxo := &types.UTXO{
		TxID:     txid1,
		Vout:     1,
		ScriptPK: tx1.Tx.TxOut[1].PkScript,
		Amount:   btcutil.Amount(tx1.Tx.TxOut[1].Value),
		Addr:     tx1.ChangeAddress,
	}

	// the second tx consumes the second output (index 1)
	// of the first tx, as the output at index 0 is OP_RETURN
	tx2, err := rl.buildTxWithData(
		changeUtxo,
		data2,
	)
	if err != nil {
		return nil, nil, err
	}

	_, err = rl.sendTxToBTC(tx2.Tx)
	if err != nil {
		return nil, nil, err
	}

	// TODO: if tx1 succeeds but tx2 fails, we should not resent tx1

	return tx1, tx2, nil
}

// PickHighUTXO picks a UTXO that has the highest amount
func (rl *Relayer) PickHighUTXO() (*types.UTXO, error) {
	log.Logger.Debugf("Searching for unspent transactions...")
	utxos, err := rl.ListUnspent()
	if err != nil {
		return nil, err
	}

	if len(utxos) == 0 {
		return nil, errors.New("lack of spendable transactions in the wallet")
	}

	log.Logger.Debugf("Found %v unspent transactions", len(utxos))

	topUtxo := utxos[0]
	for i, utxo := range utxos {
		log.Logger.Debugf("tx %v id: %v, amount: %v, confirmations: %v", i+1, utxo.TxID, utxo.Amount, utxo.Confirmations)
		if topUtxo.Amount < utxo.Amount {
			topUtxo = utxo
		}
	}

	// the following checks might cause panicking situations
	// because each of them indicates terrible errors brought
	// by btcclient
	prevPKScript, err := hex.DecodeString(topUtxo.ScriptPubKey)
	if err != nil {
		panic(err)
	}
	txID, err := chainhash.NewHashFromStr(topUtxo.TxID)
	if err != nil {
		panic(err)
	}
	prevAddr, err := btcutil.DecodeAddress(topUtxo.Address, rl.GetNetParams())
	if err != nil {
		panic(err)
	}
	amount, err := btcutil.NewAmount(topUtxo.Amount)
	if err != nil {
		panic(err)
	}

	// TODO: consider dust, reference: https://www.oreilly.com/library/view/mastering-bitcoin/9781491902639/ch08.html#tx_verification
	if uint64(amount.ToUnit(btcutil.AmountSatoshi)) < rl.GetMaxTxFee()*2 {
		return nil, errors.New("insufficient fees")
	}

	log.Logger.Debugf("pick utxo with id: %v, amount: %v, confirmations: %v", topUtxo.TxID, topUtxo.Amount, topUtxo.Confirmations)

	utxo := &types.UTXO{
		TxID:     txID,
		Vout:     topUtxo.Vout,
		ScriptPK: prevPKScript,
		Amount:   amount,
		Addr:     prevAddr,
	}

	return utxo, nil
}

// buildTxWithData builds a tx with data inserted as OP_RETURN
// note that OP_RETURN is set as the first output of the tx (index 0)
// and the rest of the balance is sent to a new change address
// as the second output with index 1
func (rl *Relayer) buildTxWithData(
	utxo *types.UTXO,
	data []byte,
) (*types.BtcTxInfo, error) {
	log.Logger.Debugf("Building a BTC tx using %v with data %x", utxo.TxID.String(), data)
	tx := wire.NewMsgTx(wire.TxVersion)

	outPoint := wire.NewOutPoint(utxo.TxID, utxo.Vout)
	txIn := wire.NewTxIn(outPoint, nil, nil)
	// Enable replace-by-fee
	// See https://river.com/learn/terms/r/replace-by-fee-rbf
	txIn.Sequence = math.MaxUint32 - 2
	tx.AddTxIn(txIn)

	// get private key
	err := rl.WalletPassphrase(rl.GetWalletPass(), rl.GetWalletLockTime())
	if err != nil {
		return nil, err
	}
	wif, err := rl.DumpPrivKey(utxo.Addr)
	if err != nil {
		return nil, err
	}

	// add signature/witness depending on the type of the previous address
	// if not segwit, add signature; otherwise, add witness
	segwit, err := isSegWit(utxo.Addr)
	if err != nil {
		panic(err)
	}

	// build txout for data
	builder := txscript.NewScriptBuilder()
	dataScript, err := builder.AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		return nil, err
	}
	tx.AddTxOut(wire.NewTxOut(0, dataScript))

	// build txout for change
	changeAddr, err := rl.GetChangeAddress()
	if err != nil {
		return nil, err
	}
	log.Logger.Debugf("Got a change address %v", changeAddr.String())

	changeScript, err := txscript.PayToAddrScript(changeAddr)
	if err != nil {
		return nil, err
	}
	copiedTx := &wire.MsgTx{}
	err = copier.Copy(copiedTx, tx)
	if err != nil {
		return nil, err
	}
	txSize, err := calTxSize(copiedTx, utxo, changeScript, segwit, wif.PrivKey)
	if err != nil {
		return nil, err
	}
	txFee := rl.GetTxFee(txSize)
	utxoAmount := uint64(utxo.Amount.ToUnit(btcutil.AmountSatoshi))
	change := utxoAmount - txFee
	tx.AddTxOut(wire.NewTxOut(int64(change), changeScript))

	// add unlocking script into the input of the tx
	tx, err = completeTxIn(tx, segwit, wif.PrivKey, utxo)
	if err != nil {
		return nil, err
	}

	// serialization
	var signedTxHex bytes.Buffer
	err = tx.Serialize(&signedTxHex)
	if err != nil {
		return nil, err
	}
	log.Logger.Debugf("Successfully composed a BTC tx with balance of input: %v satoshi, "+
		"tx fee: %v satoshi, output value: %v, estimated tx size: %v, actual tx size: %v, hex: %v",
		int64(utxo.Amount.ToUnit(btcutil.AmountSatoshi)), txFee, change, txSize, tx.SerializeSizeStripped(),
		hex.EncodeToString(signedTxHex.Bytes()))
	return &types.BtcTxInfo{
		Tx:            tx,
		ChangeAddress: changeAddr,
		Size:          txSize,
		UtxoAmount:    utxoAmount,
	}, nil
}

func (rl *Relayer) sendTxToBTC(tx *wire.MsgTx) (*chainhash.Hash, error) {
	log.Logger.Debugf("Sending tx %v to BTC", tx.TxHash().String())
	ha, err := rl.SendRawTransaction(tx, true)
	if err != nil {
		return nil, err
	}
	log.Logger.Debugf("Successfully sent tx %v to BTC", tx.TxHash().String())
	return ha, nil
}
