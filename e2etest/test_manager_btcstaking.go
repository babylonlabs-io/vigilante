package e2etest

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/v4/client/babylonclient"

	sdkmath "cosmossdk.io/math"
	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon/v4/btcstaking"
	txformat "github.com/babylonlabs-io/babylon/v4/btctxformatter"
	"github.com/babylonlabs-io/babylon/v4/crypto/eots"
	asig "github.com/babylonlabs-io/babylon/v4/crypto/schnorr-adaptor-signature"
	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	btcctypes "github.com/babylonlabs-io/babylon/v4/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/v4/x/btclightclient/types"
	bstypes "github.com/babylonlabs-io/babylon/v4/x/btcstaking/types"
	ckpttypes "github.com/babylonlabs-io/babylon/v4/x/checkpointing/types"
	ftypes "github.com/babylonlabs-io/babylon/v4/x/finality/types"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
)

var (
	r = rand.New(rand.NewSource(time.Now().Unix()))
)

func (tm *TestManager) getBTCUnbondingTime(t *testing.T) uint32 {
	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)

	return bsParams.Params.UnbondingTimeBlocks
}

func (tm *TestManager) CreateFinalityProvider(t *testing.T) (*bstypes.FinalityProvider, *btcec.PrivateKey) {
	var err error
	signerAddr := tm.BabylonClient.MustGetAddr()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	fpSK, _, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	btcFp, err := datagen.GenCustomFinalityProvider(r, fpSK, addr)
	require.NoError(t, err)

	zero := sdkmath.LegacyZeroDec()
	commission := bstypes.NewCommissionRates(zero, zero, zero)
	msgNewVal := &bstypes.MsgCreateFinalityProvider{
		Addr:        signerAddr,
		Description: &stakingtypes.Description{Moniker: datagen.GenRandomHexStr(r, 10)},
		Commission:  commission,
		BtcPk:       btcFp.BtcPk,
		Pop:         btcFp.Pop,
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgNewVal, nil, nil)
	require.NoError(t, err)

	return btcFp, fpSK
}

func (tm *TestManager) CreateFinalityProviderBSN(t *testing.T, bsnID string) (*bstypes.FinalityProvider, *btcec.PrivateKey) {
	var err error
	signerAddr := tm.BabylonClient.MustGetAddr()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	fpSK, _, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	btcFp, err := datagen.GenCustomFinalityProvider(r, fpSK, addr)
	require.NoError(t, err)

	zero := sdkmath.LegacyZeroDec()
	commission := bstypes.NewCommissionRates(zero, zero, zero)
	msgNewVal := &bstypes.MsgCreateFinalityProvider{
		Addr:        signerAddr,
		Description: &stakingtypes.Description{Moniker: datagen.GenRandomHexStr(r, 10)},
		Commission:  commission,
		BtcPk:       btcFp.BtcPk,
		Pop:         btcFp.Pop,
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgNewVal, nil, nil)
	require.NoError(t, err)

	return btcFp, fpSK
}

func (tm *TestManager) CreateBTCDelegation(
	t *testing.T,
	fpSK *btcec.PrivateKey,
) (*datagen.TestStakingSlashingInfo, *datagen.TestUnbondingSlashingInfo, *btcec.PrivateKey) {
	signerAddr := tm.BabylonClient.MustGetAddr()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	fpPK := fpSK.PubKey()

	/*
		create BTC delegation
	*/
	// generate staking tx and slashing tx
	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)
	covenantBtcPks, err := bbnPksToBtcPks(bsParams.Params.CovenantPks)
	require.NoError(t, err)
	stakingTimeBlocks := bsParams.Params.MaxStakingTimeBlocks
	// get top UTXO
	topUnspentResult, _, err := tm.BTCClient.GetHighUTXOAndSum()
	require.NoError(t, err)
	topUTXO, err := types.NewUTXO(topUnspentResult, regtestParams)
	require.NoError(t, err)
	// staking value
	stakingValue := int64(topUTXO.Amount) / 3

	// generate legitimate BTC del
	stakingMsgTx, stakingSlashingInfo, stakingMsgTxHash := tm.createStakingAndSlashingTx(t, []*btcec.PublicKey{fpPK}, bsParams, covenantBtcPks, topUTXO, stakingValue, stakingTimeBlocks)

	// send staking tx to Bitcoin node's mempool
	_, err = tm.BTCClient.SendRawTransaction(stakingMsgTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{stakingMsgTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(mBlock.Transactions))

	// wait until staking tx is on Bitcoin
	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(stakingMsgTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)
	// get spv proof of the BTC staking tx
	stakingTxInfo := getTxInfo(t, mBlock)

	// insert k empty blocks to Bitcoin
	btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
	require.NoError(t, err)
	btccParams := btccParamsResp.Params
	for i := 0; i < int(btccParams.BtcConfirmationDepth); i++ {
		tm.mineBlock(t)
	}

	stakingOutIdx, err := outIdx(stakingSlashingInfo.StakingTx, stakingSlashingInfo.StakingInfo.StakingOutput)
	require.NoError(t, err)

	// create PoP
	pop, err := datagen.NewPoPBTC(addr, tm.WalletPrivKey)
	require.NoError(t, err)
	slashingSpendPath, err := stakingSlashingInfo.StakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	// generate proper delegator sig
	require.NoError(t, err)

	delegatorSig, err := stakingSlashingInfo.SlashingTx.Sign(
		stakingMsgTx,
		stakingOutIdx,
		slashingSpendPath.GetPkScriptPath(),
		tm.WalletPrivKey,
	)
	require.NoError(t, err)

	// Generate all data necessary for unbonding
	unbondingSlashingInfo, unbondingSlashingPathSpendInfo, unbondingTxBytes, slashingTxSig := tm.createUnbondingData(
		t,
		[]*btcec.PublicKey{fpPK},
		bsParams,
		covenantBtcPks,
		stakingSlashingInfo,
		stakingMsgTxHash,
		stakingOutIdx,
		stakingTimeBlocks,
		uint16(bsParams.Params.UnbondingTimeBlocks),
	)

	tm.CatchUpBTCLightClient(t)

	// 	Build a message to send
	// submit BTC delegation to Babylon
	msgBTCDel := &bstypes.MsgCreateBTCDelegation{
		StakerAddr:   signerAddr,
		Pop:          pop,
		BtcPk:        bbn.NewBIP340PubKeyFromBTCPK(tm.WalletPrivKey.PubKey()),
		FpBtcPkList:  []bbn.BIP340PubKey{*bbn.NewBIP340PubKeyFromBTCPK(fpPK)},
		StakingTime:  stakingTimeBlocks,
		StakingValue: stakingValue,
		StakingTx:    stakingTxInfo.Transaction,
		StakingTxInclusionProof: &bstypes.InclusionProof{
			Key:   stakingTxInfo.Key,
			Proof: stakingTxInfo.Proof,
		},
		SlashingTx:           stakingSlashingInfo.SlashingTx,
		DelegatorSlashingSig: delegatorSig,
		// Unbonding related data
		UnbondingTime:                 tm.getBTCUnbondingTime(t),
		UnbondingTx:                   unbondingTxBytes,
		UnbondingValue:                unbondingSlashingInfo.UnbondingInfo.UnbondingOutput.Value,
		UnbondingSlashingTx:           unbondingSlashingInfo.SlashingTx,
		DelegatorUnbondingSlashingSig: slashingTxSig,
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgBTCDel, nil, nil)
	require.NoError(t, err)
	t.Logf("submitted MsgCreateBTCDelegation")

	// generate and insert new covenant signature, to activate the BTC delegation
	tm.addCovenantSig(
		t,
		signerAddr,
		stakingMsgTx,
		stakingMsgTxHash,
		[]*btcec.PublicKey{fpPK}, slashingSpendPath,
		stakingSlashingInfo,
		unbondingSlashingInfo,
		unbondingSlashingPathSpendInfo,
		stakingOutIdx,
	)

	return stakingSlashingInfo, unbondingSlashingInfo, tm.WalletPrivKey
}

