package submitter

import (
	"github.com/babylonlabs-io/vigilante/submitter/poller"

	btcctypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
)

type BabylonQueryClient interface {
	poller.BabylonQueryClient
	BTCCheckpointParams() (*btcctypes.QueryParamsResponse, error)
}
