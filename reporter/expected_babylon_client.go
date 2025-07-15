package reporter

import (
	"context"

	"cosmossdk.io/errors"
	"github.com/babylonlabs-io/babylon/v3/client/babylonclient"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/babylonlabs-io/babylon/v3/client/config"
	btcctypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/v3/x/btclightclient/types"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type BabylonClient interface {
	MustGetAddr() string
	GetConfig() *config.BabylonConfig
	BTCCheckpointParams() (*btcctypes.QueryParamsResponse, error)
	InsertHeaders(ctx context.Context, msgs *btclctypes.MsgInsertHeaders) (*babylonclient.RelayerTxResponse, error)
	ContainsBTCBlock(blockHash *chainhash.Hash) (*btclctypes.QueryContainsBytesResponse, error)
	BTCHeaderChainTip() (*btclctypes.QueryTipResponse, error)
	BTCBaseHeader() (*btclctypes.QueryBaseHeaderResponse, error)
	InsertBTCSpvProof(ctx context.Context, msg *btcctypes.MsgInsertBTCSpvProof) (*babylonclient.RelayerTxResponse, error)
	ReliablySendMsg(ctx context.Context, msg sdk.Msg, expectedErrors []*errors.Error, unrecoverableErrors []*errors.Error, retries ...uint) (*babylonclient.RelayerTxResponse, error)
	Stop() error
}