// CreateMultisigBTCDelegation create multisig btc delegation with inclusion
// proof and send covenant signatures to babylon
func (tm *TestManager) CreateMultisigBTCDelegation(
	t *testing.T,
	fpSK *btcec.PrivateKey,
) (*datagen.TestStakingSlashingInfo, *datagen.TestUnbondingSlashingInfo, []*btcec.PrivateKey) {
	signerAddr := tm.BabylonClient.MustGetAddr()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	fpPK := fpSK.PubKey()

	/*
		create BTC delegation
	*/
	// generate staking tx and slashing tx
	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)
	covenantBtcPks, err := bbnPksToBtcPks(bsParams.Params.CovenantPks)
	require.NoError(t, err)
	stakingTimeBlocks := bsParams.Params.MaxStakingTimeBlocks
	// get top UTXO
	topUnspentResult, _, err := tm.BTCClient.GetHighUTXOAndSum()
	require.NoError(t, err)
	topUTXO, err := types.NewUTXO(topUnspentResult, regtestParams)
	require.NoError(t, err)
	// staking value
	stakingValue := int64(topUTXO.Amount) / 3

	// generate legitimate BTC del
	stakingMsgTx, stakingSlashingInfo, stakingMsgTxHash := tm.createMultisigStakingAndSlashingTx(t, []*btcec.PublicKey{fpPK}, bsParams, covenantBtcPks, topUTXO, stakingValue, stakingTimeBlocks)

	// send staking tx to Bitcoin node's mempool
	_, err = tm.BTCClient.SendRawTransaction(stakingMsgTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{stakingMsgTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(mBlock.Transactions))

	// wait until staking tx is on Bitcoin
	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(stakingMsgTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)
	// get spv proof of the BTC staking tx
	stakingTxInfo := getTxInfo(t, mBlock)

	// insert k empty blocks to Bitcoin
	btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
	require.NoError(t, err)
	btccParams := btccParamsResp.Params
	for i := 0; i < int(btccParams.BtcConfirmationDepth); i++ {
		tm.mineBlock(t)
	}

	stakingOutIdx, err := outIdx(stakingSlashingInfo.StakingTx, stakingSlashingInfo.StakingInfo.StakingOutput)
	require.NoError(t, err)

	// create PoP
	pop, err := datagen.NewPoPBTC(addr, tm.WalletPrivKey)
	require.NoError(t, err)
	slashingSpendPath, err := stakingSlashingInfo.StakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	// generate proper delegator sig
	require.NoError(t, err)

	// main staker's delegator slashing sig
	delegatorSig, err := stakingSlashingInfo.SlashingTx.Sign(
		stakingMsgTx,
		stakingOutIdx,
		slashingSpendPath.GetPkScriptPath(),
		tm.WalletPrivKey,
	)
	require.NoError(t, err)

	// extra staker's delegator slashing sig for multisig info
	var delegatorSi []*bstypes.SignatureInfo
	for _, sk := range tm.MultisigStakerPrivKeys {
		sig, err := stakingSlashingInfo.SlashingTx.Sign(
			stakingMsgTx,
			stakingOutIdx,
			slashingSpendPath.GetPkScriptPath(),
			sk,
		)
		require.NoError(t, err)
		delegatorSi = append(delegatorSi, &bstypes.SignatureInfo{
			Pk:  bbn.NewBIP340PubKeyFromBTCPK(sk.PubKey()),
			Sig: sig,
		})
	}

	// Generate all data necessary for unbonding
	unbondingSlashingInfo, unbondingSlashingPathSpendInfo, unbondingTxBytes, slashingTxSig, slashingSi := tm.createMultisigUnbondingData(
		t,
		[]*btcec.PublicKey{fpPK},
		bsParams,
		covenantBtcPks,
		stakingSlashingInfo,
		stakingMsgTxHash,
		stakingOutIdx,
		stakingTimeBlocks,
		uint16(bsParams.Params.UnbondingTimeBlocks),
	)

	// construct extra staker pks in BIP340PubKey
	var extraStakerPks []bbn.BIP340PubKey
	for _, sk := range tm.MultisigStakerPrivKeys {
		extraStakerPks = append(extraStakerPks, *bbn.NewBIP340PubKeyFromBTCPK(sk.PubKey()))
	}

	tm.CatchUpBTCLightClient(t)

	// 	Build a message to send
	// submit BTC delegation to Babylon
	msgBTCDel := &bstypes.MsgCreateBTCDelegation{
		StakerAddr:   signerAddr,
		Pop:          pop,
		BtcPk:        bbn.NewBIP340PubKeyFromBTCPK(tm.WalletPrivKey.PubKey()),
		FpBtcPkList:  []bbn.BIP340PubKey{*bbn.NewBIP340PubKeyFromBTCPK(fpPK)},
		StakingTime:  stakingTimeBlocks,
		StakingValue: stakingValue,
		StakingTx:    stakingTxInfo.Transaction,
		StakingTxInclusionProof: &bstypes.InclusionProof{
			Key:   stakingTxInfo.Key,
			Proof: stakingTxInfo.Proof,
		},
		SlashingTx:           stakingSlashingInfo.SlashingTx,
		DelegatorSlashingSig: delegatorSig,
		// Unbonding related data
		UnbondingTime:                 tm.getBTCUnbondingTime(t),
		UnbondingTx:                   unbondingTxBytes,
		UnbondingValue:                unbondingSlashingInfo.UnbondingInfo.UnbondingOutput.Value,
		UnbondingSlashingTx:           unbondingSlashingInfo.SlashingTx,
		DelegatorUnbondingSlashingSig: slashingTxSig,
		// multisig btc delegation related data
		MultisigInfo: &bstypes.AdditionalStakerInfo{
			StakerBtcPkList:                extraStakerPks,
			StakerQuorum:                   tm.MultisigStakerQuorum,
			DelegatorSlashingSigs:          delegatorSi,
			DelegatorUnbondingSlashingSigs: slashingSi,
		},
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgBTCDel, nil, nil)
	require.NoError(t, err)
	t.Logf("submitted MsgCreateBTCDelegation")

	// generate and insert new covenant signature, to activate the BTC delegation
	tm.addCovenantSig(
		t,
		signerAddr,
		stakingMsgTx,
		stakingMsgTxHash,
		[]*btcec.PublicKey{fpPK}, slashingSpendPath,
		stakingSlashingInfo,
		unbondingSlashingInfo,
		unbondingSlashingPathSpendInfo,
		stakingOutIdx,
	)

	return stakingSlashingInfo, unbondingSlashingInfo, tm.stakerPrivKeys()
}

func (tm *TestManager) CreateBTCDelegationWithoutIncl(
	t *testing.T,
	fpSK *btcec.PrivateKey,
) (*wire.MsgTx, *datagen.TestStakingSlashingInfo, *datagen.TestUnbondingSlashingInfo, *btcec.PrivateKey) {
	signerAddr := tm.BabylonClient.MustGetAddr()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	fpPK := fpSK.PubKey()

	/*
		create BTC delegation
	*/
	// generate staking tx and slashing tx
	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)
	covenantBtcPks, err := bbnPksToBtcPks(bsParams.Params.CovenantPks)
	require.NoError(t, err)
	stakingTimeBlocks := bsParams.Params.MaxStakingTimeBlocks
	// get top UTXO
	topUnspentResult, _, err := tm.BTCClient.GetHighUTXOAndSum()
	require.NoError(t, err)
	topUTXO, err := types.NewUTXO(topUnspentResult, regtestParams)
	require.NoError(t, err)
	// staking value
	stakingValue := int64(topUTXO.Amount) / 3

	// generate legitimate BTC del
	stakingMsgTx, stakingSlashingInfo, stakingMsgTxHash := tm.createStakingAndSlashingTx(t, []*btcec.PublicKey{fpPK}, bsParams, covenantBtcPks, topUTXO, stakingValue, stakingTimeBlocks)

	stakingOutIdx, err := outIdx(stakingSlashingInfo.StakingTx, stakingSlashingInfo.StakingInfo.StakingOutput)
	require.NoError(t, err)

	// create PoP
	pop, err := datagen.NewPoPBTC(addr, tm.WalletPrivKey)
	require.NoError(t, err)
	slashingSpendPath, err := stakingSlashingInfo.StakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	// generate proper delegator sig
	require.NoError(t, err)

	delegatorSig, err := stakingSlashingInfo.SlashingTx.Sign(
		stakingMsgTx,
		stakingOutIdx,
		slashingSpendPath.GetPkScriptPath(),
		tm.WalletPrivKey,
	)
	require.NoError(t, err)

	// Generate all data necessary for unbonding
	unbondingSlashingInfo, unbondingSlashingPathSpendInfo, unbondingTxBytes, slashingTxSig := tm.createUnbondingData(
		t,
		[]*btcec.PublicKey{fpPK},
		bsParams,
		covenantBtcPks,
		stakingSlashingInfo,
		stakingMsgTxHash,
		stakingOutIdx,
		stakingTimeBlocks,
		uint16(bsParams.Params.UnbondingTimeBlocks),
	)

	var stakingTxBuf bytes.Buffer
	err = stakingMsgTx.Serialize(&stakingTxBuf)
	require.NoError(t, err)

	// submit BTC delegation to Babylon
	msgBTCDel := &bstypes.MsgCreateBTCDelegation{
		StakerAddr:              signerAddr,
		Pop:                     pop,
		BtcPk:                   bbn.NewBIP340PubKeyFromBTCPK(tm.WalletPrivKey.PubKey()),
		FpBtcPkList:             []bbn.BIP340PubKey{*bbn.NewBIP340PubKeyFromBTCPK(fpPK)},
		StakingTime:             stakingTimeBlocks,
		StakingValue:            stakingValue,
		StakingTx:               stakingTxBuf.Bytes(),
		StakingTxInclusionProof: nil,
		SlashingTx:              stakingSlashingInfo.SlashingTx,
		DelegatorSlashingSig:    delegatorSig,
		// Unbonding related data
		UnbondingTime:                 uint32(tm.getBTCUnbondingTime(t)),
		UnbondingTx:                   unbondingTxBytes,
		UnbondingValue:                unbondingSlashingInfo.UnbondingInfo.UnbondingOutput.Value,
		UnbondingSlashingTx:           unbondingSlashingInfo.SlashingTx,
		DelegatorUnbondingSlashingSig: slashingTxSig,
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgBTCDel, nil, nil)
	require.NoError(t, err)
	t.Logf("submitted MsgCreateBTCDelegation")

	// generate and insert new covenant signature, to activate the BTC delegation
	tm.addCovenantSig(
		t,
		signerAddr,
		stakingMsgTx,
		stakingMsgTxHash,
		[]*btcec.PublicKey{fpPK}, slashingSpendPath,
		stakingSlashingInfo,
		unbondingSlashingInfo,
		unbondingSlashingPathSpendInfo,
		stakingOutIdx,
	)

	return stakingMsgTx, stakingSlashingInfo, unbondingSlashingInfo, tm.WalletPrivKey
}

