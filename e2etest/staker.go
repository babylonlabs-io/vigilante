package e2etest

import (
	"bytes"
	"context"
	"github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/types"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"testing"
)

type Staker struct {
	stakingMsgTx                   *wire.MsgTx
	stakingSlashingInfo            *datagen.TestStakingSlashingInfo
	stakingMsgTxHash               *chainhash.Hash
	unbondingSlashingInfo          *datagen.TestUnbondingSlashingInfo
	unbondingSlashingPathSpendInfo *btcstaking.SpendInfo
	slashingTxSig                  *bbn.BIP340Signature
	unbondingTxBytes               []byte
	stakingValue                   int64
	stakingTimeBlocks              uint32
	stakingTxInfo                  *btcctypes.TransactionInfo
	delegatorSig                   *bbn.BIP340Signature
	pop                            *bstypes.ProofOfPossessionBTC
	stakingOutIdx                  uint32
	slashingSpendPath              *btcstaking.SpendInfo
}

func (s *Staker) CreateStakingTx(
	t *testing.T,
	tm *TestManager,
	fpSK *btcec.PrivateKey,
	topUTXO *types.UTXO,
	addr sdk.AccAddress) {
	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)

	stakingValue := int64(topUTXO.Amount) / 3
	stakingTimeBlocks := bsParams.Params.MaxStakingTimeBlocks

	covenantBtcPks, err := bbnPksToBtcPks(bsParams.Params.CovenantPks)
	require.NoError(t, err)

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

	s.stakingMsgTx = stakingMsgTx
	s.stakingSlashingInfo = stakingSlashingInfo
	s.stakingMsgTxHash = stakingMsgTxHash
	s.stakingTimeBlocks = stakingTimeBlocks
	s.delegatorSig = delegatorSig
	s.pop = pop
	s.stakingOutIdx = stakingOutIdx
	s.slashingSpendPath = slashingSpendPath
}

func (s *Staker) CreateUnbondingData(
	t *testing.T,
	tm *TestManager,
	fpSK *btcec.PrivateKey) {
	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)
	covenantBtcPks, err := bbnPksToBtcPks(bsParams.Params.CovenantPks)
	require.NoError(t, err)

	unbondingSlashingInfo, unbondingSlashingPathSpendInfo, unbondingTxBytes, slashingTxSig := tm.createUnbondingData(
		t,
		fpSK.PubKey(),
		bsParams,
		covenantBtcPks,
		s.stakingSlashingInfo,
		s.stakingMsgTxHash,
		s.stakingOutIdx,
		s.stakingTimeBlocks,
	)

	s.unbondingSlashingInfo = unbondingSlashingInfo
	s.unbondingSlashingPathSpendInfo = unbondingSlashingPathSpendInfo
	s.unbondingTxBytes = unbondingTxBytes
	s.slashingTxSig = slashingTxSig
}

func (s *Staker) PrepareUnbondingTx(
	t *testing.T,
	tm *TestManager,
) {
	unbondingPathSpendInfo, err := s.stakingSlashingInfo.StakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)
	stakingOutIdx, err := outIdx(s.unbondingSlashingInfo.UnbondingTx, s.unbondingSlashingInfo.UnbondingInfo.UnbondingOutput)
	require.NoError(t, err)

	delSK := tm.WalletPrivKey
	unbondingTxSchnorrSig, err := btcstaking.SignTxWithOneScriptSpendInputStrict(
		s.unbondingSlashingInfo.UnbondingTx,
		s.stakingSlashingInfo.StakingTx,
		stakingOutIdx,
		unbondingPathSpendInfo.GetPkScriptPath(),
		delSK,
	)
	require.NoError(t, err)

	resp, err := tm.BabylonClient.BTCDelegation(s.stakingSlashingInfo.StakingTx.TxHash().String())
	require.NoError(t, err)

	covenantSigs := resp.BtcDelegation.UndelegationResponse.CovenantUnbondingSigList
	witness, err := unbondingPathSpendInfo.CreateUnbondingPathWitness(
		[]*schnorr.Signature{covenantSigs[0].Sig.MustToBTCSig()},
		unbondingTxSchnorrSig,
	)
	s.unbondingSlashingInfo.UnbondingTx.TxIn[0].Witness = witness
}

func (s *Staker) SendTxAndWait(
	t *testing.T,
	tm *TestManager,
	tx *wire.MsgTx) {
	// send tx to Bitcoin node's mempool
	_, err := tm.BTCClient.SendRawTransaction(tx, true)
	require.NoError(t, err)

	hash := tx.TxHash()
	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&hash})) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func getTxInfoByHash(t *testing.T, hash *chainhash.Hash, block *wire.MsgBlock) *btcctypes.TransactionInfo {
	mHeaderBytes := bbn.NewBTCHeaderBytesFromBlockHeader(&block.Header)
	var txBytes [][]byte
	txIndex := 0
	for i, tx := range block.Transactions {
		buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
		_ = tx.Serialize(buf)
		txBytes = append(txBytes, buf.Bytes())
		h := tx.TxHash()
		if hash.IsEqual(&h) {
			txIndex = i
		}
	}
	spvProof, err := btcctypes.SpvProofFromHeaderAndTransactions(&mHeaderBytes, txBytes, uint(txIndex))
	require.NoError(t, err)
	return btcctypes.NewTransactionInfoFromSpvProof(spvProof)
}

func (s *Staker) SendDelegation(t *testing.T,
	tm *TestManager,
	signerAddr string,
	fpPK *btcec.PublicKey) {
	require.NotNil(t, s.stakingSlashingInfo)

	msgBTCDel := &bstypes.MsgCreateBTCDelegation{
		StakerAddr:   signerAddr,
		Pop:          s.pop,
		BtcPk:        bbn.NewBIP340PubKeyFromBTCPK(tm.WalletPrivKey.PubKey()),
		FpBtcPkList:  []bbn.BIP340PubKey{*bbn.NewBIP340PubKeyFromBTCPK(fpPK)},
		StakingTime:  s.stakingTimeBlocks,
		StakingValue: s.stakingValue,
		StakingTx:    s.stakingTxInfo.Transaction,
		StakingTxInclusionProof: &bstypes.InclusionProof{
			Key:   s.stakingTxInfo.Key,
			Proof: s.stakingTxInfo.Proof,
		},
		SlashingTx:           s.stakingSlashingInfo.SlashingTx,
		DelegatorSlashingSig: s.delegatorSig,
		// Unbonding related data
		UnbondingTime:                 tm.getBTCUnbondingTime(t),
		UnbondingTx:                   s.unbondingTxBytes,
		UnbondingValue:                s.unbondingSlashingInfo.UnbondingInfo.UnbondingOutput.Value,
		UnbondingSlashingTx:           s.unbondingSlashingInfo.SlashingTx,
		DelegatorUnbondingSlashingSig: s.slashingTxSig,
	}
	_, err := tm.BabylonClient.ReliablySendMsg(context.Background(), msgBTCDel, nil, nil)
	require.NoError(t, err)
	t.Logf("submitted MsgCreateBTCDelegation")
}

// AddCov generate and insert new covenant signature, to activate the BTC delegation
func (s *Staker) AddCov(t *testing.T,
	tm *TestManager, signerAddr string, fpSK *btcec.PrivateKey) {
	tm.addCovenantSig(
		t,
		signerAddr,
		s.stakingMsgTx,
		s.stakingMsgTxHash,
		fpSK,
		s.slashingSpendPath,
		s.stakingSlashingInfo,
		s.unbondingSlashingInfo,
		s.unbondingSlashingPathSpendInfo,
		s.stakingOutIdx,
	)
}
