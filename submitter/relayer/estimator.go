package relayer

import (
	"fmt"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"

	"github.com/babylonlabs-io/vigilante/config"
)

// NewFeeEstimator creates a fee estimator for bitcoind
func NewFeeEstimator(cfg *config.BTCConfig) (chainfee.Estimator, error) {
	// TODO Currently we are not using Params field of rpcclient.ConnConfig due to bug in btcd
	// when handling signet.
	// todo(lazar955): check if we should start specifying this, considering we are no longer using btcd based on comment above ^^
	connCfg := &rpcclient.ConnConfig{
		// this will work with node loaded with multiple wallets
		Host:         cfg.Endpoint + "/wallet/" + cfg.WalletName,
		HTTPPostMode: true,
		User:         cfg.Username,
		Pass:         cfg.Password,
		DisableTLS:   cfg.DisableClientTLS,
	}

	estimator, err := chainfee.NewBitcoindEstimator(
		*connCfg, cfg.EstimateMode, cfg.DefaultFee.FeePerKWeight(),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create fee estimator: %w", err)
	}

	if err := estimator.Start(); err != nil {
		return nil, fmt.Errorf("failed to initiate the fee estimator: %w", err)
	}

	return estimator, nil
}