func (tm *TestManager) createStakingAndSlashingTx(
	t *testing.T, fpPKs []*btcec.PublicKey,
	bsParams *bstypes.QueryParamsResponse,
	covenantBtcPks []*btcec.PublicKey,
	topUTXO *types.UTXO,
	stakingValue int64,
	stakingTimeBlocks uint32,
) (*wire.MsgTx, *datagen.TestStakingSlashingInfo, *chainhash.Hash) {
	// generate staking tx and slashing tx
	// generate legitimate BTC del
	stakingSlashingInfo := datagen.GenBTCStakingSlashingInfoWithOutPoint(
		r,
		t,
		regtestParams,
		topUTXO.GetOutPoint(),
		tm.WalletPrivKey,
		fpPKs,
		covenantBtcPks,
		bsParams.Params.CovenantQuorum,
		uint16(stakingTimeBlocks),
		stakingValue,
		bsParams.Params.SlashingPkScript,
		bsParams.Params.SlashingRate,
		uint16(tm.getBTCUnbondingTime(t)),
	)
	// sign staking tx and overwrite the staking tx to the signed version
	// NOTE: the tx hash has changed here since stakingMsgTx is pre-segwit
	stakingMsgTx, signed, err := tm.BTCClient.SignRawTransactionWithWallet(stakingSlashingInfo.StakingTx)
	require.NoError(t, err)
	require.True(t, signed)
	// overwrite staking tx
	stakingSlashingInfo.StakingTx = stakingMsgTx
	// get signed staking tx hash
	stakingMsgTxHash1 := stakingSlashingInfo.StakingTx.TxHash()
	stakingMsgTxHash := &stakingMsgTxHash1
	t.Logf("signed staking tx hash: %s", stakingMsgTxHash.String())

	// change outpoint tx hash of slashing tx to the txhash of the signed staking tx
	slashingMsgTx, err := stakingSlashingInfo.SlashingTx.ToMsgTx()
	require.NoError(t, err)
	slashingMsgTx.TxIn[0].PreviousOutPoint.Hash = stakingSlashingInfo.StakingTx.TxHash()
	// update slashing tx
	stakingSlashingInfo.SlashingTx, err = bstypes.NewBTCSlashingTxFromMsgTx(slashingMsgTx)
	require.NoError(t, err)

	return stakingSlashingInfo.StakingTx, stakingSlashingInfo, stakingMsgTxHash
}

func (tm *TestManager) createMultisigStakingAndSlashingTx(
	t *testing.T, fpPKs []*btcec.PublicKey,
	bsParams *bstypes.QueryParamsResponse,
	covenantBtcPks []*btcec.PublicKey,
	topUTXO *types.UTXO,
	stakingValue int64,
	stakingTimeBlocks uint32,
) (*wire.MsgTx, *datagen.TestStakingSlashingInfo, *chainhash.Hash) {
	// generate staking tx and slashing tx
	// generate legitimate BTC del
	stakingSlashingInfo := datagen.GenMultisigBTCStakingSlashingInfoWithOutpoint(
		r,
		t,
		regtestParams,
		topUTXO.GetOutPoint(),
		tm.stakerPrivKeys(),
		tm.MultisigStakerQuorum,
		fpPKs,
		covenantBtcPks,
		bsParams.Params.CovenantQuorum,
		uint16(stakingTimeBlocks),
		stakingValue,
		bsParams.Params.SlashingPkScript,
		bsParams.Params.SlashingRate,
		uint16(tm.getBTCUnbondingTime(t)),
	)
	// sign staking tx and overwrite the staking tx to the signed version
	// NOTE: the tx hash has changed here since stakingMsgTx is pre-segwit
	stakingMsgTx, signed, err := tm.BTCClient.SignRawTransactionWithWallet(stakingSlashingInfo.StakingTx)
	require.NoError(t, err)
	require.True(t, signed)
	// overwrite staking tx
	stakingSlashingInfo.StakingTx = stakingMsgTx
	// get signed staking tx hash
	stakingMsgTxHash1 := stakingSlashingInfo.StakingTx.TxHash()
	stakingMsgTxHash := &stakingMsgTxHash1
	t.Logf("signed staking tx hash: %s", stakingMsgTxHash.String())

	// change outpoint tx hash of slashing tx to the txhash of the signed staking tx
	slashingMsgTx, err := stakingSlashingInfo.SlashingTx.ToMsgTx()
	require.NoError(t, err)
	slashingMsgTx.TxIn[0].PreviousOutPoint.Hash = stakingSlashingInfo.StakingTx.TxHash()
	// update slashing tx
	stakingSlashingInfo.SlashingTx, err = bstypes.NewBTCSlashingTxFromMsgTx(slashingMsgTx)
	require.NoError(t, err)

	return stakingSlashingInfo.StakingTx, stakingSlashingInfo, stakingMsgTxHash
}

func (tm *TestManager) createStakeExpStakingAndSlashingTx(
	t *testing.T, fpSK *btcec.PrivateKey,
	bsParams *bstypes.QueryParamsResponse,
	covenantBtcPks []*btcec.PublicKey,
	stakingValue int64,
	stakingTimeBlocks uint32,
	prevStakeTxHash string,
	prevDelStakingOutputIdx uint32,
	fundingTx *wire.MsgTx,
) (*wire.MsgTx, *datagen.TestStakingSlashingInfo, *chainhash.Hash) {
	// generate staking tx and slashing tx
	fpPK := fpSK.PubKey()

	// Convert prevStakeTxHash string to OutPoint
	prevHash, err := chainhash.NewHashFromStr(prevStakeTxHash)
	require.NoError(t, err)
	prevStakingOutPoint := wire.NewOutPoint(prevHash, prevDelStakingOutputIdx)

	// Convert fundingTxHash to OutPoint
	fundingTxHash := fundingTx.TxHash()
	fundingOutPoint := wire.NewOutPoint(&fundingTxHash, 0)
	outPoints := []*wire.OutPoint{prevStakingOutPoint, fundingOutPoint}

	// generate legitimate BTC del
	stakingSlashingInfo := datagen.GenBTCStakingSlashingInfoWithInputs(
		r,
		t,
		regtestParams,
		outPoints,
		tm.WalletPrivKey,
		[]*btcec.PublicKey{fpPK},
		covenantBtcPks,
		bsParams.Params.CovenantQuorum,
		uint16(stakingTimeBlocks),
		stakingValue,
		bsParams.Params.SlashingPkScript,
		bsParams.Params.SlashingRate,
		uint16(tm.getBTCUnbondingTime(t)),
	)

	stakingTxHash := stakingSlashingInfo.StakingTx.TxHash()
	return stakingSlashingInfo.StakingTx, stakingSlashingInfo, &stakingTxHash
}

func (tm *TestManager) createMultisigStakeExpStakingAndSlashingTx(
	t *testing.T, fpSK *btcec.PrivateKey,
	bsParams *bstypes.QueryParamsResponse,
	covenantBtcPks []*btcec.PublicKey,
	stakingValue int64,
	stakingTimeBlocks uint32,
	prevStakeTxHash string,
	prevDelStakingOutputIdx uint32,
	fundingTx *wire.MsgTx,
) (*wire.MsgTx, *datagen.TestStakingSlashingInfo, *chainhash.Hash) {
	// generate staking tx and slashing tx
	fpPK := fpSK.PubKey()

	// Convert prevStakeTxHash string to OutPoint
	prevHash, err := chainhash.NewHashFromStr(prevStakeTxHash)
	require.NoError(t, err)
	prevStakingOutPoint := wire.NewOutPoint(prevHash, prevDelStakingOutputIdx)

	// Convert fundingTxHash to OutPoint
	fundingTxHash := fundingTx.TxHash()
	fundingOutPoint := wire.NewOutPoint(&fundingTxHash, 0)
	outPoints := []*wire.OutPoint{prevStakingOutPoint, fundingOutPoint}

	// generate legitimate BTC del
	stakingSlashingInfo := datagen.GenMultisigBTCStakingSlashingInfoWithInputs(
		r,
		t,
		regtestParams,
		outPoints,
		tm.stakerPrivKeys(),
		tm.MultisigStakerQuorum,
		[]*btcec.PublicKey{fpPK},
		covenantBtcPks,
		bsParams.Params.CovenantQuorum,
		uint16(stakingTimeBlocks),
		stakingValue,
		bsParams.Params.SlashingPkScript,
		bsParams.Params.SlashingRate,
		uint16(tm.getBTCUnbondingTime(t)),
		10000,
	)

	stakingTxHash := stakingSlashingInfo.StakingTx.TxHash()
	return stakingSlashingInfo.StakingTx, stakingSlashingInfo, &stakingTxHash
}

