package worker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zomato/cohort-service/internal/controller"
	pgrepo "github.com/zomato/cohort-service/internal/repository/postgres"
	rdsrepo "github.com/zomato/cohort-service/internal/repository/redis"
	"github.com/zomato/cohort-service/internal/service"
	"github.com/zomato/cohort-service/internal/worker"
)

// TestE2E boots the full stack (HTTP + worker) against real Postgres and Redis,
// then drives the demo runbook: create static cohort, create SQL cohort,
// refresh both, and verify both lookup APIs return correct answers.
func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e skipped in -short")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	pgDSN := getenv("POSTGRES_DSN", "postgres://cohort:cohort@127.0.0.1:5432/cohort?sslmode=disable")
	rdsAddr := getenv("REDIS_ADDR", "127.0.0.1:6379")

	pool, err := pgrepo.NewPool(ctx, pgDSN)
	if err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	defer pool.Close()
	if err := pgrepo.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE cohorts, cohort_refresh_runs RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	rds, err := rdsrepo.NewClient(ctx, rdsAddr)
	if err != nil {
		t.Skipf("redis unreachable: %v", err)
	}
	defer rds.Close()
	if err := rds.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushdb: %v", err)
	}

	streamName := fmt.Sprintf("stream:e2e:%d", time.Now().UnixNano())
	stream, err := rdsrepo.NewStreamRepo(ctx, rds, streamName, "e2e-group")
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	cohortRepo := pgrepo.NewCohortRepo(pool)
	userRepo := pgrepo.NewUserRepo(pool, 30*time.Second)
	memRepo := rdsrepo.NewMembershipRepo(rds)

	cohortSvc := service.NewCohortService(cohortRepo, memRepo, stream)
	lookupSvc := service.NewLookupService(memRepo)
	refreshSvc := service.NewRefreshService(cohortRepo, userRepo, memRepo)

	ctrl := controller.New(cohortSvc, lookupSvc)

	// Pick a free port so parallel runs don't collide.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	base := "http://" + ln.Addr().String()
	srv := &http.Server{Handler: controller.NewRouter(ctrl), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	// Worker with a silent logger to keep test output clean.
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	go worker.NewRefresher(stream, refreshSvc, "e2e-w-1", log.New(io.Discard, "", 0)).Run(workerCtx)

	// --- Static cohort ---
	staticID := createCohort(t, base, map[string]any{
		"name": "vip-manual", "type": "static", "static_users": []string{"u_000001", "u_000002", "u_000003"},
	})
	refreshCohort(t, base, staticID)
	waitForSize(t, base, staticID, 3)

	// --- SQL cohort ---
	sqlID := createCohort(t, base, map[string]any{
		"name":      "delhi-any-ltv",
		"type":      "sql",
		"sql_query": "SELECT id FROM users WHERE city='Delhi' LIMIT 5",
	})
	refreshCohort(t, base, sqlID)
	waitForSize(t, base, sqlID, 5)

	// --- Point membership check ---
	if !isMember(t, base, staticID, "u_000001") {
		t.Fatal("u_000001 should be in static cohort")
	}
	if isMember(t, base, staticID, "u_999999") {
		t.Fatal("u_999999 should NOT be in static cohort")
	}

	// --- Reverse lookup ---
	got := userCohorts(t, base, "u_000001")
	if len(got) != 1 || got[0] != staticID {
		t.Fatalf("u_000001 cohorts = %v, want [%s]", got, staticID)
	}
}

// --- HTTP helpers -----------------------------------------------------------

func createCohort(t *testing.T, base string, payload map[string]any) uuid.UUID {
	t.Helper()
	body, _ := json.Marshal(payload)
	resp := do(t, http.MethodPost, base+"/cohorts", body)
	if resp.status != http.StatusCreated {
		t.Fatalf("create %s status = %d, body = %s", payload["name"], resp.status, resp.body)
	}
	var out struct{ ID uuid.UUID `json:"id"` }
	_ = json.Unmarshal(resp.body, &out)
	return out.ID
}

func refreshCohort(t *testing.T, base string, id uuid.UUID) {
	t.Helper()
	resp := do(t, http.MethodPost, fmt.Sprintf("%s/cohorts/%s/refresh", base, id), nil)
	if resp.status != http.StatusAccepted {
		t.Fatalf("refresh status = %d, body = %s", resp.status, resp.body)
	}
}

func waitForSize(t *testing.T, base string, id uuid.UUID, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp := do(t, http.MethodGet, fmt.Sprintf("%s/cohorts/%s", base, id), nil)
		if resp.status == http.StatusOK {
			var c struct{ Size int `json:"size"` }
			_ = json.Unmarshal(resp.body, &c)
			if c.Size == want {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("cohort %s never reached size %d", id, want)
}

func isMember(t *testing.T, base string, cohortID uuid.UUID, userID string) bool {
	t.Helper()
	resp := do(t, http.MethodGet, fmt.Sprintf("%s/cohorts/%s/members/%s", base, cohortID, userID), nil)
	if resp.status != http.StatusOK {
		t.Fatalf("is-member status = %d, body = %s", resp.status, resp.body)
	}
	var out struct{ Member bool `json:"member"` }
	_ = json.Unmarshal(resp.body, &out)
	return out.Member
}

func userCohorts(t *testing.T, base string, userID string) []uuid.UUID {
	t.Helper()
	resp := do(t, http.MethodGet, fmt.Sprintf("%s/users/%s/cohorts", base, userID), nil)
	if resp.status != http.StatusOK {
		t.Fatalf("user-cohorts status = %d, body = %s", resp.status, resp.body)
	}
	var out struct{ CohortIDs []uuid.UUID `json:"cohort_ids"` }
	_ = json.Unmarshal(resp.body, &out)
	return out.CohortIDs
}

type httpResp struct {
	status int
	body   []byte
}

func do(t *testing.T, method, url string, body []byte) httpResp {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, reqBody)
	req.Header.Set("content-type", "application/json")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http %s %s: %v", method, url, err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return httpResp{status: r.StatusCode, body: b}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
