package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ChargeRequest struct {
	OrderID        string `json:"order_id"`
	AmountMinor    int64  `json:"amount_minor"`
	Currency       string `json:"currency"`
	IdempotencyKey string `json:"idempotency_key"`
}

type Receipt struct {
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	AuthCode      string `json:"auth_code"`
}

type GatewayError struct {
	Status int
	Body   string
}

func (e *GatewayError) Error() string {
	return fmt.Sprintf("gateway responded %d: %s", e.Status, e.Body)
}

func (e *GatewayError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= http.StatusInternalServerError
}

type Client struct {
	http        *http.Client
	baseURL     string
	maxAttempts int
	backoff     time.Duration
}

func NewClient(baseURL string) *Client {
	return &Client{
		http:        &http.Client{Timeout: 5 * time.Second},
		baseURL:     baseURL,
		maxAttempts: 3,
		backoff:     10 * time.Millisecond,
	}
}

func (c *Client) Charge(ctx context.Context, req ChargeRequest) (*Receipt, error) {
	var receipt *Receipt
	var err error

	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		receipt, err = c.attempt(ctx, req)

		fmt.Println("check receipt : ", receipt)
		fmt.Println("check err : ", err)

		if err != nil {
			var ge *GatewayError
			if errors.As(err, &ge) && !ge.Retryable() {
				return nil, fmt.Errorf("charge %s: %w", req.OrderID, err)
			}
			if attempt < c.maxAttempts {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(c.backoff * time.Duration(attempt)):
				}
			}
			continue
		}
		return receipt, nil
	}

	if err != nil {
		return nil, fmt.Errorf("charge %s after %d attempts: %w", req.OrderID, c.maxAttempts, err)
	}
	return receipt, nil
}

func (c *Client) attempt(ctx context.Context, req ChargeRequest) (*Receipt, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode charge: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/charges", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", req.IdempotencyKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call gateway: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gateway response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &GatewayError{Status: resp.StatusCode, Body: string(body)}
	}

	var receipt Receipt
	if err := json.Unmarshal(body, &receipt); err != nil {
		return nil, fmt.Errorf("decode receipt: %w", err)
	}
	return &receipt, nil
}
