package domain

import (
	"errors"
	"testing"
)

func TestParseCohortType(t *testing.T) {
	tests := []struct {
		in      string
		want    CohortType
		wantErr bool
	}{
		{"static", CohortTypeStatic, false},
		{"sql", CohortTypeSQL, false},
		{"", "", true},
		{"unknown", "", true},
		{"STATIC", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseCohortType(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				if !errors.Is(err, ErrInvalidDefinition) {
					t.Fatalf("err = %v, want wrap of ErrInvalidDefinition", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseRefreshStatus(t *testing.T) {
	tests := []struct {
		in      string
		want    RefreshStatus
		wantErr bool
	}{
		{"pending", RefreshStatusPending, false},
		{"running", RefreshStatusRunning, false},
		{"succeeded", RefreshStatusSucceeded, false},
		{"failed", RefreshStatusFailed, false},
		{"", "", true},
		{"nope", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseRefreshStatus(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
