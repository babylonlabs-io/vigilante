package monitor

import (
	"fmt"
	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	bbn "github.com/babylonlabs-io/babylon/v3/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/btcsuite/btcd/wire"
	"github.com/cosmos/cosmos-sdk/types/query"
)

type Delegation struct {
	StakingTx             *wire.MsgTx
	StakingOutputIdx      uint32
	DelegationStartHeight uint32
	UnbondingOutput       *wire.TxOut
	HasProof              bool
	Status                string
}

type BabylonAdaptorClient interface {
	DelegationsByStatus(status btcstakingtypes.BTCDelegationStatus, cursor []byte, limit uint64) ([]Delegation, []byte, error)
	BTCDelegation(stakingTxHashHex string) (*Delegation, error)
	GetConfirmationDepth() (uint32, error)
}

type BabylonAdaptorClientAdapter struct {
	babylonClient *bbnclient.Client
	cfg           *config.BTCStakingTrackerConfig
}

func NewBabylonAdaptorClientAdapter(babylonClient *bbnclient.Client, cfg *config.BTCStakingTrackerConfig) *BabylonAdaptorClientAdapter {
	return &BabylonAdaptorClientAdapter{
		babylonClient: babylonClient,
		cfg:           cfg,
	}
}

func (bc *BabylonAdaptorClientAdapter) GetConfirmationDepth() (uint32, error) {
	params, err := bc.babylonClient.BTCCheckpointParams()
	if err != nil {
		return 0, fmt.Errorf("failed to get babylon params: %w", err)
	}

	return params.Params.BtcConfirmationDepth, nil
}

func (bc *BabylonAdaptorClientAdapter) DelegationsByStatus(status btcstakingtypes.BTCDelegationStatus, cursor []byte, limit uint64) ([]Delegation, []byte, error) {
	resp, err := bc.babylonClient.BTCDelegations(status, &query.PageRequest{
		Key:   cursor,
		Limit: limit,
	})
	if err != nil {
		return nil, nil, err
	}

	delegations := make([]Delegation, len(resp.BtcDelegations))

	for i, delegation := range resp.BtcDelegations {
		var unbondingOutput *wire.TxOut
		if delegation.UndelegationResponse != nil {
			unbondingTx, _, err := bbn.NewBTCTxFromHex(delegation.UndelegationResponse.UnbondingTxHex)
			if err != nil {
				return nil, nil, fmt.Errorf("error converting unbonding tx: %w", err)
			}
			unbondingOutput = unbondingTx.TxOut[0]
		}

		stakingTx, _, err := bbn.NewBTCTxFromHex(delegation.StakingTxHex)
		if err != nil {
			return nil, nil,
				fmt.Errorf("error converting btc staking tx from hex")
		}

		delegations[i] = Delegation{
			StakingTx:             stakingTx,
			StakingOutputIdx:      delegation.StakingOutputIdx,
			DelegationStartHeight: delegation.StartHeight,
			UnbondingOutput:       unbondingOutput,
			HasProof:              delegation.StartHeight > 0,
			Status:                delegation.StatusDesc,
		}
	}

	var nextCursor []byte
	if resp.Pagination != nil && resp.Pagination.NextKey != nil {
		nextCursor = resp.Pagination.NextKey
	}

	return delegations, nextCursor, nil
}

func (bc *BabylonAdaptorClientAdapter) BTCDelegation(stakingTxHashHex string) (*Delegation, error) {
	resp, err := bc.babylonClient.BTCDelegation(stakingTxHashHex)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve delegation: %w", err)
	}

	stakingTx, _, err := bbn.NewBTCTxFromHex(resp.BtcDelegation.StakingTxHex)
	if err != nil {
		return nil, err
	}

	var unbondingOutput *wire.TxOut
	if resp.BtcDelegation.UndelegationResponse != nil {
		unbondingTx, _, err := bbn.NewBTCTxFromHex(resp.BtcDelegation.UndelegationResponse.UnbondingTxHex)
		if err != nil {
			return nil, fmt.Errorf("error converting unbonding tx: %w", err)
		}
		unbondingOutput = unbondingTx.TxOut[0]
	}

	return &Delegation{
		StakingTx:             stakingTx,
		StakingOutputIdx:      resp.BtcDelegation.StakingOutputIdx,
		DelegationStartHeight: resp.BtcDelegation.StartHeight,
		UnbondingOutput:       unbondingOutput,
		HasProof:              resp.BtcDelegation.StartHeight > 0,
		Status:                resp.BtcDelegation.StatusDesc,
	}, nil
}
