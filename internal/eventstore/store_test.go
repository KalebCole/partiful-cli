package eventstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func ptr(s string) *string { return &s }
func TestUpsertAndFilters(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events.sqlite")
	s, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	events := []Event{{EventID: "a", Title: ptr("First"), Start: ptr("2026-08-02T01:00:00+02:00"), MyRSVP: ptr("going")}, {EventID: "b", Start: ptr("2026-08-03T00:00:00Z"), MyRSVP: ptr("declined")}, {EventID: "c", MyRSVP: ptr("going")}}
	for i := 0; i < 2; i++ {
		if err = s.Upsert(ctx, "account", events); err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.List(ctx, Filter{Limit: 50})
	if err != nil || len(items) != 3 {
		t.Fatalf("idempotency: %v %v", items, err)
	}
	events[0].Title = ptr("Updated")
	if err = s.Upsert(ctx, "account", events[:1]); err != nil {
		t.Fatal(err)
	}
	items, err = s.List(ctx, Filter{Status: "going", Since: "2026-08-01T23:00:00Z", Until: "2026-08-02T01:00:00+02:00", Limit: 50})
	if err != nil || len(items) != 1 || *items[0].Title != "Updated" {
		t.Fatalf("filters: %v %v", items, err)
	}
	items, err = s.List(ctx, Filter{Status: "going", Limit: 1})
	if err != nil || len(items) != 1 {
		t.Fatalf("limit: %v %v", items, err)
	}
	if err = s.Upsert(ctx, "other", events); !errors.Is(err, ErrAccountMismatch) {
		t.Fatalf("account isolation: %v", err)
	}
	bad := []Event{{EventID: "new"}, {EventID: "bad", Start: ptr("invalid")}}
	if err = s.Upsert(ctx, "account", bad); err == nil {
		t.Fatal("invalid time accepted")
	}
	items, err = s.List(ctx, Filter{Limit: 50})
	if err != nil || len(items) != 3 {
		t.Fatalf("rollback: %v %v", items, err)
	}
	s.Close()
	s, err = Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.List(ctx, Filter{Limit: 50}); err != nil {
		t.Fatal(err)
	}
	if err = s.Upsert(ctx, "account", events); err == nil {
		t.Fatal("read-only store accepted write")
	}
}
func TestReadOnlyMissingDoesNotCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	if s, err := Open(path, true); err == nil {
		s.Close()
		t.Fatal("opened missing database")
	}
}
