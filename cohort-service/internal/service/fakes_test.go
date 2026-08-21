package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/zomato/cohort-service/internal/domain"
	streamrepo "github.com/zomato/cohort-service/internal/repository/redis"
)

// --- fake cohort repository ------------------------------------------------

type fakeCohortRepo struct {
	mu       sync.Mutex
	cohorts  map[uuid.UUID]domain.Cohort
	runs     map[uuid.UUID]domain.RefreshRun
	byName   map[string]uuid.UUID
	failNext error
}

func newFakeCohortRepo() *fakeCohortRepo {
	return &fakeCohortRepo{
		cohorts: make(map[uuid.UUID]domain.Cohort),
		runs:    make(map[uuid.UUID]domain.RefreshRun),
		byName:  make(map[string]uuid.UUID),
	}
}

func (f *fakeCohortRepo) Create(_ context.Context, c domain.Cohort) (domain.Cohort, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byName[c.Name]; ok {
		return domain.Cohort{}, domain.ErrConflict
	}
	c.ID = uuid.New()
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	f.cohorts[c.ID] = c
	f.byName[c.Name] = c.ID
	return c, nil
}

func (f *fakeCohortRepo) Get(_ context.Context, id uuid.UUID) (domain.Cohort, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.cohorts[id]
	if !ok {
		return domain.Cohort{}, domain.ErrNotFound
	}
	return c, nil
}

func (f *fakeCohortRepo) List(_ context.Context) ([]domain.Cohort, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.Cohort, 0, len(f.cohorts))
	for _, c := range f.cohorts {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeCohortRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.cohorts[id]
	if !ok {
		return domain.ErrNotFound
	}
	delete(f.cohorts, id)
	delete(f.byName, c.Name)
	return nil
}

func (f *fakeCohortRepo) MarkRefreshed(_ context.Context, id uuid.UUID, size int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.cohorts[id]
	if !ok {
		return domain.ErrNotFound
	}
	now := time.Now()
	c.Size = size
	c.LastRefreshedAt = &now
	f.cohorts[id] = c
	return nil
}

func (f *fakeCohortRepo) CreateRefreshRun(_ context.Context, cohortID uuid.UUID) (domain.RefreshRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return domain.RefreshRun{}, err
	}
	r := domain.RefreshRun{
		ID: uuid.New(), CohortID: cohortID, Status: domain.RefreshStatusPending,
		StartedAt: time.Now(),
	}
	f.runs[r.ID] = r
	return r, nil
}

func (f *fakeCohortRepo) FinishRefreshRun(_ context.Context, id uuid.UUID, st domain.RefreshStatus, added, removed int, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.runs[id]
	if !ok {
		return domain.ErrNotFound
	}
	r.Status = st
	r.AddedCount = added
	r.RemovedCount = removed
	r.Error = errMsg
	now := time.Now()
	r.FinishedAt = &now
	f.runs[id] = r
	return nil
}

func (f *fakeCohortRepo) LatestRefreshRun(_ context.Context, cohortID uuid.UUID) (domain.RefreshRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var latest domain.RefreshRun
	var found bool
	for _, r := range f.runs {
		if r.CohortID != cohortID {
			continue
		}
		if !found || r.StartedAt.After(latest.StartedAt) {
			latest = r
			found = true
		}
	}
	if !found {
		return domain.RefreshRun{}, domain.ErrNotFound
	}
	return latest, nil
}

// --- fake membership repository -------------------------------------------

type fakeMembershipRepo struct {
	mu      sync.Mutex
	forward map[uuid.UUID]map[string]struct{} // cohortID -> set of userIDs
	reverse map[string]map[uuid.UUID]struct{} // userID -> set of cohortIDs

	purgeCalls int
	applyCalls int
}

func newFakeMembershipRepo() *fakeMembershipRepo {
	return &fakeMembershipRepo{
		forward: make(map[uuid.UUID]map[string]struct{}),
		reverse: make(map[string]map[uuid.UUID]struct{}),
	}
}

func (f *fakeMembershipRepo) Apply(_ context.Context, cohortID uuid.UUID, adds, removes []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applyCalls++
	if f.forward[cohortID] == nil {
		f.forward[cohortID] = map[string]struct{}{}
	}
	for _, u := range adds {
		f.forward[cohortID][u] = struct{}{}
		if f.reverse[u] == nil {
			f.reverse[u] = map[uuid.UUID]struct{}{}
		}
		f.reverse[u][cohortID] = struct{}{}
	}
	for _, u := range removes {
		delete(f.forward[cohortID], u)
		if f.reverse[u] != nil {
			delete(f.reverse[u], cohortID)
		}
	}
	return nil
}

func (f *fakeMembershipRepo) CurrentMembers(_ context.Context, cohortID uuid.UUID) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.forward[cohortID]))
	for u := range f.forward[cohortID] {
		out = append(out, u)
	}
	return out, nil
}

func (f *fakeMembershipRepo) Purge(_ context.Context, cohortID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.purgeCalls++
	for u := range f.forward[cohortID] {
		if f.reverse[u] != nil {
			delete(f.reverse[u], cohortID)
		}
	}
	delete(f.forward, cohortID)
	return nil
}

func (f *fakeMembershipRepo) IsMember(_ context.Context, cohortID uuid.UUID, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.forward[cohortID][userID]
	return ok, nil
}

func (f *fakeMembershipRepo) CohortsForUser(_ context.Context, userID string) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]uuid.UUID, 0, len(f.reverse[userID]))
	for c := range f.reverse[userID] {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeMembershipRepo) Size(_ context.Context, cohortID uuid.UUID) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.forward[cohortID]), nil
}

// --- fake stream producer -------------------------------------------------

type fakeProducer struct {
	mu       sync.Mutex
	jobs     []streamrepo.RefreshJob
	failNext error
}

func (f *fakeProducer) Publish(_ context.Context, job streamrepo.RefreshJob) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return "", err
	}
	f.jobs = append(f.jobs, job)
	return "msg-" + uuid.NewString(), nil
}

// --- fake user repository -------------------------------------------------

type fakeUserRepo struct {
	rows []string
	err  error
}

func (f *fakeUserRepo) ResolveSQL(_ context.Context, _ string, emit func(string) error) error {
	if f.err != nil {
		return f.err
	}
	for _, r := range f.rows {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}
