package stakingeventwatcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdkerrors "cosmossdk.io/errors"
	"github.com/avast/retry-go/v4"
	btclctypes "github.com/babylonlabs-io/babylon/v3/x/btclightclient/types"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/cosmos/cosmos-sdk/client"

	"errors"

	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	bbn "github.com/babylonlabs-io/babylon/v3/types"
	btcctypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/cosmos/cosmos-sdk/types/query"
)

const stakingTxHashKey = "staking_tx_hash"

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
	Status                string
	IsStakeExpansion      bool
}

type BabylonParams struct {
	ConfirmationTimeBlocks uint32 // K-deep
}

type BabylonNodeAdapter interface {
	DelegationsByStatus(status btcstakingtypes.BTCDelegationStatus, cursor []byte, limit uint64) ([]Delegation, []byte, error)
	IsDelegationActive(stakingTxHash chainhash.Hash) (bool, error)
	IsDelegationVerified(stakingTxHash chainhash.Hash) (bool, error)
	ReportUnbonding(ctx context.Context, stakingTxHash chainhash.Hash, stakeSpendingTx *wire.MsgTx,
		inclusionProof *btcstakingtypes.InclusionProof, fundingTxs [][]byte) error
	BtcClientTipHeight() (uint32, error)
	ActivateDelegation(ctx context.Context, stakingTxHash chainhash.Hash, proof *btcctypes.BTCSpvProof) error
	QueryHeaderDepth(headerHash *chainhash.Hash) (uint32, error)
	Params() (*BabylonParams, error)
	CometBFTTipHeight(ctx context.Context) (int64, error)
	DelegationsModifiedInBlock(ctx context.Context, height int64, eventTypes []string) ([]string, error)
	BTCDelegation(stakingTxHash string) (*Delegation, error)
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

// DelegationsByStatus - returns btc delegations by Status
func (bca *BabylonClientAdapter) DelegationsByStatus(status btcstakingtypes.BTCDelegationStatus, cursor []byte, limit uint64) ([]Delegation, []byte, error) {
	resp, err := bca.babylonClient.BTCDelegations(
		status,
		&query.PageRequest{
			Key:   cursor,
			Limit: limit,
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve delegations from babylon: %w", err)
	}

	delegations := make([]Delegation, len(resp.BtcDelegations))

	for i, delegation := range resp.BtcDelegations {
		stakingTx, _, err := bbn.NewBTCTxFromHex(delegation.StakingTxHex)
		if err != nil {
			return nil, nil, err
		}

		unbondingTx, _, err := bbn.NewBTCTxFromHex(delegation.UndelegationResponse.UnbondingTxHex)
		if err != nil {
			return nil, nil, err
		}

		delegations[i] = Delegation{
			StakingTx:             stakingTx,
			StakingOutputIdx:      delegation.StakingOutputIdx,
			DelegationStartHeight: delegation.StartHeight,
			UnbondingOutput:       unbondingTx.TxOut[0],
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

// IsDelegationActive method for BabylonClientAdapter
func (bca *BabylonClientAdapter) IsDelegationActive(stakingTxHash chainhash.Hash) (bool, error) {
	resp, err := bca.babylonClient.BTCDelegation(stakingTxHash.String())

	if err != nil {
		return false, fmt.Errorf("failed to retrieve delegation from babyln: %w", err)
	}

	return resp.BtcDelegation.Active, nil
}

// IsDelegationVerified method for BabylonClientAdapter checks if delegation is in Status verified
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
	inclusionProof *btcstakingtypes.InclusionProof,
	fundingTxs [][]byte) error {
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
		FundingTransactions:           fundingTxs,
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
		return fmt.Errorf("msg MsgAddBTCDelegationInclusionProof failed execution with code %d and error %w", resp.Code, err)
	}

	if err != nil {
		return fmt.Errorf("failed to report MsgAddBTCDelegationInclusionProof: %w", err)
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

func (bca *BabylonClientAdapter) CometBFTTipHeight(ctx context.Context) (int64, error) {
	res, err := bca.babylonClient.RPCClient.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve cometbft tip height: %w", err)
	}

	return res.SyncInfo.LatestBlockHeight, nil
}

func (bca *BabylonClientAdapter) StakingTxHashesByEvent(ctx context.Context, eventType string, criteria string, page, count *int) ([]string, error) {
	res, err := bca.babylonClient.RPCClient.TxSearch(ctx, criteria, false, page, count, "asc")
	if err != nil {
		return nil, fmt.Errorf("failed to do tx_search for: %s ,err: %w", criteria, err)
	}

	const stakingTxHashKey = "staking_tx_hash"

	var stakingTxHashes []string
	for _, tx := range res.Txs {
		for _, event := range tx.TxResult.Events {
			if event.Type == eventType {
				for _, attr := range event.Attributes {
					if attr.Key == stakingTxHashKey {
						stakingTxHash := strings.ReplaceAll(attr.Value, `"`, "")
						stakingTxHashes = append(stakingTxHashes, stakingTxHash)
					}
				}
			}
		}
	}

	return stakingTxHashes, nil
}

func (bca *BabylonClientAdapter) DelegationsModifiedInBlock(
	ctx context.Context,
	height int64,
	eventTypes []string,
) ([]string, error) {
	var stakingTxHashes []string
	events := make(map[string]bool)

	for _, eventType := range eventTypes {
		events[eventType] = true
	}

	res, err := bca.babylonClient.RPCClient.BlockResults(ctx, &height)
	if err != nil {
		return nil, fmt.Errorf("failed to do block_results for: %d ,err: %w", height, err)
	}

	// iterate over all tx results
	for _, txResult := range res.TxsResults {
		for _, event := range txResult.Events {
			// check if event type is in the list of event types we are interested in
			if _, ok := events[event.Type]; !ok {
				continue
			}
			for _, attr := range event.Attributes {
				if attr.Key == stakingTxHashKey {
					stakingTxHash := strings.ReplaceAll(attr.Value, `"`, "")
					stakingTxHashes = append(stakingTxHashes, stakingTxHash)
				}
			}
		}
	}

	// iterate over `FinalizeBlockEvents`
	// we need to check `FinalizeBlockEvents`, as events about stake expiration
	// are included here
	for _, finBlockEvent := range res.FinalizeBlockEvents {
		if _, ok := events[finBlockEvent.Type]; !ok {
			continue
		}

		for _, attr := range finBlockEvent.Attributes {
			if attr.Key == stakingTxHashKey {
				stakingTxHash := strings.ReplaceAll(attr.Value, `"`, "")
				stakingTxHashes = append(stakingTxHashes, stakingTxHash)
			}
		}
	}

	return stakingTxHashes, nil
}

// BTCDelegation method for BabylonClientAdapter to get BTC delegation
func (bca *BabylonClientAdapter) BTCDelegation(stakingTxHash string) (*Delegation, error) {
	resp, err := bca.babylonClient.BTCDelegation(stakingTxHash)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve delegation from babylon: %w", err)
	}

	delegation := resp.BtcDelegation

	stakingTx, _, err := bbn.NewBTCTxFromHex(delegation.StakingTxHex)
	if err != nil {
		return nil, err
	}

	unbondingTx, _, err := bbn.NewBTCTxFromHex(delegation.UndelegationResponse.UnbondingTxHex)
	if err != nil {
		return nil, err
	}

	return &Delegation{
		StakingTx:             stakingTx,
		StakingOutputIdx:      delegation.StakingOutputIdx,
		DelegationStartHeight: delegation.StartHeight,
		UnbondingOutput:       unbondingTx.TxOut[0],
		HasProof:              delegation.StartHeight > 0,
		Status:                delegation.StatusDesc,
		IsStakeExpansion:      delegation.StkExp != nil,
	}, nil
}
