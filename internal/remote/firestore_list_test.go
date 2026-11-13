package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFirestoreListPaging(t *testing.T) {
	callers := map[string]func(Client) (int, error){
		"blast guests": func(c Client) (int, error) {
			v, e := c.ListEventGuests(context.Background(), "test-token", "event-example")
			return len(v), e
		},
		"host guests": func(c Client) (int, error) {
			v, e := c.getHostGuests(context.Background(), "test-token", "event-example")
			return len(v.Guests), e
		},
		"blast count": func(c Client) (int, error) {
			return c.CountTextBlasts(context.Background(), "test-token", "event-example")
		},
	}
	const doc = `{"name":"projects/getpartiful/databases/(default)/documents/events/event-example/guests/g1","fields":{"name":{"stringValue":"Example"},"status":{"stringValue":"GOING"},"count":{"integerValue":"1"}}}`
	page := func(n int, token string) string {
		return `{"documents":[` + strings.TrimSuffix(strings.Repeat(doc+",", n), ",") + `],"nextPageToken":"` + token + `"}`
	}
	for name, call := range callers {
		for _, tc := range []struct {
			name                 string
			pages                []string
			wantCalls, wantCount int
			wantError            string
		}{
			{"repeated token", []string{page(1, "same"), page(1, "same")}, 2, 0, "repeated nextPageToken"},
			{"token cycle", []string{page(1, "a"), page(1, "b"), page(1, "a")}, 3, 0, "repeated nextPageToken"},
			{"page cap", []string{page(0, "1"), page(0, "2"), page(0, "3"), page(0, "4"), page(0, "5"), page(0, "6"), page(0, "7"), page(0, "8"), page(0, "9"), page(0, "10")}, 10, 0, "page safety cap"},
			{"running document cap", []string{page(6000, "next"), page(4001, "more")}, 2, 0, "document safety cap"},
			{"normal multi-page", []string{page(1, "next"), page(1, "")}, 2, 2, ""},
			{"exact document cap", []string{page(1000, "1"), page(1000, "2"), page(1000, "3"), page(1000, "4"), page(1000, "5"), page(1000, "6"), page(1000, "7"), page(1000, "8"), page(1000, "9"), page(1000, "")}, 10, 10000, ""},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				calls := 0
				client := Client{HTTP: guestHTTPClientFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					if calls > len(tc.pages) {
						return nil, fmt.Errorf("unexpected extra page request")
					}
					if req.URL.Query().Get("pageSize") != "1000" {
						t.Fatal("incorrect page size")
					}
					wantToken := ""
					if calls > 1 {
						var previous struct {
							Next string `json:"nextPageToken"`
						}
						if err := json.Unmarshal([]byte(tc.pages[calls-2]), &previous); err != nil {
							t.Fatal(err)
						}
						wantToken = previous.Next
					}
					if req.URL.Query().Get("pageToken") != wantToken {
						t.Fatal("incorrect continuation page token")
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(tc.pages[calls-1]))}, nil
				})}
				count, err := call(client)
				if calls != tc.wantCalls {
					t.Fatalf("calls=%d, want %d", calls, tc.wantCalls)
				}
				if tc.wantError != "" {
					if !errors.Is(err, ErrProtocolChanged) || !strings.Contains(err.Error(), tc.wantError) || count != 0 {
						t.Fatalf("count/error=%d/%v, want protocol error containing %q and no partial result", count, err, tc.wantError)
					}
					return
				}
				if err != nil || count != tc.wantCount {
					t.Fatalf("count/error=%d/%v, want %d/nil", count, err, tc.wantCount)
				}
			})
		}
	}
}

func TestFirestoreListUsesCallerPageCap(t *testing.T) {
	calls := 0
	client := Client{HTTP: guestHTTPClientFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls > 2 {
			return nil, errors.New("extra page")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"nextPageToken":"page-%d"}`, calls)))}, nil
	})}
	documents, err := client.listFirestoreDocuments(context.Background(), "test-token", "events/event-example/hostMessages", 2)
	if calls != 2 || documents != nil || !errors.Is(err, ErrProtocolChanged) || !strings.Contains(err.Error(), "page safety cap (2 pages)") {
		t.Fatalf("calls/documents/error=%d/%d/%v", calls, len(documents), err)
	}
}
