package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config is the runtime configuration for the cohort service.
type Config struct {
	HTTPAddr             string
	PostgresDSN          string
	PostgresReaderDSN    string
	RedisAddr            string
	RefreshStream        string
	RefreshConsumerGroup string
	RefreshConsumerName  string
	SQLStatementTimeout  time.Duration
}

// Load reads config from environment variables with sensible defaults for local dev.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:             getenv("HTTP_ADDR", "127.0.0.1:8080"),
		PostgresDSN:          getenv("POSTGRES_DSN", "postgres://cohort:cohort@127.0.0.1:5432/cohort?sslmode=disable"),
		PostgresReaderDSN:    getenv("POSTGRES_READER_DSN", ""),
		RedisAddr:            getenv("REDIS_ADDR", "127.0.0.1:6379"),
		RefreshStream:        getenv("REFRESH_STREAM", "stream:cohort-refresh"),
		RefreshConsumerGroup: getenv("REFRESH_CONSUMER_GROUP", "cohort-refreshers"),
		RefreshConsumerName:  getenv("REFRESH_CONSUMER_NAME", "worker-1"),
	}
	timeoutStr := getenv("SQL_STATEMENT_TIMEOUT", "30s")
	d, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return Config{}, fmt.Errorf("SQL_STATEMENT_TIMEOUT: %w", err)
	}
	c.SQLStatementTimeout = d
	if c.PostgresReaderDSN == "" {
		c.PostgresReaderDSN = c.PostgresDSN
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate checks that required fields are populated and well-formed.
func (c Config) Validate() error {
	if strings.TrimSpace(c.HTTPAddr) == "" {
		return errors.New("HTTP_ADDR is required")
	}
	if strings.TrimSpace(c.PostgresDSN) == "" {
		return errors.New("POSTGRES_DSN is required")
	}
	if strings.TrimSpace(c.RedisAddr) == "" {
		return errors.New("REDIS_ADDR is required")
	}
	if strings.TrimSpace(c.RefreshStream) == "" {
		return errors.New("REFRESH_STREAM is required")
	}
	if strings.TrimSpace(c.RefreshConsumerGroup) == "" {
		return errors.New("REFRESH_CONSUMER_GROUP is required")
	}
	if c.SQLStatementTimeout <= 0 {
		return errors.New("SQL_STATEMENT_TIMEOUT must be positive")
	}
	return nil
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
