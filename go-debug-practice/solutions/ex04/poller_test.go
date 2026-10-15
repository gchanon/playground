package fx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type connCounter struct {
	mu    sync.Mutex
	seen  map[net.Conn]struct{}
	total int
}

func (c *connCounter) track(conn net.Conn, state http.ConnState) {
	if state != http.StateNew {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen == nil {
		c.seen = make(map[net.Conn]struct{})
	}
	if _, ok := c.seen[conn]; !ok {
		c.seen[conn] = struct{}{}
		c.total++
	}
}

func (c *connCounter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

// newRateServer quotes every pair except those whose name ends in "X", which
// the provider answers with 404 (plus a JSON error body), as the real one does.
func newRateServer(t *testing.T) (*httptest.Server, *connCounter) {
	t.Helper()
	counter := &connCounter{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pair := strings.TrimPrefix(r.URL.Path, "/v1/rates/")
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(pair, "X") {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "pair not quoted",
				"pair":  pair,
			})
			return
		}

		_ = json.NewEncoder(w).Encode(Rate{
			Pair:      pair,
			Bid:       32.10,
			Ask:       32.30,
			AsOfMilli: time.Now().UnixMilli(),
		})
	}))
	srv.Config.ConnState = counter.track
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, counter
}

func TestRateReturnsUpstreamValue(t *testing.T) {
	srv, _ := newRateServer(t)
	p := NewPoller(srv.URL)

	r, err := p.Rate(context.Background(), "USDTHB")
	if err != nil {
		t.Fatalf("Rate: %v", err)
	}
	if r.Mid() != 32.20 {
		t.Fatalf("Mid() = %v, want 32.2", r.Mid())
	}
	if _, ok := p.Cached("USDTHB"); !ok {
		t.Fatal("Cached(USDTHB) missing after a successful fetch")
	}

	if _, err := p.Rate(context.Background(), "USDXAUX"); !errors.Is(err, ErrPairNotQuoted) {
		t.Fatalf("Rate(unquoted pair) error = %v, want ErrPairNotQuoted", err)
	}
}

func TestSnapshotReusesUpstreamConnections(t *testing.T) {
	srv, counter := newRateServer(t)
	p := NewPoller(srv.URL)

	pairs := make([]string, 0, 24)
	for i := 0; i < 24; i++ {
		pair := fmt.Sprintf("USDTHB%02d", i)
		if i%2 == 1 {
			pair += "X" // the provider does not quote these
		}
		pairs = append(pairs, pair)
	}

	for round := 0; round < 2; round++ {
		got, err := p.Snapshot(context.Background(), pairs)
		if err != nil {
			t.Fatalf("Snapshot round %d: %v", round, err)
		}
		if len(got) != 12 {
			t.Fatalf("Snapshot round %d returned %d rates, want 12", round, len(got))
		}
	}

	if got := counter.count(); got > 2 {
		t.Fatalf("poller opened %d upstream connections for %d sequential requests; want at most 2",
			got, len(pairs)*2)
	}
}
