package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Shard struct {
	Name string
	DSN  string
}

type Counter interface {
	CountOrders(ctx context.Context, shard Shard) (int, error)
}

type Report struct {
	PerShard  map[string]int
	Total     int
	Collected time.Time
}

func Aggregate(ctx context.Context, counter Counter, shards []Shard) (*Report, error) {
	perShard := make(map[string]int, len(shards))

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
	)

	for _, shard := range shards {
		wg.Add(1)
		go func(shard Shard) {

			defer wg.Done()

			n, err := counter.CountOrders(ctx, shard)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("count %s: %w", shard.Name, err)
				}
				return
			}
			perShard[shard.Name] = n
		}(shard)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	report := &Report{
		PerShard:  perShard,
		Collected: time.Now(),
	}
	for _, n := range perShard {
		report.Total += n
	}
	return report, nil
}

func ShardsFromDSNs(dsns map[string]string) []Shard {
	shards := make([]Shard, 0, len(dsns))
	for name, dsn := range dsns {
		shards = append(shards, Shard{Name: name, DSN: dsn})
	}
	return shards
}
