package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type fakeCounter struct {
	counts map[string]int
	delay  time.Duration
}

func (f *fakeCounter) CountOrders(ctx context.Context, shard Shard) (int, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	n, ok := f.counts[shard.Name]
	if !ok {
		return 0, fmt.Errorf("unknown shard %q", shard.Name)
	}
	return n, nil
}

func shards() []Shard {
	out := make([]Shard, 0, 6)
	for i := 1; i <= 6; i++ {
		out = append(out, Shard{
			Name: fmt.Sprintf("shard-%02d", i),
			DSN:  fmt.Sprintf("postgres://db-%02d/orders", i),
		})
	}
	return out
}

func TestAggregateCollectsEveryShard(t *testing.T) {
	all := shards()

	counts := make(map[string]int, len(all))
	wantTotal := 0
	for i, s := range all {
		counts[s.Name] = (i + 1) * 100
		wantTotal += (i + 1) * 100
	}

	counter := &fakeCounter{counts: counts, delay: time.Millisecond}

	for run := 0; run < 25; run++ {
		report, err := Aggregate(context.Background(), counter, all)
		if err != nil {
			t.Fatalf("run %d: Aggregate: %v", run, err)
		}
		if len(report.PerShard) != len(all) {
			t.Fatalf("run %d: got %d shards in report, want %d (%v)",
				run, len(report.PerShard), len(all), report.PerShard)
		}
		for name, want := range counts {
			if got := report.PerShard[name]; got != want {
				t.Fatalf("run %d: %s = %d, want %d", run, name, got, want)
			}
		}
		if report.Total != wantTotal {
			t.Fatalf("run %d: Total = %d, want %d", run, report.Total, wantTotal)
		}
	}
}

func TestAggregateReportsShardError(t *testing.T) {
	all := shards()
	counts := map[string]int{}
	for _, s := range all[:len(all)-1] {
		counts[s.Name] = 10
	}

	counter := &fakeCounter{counts: counts, delay: time.Millisecond}

	if _, err := Aggregate(context.Background(), counter, all); err == nil {
		t.Fatal("Aggregate returned no error although one shard failed")
	}
}
