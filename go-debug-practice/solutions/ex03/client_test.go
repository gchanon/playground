package payments

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func newRequest() ChargeRequest {
	return ChargeRequest{
		OrderID:        "ORD-3001",
		AmountMinor:    129900,
		Currency:       "THB",
		IdempotencyKey: "key-3001",
	}
}

func TestChargeSucceedsAfterTransientFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"transaction_id":"txn_123","status":"captured","auth_code":"A1B2"}`))
	}))
	defer srv.Close()

	receipt, err := NewClient(srv.URL).Charge(context.Background(), newRequest())
	if err != nil {
		t.Fatalf("Charge: %v", err)
	}
	if receipt == nil || receipt.TransactionID != "txn_123" {
		t.Fatalf("receipt = %+v, want transaction_id txn_123", receipt)
	}
}

func TestChargeReportsFailureWhenGatewayIsDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"upstream unavailable"}`))
	}))
	defer srv.Close()

	receipt, err := NewClient(srv.URL).Charge(context.Background(), newRequest())
	if err == nil {
		t.Fatalf("Charge returned no error; receipt = %+v", receipt)
	}
	if receipt != nil {
		t.Fatalf("Charge returned a receipt alongside an error: %+v", receipt)
	}
}

func TestChargeDoesNotRetryDeclines(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":"card declined"}`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL).Charge(context.Background(), newRequest()); err == nil {
		t.Fatal("Charge returned no error for a declined card")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("gateway called %d times for a decline, want 1", got)
	}
}
