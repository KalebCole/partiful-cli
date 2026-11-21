package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
	"github.com/KalebCole/partiful-cli/internal/eventstore"
)

func TestOfflineSyncAndRead(t *testing.T) {
	for _, when := range []string{"past", "upcoming"} {
		t.Run(when, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "events.sqlite")
			calls := 0
			deps := app.Dependencies{Files: fakeFilesystem{readFile: func(string) ([]byte, error) {
				return []byte(`{"accessToken":"private-access-token","userId":"private-current-account","expiresAt":"2026-08-12T02:00:00Z"}`), nil
			}}, CredentialsPath: "/config/partiful/credentials.json", Now: func() time.Time { return time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC) }, HTTP: scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
				calls++
				endpoint := "getMyPastEventsForHomePage"
				if when == "upcoming" {
					endpoint = "getMyUpcomingEventsForHomePage"
				}
				if r.URL.Path != "/"+endpoint {
					t.Fatalf("unexpected call %s", r.URL.Path)
				}
				return jsonResponse(200, `{"result":{"data":{"`+when+`Events":[{"id":"a","title":"Example","startDate":"2026-08-01T00:00:00Z","guest":{"status":"GOING"},"ownerIds":["private-owner"]},{"id":"b","guest":{"status":"DECLINED"}}]}}}`), nil
			}}}
			for i := 0; i < 2; i++ {
				deps.AuthRandom = strings.NewReader("0123456789abcdef")
				r := executeChecked(t, context.Background(), app.Request{Argv: []string{"events", "sync", "--when", when, "--db", path}}, deps)
				if r.ExitCode != 0 {
					t.Fatal(r)
				}
			}
			if calls != 2 {
				t.Fatalf("calls=%d", calls)
			}
			document, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(document, []byte("private-")) {
				t.Fatal("raw private field persisted")
			}
			offline := app.Dependencies{HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) { t.Fatal("offline accessed network"); return nil, nil }}}
			r := executeChecked(t, context.Background(), app.Request{Argv: []string{"events", "offline", "--db", path}}, offline)
			var envelope struct {
				Data struct {
					Items []struct {
						EventID string `json:"eventId"`
						MyRSVP  string `json:"myRsvp"`
					} `json:"items"`
				} `json:"data"`
			}
			json.Unmarshal([]byte(r.Stdout), &envelope)
			if r.ExitCode != 0 || len(envelope.Data.Items) != 1 || envelope.Data.Items[0].MyRSVP != "going" {
				t.Fatal(r)
			}
			r = executeChecked(t, context.Background(), app.Request{Argv: []string{"events", "offline", "--db", path, "--status", "all"}}, offline)
			json.Unmarshal([]byte(r.Stdout), &envelope)
			if len(envelope.Data.Items) != 2 {
				t.Fatal(r)
			}
		})
	}
}
func TestOfflineSyncDryRunDoesNotCreateDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")
	calls := 0
	deps := app.Dependencies{Files: fakeFilesystem{readFile: func(string) ([]byte, error) {
		return []byte(`{"accessToken":"private-access-token","userId":"private-current-account","expiresAt":"2026-08-12T02:00:00Z"}`), nil
	}}, CredentialsPath: "/config/partiful/credentials.json", Now: func() time.Time { return time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC) }, HTTP: scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/getMyPastEventsForHomePage" {
			t.Fatalf("unexpected call %s", r.URL.Path)
		}
		return jsonResponse(200, `{"result":{"data":{"pastEvents":[{"id":"a","title":"private-title","guest":{"status":"GOING"},"ownerIds":["private-owner"]},{"id":"b","guest":{"status":"DECLINED"}}]}}}`), nil
	}}}
	args := []string{"events", "sync", "--when", "past", "--limit", "1", "--db", path}
	deps.AuthRandom = strings.NewReader("0123456789abcdef")
	r := executeChecked(t, context.Background(), app.Request{Argv: append(append([]string{}, args...), "--dry-run")}, deps)
	if r.ExitCode != 0 || calls != 1 {
		t.Fatalf("dry run: exit=%d calls=%d", r.ExitCode, calls)
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if _, err := os.Stat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("dry run created database or sidecar %q: %v", suffix, err)
		}
	}
	var preview struct {
		Data struct {
			Operation string `json:"operation"`
			Input     struct {
				When  string `json:"when"`
				Limit int    `json:"limit"`
			} `json:"input"`
			Effects   []string `json:"effects"`
			Synced    int      `json:"synced"`
			Available int      `json:"available"`
			HasMore   bool     `json:"hasMore"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Data.Operation != "events.sync" || preview.Data.Input.When != "past" || preview.Data.Input.Limit != 1 || len(preview.Data.Effects) == 0 || preview.Data.Synced != 1 || preview.Data.Available != 2 || !preview.Data.HasMore {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if strings.Contains(r.Stdout, "private-") || strings.Contains(r.Stdout, path) {
		t.Fatal("preview exposed private data")
	}
	deps.AuthRandom = strings.NewReader("0123456789abcdef")
	r = executeChecked(t, context.Background(), app.Request{Argv: args}, deps)
	if r.ExitCode != 0 || calls != 2 {
		t.Fatalf("sync: exit=%d calls=%d", r.ExitCode, calls)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sync did not create database: %v", err)
	}
}

func TestOfflineErrors(t *testing.T) {
	for _, args := range [][]string{{"events", "sync"}, {"events", "offline", "--status", "attended"}, {"events", "offline", "--limit", "0"}, {"events", "offline", "--since", "yesterday"}, {"events", "offline", "--since", "2026-02-01T00:00:00Z", "--until", "2026-01-01T00:00:00Z"}, {"events", "offline", "--db", ""}} {
		r := executeChecked(t, context.Background(), app.Request{Argv: args}, app.Dependencies{})
		if r.ExitCode != 2 {
			t.Fatal(r)
		}
	}
	r := executeChecked(t, context.Background(), app.Request{Argv: []string{"events", "offline", "--db", filepath.Join(t.TempDir(), "missing.sqlite")}}, app.Dependencies{})
	if r.ExitCode != 10 {
		t.Fatal(r)
	}
}

func TestSyncRejectsProtocolChangeBeforeWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")
	deps := app.Dependencies{Files: fakeFilesystem{readFile: func(string) ([]byte, error) {
		return []byte(`{"accessToken":"private-access-token","userId":"private-current-account","expiresAt":"2026-08-12T02:00:00Z"}`), nil
	}}, CredentialsPath: "/config/partiful/credentials.json", Now: func() time.Time { return time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC) }, AuthRandom: strings.NewReader("0123456789abcdef"), HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		return jsonResponse(200, `{"result":{"data":{"pastEvents":[{"id":"a","guest":{"status":"GOING"}},{"id":"b","guest":{"status":"UNKNOWN"}}]}}}`), nil
	}}}
	r := executeChecked(t, context.Background(), app.Request{Argv: []string{"events", "sync", "--when", "past", "--limit", "1", "--db", path}}, deps)
	if r.ExitCode != 9 {
		t.Fatal(r)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed sync created store: %v", err)
	}
}

func TestOfflineSyncUnknownDates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")
	calls := 0
	deps := app.Dependencies{Files: fakeFilesystem{readFile: func(string) ([]byte, error) {
		return []byte(`{"accessToken":"private-access-token","userId":"private-current-account","expiresAt":"2026-08-12T02:00:00Z"}`), nil
	}}, CredentialsPath: "/config/partiful/credentials.json", Now: func() time.Time { return time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC) }, AuthRandom: strings.NewReader("0123456789abcdef"), HTTP: scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/getMyUpcomingEventsForHomePage" {
			t.Fatalf("unexpected call %s", r.URL.Path)
		}
		return jsonResponse(200, `{"result":{"data":{"upcomingEvents":[{"id":"tbd","startDate":"TBD","endDate":"TBD","guest":{"status":"GOING"}},{"id":"unknown-start","startDate":"not a timestamp","endDate":"2026-08-13T01:00:00Z","guest":{"status":"GOING"}},{"id":"known-start","startDate":"2026-08-13T00:00:00Z","endDate":"not a timestamp","guest":{"status":"GOING"}}]}}}`), nil
	}}}
	r := executeChecked(t, context.Background(), app.Request{Argv: []string{"events", "sync", "--when", "upcoming", "--db", path}}, deps)
	if r.ExitCode != 0 || calls != 1 {
		t.Fatalf("sync: exit=%d calls=%d", r.ExitCode, calls)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, want := range []struct {
		id                 string
		startNull, endNull bool
	}{{"tbd", true, true}, {"unknown-start", true, false}, {"known-start", false, true}} {
		var startNull, endNull bool
		if err := db.QueryRow("SELECT start IS NULL, end IS NULL FROM events WHERE event_id=?", want.id).Scan(&startNull, &endNull); err != nil {
			t.Fatal(err)
		}
		if startNull != want.startNull || endNull != want.endNull {
			t.Fatalf("%s: start null=%t end null=%t", want.id, startNull, endNull)
		}
	}
	offline := app.Dependencies{HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) { t.Fatal("offline accessed network"); return nil, nil }}}
	for _, bounds := range [][]string{nil, {"--since", "2026-08-13T00:00:00Z"}, {"--until", "2026-08-13T00:00:00Z"}, {"--since", "2026-08-13T00:00:00Z", "--until", "2026-08-13T00:00:00Z"}} {
		r := executeChecked(t, context.Background(), app.Request{Argv: append([]string{"events", "offline", "--db", path}, bounds...)}, offline)
		var envelope struct {
			Data struct {
				Items []eventstore.Event `json:"items"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(r.Stdout), &envelope); err != nil {
			t.Fatal(err)
		}
		want := 1
		if len(bounds) == 0 {
			want = 3
		}
		if r.ExitCode != 0 || len(envelope.Data.Items) != want || envelope.Data.Items[0].EventID != "known-start" {
			t.Fatalf("offline bounds=%v: exit=%d items=%v", bounds, r.ExitCode, envelope.Data.Items)
		}
	}
}
