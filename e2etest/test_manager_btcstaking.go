package e2etest

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/babylonlabs-io/babylon/client/babylonclient"
	"math/rand"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon/btcstaking"
	txformat "github.com/babylonlabs-io/babylon/btctxformatter"
	"github.com/babylonlabs-io/babylon/crypto/eots"
	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/types"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	ckpttypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	ftypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
)

var (
	r = rand.New(rand.NewSource(time.Now().Unix()))

	// covenant
	covenantSk, _ = btcec.PrivKeyFromBytes(
		[]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
	)
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
	btcFp, err := datagen.GenRandomFinalityProviderWithBTCBabylonSKs(r, fpSK, addr)
	require.NoError(t, err)

	/*
		create finality provider
	*/
	commission := sdkmath.LegacyZeroDec()
	msgNewVal := &bstypes.MsgCreateFinalityProvider{
		Addr:        signerAddr,
		Description: &stakingtypes.Description{Moniker: datagen.GenRandomHexStr(r, 10)},
		Commission:  &commission,
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
	stakingMsgTx, stakingSlashingInfo, stakingMsgTxHash := tm.createStakingAndSlashingTx(t, fpSK, bsParams, covenantBtcPks, topUTXO, stakingValue, stakingTimeBlocks)

	// send staking tx to Bitcoin node's mempool
	_, err = tm.BTCClient.SendRawTransaction(stakingMsgTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{stakingMsgTxHash})) == 1
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
	pop, err := bstypes.NewPoPBTC(addr, tm.WalletPrivKey)
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
		fpPK,
		bsParams,
		covenantBtcPks,
		stakingSlashingInfo,
		stakingMsgTxHash,
		stakingOutIdx,
		stakingTimeBlocks,
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
		fpSK, slashingSpendPath,
		stakingSlashingInfo,
		unbondingSlashingInfo,
		unbondingSlashingPathSpendInfo,
		stakingOutIdx,
	)

	return stakingSlashingInfo, unbondingSlashingInfo, tm.WalletPrivKey
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
	stakingMsgTx, stakingSlashingInfo, stakingMsgTxHash := tm.createStakingAndSlashingTx(t, fpSK, bsParams, covenantBtcPks, topUTXO, stakingValue, stakingTimeBlocks)

	stakingOutIdx, err := outIdx(stakingSlashingInfo.StakingTx, stakingSlashingInfo.StakingInfo.StakingOutput)
	require.NoError(t, err)

	// create PoP
	pop, err := bstypes.NewPoPBTC(addr, tm.WalletPrivKey)
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
		fpPK,
		bsParams,
		covenantBtcPks,
		stakingSlashingInfo,
		stakingMsgTxHash,
		stakingOutIdx,
		stakingTimeBlocks,
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
		fpSK, slashingSpendPath,
		stakingSlashingInfo,
		unbondingSlashingInfo,
		unbondingSlashingPathSpendInfo,
		stakingOutIdx,
	)

	return stakingMsgTx, stakingSlashingInfo, unbondingSlashingInfo, tm.WalletPrivKey
}

