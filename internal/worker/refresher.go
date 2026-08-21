package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	streamrepo "github.com/zomato/cohort-service/internal/repository/redis"
)

// Consumer pulls jobs off the refresh stream.
type Consumer interface {
	Consume(ctx context.Context, consumerName string, block time.Duration) (*streamrepo.RefreshJob, error)
	Ack(ctx context.Context, msgID string) error
}

// RefreshExecutor runs a single refresh job.
type RefreshExecutor interface {
	Run(ctx context.Context, cohortID, runID uuid.UUID) error
}

// Refresher is the long-running consumer loop. Owns no business logic.
type Refresher struct {
	consumer     Consumer
	executor     RefreshExecutor
	consumerName string
	blockTimeout time.Duration
	logger       *log.Logger
}

// NewRefresher constructs a worker bound to the stream and executor.
func NewRefresher(c Consumer, e RefreshExecutor, consumerName string, logger *log.Logger) *Refresher {
	if logger == nil {
		logger = log.Default()
	}
	return &Refresher{
		consumer:     c,
		executor:     e,
		consumerName: consumerName,
		blockTimeout: 2 * time.Second,
		logger:       logger,
	}
}

// Run blocks until ctx is cancelled, consuming jobs one at a time.
// Each job is acked after Executor.Run returns — including error paths, because
// the run row's terminal status is what carries the retry decision, not the stream.
func (r *Refresher) Run(ctx context.Context) {
	r.logger.Printf("worker %s: starting consume loop", r.consumerName)
	for {
		if ctx.Err() != nil {
			r.logger.Printf("worker %s: context done, exiting", r.consumerName)
			return
		}
		job, err := r.consumer.Consume(ctx, r.consumerName, r.blockTimeout)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			r.logger.Printf("worker %s: consume error: %v", r.consumerName, err)
			continue
		}
		if job == nil {
			continue
		}
		if err := r.executor.Run(ctx, job.CohortID, job.RunID); err != nil {
			r.logger.Printf("worker %s: refresh %s failed: %v", r.consumerName, job.RunID, err)
		}
		if err := r.consumer.Ack(ctx, job.MsgID); err != nil {
			r.logger.Printf("worker %s: ack %s failed: %v", r.consumerName, job.MsgID, err)
		}
	}
}
