package app

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type exploreModeCursorFiles struct{}

func (exploreModeCursorFiles) ReadFile(string) ([]byte, error) {
	return []byte(`{"accessToken":"token","userId":"user","expiresAt":"2026-09-07T00:00:00Z"}`), nil
}
func (exploreModeCursorFiles) Remove(string) error                  { return nil }
func (exploreModeCursorFiles) WriteFileAtomic(string, []byte) error { return nil }
func (exploreModeCursorFiles) WithLock(_ string, fn func()) error   { fn(); return nil }

type exploreModeCursorKey struct{}

func exploreModeCursorKeyBytes() []byte { return bytes.Repeat([]byte("k"), CursorKeySize) }
func (exploreModeCursorKey) Key() ([]byte, error) {
	return exploreModeCursorKeyBytes(), nil
}

type exploreModeCursorHTTP struct{ calls *int }

func (h exploreModeCursorHTTP) Do(*http.Request) (*http.Response, error) {
	*h.calls++
	return nil, errors.New("unexpected Explore network call")
}

func TestExecuteExploreFeedRejectsValidSectionCursorBeforeNetwork(t *testing.T) {
	// This would stop failing if executeExploreFeed stopped rejecting section cursors.
	options := exploreOptions{city: "Seattle", state: "WA", country: "US", date: "anytime"}
	hash := exploreFilterHash(options)
	cursor, err := encodeExploreCursor(exploreCursorPayload{
		Version:    1,
		FilterHash: hex.EncodeToString(hash[:]),
		Mode:       "section",
	}, exploreModeCursorKeyBytes(), strings.NewReader("0123456789ab"))
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	result := Execute(context.Background(), Request{Argv: []string{
		"explore", "list", "--city", "Seattle", "--state", "WA", "--country", "US", "--cursor", cursor,
	}}, Dependencies{
		Files:           exploreModeCursorFiles{},
		CredentialsPath: "/credentials",
		Now:             func() time.Time { return time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC) },
		HTTP:            exploreModeCursorHTTP{calls: &calls},
		CursorKeys:      exploreModeCursorKey{},
	})

	if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"code":"CURSOR_INVALID"`) {
		t.Fatalf("result=%#v; want input-invalid CURSOR_INVALID", result)
	}
	if calls != 0 {
		t.Fatalf("Explore network calls=%d; want 0", calls)
	}
}