func (tm *TestManager) createStakingAndSlashingTx(
	t *testing.T, fpSK *btcec.PrivateKey,
	bsParams *bstypes.QueryParamsResponse,
	covenantBtcPks []*btcec.PublicKey,
	topUTXO *types.UTXO,
	stakingValue int64,
	stakingTimeBlocks uint32,
) (*wire.MsgTx, *datagen.TestStakingSlashingInfo, *chainhash.Hash) {
	// generate staking tx and slashing tx
	fpPK := fpSK.PubKey()

	// generate legitimate BTC del
	stakingSlashingInfo := datagen.GenBTCStakingSlashingInfoWithOutPoint(
		r,
		t,
		regtestParams,
		topUTXO.GetOutPoint(),
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

	return stakingMsgTx, stakingSlashingInfo, stakingMsgTxHash
}

func (tm *TestManager) createUnbondingData(
	t *testing.T,
	fpPK *btcec.PublicKey,
	bsParams *bstypes.QueryParamsResponse,
	covenantBtcPks []*btcec.PublicKey,
	stakingSlashingInfo *datagen.TestStakingSlashingInfo,
	stakingMsgTxHash *chainhash.Hash,
	stakingOutIdx uint32,
	stakingTimeBlocks uint32,
) (*datagen.TestUnbondingSlashingInfo, *btcstaking.SpendInfo, []byte, *bbn.BIP340Signature) {
	fee := int64(1000)
	unbondingValue := stakingSlashingInfo.StakingInfo.StakingOutput.Value - fee
	unbondingSlashingInfo := datagen.GenBTCUnbondingSlashingInfo(
		r,
		t,
		regtestParams,
		tm.WalletPrivKey,
		[]*btcec.PublicKey{fpPK},
		covenantBtcPks,
		bsParams.Params.CovenantQuorum,
		wire.NewOutPoint(stakingMsgTxHash, stakingOutIdx),
		uint16(stakingTimeBlocks),
		unbondingValue,
		bsParams.Params.SlashingPkScript,
		bsParams.Params.SlashingRate,
		uint16(tm.getBTCUnbondingTime(t)),
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

func (tm *TestManager) addCovenantSig(
	t *testing.T,
	signerAddr string,
	stakingMsgTx *wire.MsgTx,
	stakingMsgTxHash *chainhash.Hash,
	fpSK *btcec.PrivateKey,
	slashingSpendPath *btcstaking.SpendInfo,
	stakingSlashingInfo *datagen.TestStakingSlashingInfo,
	unbondingSlashingInfo *datagen.TestUnbondingSlashingInfo,
	unbondingSlashingPathSpendInfo *btcstaking.SpendInfo,
	stakingOutIdx uint32,
) {
	// TODO: Make this handle multiple covenant signatures
	fpEncKey, err := asig.NewEncryptionKeyFromBTCPK(fpSK.PubKey())
	require.NoError(t, err)
	covenantSig, err := stakingSlashingInfo.SlashingTx.EncSign(
		stakingMsgTx,
		stakingOutIdx,
		slashingSpendPath.GetPkScriptPath(),
		covenantSk,
		fpEncKey,
	)
	require.NoError(t, err)
	// TODO: Add covenant sigs for all covenants
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
	covenantSlashingSig, err := unbondingSlashingInfo.SlashingTx.EncSign(
		unbondingSlashingInfo.UnbondingTx,
		0, // Only one output in the unbonding transaction
		unbondingSlashingPathSpendInfo.GetPkScriptPath(),
		covenantSk,
		fpEncKey,
	)
	require.NoError(t, err)
	msgAddCovenantSig := &bstypes.MsgAddCovenantSigs{
		Signer:                  signerAddr,
		Pk:                      bbn.NewBIP340PubKeyFromBTCPK(covenantSk.PubKey()),
		StakingTxHash:           stakingMsgTxHash.String(),
		SlashingTxSigs:          [][]byte{covenantSig.MustMarshal()},
		UnbondingTxSig:          covenantUnbondingSig,
		SlashingUnbondingTxSigs: [][]byte{covenantSlashingSig.MustMarshal()},
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msgAddCovenantSig, nil, nil)
	require.NoError(t, err)
	t.Logf("submitted covenant signature")
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

	var unbondingTxBuf bytes.Buffer
	err = unbondingSlashingInfo.UnbondingTx.Serialize(&unbondingTxBuf)
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
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{unbondingTxHash})) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(mBlock.Transactions))

	catchUpLightClientFunc()

	unbondingTxInfo := getTxInfo(t, mBlock)
	msgUndel := &bstypes.MsgBTCUndelegate{
		Signer:          signerAddr,
		StakingTxHash:   stakingSlashingInfo.StakingTx.TxHash().String(),
		StakeSpendingTx: unbondingTxBuf.Bytes(),
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
	msgToSign := append(sdk.Uint64ToBigEndian(activatedHeight), blockToVote.Block.AppHash...)
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
	invalidMsgToSign := append(sdk.Uint64ToBigEndian(activatedHeight), invalidAppHash...)
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