func (tm *TestManager) createUnbondingData(
	t *testing.T,
	fpPKs []*btcec.PublicKey,
	bsParams *bstypes.QueryParamsResponse,
	covenantBtcPks []*btcec.PublicKey,
	stakingSlashingInfo *datagen.TestStakingSlashingInfo,
	stakingMsgTxHash *chainhash.Hash,
	stakingOutIdx uint32,
	stakingTimeBlocks uint32,
	unbondingTime uint16,
) (*datagen.TestUnbondingSlashingInfo, *btcstaking.SpendInfo, []byte, *bbn.BIP340Signature) {
	fee := int64(1000)
	unbondingValue := stakingSlashingInfo.StakingInfo.StakingOutput.Value - fee
	unbondingSlashingInfo := datagen.GenBTCUnbondingSlashingInfo(
		r,
		t,
		regtestParams,
		tm.WalletPrivKey,
		fpPKs,
		covenantBtcPks,
		bsParams.Params.CovenantQuorum,
		wire.NewOutPoint(stakingMsgTxHash, stakingOutIdx),
		uint16(stakingTimeBlocks),
		unbondingValue,
		bsParams.Params.SlashingPkScript,
		bsParams.Params.SlashingRate,
		unbondingTime,
	)
	unbondingTxBytes, err := bbn.SerializeBTCTx(unbondingSlashingInfo.UnbondingTx)
	require.NoError(t, err)

	unbondingSlashingPathSpendInfo, err := unbondingSlashingInfo.UnbondingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	slashingTxSig, err := unbondingSlashingInfo.SlashingTx.Sign(
		unbondingSlashingInfo.UnbondingTx,
		0, // Only one output in the unbonding tx
		unbondingSlashingPathSpendInfo.GetPkScriptPath(),
		tm.WalletPrivKey,
	)
	require.NoError(t, err)

	return unbondingSlashingInfo, unbondingSlashingPathSpendInfo, unbondingTxBytes, slashingTxSig
}

func (tm *TestManager) createMultisigUnbondingData(
	t *testing.T,
	fpPKs []*btcec.PublicKey,
	bsParams *bstypes.QueryParamsResponse,
	covenantBtcPks []*btcec.PublicKey,
	stakingSlashingInfo *datagen.TestStakingSlashingInfo,
	stakingMsgTxHash *chainhash.Hash,
	stakingOutIdx uint32,
	stakingTimeBlocks uint32,
	unbondingTime uint16,
) (*datagen.TestUnbondingSlashingInfo, *btcstaking.SpendInfo, []byte, *bbn.BIP340Signature, []*bstypes.SignatureInfo) {
	fee := int64(1000)
	unbondingValue := stakingSlashingInfo.StakingInfo.StakingOutput.Value - fee
	unbondingSlashingInfo := datagen.GenMultisigBTCUnbondingSlashingInfo(
		r,
		t,
		regtestParams,
		tm.stakerPrivKeys(),
		tm.MultisigStakerQuorum,
		fpPKs,
		covenantBtcPks,
		bsParams.Params.CovenantQuorum,
		wire.NewOutPoint(stakingMsgTxHash, stakingOutIdx),
		uint16(stakingTimeBlocks),
		unbondingValue,
		bsParams.Params.SlashingPkScript,
		bsParams.Params.SlashingRate,
		unbondingTime,
	)
	unbondingTxBytes, err := bbn.SerializeBTCTx(unbondingSlashingInfo.UnbondingTx)
	require.NoError(t, err)

	unbondingSlashingPathSpendInfo, err := unbondingSlashingInfo.UnbondingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	// signature of the main staker
	slashingTxSig, err := unbondingSlashingInfo.SlashingTx.Sign(
		unbondingSlashingInfo.UnbondingTx,
		0, // Only one output in the unbonding tx
		unbondingSlashingPathSpendInfo.GetPkScriptPath(),
		tm.WalletPrivKey,
	)
	require.NoError(t, err)

	// SignatureInfo of extra stakers
	var si []*bstypes.SignatureInfo
	for _, sk := range tm.MultisigStakerPrivKeys {
		sig, err := unbondingSlashingInfo.SlashingTx.Sign(
			unbondingSlashingInfo.UnbondingTx,
			0,
			unbondingSlashingPathSpendInfo.GetPkScriptPath(),
			sk,
		)
		require.NoError(t, err)
		si = append(si, &bstypes.SignatureInfo{
			Pk:  bbn.NewBIP340PubKeyFromBTCPK(sk.PubKey()),
			Sig: sig,
		})
	}

	return unbondingSlashingInfo, unbondingSlashingPathSpendInfo, unbondingTxBytes, slashingTxSig, si
}

func (tm *TestManager) addCovenantSig(
	t *testing.T,
	signerAddr string,
	stakingMsgTx *wire.MsgTx,
	stakingMsgTxHash *chainhash.Hash,
	fpPks []*btcec.PublicKey,
	slashingSpendPath *btcstaking.SpendInfo,
	stakingSlashingInfo *datagen.TestStakingSlashingInfo,
	unbondingSlashingInfo *datagen.TestUnbondingSlashingInfo,
	unbondingSlashingPathSpendInfo *btcstaking.SpendInfo,
	stakingOutIdx uint32,
) []*bbn.BIP340Signature {
	var unbondingTxSigs []*bbn.BIP340Signature
	for _, key := range tm.CovenantPrivKeys {
		msgAddCovenantSig := tm.createMsgAddCovenantSigs(t,
			signerAddr,
			stakingMsgTx,
			stakingMsgTxHash,
			fpPks,
			slashingSpendPath,
			stakingSlashingInfo,
			unbondingSlashingInfo,
			unbondingSlashingPathSpendInfo,
			stakingOutIdx, key)
		_, err := tm.BabylonClient.ReliablySendMsg(t.Context(), msgAddCovenantSig, nil, nil)
		require.NoError(t, err)
		unbondingTxSigs = append(unbondingTxSigs, msgAddCovenantSig.UnbondingTxSig)
		t.Logf("submitted covenant signature for key %s", hex.EncodeToString(key.PubKey().SerializeCompressed()))
	}

	return unbondingTxSigs
}

func (tm *TestManager) addCovenantSigStkExp(
	t *testing.T,
	signerAddr string,
	stakingExpMsgTx *wire.MsgTx,
	stakingMsgTxHash *chainhash.Hash,
	fpSK *btcec.PrivateKey,
	slashingSpendPath *btcstaking.SpendInfo,
	stakingSlashingInfo *datagen.TestStakingSlashingInfo,
	unbondingSlashingInfo *datagen.TestUnbondingSlashingInfo,
	unbondingSlashingPathSpendInfo *btcstaking.SpendInfo,
	stakingOutIdx uint32,
	prevStakingSlashingInfo *datagen.TestStakingSlashingInfo,
	fundingTxOut *wire.TxOut,
) []*bbn.BIP340Signature {
	var unbondingTxSigs []*bbn.BIP340Signature
	for _, key := range tm.CovenantPrivKeys {
		msgAddCovenantSig := tm.createMsgAddCovenantSigs(t,
			signerAddr,
			stakingExpMsgTx,
			stakingMsgTxHash,
			[]*btcec.PublicKey{fpSK.PubKey()},
			slashingSpendPath,
			stakingSlashingInfo,
			unbondingSlashingInfo,
			unbondingSlashingPathSpendInfo,
			stakingOutIdx, key,
		)

		stkExpSig := tm.signStakeExpansionTx(t, stakingExpMsgTx, prevStakingSlashingInfo, fundingTxOut, key)
		msgAddCovenantSig.StakeExpansionTxSig = stkExpSig

		_, err := tm.BabylonClient.ReliablySendMsg(context.Background(), msgAddCovenantSig, nil, nil)
		require.NoError(t, err)
		unbondingTxSigs = append(unbondingTxSigs, msgAddCovenantSig.UnbondingTxSig)

	}
	t.Logf("submitted covenant signature")
	return unbondingTxSigs
}

