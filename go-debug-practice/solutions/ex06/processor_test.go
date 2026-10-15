package batch

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

type memSink struct {
	writes  int32
	latency time.Duration
	failOn  string
}

func (s *memSink) Write(ctx context.Context, job Job) error {
	if s.latency > 0 {
		select {
		case <-time.After(s.latency):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	atomic.AddInt32(&s.writes, 1)
	if job.ID == s.failOn {
		return fmt.Errorf("sink rejected %s", job.ID)
	}
	return nil
}

func jobs(n int) []Job {
	out := make([]Job, 0, n)
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("job-%02d", i)
		out = append(out, Job{ID: id, Topic: "orders", Payload: []byte(id)})
	}
	return out
}

func runWithDeadline(t *testing.T, p *Processor, js []Job, d time.Duration) error {
	t.Helper()

	type result struct{ err error }
	done := make(chan result, 1)

	go func() {
		done <- result{err: p.Run(context.Background(), js)}
	}()

	select {
	case r := <-done:
		return r.err
	case <-time.After(d):
		t.Fatalf("Run did not return within %s for %d jobs", d, len(js))
		return nil
	}
}

func TestRunSmallBatchWithinConcurrencyLimit(t *testing.T) {
	sink := &memSink{latency: 5 * time.Millisecond}
	p := New(sink, 4)

	js := jobs(4)
	if err := runWithDeadline(t, p, js, 2*time.Second); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{"job-01", "job-02", "job-03", "job-04"}
	if got := p.Delivered(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Delivered() = %v, want %v", got, want)
	}
}

func TestRunBatchLargerThanConcurrencyLimit(t *testing.T) {
	sink := &memSink{latency: 5 * time.Millisecond}
	p := New(sink, 4)

	js := jobs(12)
	if err := runWithDeadline(t, p, js, 3*time.Second); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := len(p.Delivered()); got != len(js) {
		t.Fatalf("delivered %d jobs, want %d", got, len(js))
	}
	if got := atomic.LoadInt32(&sink.writes); got != int32(len(js)) {
		t.Fatalf("sink saw %d writes, want %d", got, len(js))
	}
}

func TestRunSurfacesSinkError(t *testing.T) {
	sink := &memSink{latency: time.Millisecond, failOn: "job-05"}
	p := New(sink, 4)

	err := runWithDeadline(t, p, jobs(8), 3*time.Second)
	if err == nil {
		t.Fatal("Run returned no error although the sink rejected a job")
	}
}
