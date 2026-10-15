package batch

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Job struct {
	ID      string
	Topic   string
	Payload []byte
}

type Sink interface {
	Write(ctx context.Context, job Job) error
}

type Processor struct {
	sink    Sink
	sem     chan struct{}
	timeout time.Duration

	mu        sync.Mutex
	delivered []string
}

func New(sink Sink, concurrency int) *Processor {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Processor{
		sink:    sink,
		sem:     make(chan struct{}, concurrency),
		timeout: 10 * time.Second,
	}
}

func (p *Processor) Run(ctx context.Context, jobs []Job) error {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, len(jobs))

	for _, job := range jobs {
		select {
		case p.sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		}

		wg.Add(1)
		go func(job Job) {
			defer wg.Done()
			defer func() { <-p.sem }()

			if err := p.sink.Write(ctx, job); err != nil {
				errs <- fmt.Errorf("write %s: %w", job.ID, err)
				return
			}

			p.mu.Lock()
			p.delivered = append(p.delivered, job.ID)
			p.mu.Unlock()
		}(job)
	}

	go func() {
		wg.Wait()
		close(errs)
	}()

	if err := <-errs; err != nil {
		return err
	}
	return nil
}

func (p *Processor) Delivered() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]string, len(p.delivered))
	copy(out, p.delivered)
	sort.Strings(out)
	return out
}
