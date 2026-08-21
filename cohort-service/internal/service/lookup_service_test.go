package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestLookupService_IsMember(t *testing.T) {
	mr := newFakeMembershipRepo()
	svc := NewLookupService(mr)
	ctx := context.Background()

	cohortID := uuid.New()
	_ = mr.Apply(ctx, cohortID, []string{"present"}, nil)

	got, err := svc.IsMember(ctx, cohortID, "present")
	if err != nil || !got {
		t.Fatalf("present: got=%v err=%v", got, err)
	}
	got, err = svc.IsMember(ctx, cohortID, "absent")
	if err != nil || got {
		t.Fatalf("absent: got=%v err=%v", got, err)
	}
}

func TestLookupService_CohortsForUser(t *testing.T) {
	mr := newFakeMembershipRepo()
	svc := NewLookupService(mr)
	ctx := context.Background()

	c1, c2 := uuid.New(), uuid.New()
	_ = mr.Apply(ctx, c1, []string{"u"}, nil)
	_ = mr.Apply(ctx, c2, []string{"u"}, nil)

	ids, err := svc.CohortsForUser(ctx, "u")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len = %d, want 2 (%v)", len(ids), ids)
	}
}

func TestLookupService_UnknownUser(t *testing.T) {
	svc := NewLookupService(newFakeMembershipRepo())
	ids, err := svc.CohortsForUser(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("len = %d, want 0", len(ids))
	}
}
