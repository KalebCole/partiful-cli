package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/KalebCole/partiful-cli/internal/app"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
