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
	if _, err = s.db.ExecContext(ctx, `CREATE TRIGGER reject_bad BEFORE INSERT ON events WHEN NEW.event_id='bad' BEGIN SELECT RAISE(ABORT, 'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	bad := []Event{{EventID: "new"}, {EventID: "bad"}}
	if err = s.Upsert(ctx, "account", bad); err == nil {
		t.Fatal("failed insert accepted")
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

func TestUpsertUnknownDates(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "events.sqlite"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, dates := range []struct{ start, end string }{
		{"2026-08-13T00:00:00Z", "2026-08-13T01:00:00Z"},
		{"TBD", "not a timestamp"},
		{"", ""},
	} {
		if err := s.Upsert(ctx, "account", []Event{{EventID: "a", Start: ptr(dates.start), End: ptr(dates.end)}}); err != nil {
			t.Fatal(err)
		}
		var startNull, endNull bool
		if err := s.db.QueryRowContext(ctx, "SELECT start IS NULL, end IS NULL FROM events WHERE event_id='a'").Scan(&startNull, &endNull); err != nil {
			t.Fatal(err)
		}
		unknown := dates.start != "2026-08-13T00:00:00Z"
		if startNull != unknown || endNull != unknown {
			t.Fatalf("start null=%t end null=%t, want %t", startNull, endNull, unknown)
		}
	}
}
