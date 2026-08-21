# Cohort Service

Central service for defining user cohorts (static lists or SQL queries) and
answering the two hot-path questions:

- Is user `u` in cohort `c`?
- Which cohorts does user `u` belong to?

Written in Go using Postgres (metadata), Redis (membership sets + refresh
stream), and chi (HTTP router). Layered as controller → service → repository.

## Prerequisites

- Docker Desktop (for Postgres + Redis containers)
- Go 1.25+
- `curl` and `jq` for the manual runbook below

## Layout

```
cmd/server/                # entrypoint (HTTP + worker in one process)
internal/
  config/                  # env-driven config
  domain/                  # Cohort, RefreshRun, sentinel errors
  controller/              # HTTP handlers, DTOs, error mapping, router
  service/                 # cohort, lookup, refresh — with port interfaces
  repository/
    postgres/              # metadata CRUD, embedded migrations, user SQL execution
    redis/                 # membership sets + Redis Streams job queue
  worker/                  # long-running refresh consumer loop
```

## Quick start

```bash
make up          # docker-compose up -d postgres redis (waits for healthy)
make run         # starts server on :8080; runs migrations, then HTTP + worker
```

Server logs will include `server listening on 127.0.0.1:8080` and
`worker <name>: starting consume loop`.

## Manual demo runbook

Once `make run` is going, in another terminal:

### 1. Create a static cohort and refresh it

```bash
STATIC_ID=$(curl -s -X POST localhost:8080/cohorts \
  -H 'content-type: application/json' \
  -d '{"name":"vip-manual","type":"static","static_users":["u_000001","u_000002","u_000003"]}' \
  | jq -r .id)
curl -s -X POST localhost:8080/cohorts/$STATIC_ID/refresh
```

### 2. Create a SQL cohort and refresh it

The migration seeds 10,000 mock users across six cities.

```bash
SQL_ID=$(curl -s -X POST localhost:8080/cohorts \
  -H 'content-type: application/json' \
  -d '{"name":"delhi-high-ltv","type":"sql",
       "sql_query":"SELECT id FROM users WHERE city='\''Delhi'\'' AND ltv_cents > 1000000"}' \
  | jq -r .id)
curl -s -X POST localhost:8080/cohorts/$SQL_ID/refresh

# Wait a moment for the worker, then confirm size is populated:
sleep 1
curl -s localhost:8080/cohorts/$SQL_ID | jq '{size,last_refreshed_at}'
```

### 3. Point membership check (hot path #1)

```bash
curl -s localhost:8080/cohorts/$STATIC_ID/members/u_000001   # {"member":true}
curl -s localhost:8080/cohorts/$STATIC_ID/members/u_999999   # {"member":false}
```

### 4. Reverse lookup (hot path #2)

```bash
curl -s localhost:8080/users/u_000001/cohorts | jq
# {"cohort_ids": ["<STATIC_ID>"]}
```

### 5. Prove diff is correct (add + remove)

```bash
# Mutate a user so they no longer match the SQL, then refresh.
SAMPLE=$(docker exec cohort-service-postgres-1 psql -U cohort -d cohort -tAc \
  "SELECT id FROM users WHERE city='Delhi' AND ltv_cents > 1000000 LIMIT 1")

docker exec cohort-service-postgres-1 psql -U cohort -d cohort -c \
  "UPDATE users SET ltv_cents = 0 WHERE id = '$SAMPLE';"

curl -s -X POST localhost:8080/cohorts/$SQL_ID/refresh
sleep 1

curl -s localhost:8080/cohorts/$SQL_ID/members/$SAMPLE       # {"member":false}
curl -s localhost:8080/users/$SAMPLE/cohorts | jq            # SQL cohort no longer in list
```

### 6. Inspect Redis directly (optional)

```bash
docker exec cohort-service-redis-1 redis-cli SISMEMBER cohort:$STATIC_ID:members u_000001
docker exec cohort-service-redis-1 redis-cli SMEMBERS user:u_000001:cohorts
docker exec cohort-service-redis-1 redis-cli XLEN stream:cohort-refresh
```

## API

| Method | Path                                  | Purpose                             |
|--------|---------------------------------------|-------------------------------------|
| GET    | `/healthz`                            | liveness                            |
| POST   | `/cohorts`                            | create (static or SQL)              |
| GET    | `/cohorts`                            | list                                |
| GET    | `/cohorts/{id}`                       | read                                |
| DELETE | `/cohorts/{id}`                       | delete (purges Redis + metadata)    |
| POST   | `/cohorts/{id}/refresh`               | enqueue a refresh job               |
| GET    | `/cohorts/{id}/members/{userId}`      | **point membership check**          |
| GET    | `/users/{userId}/cohorts`             | **reverse lookup**                  |

Errors follow domain sentinels: `ErrNotFound` → 404, `ErrConflict` → 409,
`ErrInvalidDefinition` → 400.

## Configuration

Env-driven, with defaults for local docker-compose. See `internal/config/config.go`.

| Var                        | Default                                                  |
|----------------------------|----------------------------------------------------------|
| `HTTP_ADDR`                | `127.0.0.1:8080`                                         |
| `POSTGRES_DSN`             | `postgres://cohort:cohort@127.0.0.1:5432/cohort?sslmode=disable` |
| `POSTGRES_READER_DSN`      | falls back to `POSTGRES_DSN`                             |
| `REDIS_ADDR`               | `127.0.0.1:6379`                                         |
| `REFRESH_STREAM`           | `stream:cohort-refresh`                                  |
| `REFRESH_CONSUMER_GROUP`   | `cohort-refreshers`                                      |
| `REFRESH_CONSUMER_NAME`    | `worker-1`                                               |
| `SQL_STATEMENT_TIMEOUT`    | `30s`                                                    |

## Testing

```bash
make test         # full suite (requires postgres + redis via `make up`)
make test-short   # unit tests only; skips integration + e2e
make lint         # golangci-lint (falls back to `go vet` if not installed)
```

## Notes on the production design

Included in the design doc, not implemented here:

- Scheduled per-cohort refresh (cron loop → publish to stream).
- Read-only Postgres role for SQL execution (migration 0003 creates the role;
  the POC still uses the primary pool as the reader for simplicity).
- Roaring Bitmaps for very dense (>5M) cohorts.
- Multi-worker consumer scaling — supported by Redis Streams consumer groups
  today, but the POC runs one consumer in-process.
- Metrics / tracing / auth / rate limiting.

## Shutdown

`SIGINT` / `SIGTERM` triggers graceful shutdown: HTTP stops accepting new
connections, the worker's consume loop exits at its next block-timeout tick,
Postgres/Redis pools close.