func (tm *TestManager) createMsgAddCovenantSigs(
	t *testing.T,
	signerAddr string,
	stakingMsgTx *wire.MsgTx,
	stakingMsgTxHash *chainhash.Hash,
	fpPKs []*btcec.PublicKey,
	slashingSpendPath *btcstaking.SpendInfo,
	stakingSlashingInfo *datagen.TestStakingSlashingInfo,
	unbondingSlashingInfo *datagen.TestUnbondingSlashingInfo,
	unbondingSlashingPathSpendInfo *btcstaking.SpendInfo,
	stakingOutIdx uint32,
	covenantSk *btcec.PrivateKey,
) *bstypes.MsgAddCovenantSigs {
	var slashingTxSigs [][]byte
	for _, fpPK := range fpPKs {
		fpEncKey, err := asig.NewEncryptionKeyFromBTCPK(fpPK)
		require.NoError(t, err)
		covenantSig, err := stakingSlashingInfo.SlashingTx.EncSign(
			stakingMsgTx,
			stakingOutIdx,
			slashingSpendPath.GetPkScriptPath(),
			covenantSk,
			fpEncKey,
		)
		require.NoError(t, err)
		slashingTxSigs = append(slashingTxSigs, covenantSig.MustMarshal())
	}

	// add covenant sigs
	// covenant Schnorr sig on unbonding tx
	unbondingPathSpendInfo, err := stakingSlashingInfo.StakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)
	unbondingTxCovenantSchnorrSig, err := btcstaking.SignTxWithOneScriptSpendInputStrict(
		unbondingSlashingInfo.UnbondingTx,
		stakingSlashingInfo.StakingTx,
		stakingOutIdx,
		unbondingPathSpendInfo.GetPkScriptPath(),
		covenantSk,
	)
	require.NoError(t, err)
	covenantUnbondingSig := bbn.NewBIP340SignatureFromBTCSig(unbondingTxCovenantSchnorrSig)
	// covenant adaptor sig on unbonding slashing tx
	require.NoError(t, err)

	var slashingUnbondingTxSigs [][]byte
	for _, fpPK := range fpPKs {
		fpEncKey, err := asig.NewEncryptionKeyFromBTCPK(fpPK)
		covenantSlashingSig, err := unbondingSlashingInfo.SlashingTx.EncSign(
			unbondingSlashingInfo.UnbondingTx,
			0, // Only one output in the unbonding transaction
			unbondingSlashingPathSpendInfo.GetPkScriptPath(),
			covenantSk,
			fpEncKey,
		)
		require.NoError(t, err)
		slashingUnbondingTxSigs = append(slashingUnbondingTxSigs, covenantSlashingSig.MustMarshal())
	}

	msgAddCovenantSig := &bstypes.MsgAddCovenantSigs{
		Signer:                  signerAddr,
		Pk:                      bbn.NewBIP340PubKeyFromBTCPK(covenantSk.PubKey()),
		StakingTxHash:           stakingMsgTxHash.String(),
		SlashingTxSigs:          slashingTxSigs,
		UnbondingTxSig:          covenantUnbondingSig,
		SlashingUnbondingTxSigs: slashingUnbondingTxSigs,
	}
	return msgAddCovenantSig

}

// signStakeExpansionTx signs a stake expansion transaction for covenant participation
// This is simplified to work with the existing test structures
func (tm *TestManager) signStakeExpansionTx(
	t *testing.T,
	expansionStakingTx *wire.MsgTx,
	originalStakingSlashingInfo *datagen.TestStakingSlashingInfo,
	fundingTxOut *wire.TxOut,
	covenantSk *btcec.PrivateKey,
) *bbn.BIP340Signature {
	// Get the unbonding path spend info from the original staking transaction
	// This is what the expansion transaction will spend from
	prevDelUnbondPathSpendInfo, err := originalStakingSlashingInfo.StakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	// Sign the expansion transaction with the covenant key
	// This assumes the expansion tx has the original staking output as first input
	// and the funding output as second input
	sig, err := btcstaking.SignTxForFirstScriptSpendWithTwoInputsFromScript(
		expansionStakingTx,
		originalStakingSlashingInfo.StakingTx.TxOut[0],
		fundingTxOut,
		covenantSk,
		prevDelUnbondPathSpendInfo.GetPkScriptPath(),
	)
	require.NoError(t, err)

	return bbn.NewBIP340SignatureFromBTCSig(sig)
}

