// Package fetcher provides a bounded-concurrency HTTP fetcher for enriching
// records from third-party endpoints.
package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Result holds the body of one successfully fetched URL.
type Result struct {
	URL        string
	StatusCode int
	Body       []byte
}

// Option configures a Fetcher.
type Option func(*Fetcher)

// WithConcurrency sets the maximum number of in-flight requests.
func WithConcurrency(n int) Option {
	return func(f *Fetcher) {
		if n > 0 {
			f.concurrency = n
		}
	}
}

// WithHTTPClient overrides the HTTP client used for requests.
func WithHTTPClient(c *http.Client) Option {
	return func(f *Fetcher) {
		if c != nil {
			f.client = c
		}
	}
}

// Fetcher fetches many URLs concurrently with a bounded worker pool.
type Fetcher struct {
	client      *http.Client
	concurrency int
	maxBodySize int64
}

// New returns a Fetcher with sensible defaults.
func New(opts ...Option) *Fetcher {
	f := &Fetcher{
		client:      &http.Client{Timeout: 30 * time.Second},
		concurrency: 8,
		maxBodySize: 1 << 20,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// FetchAll fetches every URL concurrently and returns the successful results.
// It returns the first error encountered, if any.
func (f *Fetcher) FetchAll(ctx context.Context, urls []string) ([]Result, error) {
	if len(urls) == 0 {
		return nil, nil
	}

	results := make(chan Result, len(urls))
	errs := make(chan error, len(urls))
	sem := make(chan struct{}, f.concurrency)

	for _, url := range urls {
		go func(url string) {
			sem <- struct{}{}
			defer func() { <-sem }()

			res, err := f.fetchOne(url, ctx)
			if err != nil {

				errs <- err
				return
			}
			results <- res

		}(url)
	}

	out := make([]Result, 0, len(urls))
	for i := 0; i < len(urls); i++ {
		select {
		case res := <-results:
			out = append(out, res)
		case err := <-errs:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return out, nil
}

func (f *Fetcher) fetchOne(url string, ctx context.Context) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBodySize))
	if err != nil {
		return Result{}, fmt.Errorf("read body of %s: %w", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("fetch %s: unexpected status %d", url, resp.StatusCode)
	}

	return Result{URL: url, StatusCode: resp.StatusCode, Body: body}, nil
}
