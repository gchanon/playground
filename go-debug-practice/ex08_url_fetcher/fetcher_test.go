package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

// enrichmentServer answers /boom with 500 immediately, any path containing
// "warm" immediately with 200, and everything else after delay.
func enrichmentServer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/boom"):
			w.WriteHeader(http.StatusInternalServerError)
			return
		case strings.Contains(r.URL.Path, "warm"):
		default:
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"path":%q}`, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func settle(baseline, slack int, within time.Duration) (int, bool) {
	deadline := time.Now().Add(within)
	n := runtime.NumGoroutine()
	for {
		n = runtime.NumGoroutine()
		if n <= baseline+slack {
			return n, true
		}
		if time.Now().After(deadline) {
			return n, false
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func warmup(t *testing.T, f *Fetcher, srv *httptest.Server) int {
	t.Helper()
	if _, err := f.FetchAll(context.Background(), []string{srv.URL + "/records/warm"}); err != nil {
		t.Fatalf("warmup: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	return runtime.NumGoroutine()
}

func TestFetchAllReturnsEveryResult(t *testing.T) {
	srv := enrichmentServer(t, 5*time.Millisecond)

	urls := make([]string, 0, 16)
	for i := 0; i < 16; i++ {
		urls = append(urls, fmt.Sprintf("%s/records/%d", srv.URL, i))
	}

	got, err := New(WithConcurrency(4)).FetchAll(context.Background(), urls)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(got) != len(urls) {
		t.Fatalf("FetchAll returned %d results, want %d", len(got), len(urls))
	}
}

func TestFetchAllDoesNotLeakOnError(t *testing.T) {
	srv := enrichmentServer(t, 100*time.Millisecond)
	f := New(WithConcurrency(4))
	baseline := warmup(t, f, srv)

	urls := []string{srv.URL + "/boom"}
	for i := 0; i < 31; i++ {
		urls = append(urls, fmt.Sprintf("%s/records/%d", srv.URL, i))
	}

	if _, err := f.FetchAll(context.Background(), urls); err == nil {
		t.Fatal("FetchAll returned no error although one URL answers 500")
	}

	if n, ok := settle(baseline, 10, 4*time.Second); !ok {
		t.Fatalf("goroutine count stayed at %d (baseline %d) after FetchAll returned an error", n, baseline)
	}
}

func TestFetchAllHonoursContextCancellation(t *testing.T) {
	srv := enrichmentServer(t, 3*time.Second)
	f := New(WithConcurrency(8))
	baseline := warmup(t, f, srv)

	urls := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		urls = append(urls, fmt.Sprintf("%s/records/%d", srv.URL, i))
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	if _, err := f.FetchAll(ctx, urls); err == nil {
		t.Fatal("FetchAll returned no error although the context was cancelled")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("FetchAll took %s to return after cancellation, want well under 1s", elapsed)
	}

	if n, ok := settle(baseline, 10, 1500*time.Millisecond); !ok {
		t.Fatalf("in-flight work outlived the cancelled context: %d goroutines running, baseline %d", n, baseline)
	}
}
