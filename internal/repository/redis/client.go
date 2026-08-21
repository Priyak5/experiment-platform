package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewClient constructs a go-redis client bound to addr and pings it.
func NewClient(ctx context.Context, addr string) (*redis.Client, error) {
	c := redis.NewClient(&redis.Options{Addr: addr})
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("ping redis at %s: %w", addr, err)
	}
	return c, nil
}
