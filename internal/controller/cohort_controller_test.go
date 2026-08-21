package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/zomato/cohort-service/internal/domain"
	"github.com/zomato/cohort-service/internal/service"
)

type fakeCohortSvc struct {
	created  domain.Cohort
	list     []domain.Cohort
	getErr   error
	createIn service.CreateInput
	createErr error
	deleteErr error
	refreshRun domain.RefreshRun
	refreshErr error
}

func (f *fakeCohortSvc) Create(_ context.Context, in service.CreateInput) (domain.Cohort, error) {
	f.createIn = in
	if f.createErr != nil {
		return domain.Cohort{}, f.createErr
	}
	f.created.Name = in.Name
	f.created.Type = in.Type
	f.created.ID = uuid.New()
	return f.created, nil
}
func (f *fakeCohortSvc) Get(_ context.Context, id uuid.UUID) (domain.Cohort, error) {
	if f.getErr != nil {
		return domain.Cohort{}, f.getErr
	}
	return domain.Cohort{ID: id, Name: "x", Type: domain.CohortTypeStatic}, nil
}
func (f *fakeCohortSvc) List(_ context.Context) ([]domain.Cohort, error) { return f.list, nil }
func (f *fakeCohortSvc) Delete(_ context.Context, _ uuid.UUID) error    { return f.deleteErr }
func (f *fakeCohortSvc) EnqueueRefresh(_ context.Context, cohortID uuid.UUID) (domain.RefreshRun, error) {
	if f.refreshErr != nil {
		return domain.RefreshRun{}, f.refreshErr
	}
	f.refreshRun.ID = uuid.New()
	f.refreshRun.CohortID = cohortID
	return f.refreshRun, nil
}

type fakeLookupSvc struct {
	member   bool
	cohorts  []uuid.UUID
	err      error
}

func (f *fakeLookupSvc) IsMember(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
	return f.member, f.err
}
func (f *fakeLookupSvc) CohortsForUser(_ context.Context, _ string) ([]uuid.UUID, error) {
	return f.cohorts, f.err
}

func newTestRouter(cs CohortServiceAPI, ls LookupServiceAPI) http.Handler {
	return NewRouter(New(cs, ls))
}

func TestCreate_Static_OK(t *testing.T) {
	cs, ls := &fakeCohortSvc{}, &fakeLookupSvc{}
	body := `{"name":"n","type":"static","static_users":["u1"]}`
	req := httptest.NewRequest(http.MethodPost, "/cohorts", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	newTestRouter(cs, ls).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if cs.createIn.Name != "n" || cs.createIn.Type != domain.CohortTypeStatic {
		t.Fatalf("service got: %+v", cs.createIn)
	}
}

func TestCreate_BadType_400(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/cohorts",
		bytes.NewBufferString(`{"name":"n","type":"garbage"}`))
	rec := httptest.NewRecorder()
	newTestRouter(&fakeCohortSvc{}, &fakeLookupSvc{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestCreate_MalformedJSON_400(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/cohorts", bytes.NewBufferString(`{`))
	rec := httptest.NewRecorder()
	newTestRouter(&fakeCohortSvc{}, &fakeLookupSvc{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestCreate_ConflictMapping(t *testing.T) {
	cs := &fakeCohortSvc{createErr: domain.ErrConflict}
	req := httptest.NewRequest(http.MethodPost, "/cohorts",
		bytes.NewBufferString(`{"name":"n","type":"static","static_users":["u"]}`))
	rec := httptest.NewRecorder()
	newTestRouter(cs, &fakeLookupSvc{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestCreate_InvalidDefinitionMapping(t *testing.T) {
	cs := &fakeCohortSvc{createErr: domain.ErrInvalidDefinition}
	req := httptest.NewRequest(http.MethodPost, "/cohorts",
		bytes.NewBufferString(`{"name":"n","type":"static","static_users":["u"]}`))
	rec := httptest.NewRecorder()
	newTestRouter(cs, &fakeLookupSvc{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestGet_NotFound_404(t *testing.T) {
	cs := &fakeCohortSvc{getErr: domain.ErrNotFound}
	req := httptest.NewRequest(http.MethodGet, "/cohorts/"+uuid.NewString(), nil)
	rec := httptest.NewRecorder()
	newTestRouter(cs, &fakeLookupSvc{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestGet_BadUUID_400(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/cohorts/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	newTestRouter(&fakeCohortSvc{}, &fakeLookupSvc{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestDelete_NoContent(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/cohorts/"+uuid.NewString(), nil)
	rec := httptest.NewRecorder()
	newTestRouter(&fakeCohortSvc{}, &fakeLookupSvc{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestRefresh_Accepted(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/cohorts/"+uuid.NewString()+"/refresh", nil)
	rec := httptest.NewRecorder()
	newTestRouter(&fakeCohortSvc{}, &fakeLookupSvc{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestRefresh_UnknownCohort_404(t *testing.T) {
	cs := &fakeCohortSvc{refreshErr: domain.ErrNotFound}
	req := httptest.NewRequest(http.MethodPost, "/cohorts/"+uuid.NewString()+"/refresh", nil)
	rec := httptest.NewRecorder()
	newTestRouter(cs, &fakeLookupSvc{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestIsMember_TrueFalse(t *testing.T) {
	for _, want := range []bool{true, false} {
		ls := &fakeLookupSvc{member: want}
		req := httptest.NewRequest(http.MethodGet, "/cohorts/"+uuid.NewString()+"/members/u1", nil)
		rec := httptest.NewRecorder()
		newTestRouter(&fakeCohortSvc{}, ls).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d", rec.Code)
		}
		var got isMemberResponse
		_ = json.NewDecoder(rec.Body).Decode(&got)
		if got.Member != want {
			t.Fatalf("member = %v, want %v", got.Member, want)
		}
	}
}

func TestUserCohorts(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	ls := &fakeLookupSvc{cohorts: ids}
	req := httptest.NewRequest(http.MethodGet, "/users/u1/cohorts", nil)
	rec := httptest.NewRecorder()
	newTestRouter(&fakeCohortSvc{}, ls).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var got cohortsForUserResponse
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if len(got.CohortIDs) != 2 {
		t.Fatalf("len = %d, want 2", len(got.CohortIDs))
	}
}

func TestUserCohorts_EmptyReturnsArray(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users/u1/cohorts", nil)
	rec := httptest.NewRecorder()
	newTestRouter(&fakeCohortSvc{}, &fakeLookupSvc{}).ServeHTTP(rec, req)
	// The body should not be null; an empty array is easier for clients.
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"cohort_ids":[]`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLookup_InternalError_500(t *testing.T) {
	ls := &fakeLookupSvc{err: errors.New("boom")}
	req := httptest.NewRequest(http.MethodGet, "/cohorts/"+uuid.NewString()+"/members/u1", nil)
	rec := httptest.NewRecorder()
	newTestRouter(&fakeCohortSvc{}, ls).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d", rec.Code)
	}
}
