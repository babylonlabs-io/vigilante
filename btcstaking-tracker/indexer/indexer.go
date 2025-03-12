package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/avast/retry-go/v4"
	"go.uber.org/zap"
	"io"
	"net/http"
	"time"
)

const maxResponseSize = 512 * 1024 // 0.5MB in bytes

type OutspendResponse struct {
	Spent  bool   `json:"spent"`
	TxID   string `json:"txid"`
	Vin    int    `json:"vin"`
	Status struct {
		Confirmed   bool   `json:"confirmed"`
		BlockHeight int    `json:"block_height"`
		BlockHash   string `json:"block_hash"`
		BlockTime   int64  `json:"block_time"`
	} `json:"status"`
}

// HTTPIndexerClient implements Client with HTTP requests.
type HTTPIndexerClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.SugaredLogger
}

func NewHTTPIndexerClient(baseURL string, timeout time.Duration, logger zap.Logger) *HTTPIndexerClient {
	return &HTTPIndexerClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger.Sugar(),
	}
}

// GetOutspend fetches outspend data with retries.
func (c *HTTPIndexerClient) GetOutspend(ctx context.Context, txID string, vout uint32) (*OutspendResponse, error) {
	var response OutspendResponse
	url := fmt.Sprintf("%s/tx/%s/outspend/%d", c.baseURL, txID, vout)

	err := retry.Do(
		func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return err
			}

			resp, err := c.httpClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			}

			limitedReader := io.LimitReader(resp.Body, maxResponseSize)
			if err := json.NewDecoder(limitedReader).Decode(&response); err != nil {
				if err == io.EOF {
					return fmt.Errorf("response exceeded maximum size of %d bytes", maxResponseSize)
				}

				return err
			}

			return nil
		},
		retry.Context(ctx),
		retry.Delay(1*time.Second),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			c.logger.Debugf("retrying to getOutspend: Attempt: %d. Err: %v", n, err)
		}),
	)

	if err != nil {
		return nil, err
	}

	return &response, nil
}
