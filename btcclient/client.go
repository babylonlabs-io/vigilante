// Copyright (c) 2022-2022 The Babylon developers
// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcclient

import (
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/rpcclient"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/vigilante/config"
)

var _ BTCClient = &Client{}

// Client represents a persistent client connection to a bitcoin RPC server
// for information regarding the current best block chain.
type Client struct {
	*rpcclient.Client

	params *chaincfg.Params
	cfg    *config.BTCConfig
	logger *zap.SugaredLogger

	// retry attributes
	retrySleepTime    time.Duration
	maxRetrySleepTime time.Duration
	maxRetryTimes     uint
}

func (c *Client) GetTipBlockVerbose() (*btcjson.GetBlockVerboseResult, error) {
	tipBtcHash, err := c.GetBestBlockHash()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain BTC block tip: %w", err)
	}
	tipBlock, err := c.GetBlockVerbose(tipBtcHash)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain BTC tip block: %w", err)
	}

	return tipBlock, nil
}

func (c *Client) Stop() {
	c.Shutdown()
}
