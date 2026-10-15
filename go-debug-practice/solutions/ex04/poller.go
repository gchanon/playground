package fx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

// ErrPairNotQuoted is returned when the provider has no quote for a pair.
var ErrPairNotQuoted = errors.New("pair not quoted")

type Rate struct {
	Pair      string  `json:"pair"`
	Bid       float64 `json:"bid"`
	Ask       float64 `json:"ask"`
	AsOfMilli int64   `json:"as_of_milli"`
}

func (r Rate) Mid() float64 {
	return (r.Bid + r.Ask) / 2
}

type Poller struct {
	client  *http.Client
	baseURL string

	mu    sync.RWMutex
	last  map[string]Rate
	stale time.Duration
}

func NewPoller(baseURL string) *Poller {
	return &Poller{
		baseURL: baseURL,
		last:    make(map[string]Rate),
		stale:   30 * time.Second,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        32,
				MaxIdleConnsPerHost: 32,
				IdleConnTimeout:     60 * time.Second,
			},
		},
	}
}

func (p *Poller) Rate(ctx context.Context, pair string) (Rate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v1/rates/"+pair, nil)
	if err != nil {
		return Rate{}, fmt.Errorf("build request for %s: %w", pair, err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return Rate{}, fmt.Errorf("fetch %s: %w", pair, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return Rate{}, fmt.Errorf("fetch %s: %w", pair, ErrPairNotQuoted)
	}
	if resp.StatusCode != http.StatusOK {
		return Rate{}, fmt.Errorf("fetch %s: upstream returned %d", pair, resp.StatusCode)
	}

	var rate Rate
	if err := json.NewDecoder(resp.Body).Decode(&rate); err != nil {
		return Rate{}, fmt.Errorf("decode %s: %w", pair, err)
	}
	if rate.Pair == "" {
		rate.Pair = pair
	}

	p.mu.Lock()
	p.last[pair] = rate
	p.mu.Unlock()

	return rate, nil
}

func (p *Poller) Snapshot(ctx context.Context, pairs []string) (map[string]Rate, error) {
	out := make(map[string]Rate, len(pairs))
	for _, pair := range pairs {
		rate, err := p.Rate(ctx, pair)
		if errors.Is(err, ErrPairNotQuoted) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[pair] = rate
	}
	return out, nil
}

func (p *Poller) Cached(pair string) (Rate, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	r, ok := p.last[pair]
	if !ok {
		return Rate{}, false
	}
	if time.Since(time.UnixMilli(r.AsOfMilli)) > p.stale {
		return Rate{}, false
	}
	return r, true
}

func (p *Poller) TrackedPairs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pairs := make([]string, 0, len(p.last))
	for pair := range p.last {
		pairs = append(pairs, pair)
	}
	sort.Strings(pairs)
	return pairs
}
