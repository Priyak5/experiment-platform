package redis

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestStream_PublishConsumeAck(t *testing.T) {
	c := setupTestRedis(t)
	ctx := context.Background()

	s, err := NewStreamRepo(ctx, c, "stream:test", "test-group")
	if err != nil {
		t.Fatalf("new stream repo: %v", err)
	}

	cohortID, runID := uuid.New(), uuid.New()
	msgID, err := s.Publish(ctx, RefreshJob{CohortID: cohortID, RunID: runID})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if msgID == "" {
		t.Fatal("empty msg id")
	}

	job, err := s.Consume(ctx, "worker-1", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if job == nil {
		t.Fatal("nil job")
	}
	if job.CohortID != cohortID || job.RunID != runID {
		t.Fatalf("job mismatch: %+v", job)
	}
	if err := s.Ack(ctx, job.MsgID); err != nil {
		t.Fatalf("ack: %v", err)
	}
}

func TestStream_ConsumeBlockTimeout(t *testing.T) {
	c := setupTestRedis(t)
	ctx := context.Background()
	s, err := NewStreamRepo(ctx, c, "stream:empty", "g")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	start := time.Now()
	job, err := s.Consume(ctx, "w", 200*time.Millisecond)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if job != nil {
		t.Fatalf("expected nil job on timeout, got %+v", job)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatalf("consume returned too fast: %v", time.Since(start))
	}
}

func TestStream_GroupIdempotentCreate(t *testing.T) {
	c := setupTestRedis(t)
	ctx := context.Background()
	if _, err := NewStreamRepo(ctx, c, "stream:x", "g1"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := NewStreamRepo(ctx, c, "stream:x", "g1"); err != nil {
		t.Fatalf("second: %v", err)
	}
}