func (tm *TestManager) Undelegate(
	t *testing.T,
	stakingSlashingInfo *datagen.TestStakingSlashingInfo,
	unbondingSlashingInfo *datagen.TestUnbondingSlashingInfo,
	delSK *btcec.PrivateKey,
	catchUpLightClientFunc func()) (*datagen.TestUnbondingSlashingInfo, *schnorr.Signature) {
	signerAddr := tm.BabylonClient.MustGetAddr()

	// TODO: This generates unbonding tx signature, move it to undelegate
	unbondingPathSpendInfo, err := stakingSlashingInfo.StakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	// the only input to unbonding tx is the staking tx
	stakingOutIdx, err := outIdx(unbondingSlashingInfo.UnbondingTx, unbondingSlashingInfo.UnbondingInfo.UnbondingOutput)
	require.NoError(t, err)

	unbondingTxSchnorrSig, err := btcstaking.SignTxWithOneScriptSpendInputStrict(
		unbondingSlashingInfo.UnbondingTx,
		stakingSlashingInfo.StakingTx,
		stakingOutIdx,
		unbondingPathSpendInfo.GetPkScriptPath(),
		delSK,
	)
	require.NoError(t, err)

	resp, err := tm.BabylonClient.BTCDelegation(stakingSlashingInfo.StakingTx.TxHash().String())
	require.NoError(t, err)
	covenantSigs := resp.BtcDelegation.UndelegationResponse.CovenantUnbondingSigList
	witness, err := unbondingPathSpendInfo.CreateUnbondingPathWitness(
		[]*schnorr.Signature{covenantSigs[0].Sig.MustToBTCSig()},
		unbondingTxSchnorrSig,
	)
	require.NoError(t, err)
	unbondingSlashingInfo.UnbondingTx.TxIn[0].Witness = witness

	serializedUnbondingTx, err := bbn.SerializeBTCTx(unbondingSlashingInfo.UnbondingTx)
	require.NoError(t, err)

	// send unbonding tx to Bitcoin node's mempool
	unbondingTxHash, err := tm.BTCClient.SendRawTransaction(unbondingSlashingInfo.UnbondingTx, true)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(unbondingTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)
	t.Logf("submitted unbonding tx with hash %s", unbondingTxHash.String())

	// mine a block with this tx, and insert it to Bitcoin
	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{unbondingTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(mBlock.Transactions))

	catchUpLightClientFunc()

	fundingTxns := tm.getFundingTxs(t, unbondingSlashingInfo.UnbondingTx)
	require.NoError(t, err)

	unbondingTxInfo := getTxInfo(t, mBlock)
	msgUndel := &bstypes.MsgBTCUndelegate{
		Signer:              signerAddr,
		StakingTxHash:       stakingSlashingInfo.StakingTx.TxHash().String(),
		StakeSpendingTx:     serializedUnbondingTx,
		FundingTransactions: fundingTxns,
		StakeSpendingTxInclusionProof: &bstypes.InclusionProof{
			Key:   unbondingTxInfo.Key,
			Proof: unbondingTxInfo.Proof,
		},
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgUndel, nil, nil)
	require.NoError(t, err)
	t.Logf("submitted MsgBTCUndelegate")

	// wait until unbonding tx is on Bitcoin
	require.Eventually(t, func() bool {
		resp, err := tm.BTCClient.GetRawTransactionVerbose(unbondingTxHash)
		if err != nil {
			t.Logf("err of GetRawTransactionVerbose: %v", err)
			return false
		}
		return len(resp.BlockHash) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	return unbondingSlashingInfo, unbondingTxSchnorrSig
}

func (tm *TestManager) VoteAndEquivocate(t *testing.T, fpSK *btcec.PrivateKey) {
	signerAddr := tm.BabylonClient.MustGetAddr()

	// get the finality provider
	fpBTCPK := bbn.NewBIP340PubKeyFromBTCPK(fpSK.PubKey())
	fpResp, err := tm.BabylonClient.FinalityProvider(fpBTCPK.MarshalHex())
	require.NoError(t, err)
	btcFp := fpResp.FinalityProvider

	_, err = tm.BabylonClient.ActivatedHeight()
	require.Error(t, err)

	activatedHeight := uint64(1)
	commitStartHeight := activatedHeight

	/*
		commit a number of public randomness since activatedHeight
	*/
	srList, msgCommitPubRandList, err := datagen.GenRandomMsgCommitPubRandList(r, fpSK, activatedHeight, 100)
	require.NoError(t, err)
	msgCommitPubRandList.Signer = signerAddr
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgCommitPubRandList, nil, nil)
	require.NoError(t, err)
	t.Logf("committed public randomness")

	tm.mineBlock(t)

	tm.waitForFpPubRandTimestamped(t, fpSK.PubKey())

	require.Eventually(t, func() bool {
		acr, err := tm.BabylonClient.ActivatedHeight()
		if err != nil {
			return false
		}
		activatedHeight = acr.Height
		return activatedHeight > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	/*
		submit finality signature
	*/
	// get block to vote
	blockToVote, err := tm.BabylonClient.GetBlock(int64(activatedHeight))
	require.NoError(t, err)
	msgToSign := sdk.Uint64ToBigEndian(activatedHeight)
	msgToSign = append(msgToSign, blockToVote.Block.AppHash...)
	// generate EOTS signature
	idx := activatedHeight - commitStartHeight
	sig, err := eots.Sign(fpSK, srList.SRList[idx], msgToSign)
	require.NoError(t, err)
	eotsSig := bbn.NewSchnorrEOTSSigFromModNScalar(sig)
	// submit finality signature
	msgAddFinalitySig := &ftypes.MsgAddFinalitySig{
		Signer:       signerAddr,
		FpBtcPk:      btcFp.BtcPk,
		BlockHeight:  activatedHeight,
		PubRand:      &srList.PRList[idx],
		Proof:        srList.ProofList[idx].ToProto(),
		BlockAppHash: blockToVote.Block.AppHash,
		FinalitySig:  eotsSig,
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgAddFinalitySig, nil, nil)
	require.NoError(t, err)
	t.Logf("submitted finality signature")

	/*
		equivocate
	*/
	invalidAppHash := datagen.GenRandomByteArray(r, 32)
	invalidMsgToSign := sdk.Uint64ToBigEndian(activatedHeight)
	invalidMsgToSign = append(invalidMsgToSign, invalidAppHash...)
	invalidSig, err := eots.Sign(fpSK, srList.SRList[idx], invalidMsgToSign)
	require.NoError(t, err)
	invalidEotsSig := bbn.NewSchnorrEOTSSigFromModNScalar(invalidSig)
	invalidMsgAddFinalitySig := &ftypes.MsgAddFinalitySig{
		Signer:       signerAddr,
		FpBtcPk:      btcFp.BtcPk,
		BlockHeight:  activatedHeight,
		PubRand:      &srList.PRList[idx],
		Proof:        srList.ProofList[idx].ToProto(),
		BlockAppHash: invalidAppHash,
		FinalitySig:  invalidEotsSig,
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), invalidMsgAddFinalitySig, nil, nil)
	require.NoError(t, err)
	t.Logf("submitted equivocating finality signature")
}

func getTxInfo(t *testing.T, block *wire.MsgBlock) *btcctypes.TransactionInfo {
	mHeaderBytes := bbn.NewBTCHeaderBytesFromBlockHeader(&block.Header)
	var txBytes [][]byte
	for _, tx := range block.Transactions {
		buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
		_ = tx.Serialize(buf)
		txBytes = append(txBytes, buf.Bytes())
	}
	spvProof, err := btcctypes.SpvProofFromHeaderAndTransactions(&mHeaderBytes, txBytes, 1)
	require.NoError(t, err)
	return btcctypes.NewTransactionInfoFromSpvProof(spvProof)
}

// TODO: these functions should be enabled by Babylon
func bbnPksToBtcPks(pks []bbn.BIP340PubKey) ([]*btcec.PublicKey, error) {
	btcPks := make([]*btcec.PublicKey, 0, len(pks))
	for _, pk := range pks {
		btcPk, err := pk.ToBTCPK()
		if err != nil {
			return nil, err
		}
		btcPks = append(btcPks, btcPk)
	}
	return btcPks, nil
}

func outIdx(tx *wire.MsgTx, candOut *wire.TxOut) (uint32, error) {
	for idx, out := range tx.TxOut {
		if bytes.Equal(out.PkScript, candOut.PkScript) && out.Value == candOut.Value {
			return uint32(idx), nil
		}
	}
	return 0, fmt.Errorf("couldn't find output")
}

func (tm *TestManager) waitForFpPubRandTimestamped(t *testing.T, fpPk *btcec.PublicKey) {
	var lastCommittedHeight uint64
	var err error

	require.Eventually(t, func() bool {
		lastCommittedHeight, err = tm.getLastCommittedHeight(fpPk)
		if err != nil {
			return false
		}
		return lastCommittedHeight > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("public randomness is successfully committed, last committed height: %d", lastCommittedHeight)

	// wait until the last registered epoch is finalized
	currentEpoch, err := tm.BabylonClient.CurrentEpoch()
	require.NoError(t, err)

	tm.finalizeUntilEpoch(t, currentEpoch.CurrentEpoch)

	res, err := tm.BabylonClient.LatestEpochFromStatus(ckpttypes.Finalized)
	require.NoError(t, err)
	t.Logf("last finalized epoch: %d", res.RawCheckpoint.EpochNum)

	t.Logf("public randomness is successfully timestamped, last finalized epoch: %v", currentEpoch)
}

// queryLastCommittedPublicRand returns the last public randomness commitments
func (tm *TestManager) queryLastCommittedPublicRand(fpPk *btcec.PublicKey, count uint64) (map[uint64]*ftypes.PubRandCommitResponse, error) {
	fpBtcPk := bbn.NewBIP340PubKeyFromBTCPK(fpPk)

	pagination := &sdkquery.PageRequest{
		Limit:   count,
		Reverse: true,
	}

	res, err := tm.BabylonClient.QueryClient.ListPubRandCommit(fpBtcPk.MarshalHex(), pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to query committed public randomness: %w", err)
	}

	return res.PubRandCommitMap, nil
}

func (tm *TestManager) lastCommittedPublicRandWithRetry(btcPk *btcec.PublicKey, count uint64) (map[uint64]*ftypes.PubRandCommitResponse, error) {
	var response map[uint64]*ftypes.PubRandCommitResponse

	if err := retry.Do(func() error {
		resp, err := tm.queryLastCommittedPublicRand(btcPk, count)
		if err != nil {
			return err
		}
		response = resp
		return nil
	},
		retry.Attempts(tm.Config.Common.MaxRetryTimes),
		retry.Delay(tm.Config.Common.RetrySleepTime),
		retry.LastErrorOnly(true)); err != nil {
		return nil, err
	}

	return response, nil
}

func (tm *TestManager) getLastCommittedHeight(btcPk *btcec.PublicKey) (uint64, error) {
	pubRandCommitMap, err := tm.lastCommittedPublicRandWithRetry(btcPk, 1)
	if err != nil {
		return 0, err
	}

	// no committed randomness yet
	if len(pubRandCommitMap) == 0 {
		return 0, nil
	}

	if len(pubRandCommitMap) > 1 {
		return 0, fmt.Errorf("got more than one last committed public randomness")
	}
	var lastCommittedHeight uint64
	for startHeight, resp := range pubRandCommitMap {
		lastCommittedHeight = startHeight + resp.NumPubRand - 1
	}

	return lastCommittedHeight, nil
}

func (tm *TestManager) finalizeUntilEpoch(t *testing.T, epoch uint64) {
	bbnClient := tm.BabylonClient

	// wait until the checkpoint of this epoch is sealed
	require.Eventually(t, func() bool {
		lastSealedCkpt, err := bbnClient.LatestEpochFromStatus(ckpttypes.Sealed)
		if err != nil {
			return false
		}
		return epoch <= lastSealedCkpt.RawCheckpoint.EpochNum
	}, eventuallyWaitTimeOut, 1*time.Second)

	t.Logf("start finalizing epochs until %d", epoch)
	// Random source for the generation of BTC data
	r := rand.New(rand.NewSource(time.Now().Unix()))

	// get all checkpoints of these epochs
	pagination := &sdkquerytypes.PageRequest{
		Key:   ckpttypes.CkptsObjectKey(0),
		Limit: epoch,
	}
	resp, err := bbnClient.RawCheckpoints(pagination)
	require.NoError(t, err)
	require.Equal(t, int(epoch), len(resp.RawCheckpoints))

	submitterAddr, err := sdk.AccAddressFromBech32(tm.BabylonClient.MustGetAddr())
	require.NoError(t, err)

	for _, checkpoint := range resp.RawCheckpoints {
		currentBtcTipResp, err := tm.BabylonClient.QueryClient.BTCHeaderChainTip()
		require.NoError(t, err)
		tipHeader, err := bbn.NewBTCHeaderBytesFromHex(currentBtcTipResp.Header.HeaderHex)
		require.NoError(t, err)

		rawCheckpoint, err := checkpoint.Ckpt.ToRawCheckpoint()
		require.NoError(t, err)

		btcCheckpoint, err := ckpttypes.FromRawCkptToBTCCkpt(rawCheckpoint, submitterAddr)
		require.NoError(t, err)

		babylonTagBytes, err := hex.DecodeString("01020304")
		require.NoError(t, err)

		p1, p2, err := txformat.EncodeCheckpointData(
			babylonTagBytes,
			txformat.CurrentVersion,
			btcCheckpoint,
		)
		require.NoError(t, err)

		tx1 := datagen.CreatOpReturnTransaction(r, p1)

		opReturn1 := datagen.CreateBlockWithTransaction(r, tipHeader.ToBlockHeader(), tx1)
		tx2 := datagen.CreatOpReturnTransaction(r, p2)
		opReturn2 := datagen.CreateBlockWithTransaction(r, opReturn1.HeaderBytes.ToBlockHeader(), tx2)

		// insert headers and proofs
		_, err = tm.insertBtcBlockHeaders([]bbn.BTCHeaderBytes{
			opReturn1.HeaderBytes,
			opReturn2.HeaderBytes,
		})
		require.NoError(t, err)

		_, err = tm.insertSpvProofs(submitterAddr.String(), []*btcctypes.BTCSpvProof{
			opReturn1.SpvProof,
			opReturn2.SpvProof,
		})
		require.NoError(t, err)

		// wait until this checkpoint is submitted
		require.Eventually(t, func() bool {
			ckpt, err := bbnClient.RawCheckpoint(checkpoint.Ckpt.EpochNum)
			if err != nil {
				return false
			}
			return ckpt.RawCheckpoint.Status == ckpttypes.Submitted
		}, eventuallyWaitTimeOut, eventuallyPollTime)
	}

	// insert w BTC headers
	tm.insertWBTCHeaders(t, r)

	// wait until the checkpoint of this epoch is finalised
	require.Eventually(t, func() bool {
		lastFinalizedCkpt, err := bbnClient.LatestEpochFromStatus(ckpttypes.Finalized)
		if err != nil {
			t.Logf("failed to get last finalized epoch: %v", err)
			return false
		}
		return epoch <= lastFinalizedCkpt.RawCheckpoint.EpochNum
	}, eventuallyWaitTimeOut, 1*time.Second)

	t.Logf("epoch %d is finalised", epoch)
}

func (tm *TestManager) insertBtcBlockHeaders(headers []bbn.BTCHeaderBytes) (*babylonclient.RelayerTxResponse, error) {
	msg := &btclctypes.MsgInsertHeaders{
		Signer:  tm.MustGetBabylonSigner(),
		Headers: headers,
	}

	res, err := tm.BabylonClient.ReliablySendMsg(context.Background(), msg, nil, nil)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tm *TestManager) insertSpvProofs(submitter string, proofs []*btcctypes.BTCSpvProof) (*babylonclient.RelayerTxResponse, error) {
	msg := &btcctypes.MsgInsertBTCSpvProof{
		Submitter: submitter,
		Proofs:    proofs,
	}

	res, err := tm.BabylonClient.ReliablySendMsg(context.Background(), msg, nil, nil)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tm *TestManager) insertWBTCHeaders(t *testing.T, r *rand.Rand) {
	ckptParamRes, err := tm.BabylonClient.QueryClient.BTCCheckpointParams()
	require.NoError(t, err)
	btcTipResp, err := tm.BabylonClient.QueryClient.BTCHeaderChainTip()
	require.NoError(t, err)
	tipHeader, err := bbn.NewBTCHeaderBytesFromHex(btcTipResp.Header.HeaderHex)
	require.NoError(t, err)
	kHeaders := datagen.NewBTCHeaderChainFromParentInfo(r, &btclctypes.BTCHeaderInfo{
		Header: &tipHeader,
		Hash:   tipHeader.Hash(),
		Height: btcTipResp.Header.Height,
		Work:   &btcTipResp.Header.Work,
	}, uint32(ckptParamRes.Params.CheckpointFinalizationTimeout))
	_, err = tm.insertBtcBlockHeaders(kHeaders.ChainToBytes())
	require.NoError(t, err)
}

func (tm *TestManager) getFundingTxs(t *testing.T, tx *wire.MsgTx) [][]byte {
	var fundingTxs [][]byte
	for _, txIn := range tx.TxIn {
		rawTx, err := tm.BTCClient.GetRawTransaction(&txIn.PreviousOutPoint.Hash)
		require.NoError(t, err)

		serializedTx, err := bbn.SerializeBTCTx(rawTx.MsgTx())
		require.NoError(t, err)

		fundingTxs = append(fundingTxs, serializedTx)
	}

	return fundingTxs
}

// CreateBTCStakeExpansion creates a stake expansion delegation that references a previous staking transaction
func (tm *TestManager) CreateBTCStakeExpansion(
	t *testing.T,
	fpSK *btcec.PrivateKey,
	stakingValue uint64,
	previousStakingTxHash string,
	prevDelStakingOutputIdx uint32,
) (*wire.MsgTx, *wire.MsgTx, *datagen.TestStakingSlashingInfo, *datagen.TestUnbondingSlashingInfo, *btcec.PrivateKey) {
	signerAddr := tm.BabylonClient.MustGetAddr()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	fpPK := fpSK.PubKey()

	// generate staking tx and slashing tx for expansion
	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)
	covenantBtcPks, err := bbnPksToBtcPks(bsParams.Params.CovenantPks)
	require.NoError(t, err)
	stakingTimeBlocks := bsParams.Params.MaxStakingTimeBlocks

	// get top UTXO for the actual staking output
	topUnspentResult, _, err := tm.BTCClient.GetHighUTXOAndSum()
	require.NoError(t, err)
	stakingUTXO, err := types.NewUTXO(topUnspentResult, regtestParams)
	require.NoError(t, err)

	// get a different UTXO for funding the expansion (fees and potentially additional stake)
	allUnspentResult, err := tm.BTCClient.ListUnspent()
	require.NoError(t, err)
	require.True(t, len(allUnspentResult) >= 2, "Need at least 2 UTXOs for stake expansion")

	var fundingUTXO *types.UTXO
	for _, unspent := range allUnspentResult {
		if unspent.TxID != stakingUTXO.GetOutPoint().Hash.String() {
			fundingUTXO, err = types.NewUTXO(&unspent, regtestParams)
			require.NoError(t, err)
			break
		}
	}
	require.NotNil(t, fundingUTXO, "Could not find a different UTXO for funding")

	// staking value for expansion (should be >= original to not reduce stake)
	additionalStake := uint64(fundingUTXO.Amount) / 4
	totalStakingValue := stakingValue + additionalStake

	// Get the funding transaction
	fundingTxHash := fundingUTXO.GetOutPoint().Hash
	fundingRawTx, err := tm.BTCClient.GetRawTransaction(&fundingTxHash)
	require.NoError(t, err)
	fundingMsgTx := fundingRawTx.MsgTx()

	// generate expansion staking and slashing tx
	stakingMsgTx, stakingSlashingInfo, stakingMsgTxHash := tm.createStakeExpStakingAndSlashingTx(t, fpSK, bsParams, covenantBtcPks, int64(totalStakingValue), stakingTimeBlocks, previousStakingTxHash, prevDelStakingOutputIdx, fundingMsgTx)

	stakingOutIdx, err := outIdx(stakingSlashingInfo.StakingTx, stakingSlashingInfo.StakingInfo.StakingOutput)
	require.NoError(t, err)

	// create PoP
	pop, err := datagen.NewPoPBTC(addr, tm.WalletPrivKey)
	require.NoError(t, err)
	slashingSpendPath, err := stakingSlashingInfo.StakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	// generate proper delegator sig
	delegatorSig, err := stakingSlashingInfo.SlashingTx.Sign(
		stakingMsgTx,
		stakingOutIdx,
		slashingSpendPath.GetPkScriptPath(),
		tm.WalletPrivKey,
	)
	require.NoError(t, err)

	// Generate all data necessary for unbonding
	unbondingSlashingInfo, _, unbondingTxBytes, slashingTxSig := tm.createUnbondingData(
		t,
		[]*btcec.PublicKey{fpPK},
		bsParams,
		covenantBtcPks,
		stakingSlashingInfo,
		stakingMsgTxHash,
		stakingOutIdx,
		stakingTimeBlocks,
		uint16(bsParams.Params.UnbondingTimeBlocks),
	)

	// Serialize the staking transaction
	var stakingTxBuf bytes.Buffer
	err = stakingMsgTx.Serialize(&stakingTxBuf)
	require.NoError(t, err)

	// Get and serialize the separate funding transaction
	fundingTxBytes, err := bbn.SerializeBTCTx(fundingRawTx.MsgTx())
	require.NoError(t, err)

	// submit BTC stake expansion to Babylon
	msgBTCStakeExpansion := &bstypes.MsgBtcStakeExpand{
		StakerAddr:                    signerAddr,
		Pop:                           pop,
		BtcPk:                         bbn.NewBIP340PubKeyFromBTCPK(tm.WalletPrivKey.PubKey()),
		FpBtcPkList:                   []bbn.BIP340PubKey{*bbn.NewBIP340PubKeyFromBTCPK(fpPK)},
		StakingTime:                   stakingTimeBlocks,
		StakingValue:                  int64(totalStakingValue),
		StakingTx:                     stakingTxBuf.Bytes(),
		SlashingTx:                    stakingSlashingInfo.SlashingTx,
		DelegatorSlashingSig:          delegatorSig,
		UnbondingTime:                 tm.getBTCUnbondingTime(t),
		UnbondingTx:                   unbondingTxBytes,
		UnbondingValue:                unbondingSlashingInfo.UnbondingInfo.UnbondingOutput.Value,
		UnbondingSlashingTx:           unbondingSlashingInfo.SlashingTx,
		DelegatorUnbondingSlashingSig: slashingTxSig,
		PreviousStakingTxHash:         previousStakingTxHash,
		FundingTx:                     fundingTxBytes,
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgBTCStakeExpansion, nil, nil)
	require.NoError(t, err)
	t.Logf("submitted MsgBtcStakeExpand for previous staking tx: %s", previousStakingTxHash)

	return stakingMsgTx, fundingMsgTx, stakingSlashingInfo, unbondingSlashingInfo, tm.WalletPrivKey
}

// CreateMultisigBTCStakeExpansion creates a multisig stake expansion delegation that
// references a previous staking transaction
func (tm *TestManager) CreateMultisigBTCStakeExpansion(
	t *testing.T,
	fpSK *btcec.PrivateKey,
	stakingValue uint64,
	previousStakingTxHash string,
	prevDelStakingOutputIdx uint32,
) (*wire.MsgTx, *wire.MsgTx, *datagen.TestStakingSlashingInfo, *datagen.TestUnbondingSlashingInfo, []*btcec.PrivateKey) {
	signerAddr := tm.BabylonClient.MustGetAddr()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	fpPK := fpSK.PubKey()

	// generate staking tx and slashing tx for expansion
	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)
	covenantBtcPks, err := bbnPksToBtcPks(bsParams.Params.CovenantPks)
	require.NoError(t, err)
	stakingTimeBlocks := bsParams.Params.MaxStakingTimeBlocks

	// get top UTXO for the actual staking output
	topUnspentResult, _, err := tm.BTCClient.GetHighUTXOAndSum()
	require.NoError(t, err)
	stakingUTXO, err := types.NewUTXO(topUnspentResult, regtestParams)
	require.NoError(t, err)

	// get a different UTXO for funding the expansion (fees and potentially additional stake)
	allUnspentResult, err := tm.BTCClient.ListUnspent()
	require.NoError(t, err)
	require.True(t, len(allUnspentResult) >= 2, "Need at least 2 UTXOs for stake expansion")

	var fundingUTXO *types.UTXO
	for _, unspent := range allUnspentResult {
		if unspent.TxID != stakingUTXO.GetOutPoint().Hash.String() {
			fundingUTXO, err = types.NewUTXO(&unspent, regtestParams)
			require.NoError(t, err)
			break
		}
	}
	require.NotNil(t, fundingUTXO, "Could not find a different UTXO for funding")

	// staking value for expansion (should be >= original to not reduce stake)
	additionalStake := uint64(fundingUTXO.Amount) / 4
	totalStakingValue := stakingValue + additionalStake

	// Get the funding transaction
	fundingTxHash := fundingUTXO.GetOutPoint().Hash
	fundingRawTx, err := tm.BTCClient.GetRawTransaction(&fundingTxHash)
	require.NoError(t, err)
	fundingMsgTx := fundingRawTx.MsgTx()

	// generate expansion staking and slashing tx
	stakingMsgTx, stakingSlashingInfo, stakingMsgTxHash := tm.createMultisigStakeExpStakingAndSlashingTx(t, fpSK, bsParams, covenantBtcPks, int64(totalStakingValue), stakingTimeBlocks, previousStakingTxHash, prevDelStakingOutputIdx, fundingMsgTx)

	stakingOutIdx, err := outIdx(stakingSlashingInfo.StakingTx, stakingSlashingInfo.StakingInfo.StakingOutput)
	require.NoError(t, err)

	// create PoP
	pop, err := datagen.NewPoPBTC(addr, tm.WalletPrivKey)
	require.NoError(t, err)
	slashingSpendPath, err := stakingSlashingInfo.StakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	// generate proper delegator sig with main staker
	delegatorSig, err := stakingSlashingInfo.SlashingTx.Sign(
		stakingMsgTx,
		stakingOutIdx,
		slashingSpendPath.GetPkScriptPath(),
		tm.WalletPrivKey,
	)
	require.NoError(t, err)

	// generate extra delegator sigs for multisig info
	var delegatorSi []*bstypes.SignatureInfo
	for _, sk := range tm.MultisigStakerPrivKeys {
		sig, err := stakingSlashingInfo.SlashingTx.Sign(
			stakingMsgTx,
			stakingOutIdx,
			slashingSpendPath.GetPkScriptPath(),
			sk,
		)
		require.NoError(t, err)
		delegatorSi = append(delegatorSi, &bstypes.SignatureInfo{
			Pk:  bbn.NewBIP340PubKeyFromBTCPK(sk.PubKey()),
			Sig: sig,
		})
	}

	// Generate all data necessary for unbonding
	unbondingSlashingInfo, _, unbondingTxBytes, slashingTxSig, slashingSi := tm.createMultisigUnbondingData(
		t,
		[]*btcec.PublicKey{fpPK},
		bsParams,
		covenantBtcPks,
		stakingSlashingInfo,
		stakingMsgTxHash,
		stakingOutIdx,
		stakingTimeBlocks,
		uint16(bsParams.Params.UnbondingTimeBlocks),
	)

	// Serialize the staking transaction
	var stakingTxBuf bytes.Buffer
	err = stakingMsgTx.Serialize(&stakingTxBuf)
	require.NoError(t, err)

	// Get and serialize the separate funding transaction
	fundingTxBytes, err := bbn.SerializeBTCTx(fundingRawTx.MsgTx())
	require.NoError(t, err)

	// construct extra staker pks in BIP340PubKey
	var extraStakerPks []bbn.BIP340PubKey
	for _, sk := range tm.MultisigStakerPrivKeys {
		extraStakerPks = append(extraStakerPks, *bbn.NewBIP340PubKeyFromBTCPK(sk.PubKey()))
	}

	// submit BTC stake expansion to Babylon
	msgBTCStakeExpansion := &bstypes.MsgBtcStakeExpand{
		StakerAddr:                    signerAddr,
		Pop:                           pop,
		BtcPk:                         bbn.NewBIP340PubKeyFromBTCPK(tm.WalletPrivKey.PubKey()),
		FpBtcPkList:                   []bbn.BIP340PubKey{*bbn.NewBIP340PubKeyFromBTCPK(fpPK)},
		StakingTime:                   stakingTimeBlocks,
		StakingValue:                  int64(totalStakingValue),
		StakingTx:                     stakingTxBuf.Bytes(),
		SlashingTx:                    stakingSlashingInfo.SlashingTx,
		DelegatorSlashingSig:          delegatorSig,
		UnbondingTime:                 tm.getBTCUnbondingTime(t),
		UnbondingTx:                   unbondingTxBytes,
		UnbondingValue:                unbondingSlashingInfo.UnbondingInfo.UnbondingOutput.Value,
		UnbondingSlashingTx:           unbondingSlashingInfo.SlashingTx,
		DelegatorUnbondingSlashingSig: slashingTxSig,
		PreviousStakingTxHash:         previousStakingTxHash,
		FundingTx:                     fundingTxBytes,
		MultisigInfo: &bstypes.AdditionalStakerInfo{
			StakerBtcPkList:                extraStakerPks,
			StakerQuorum:                   tm.MultisigStakerQuorum,
			DelegatorSlashingSigs:          delegatorSi,
			DelegatorUnbondingSlashingSigs: slashingSi,
		},
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgBTCStakeExpansion, nil, nil)
	require.NoError(t, err)
	t.Logf("submitted MsgBtcStakeExpand for previous staking tx: %s", previousStakingTxHash)

	return stakingMsgTx, fundingMsgTx, stakingSlashingInfo, unbondingSlashingInfo, tm.stakerPrivKeys()
}

// AddWitnessToStakeExpTx adds witness to stake expansion transaction
func (tm *TestManager) AddWitnessToStakeExpTx(
	t *testing.T,
	stakingOutput *wire.TxOut,
	fundingOutput *wire.TxOut,
	stakerSk *btcec.PrivateKey,
	covenantSks []*btcec.PrivateKey,
	covenantQuorum uint32,
	finalityProviderPKs []*btcec.PublicKey,
	stakingTime uint16,
	stakingValue int64,
	spendingTx *wire.MsgTx, // this is the stake expansion transaction
	net *chaincfg.Params,
) *wire.MsgTx {
	var covenantPks []*btcec.PublicKey
	for _, sk := range covenantSks {
		covenantPks = append(covenantPks, sk.PubKey())
	}

	stakingInfo, err := btcstaking.BuildStakingInfo(
		stakerSk.PubKey(),
		finalityProviderPKs,
		covenantPks,
		covenantQuorum,
		stakingTime,
		btcutil.Amount(stakingValue),
		net,
	)
	require.NoError(t, err)

	// sanity check that what we re-build is the same as what we have in the BTC delegation
	require.Equal(t, stakingOutput, stakingInfo.StakingOutput)

	unbondingSpendInfo, err := stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	unbondingScript := unbondingSpendInfo.RevealedLeaf.Script
	require.NotNil(t, unbondingScript)

	covenantSigs := tm.GenerateSignaturesForStakeExpansion(
		t,
		covenantSks,
		spendingTx,
		stakingOutput,
		fundingOutput,
		unbondingSpendInfo.RevealedLeaf,
	)
	require.NoError(t, err)

	stakerSig, err := btcstaking.SignTxForFirstScriptSpendWithTwoInputsFromTapLeaf(
		spendingTx,
		stakingOutput,
		fundingOutput,
		stakerSk,
		unbondingSpendInfo.RevealedLeaf,
	)
	require.NoError(t, err)

	ubWitness, err := unbondingSpendInfo.CreateUnbondingPathWitness(covenantSigs, stakerSig)
	require.NoError(t, err)

	spendingTx.TxIn[0].Witness = ubWitness

	return spendingTx
}

// GenerateSignaturesForStakeExpansion generates covenant signatures for stake expansion
func (tm *TestManager) GenerateSignaturesForStakeExpansion(
	t *testing.T,
	keys []*btcec.PrivateKey,
	tx *wire.MsgTx,
	stakingOutput *wire.TxOut,
	fundingOutput *wire.TxOut,
	leaf txscript.TapLeaf,
) []*schnorr.Signature {
	// Create a map to hold signatures by public key for sorting
	sigMap := make(map[string]*schnorr.Signature)
	var pubKeys []*btcec.PublicKey

	for _, key := range keys {
		pubKey := key.PubKey()
		sig, err := btcstaking.SignTxForFirstScriptSpendWithTwoInputsFromTapLeaf(
			tx,
			stakingOutput,
			fundingOutput,
			key,
			leaf,
		)
		require.NoError(t, err)

		// Use compressed public key as map key for sorting
		pubKeyStr := string(pubKey.SerializeCompressed())
		sigMap[pubKeyStr] = sig
		pubKeys = append(pubKeys, pubKey)
	}

	// Sort public keys by their serialized bytes
	for i := 0; i < len(pubKeys)-1; i++ {
		for j := i + 1; j < len(pubKeys); j++ {
			if bytes.Compare(pubKeys[i].SerializeCompressed(), pubKeys[j].SerializeCompressed()) > 0 {
				pubKeys[i], pubKeys[j] = pubKeys[j], pubKeys[i]
			}
		}
	}

	// Create sorted signatures array
	var sigs []*schnorr.Signature = make([]*schnorr.Signature, len(pubKeys))
	for i, pubKey := range pubKeys {
		pubKeyStr := string(pubKey.SerializeCompressed())
		sigs[i] = sigMap[pubKeyStr]
	}

	return sigs
}
