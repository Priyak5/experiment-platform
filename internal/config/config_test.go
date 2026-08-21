package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

var envVars = []string{
	"HTTP_ADDR", "POSTGRES_DSN", "POSTGRES_READER_DSN", "REDIS_ADDR",
	"REFRESH_STREAM", "REFRESH_CONSUMER_GROUP", "REFRESH_CONSUMER_NAME",
	"SQL_STATEMENT_TIMEOUT",
}

func clearEnv(t *testing.T) {
	t.Helper()
	saved := make(map[string]string, len(envVars))
	for _, k := range envVars {
		if v, ok := os.LookupEnv(k); ok {
			saved[k] = v
		}
		_ = os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range envVars {
			_ = os.Unsetenv(k)
		}
		for k, v := range saved {
			_ = os.Setenv(k, v)
		}
	})
}

func TestConfig_Load_Defaults(t *testing.T) {
	clearEnv(t)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTPAddr != "127.0.0.1:8080" {
		t.Errorf("HTTPAddr = %q", c.HTTPAddr)
	}
	if !strings.Contains(c.PostgresDSN, "postgres://") {
		t.Errorf("PostgresDSN missing scheme: %q", c.PostgresDSN)
	}
	if c.RedisAddr != "127.0.0.1:6379" {
		t.Errorf("RedisAddr = %q", c.RedisAddr)
	}
	if c.RefreshStream != "stream:cohort-refresh" {
		t.Errorf("RefreshStream = %q", c.RefreshStream)
	}
	if c.SQLStatementTimeout != 30*time.Second {
		t.Errorf("SQLStatementTimeout = %v", c.SQLStatementTimeout)
	}
	if c.PostgresReaderDSN != c.PostgresDSN {
		t.Errorf("reader DSN did not fall back to primary")
	}
}

func TestConfig_Load_BadDuration(t *testing.T) {
	clearEnv(t)
	t.Setenv("SQL_STATEMENT_TIMEOUT", "notaduration")
	_, err := Load()
	if err == nil {
		t.Fatal("want error for bad duration")
	}
}

func TestConfig_Load_ReaderDSNOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("POSTGRES_DSN", "postgres://primary")
	t.Setenv("POSTGRES_READER_DSN", "postgres://reader")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.PostgresReaderDSN != "postgres://reader" {
		t.Errorf("PostgresReaderDSN = %q, want override", c.PostgresReaderDSN)
	}
}

func TestConfig_Validate(t *testing.T) {
	base := Config{
		HTTPAddr:             ":8080",
		PostgresDSN:          "postgres://x",
		RedisAddr:            "127.0.0.1:6379",
		RefreshStream:        "s",
		RefreshConsumerGroup: "g",
		SQLStatementTimeout:  1 * time.Second,
	}
	tests := []struct {
		name    string
		mut     func(*Config)
		wantErr string
	}{
		{"HTTPAddr", func(c *Config) { c.HTTPAddr = "" }, "HTTP_ADDR"},
		{"PostgresDSN", func(c *Config) { c.PostgresDSN = "" }, "POSTGRES_DSN"},
		{"RedisAddr", func(c *Config) { c.RedisAddr = "" }, "REDIS_ADDR"},
		{"RefreshStream", func(c *Config) { c.RefreshStream = "" }, "REFRESH_STREAM"},
		{"RefreshConsumerGroup", func(c *Config) { c.RefreshConsumerGroup = "" }, "REFRESH_CONSUMER_GROUP"},
		{"SQLStatementTimeout", func(c *Config) { c.SQLStatementTimeout = 0 }, "SQL_STATEMENT_TIMEOUT"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mut(&c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("want error mentioning %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err %q missing %q", err.Error(), tc.wantErr)
			}
		})
	}
}
