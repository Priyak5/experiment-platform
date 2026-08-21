package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RefreshJob is the payload flowing through the refresh stream.
type RefreshJob struct {
	CohortID uuid.UUID
	RunID    uuid.UUID
	MsgID    string // Redis stream message id, needed for Ack
}

// StreamRepo produces and consumes cohort refresh jobs on a Redis Stream
// via a named consumer group.
type StreamRepo struct {
	c      *redis.Client
	stream string
	group  string
}

// NewStreamRepo binds to a client and ensures the consumer group exists.
func NewStreamRepo(ctx context.Context, c *redis.Client, stream, group string) (*StreamRepo, error) {
	r := &StreamRepo{c: c, stream: stream, group: group}
	if err := r.ensureGroup(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *StreamRepo) ensureGroup(ctx context.Context) error {
	// MKSTREAM creates the stream if it doesn't exist.
	err := r.c.XGroupCreateMkStream(ctx, r.stream, r.group, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("xgroup create: %w", err)
	}
	return nil
}

// Publish enqueues a refresh job. Returns the stream message id.
func (r *StreamRepo) Publish(ctx context.Context, job RefreshJob) (string, error) {
	id, err := r.c.XAdd(ctx, &redis.XAddArgs{
		Stream: r.stream,
		Values: map[string]any{
			"cohort_id": job.CohortID.String(),
			"run_id":    job.RunID.String(),
		},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("xadd: %w", err)
	}
	return id, nil
}

// Consume blocks for up to `block` for the next job assigned to consumerName
// in the group. Returns a nil job (no error) when the read times out.
func (r *StreamRepo) Consume(ctx context.Context, consumerName string, block time.Duration) (*RefreshJob, error) {
	res, err := r.c.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    r.group,
		Consumer: consumerName,
		Streams:  []string{r.stream, ">"},
		Count:    1,
		Block:    block,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("xreadgroup: %w", err)
	}
	if len(res) == 0 || len(res[0].Messages) == 0 {
		return nil, nil
	}
	msg := res[0].Messages[0]

	cohortStr, _ := msg.Values["cohort_id"].(string)
	runStr, _ := msg.Values["run_id"].(string)
	cohortID, err := uuid.Parse(cohortStr)
	if err != nil {
		return nil, fmt.Errorf("bad cohort_id %q: %w", cohortStr, err)
	}
	runID, err := uuid.Parse(runStr)
	if err != nil {
		return nil, fmt.Errorf("bad run_id %q: %w", runStr, err)
	}
	return &RefreshJob{CohortID: cohortID, RunID: runID, MsgID: msg.ID}, nil
}

// Ack marks a stream message as processed.
func (r *StreamRepo) Ack(ctx context.Context, msgID string) error {
	if _, err := r.c.XAck(ctx, r.stream, r.group, msgID).Result(); err != nil {
		return fmt.Errorf("xack: %w", err)
	}
	return nil
}
