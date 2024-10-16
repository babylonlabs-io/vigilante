package stakingeventwatcher

import (
	"context"
	"fmt"

	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"

	"cosmossdk.io/errors"
	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	bbn "github.com/babylonlabs-io/babylon/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/cosmos/cosmos-sdk/types/query"
)

type Delegation struct {
	StakingTx             *wire.MsgTx
	StakingOutputIdx      uint32
	DelegationStartHeight uint32
	UnbondingOutput       *wire.TxOut
	HasProof              bool
}

type BabylonNodeAdapter interface {
	DelegationsByStatus(status btcstakingtypes.BTCDelegationStatus, offset uint64, limit uint64) ([]Delegation, error)
	IsDelegationActive(stakingTxHash chainhash.Hash) (bool, error)
	IsDelegationVerified(stakingTxHash chainhash.Hash) (bool, error)
	ReportUnbonding(ctx context.Context, stakingTxHash chainhash.Hash, stakerUnbondingSig *schnorr.Signature) error
	BtcClientTipHeight() (uint32, error)
	ActivateDelegation(ctx context.Context, stakingTxHash chainhash.Hash, proof *btcctypes.BTCSpvProof) error
}

type BabylonClientAdapter struct {
	babylonClient *bbnclient.Client
}

var _ BabylonNodeAdapter = (*BabylonClientAdapter)(nil)

func NewBabylonClientAdapter(babylonClient *bbnclient.Client) *BabylonClientAdapter {
	return &BabylonClientAdapter{
		babylonClient: babylonClient,
	}
}

// DelegationsByStatus - returns btc delegations by status
func (bca *BabylonClientAdapter) DelegationsByStatus(
	status btcstakingtypes.BTCDelegationStatus, offset uint64, limit uint64) ([]Delegation, error) {
	resp, err := bca.babylonClient.BTCDelegations(
		status,
		&query.PageRequest{
			Key:    nil,
			Offset: offset,
			Limit:  limit,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve delegations from babylon: %w", err)
	}

	delegations := make([]Delegation, len(resp.BtcDelegations))

	for i, delegation := range resp.BtcDelegations {
		stakingTx, _, err := bbn.NewBTCTxFromHex(delegation.StakingTxHex)
		if err != nil {
			return nil, err
		}

		unbondingTx, _, err := bbn.NewBTCTxFromHex(delegation.UndelegationResponse.UnbondingTxHex)
		if err != nil {
			return nil, err
		}

		delegations[i] = Delegation{
			StakingTx:             stakingTx,
			StakingOutputIdx:      delegation.StakingOutputIdx,
			DelegationStartHeight: delegation.StartHeight,
			UnbondingOutput:       unbondingTx.TxOut[0],
			HasProof:              delegation.StartHeight > 0,
		}
	}

	return delegations, nil
}

// IsDelegationActive method for BabylonClientAdapter
func (bca *BabylonClientAdapter) IsDelegationActive(stakingTxHash chainhash.Hash) (bool, error) {
	resp, err := bca.babylonClient.BTCDelegation(stakingTxHash.String())

	if err != nil {
		return false, fmt.Errorf("failed to retrieve delegation from babyln: %w", err)
	}

	return resp.BtcDelegation.Active, nil
}

// IsDelegationVerified method for BabylonClientAdapter checks if delegation is in status verified
func (bca *BabylonClientAdapter) IsDelegationVerified(stakingTxHash chainhash.Hash) (bool, error) {
	resp, err := bca.babylonClient.BTCDelegation(stakingTxHash.String())

	if err != nil {
		return false, fmt.Errorf("failed to retrieve delegation from babyln: %w", err)
	}

	return resp.BtcDelegation.StatusDesc == btcstakingtypes.BTCDelegationStatus_VERIFIED.String(), nil
}

// ReportUnbonding method for BabylonClientAdapter
func (bca *BabylonClientAdapter) ReportUnbonding(
	ctx context.Context,
	stakingTxHash chainhash.Hash,
	stakerUnbondingSig *schnorr.Signature) error {
	signer := bca.babylonClient.MustGetAddr()

	msg := btcstakingtypes.MsgBTCUndelegate{
		Signer:         signer,
		StakingTxHash:  stakingTxHash.String(),
		UnbondingTxSig: bbn.NewBIP340SignatureFromBTCSig(stakerUnbondingSig),
	}

	resp, err := bca.babylonClient.ReliablySendMsg(ctx, &msg, []*errors.Error{}, []*errors.Error{})

	if err != nil && resp != nil {
		return fmt.Errorf("msg MsgBTCUndelegate failed exeuction with code %d and error %w", resp.Code, err)
	}

	if err != nil {
		return fmt.Errorf("failed to report unbonding: %w", err)
	}

	return nil
}

// BtcClientTipHeight method for BabylonClientAdapter
func (bca *BabylonClientAdapter) BtcClientTipHeight() (uint32, error) {
	resp, err := bca.babylonClient.BTCHeaderChainTip()

	if err != nil {
		return 0, fmt.Errorf("failed to retrieve btc light client tip from babyln: %w", err)
	}

	return uint32(resp.Header.Height), nil
}

// ActivateDelegation provides inclusion proof to activate delegation
func (bca *BabylonClientAdapter) ActivateDelegation(
	ctx context.Context, stakingTxHash chainhash.Hash, proof *btcctypes.BTCSpvProof) error {
	signer := bca.babylonClient.MustGetAddr()

	msg := btcstakingtypes.MsgAddBTCDelegationInclusionProof{
		Signer:                  signer,
		StakingTxHash:           stakingTxHash.String(),
		StakingTxInclusionProof: btcstakingtypes.NewInclusionProofFromSpvProof(proof),
	}

	resp, err := bca.babylonClient.ReliablySendMsg(ctx, &msg, []*errors.Error{}, []*errors.Error{})

	if err != nil && resp != nil {
		return fmt.Errorf("msg MsgAddBTCDelegationInclusionProof failed exeuction with code %d and error %w", resp.Code, err)
	}

	if err != nil {
		return fmt.Errorf("failed to report unbonding: %w", err)
	}

	return nil
}
