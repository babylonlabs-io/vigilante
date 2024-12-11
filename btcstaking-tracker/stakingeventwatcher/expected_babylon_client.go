package stakingeventwatcher

import (
	"context"
	sdkerrors "cosmossdk.io/errors"
	"fmt"
	"github.com/avast/retry-go/v4"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/cosmos/cosmos-sdk/client"
	"strings"
	"time"

	"errors"
	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	bbn "github.com/babylonlabs-io/babylon/types"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/cosmos/cosmos-sdk/types/query"
)

var (
	ErrHeaderNotKnownToBabylon       = errors.New("btc header not known to babylon")
	ErrHeaderOnBabylonLCFork         = errors.New("btc header is on babylon btc light client fork")
	ErrBabylonBtcLightClientNotReady = errors.New("babylon btc light client is not ready to receive delegation")
)

type Delegation struct {
	StakingTx             *wire.MsgTx
	StakingOutputIdx      uint32
	DelegationStartHeight uint32
	UnbondingOutput       *wire.TxOut
	HasProof              bool
}

type BabylonParams struct {
	ConfirmationTimeBlocks uint32 // K-deep
}

type BabylonNodeAdapter interface {
	DelegationsByStatus(status btcstakingtypes.BTCDelegationStatus, offset uint64, limit uint64) ([]Delegation, error)
	IsDelegationActive(stakingTxHash chainhash.Hash) (bool, error)
	IsDelegationVerified(stakingTxHash chainhash.Hash) (bool, error)
	ReportUnbonding(ctx context.Context, stakingTxHash chainhash.Hash, stakeSpendingTx *wire.MsgTx,
		inclusionProof *btcstakingtypes.InclusionProof) error
	BtcClientTipHeight() (uint32, error)
	ActivateDelegation(ctx context.Context, stakingTxHash chainhash.Hash, proof *btcctypes.BTCSpvProof) error
	QueryHeaderDepth(headerHash *chainhash.Hash) (uint32, error)
	Params() (*BabylonParams, error)
}

type BabylonClientAdapter struct {
	babylonClient *bbnclient.Client
	cfg           *config.BTCStakingTrackerConfig
}

var _ BabylonNodeAdapter = (*BabylonClientAdapter)(nil)

func NewBabylonClientAdapter(babylonClient *bbnclient.Client, cfg *config.BTCStakingTrackerConfig) *BabylonClientAdapter {
	return &BabylonClientAdapter{
		babylonClient: babylonClient,
		cfg:           cfg,
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
	stakeSpendingTx *wire.MsgTx,
	inclusionProof *btcstakingtypes.InclusionProof) error {
	signer := bca.babylonClient.MustGetAddr()

	stakeSpendingBytes, err := bbn.SerializeBTCTx(stakeSpendingTx)
	if err != nil {
		return err
	}

	msg := btcstakingtypes.MsgBTCUndelegate{
		Signer:                        signer,
		StakingTxHash:                 stakingTxHash.String(),
		StakeSpendingTx:               stakeSpendingBytes,
		StakeSpendingTxInclusionProof: inclusionProof,
	}

	resp, err := bca.babylonClient.ReliablySendMsg(ctx, &msg, []*sdkerrors.Error{}, []*sdkerrors.Error{})
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

	return resp.Header.Height, nil
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

	resp, err := bca.babylonClient.ReliablySendMsg(ctx, &msg, []*sdkerrors.Error{}, []*sdkerrors.Error{})

	if err != nil && resp != nil {
		return fmt.Errorf("msg MsgAddBTCDelegationInclusionProof failed exeuction with code %d and error %w", resp.Code, err)
	}

	if err != nil {
		return fmt.Errorf("failed to report unbonding: %w", err)
	}

	return nil
}

func (bca *BabylonClientAdapter) QueryHeaderDepth(headerHash *chainhash.Hash) (uint32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clientCtx := client.Context{Client: bca.babylonClient.RPCClient}
	queryClient := btclctypes.NewQueryClient(clientCtx)

	var response *btclctypes.QueryHeaderDepthResponse
	if err := retry.Do(func() error {
		depthResponse, err := queryClient.HeaderDepth(ctx, &btclctypes.QueryHeaderDepthRequest{Hash: headerHash.String()})
		if err != nil {
			return err
		}
		response = depthResponse

		return nil
	},
		retry.Attempts(5),
		retry.MaxDelay(bca.cfg.RetrySubmitUnbondingTxInterval),
		retry.MaxJitter(bca.cfg.RetryJitter),
		retry.LastErrorOnly(true)); err != nil {
		if strings.Contains(err.Error(), btclctypes.ErrHeaderDoesNotExist.Error()) {
			return 0, fmt.Errorf("%s: %w", err.Error(), ErrHeaderNotKnownToBabylon)
		}

		return 0, err
	}

	return response.Depth, nil
}

func (bca *BabylonClientAdapter) Params() (*BabylonParams, error) {
	var bccParams *btcctypes.Params
	if err := retry.Do(func() error {
		response, err := bca.babylonClient.BTCCheckpointParams()
		if err != nil {
			return err
		}
		bccParams = &response.Params

		return nil
	}, retry.Attempts(5),
		retry.MaxDelay(bca.cfg.RetrySubmitUnbondingTxInterval),
		retry.MaxJitter(bca.cfg.RetryJitter),
		retry.LastErrorOnly(true)); err != nil {
		return nil, err
	}

	return &BabylonParams{ConfirmationTimeBlocks: bccParams.BtcConfirmationDepth}, nil
}
