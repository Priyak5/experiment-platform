package worker

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	streamrepo "github.com/zomato/cohort-service/internal/repository/redis"
)

type fakeConsumer struct {
	mu       sync.Mutex
	jobs     []*streamrepo.RefreshJob
	consumed int
	acked    []string
}

func (f *fakeConsumer) Consume(_ context.Context, _ string, _ time.Duration) (*streamrepo.RefreshJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.consumed >= len(f.jobs) {
		return nil, nil
	}
	j := f.jobs[f.consumed]
	f.consumed++
	return j, nil
}
func (f *fakeConsumer) Ack(_ context.Context, msgID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acked = append(f.acked, msgID)
	return nil
}

type fakeExecutor struct {
	mu    sync.Mutex
	runs  []uuid.UUID
	fail  bool
}

func (f *fakeExecutor) Run(_ context.Context, _, runID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, runID)
	if f.fail {
		return errors.New("boom")
	}
	return nil
}

func silentLogger() *log.Logger { return log.New(io.Discard, "", 0) }

func TestRefresher_ConsumesAndAcks(t *testing.T) {
	c := &fakeConsumer{jobs: []*streamrepo.RefreshJob{
		{CohortID: uuid.New(), RunID: uuid.New(), MsgID: "m1"},
		{CohortID: uuid.New(), RunID: uuid.New(), MsgID: "m2"},
	}}
	e := &fakeExecutor{}
	r := NewRefresher(c, e, "w-test", silentLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r.Run(ctx)

	if len(e.runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(e.runs))
	}
	if len(c.acked) != 2 {
		t.Fatalf("acks = %d, want 2", len(c.acked))
	}
}

func TestRefresher_AcksEvenOnRunError(t *testing.T) {
	c := &fakeConsumer{jobs: []*streamrepo.RefreshJob{
		{CohortID: uuid.New(), RunID: uuid.New(), MsgID: "m1"},
	}}
	e := &fakeExecutor{fail: true}
	r := NewRefresher(c, e, "w-test", silentLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r.Run(ctx)

	if len(c.acked) != 1 {
		t.Fatalf("acked = %v, want single ack even on run error", c.acked)
	}
}

func TestRefresher_ExitsOnContextCancel(t *testing.T) {
	c := &fakeConsumer{}
	e := &fakeExecutor{}
	r := NewRefresher(c, e, "w-test", silentLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not exit on cancel")
	}
}
