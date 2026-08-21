#!/usr/bin/env bash
# End-to-end demo: brings up compose, migrates, boots server, drives runbook.
# Exits non-zero on any assertion failure. Cleans up server + compose on exit.
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"
COMPOSE="${COMPOSE:-docker-compose}"
BIN="./bin/cohort-server-demo"

log() { printf '\033[1;34m[demo]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[demo FAIL]\033[0m %s\n' "$*" >&2; exit 1; }

SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    log "stopping server (pid $SERVER_PID)"
    kill "$SERVER_PID" 2>/dev/null || true
    for _ in $(seq 1 10); do
      kill -0 "$SERVER_PID" 2>/dev/null || break
      sleep 0.3
    done
    kill -9 "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# Fail fast if 8080 is already occupied — a leftover from an earlier attempt.
if lsof -i :8080 >/dev/null 2>&1; then
  fail "port 8080 already in use; run: lsof -i :8080 and kill the pid"
fi

log "starting postgres + redis"
$COMPOSE up -d postgres redis >/dev/null

log "waiting for postgres"
for _ in $(seq 1 30); do
  if docker exec cohort-service-postgres-1 pg_isready -U cohort -d cohort >/dev/null 2>&1; then break; fi
  sleep 1
done
log "waiting for redis"
for _ in $(seq 1 30); do
  if docker exec cohort-service-redis-1 redis-cli ping >/dev/null 2>&1; then break; fi
  sleep 1
done

log "building server binary"
mkdir -p bin
go build -o "$BIN" ./cmd/server

log "running migrations"
"$BIN" -migrate

log "resetting cohort state (idempotent demo)"
docker exec cohort-service-postgres-1 psql -U cohort -d cohort -c \
  "TRUNCATE cohorts, cohort_refresh_runs RESTART IDENTITY CASCADE" >/dev/null
docker exec cohort-service-redis-1 redis-cli FLUSHDB >/dev/null

log "booting server"
"$BIN" >/tmp/cohort-demo.log 2>&1 &
SERVER_PID=$!

log "waiting for /healthz"
for _ in $(seq 1 30); do
  if curl -fs "$BASE/healthz" >/dev/null 2>&1; then break; fi
  # bail early if the server died
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    fail "server exited before becoming ready; see /tmp/cohort-demo.log"
  fi
  sleep 1
done
curl -fs "$BASE/healthz" >/dev/null || fail "server did not come up; see /tmp/cohort-demo.log"

log "creating static cohort"
STATIC_ID=$(curl -fs -X POST "$BASE/cohorts" \
  -H 'content-type: application/json' \
  -d '{"name":"vip-manual","type":"static","static_users":["u_000001","u_000002","u_000003"]}' \
  | jq -r .id)
[[ -n "$STATIC_ID" && "$STATIC_ID" != "null" ]] || fail "static cohort create failed"
curl -fs -X POST "$BASE/cohorts/$STATIC_ID/refresh" >/dev/null

log "creating SQL cohort"
SQL_ID=$(curl -fs -X POST "$BASE/cohorts" \
  -H 'content-type: application/json' \
  -d "{\"name\":\"delhi-sample\",\"type\":\"sql\",\"sql_query\":\"SELECT id FROM users WHERE city='Delhi' LIMIT 5\"}" \
  | jq -r .id)
[[ -n "$SQL_ID" && "$SQL_ID" != "null" ]] || fail "sql cohort create failed"
curl -fs -X POST "$BASE/cohorts/$SQL_ID/refresh" >/dev/null

log "waiting for refreshes to land"
STATIC_SIZE=0; SQL_SIZE=0
for _ in $(seq 1 20); do
  STATIC_SIZE=$(curl -fs "$BASE/cohorts/$STATIC_ID" | jq -r .size)
  SQL_SIZE=$(curl -fs "$BASE/cohorts/$SQL_ID" | jq -r .size)
  [[ "$STATIC_SIZE" == "3" && "$SQL_SIZE" == "5" ]] && break
  sleep 0.5
done
[[ "$STATIC_SIZE" == "3" ]] || fail "static size = $STATIC_SIZE, want 3"
[[ "$SQL_SIZE" == "5" ]] || fail "sql size = $SQL_SIZE, want 5"

log "verifying point membership"
[[ "$(curl -fs "$BASE/cohorts/$STATIC_ID/members/u_000001" | jq -r .member)" == "true" ]] \
  || fail "u_000001 should be a member"
[[ "$(curl -fs "$BASE/cohorts/$STATIC_ID/members/u_999999" | jq -r .member)" == "false" ]] \
  || fail "u_999999 should NOT be a member"

log "verifying reverse lookup"
COUNT=$(curl -fs "$BASE/users/u_000001/cohorts" | jq '.cohort_ids | length')
[[ "$COUNT" == "1" ]] || fail "u_000001 cohort count = $COUNT, want 1"

log "verifying refresh diff"
docker exec cohort-service-postgres-1 psql -U cohort -d cohort -c \
  "UPDATE users SET city='Mumbai' WHERE id = (SELECT id FROM users WHERE city='Delhi' LIMIT 1)" >/dev/null
curl -fs -X POST "$BASE/cohorts/$SQL_ID/refresh" >/dev/null
sleep 1
SIZE_AFTER=$(curl -fs "$BASE/cohorts/$SQL_ID" | jq -r .size)
[[ "$SIZE_AFTER" == "5" ]] || fail "size after diff refresh = $SIZE_AFTER, want 5"

printf '\n\033[1;32m[demo PASS]\033[0m all assertions ok\n'
