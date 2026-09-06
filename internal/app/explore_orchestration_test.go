package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
)

type exploreCursorKeys struct{}

func (exploreCursorKeys) Key() ([]byte, error) { return bytes32(), nil }
func bytes32() []byte                          { return []byte("cursorfixturematerialabcdefghijk") }

func exploreDependencies(httpClient scriptedHTTP) app.Dependencies {
	return app.Dependencies{
		Files: fakeFilesystem{readFile: func(string) ([]byte, error) {
			return []byte(`{"accessToken":"token","userId":"user","expiresAt":"2026-09-07T00:00:00Z"}`), nil
		}},
		CredentialsPath: "/credentials", HTTP: httpClient,
		Now:        func() time.Time { return time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC) },
		CursorKeys: exploreCursorKeys{}, CursorRandom: strings.NewReader(strings.Repeat("x", 512)),
	}
}
func exploreArgs(extra ...string) []string {
	return append([]string{"explore", "list", "--city", "Seattle", "--state", "WA", "--country", "US"}, extra...)
}
func exploreEvent(id, start string) string {
	return fmt.Sprintf(`{"event":{"id":%q,"title":"%s","startDate":%q,"endDate":null,"timezone":"America/Los_Angeles","locationInfo":null}}`, id, id, start)
}
func exploreFeed(items []string, next any) string {
	return fmt.Sprintf(`{"result":{"data":{"items":[%s]},"paging":{"nextCursor":%s}}}`, strings.Join(items, ","), jsonValue(next))
}
func jsonValue(value any) string { encoded, _ := json.Marshal(value); return string(encoded) }
func exploreDecorators() string  { return `{"result":{"data":{"decoratorsByEventId":{}}}}` }
func resultPage(t *testing.T, result app.Result) struct {
	Data struct {
		Items []struct {
			EventID string `json:"eventId"`
		} `json:"items"`
	} `json:"data"`
	Meta struct {
		Page struct {
			NextCursor     *string `json:"nextCursor"`
			HasMore        bool    `json:"hasMore"`
			FilterComplete *bool   `json:"filterComplete"`
		} `json:"page"`
	} `json:"meta"`
} {
	t.Helper()
	var out struct {
		Data struct {
			Items []struct {
				EventID string `json:"eventId"`
			} `json:"items"`
		} `json:"data"`
		Meta struct {
			Page struct {
				NextCursor     *string `json:"nextCursor"`
				HasMore        bool    `json:"hasMore"`
				FilterComplete *bool   `json:"filterComplete"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExecuteExploreListRejectsDateAliasUnknownAndRepeatedFlags(t *testing.T) {
	for _, argv := range [][]string{exploreArgs("--date", "today"), exploreArgs("--date-filter", "bad"), exploreArgs("--city", "Elsewhere")} {
		result := app.Execute(context.Background(), app.Request{Argv: argv}, exploreDependencies(scriptedHTTP{do: func(*http.Request) (*http.Response, error) { t.Fatal("network called"); return nil, nil }}))
		if result.ExitCode != 2 {
			t.Fatalf("argv=%v result=%#v", argv, result)
		}
	}
}

func TestExecuteExploreListContinuesDedupesAndBatchesDecoratorsAtOneHundred(t *testing.T) {
	first, second := make([]string, 100), []string{exploreEvent("event-101", "2026-09-05T19:00:00-07:00")}
	for i := range first {
		first[i] = exploreEvent(fmt.Sprintf("event-%03d", i+1), "2026-09-05T19:00:00-07:00")
	}
	calls := []string{}
	client := scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, r.URL.Path+":"+string(body))
		switch r.URL.Path {
		case "/getDiscoverFeedV2":
			if strings.Contains(string(body), `"cursor":"next"`) {
				return jsonResponse(200, exploreFeed(second, nil)), nil
			}
			return jsonResponse(200, exploreFeed(first, "next")), nil
		case "/getDiscoverEventItemDecorators":
			return jsonResponse(200, exploreDecorators()), nil
		}
		t.Fatalf("unexpected %s", r.URL.Path)
		return nil, nil
	}}
	result := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--all", "--max-items", "101")}, exploreDependencies(client))
	out := resultPage(t, result)
	if result.ExitCode != 0 || len(out.Data.Items) != 101 || out.Meta.Page.HasMore || out.Meta.Page.NextCursor != nil {
		t.Fatalf("result=%#v items=%d page=%#v", result, len(out.Data.Items), out.Meta.Page)
	}
	var decoratorSizes []int
	for _, call := range calls {
		if strings.HasPrefix(call, "/getDiscoverEventItemDecorators:") {
			var payload struct {
				Data struct {
					Params struct {
						EventIDs []string `json:"eventIds"`
					} `json:"params"`
				} `json:"data"`
			}
			_ = json.Unmarshal([]byte(strings.TrimPrefix(call, "/getDiscoverEventItemDecorators:")), &payload)
			decoratorSizes = append(decoratorSizes, len(payload.Data.Params.EventIDs))
		}
	}
	if fmt.Sprint(decoratorSizes) != "[100 1]" {
		t.Fatalf("decorator batches=%v calls=%v", decoratorSizes, calls)
	}
}

func TestExecuteExploreListCursorContinuationFilterAndProgressFailures(t *testing.T) {
	feedCalls := 0
	client := scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/getDiscoverEventItemDecorators" {
			return jsonResponse(200, exploreDecorators()), nil
		}
		if r.URL.Path != "/getDiscoverFeedV2" {
			t.Fatalf("unexpected %s", r.URL.Path)
		}
		feedCalls++
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"cursor":"one"`) {
			return jsonResponse(200, exploreFeed([]string{exploreEvent("two", "2026-09-05T19:00:00-07:00")}, nil)), nil
		}
		return jsonResponse(200, exploreFeed([]string{exploreEvent("one", "2026-09-05T19:00:00-07:00")}, "one")), nil
	}}
	deps := exploreDependencies(client)
	first := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--limit", "1")}, deps)
	page := resultPage(t, first)
	if first.ExitCode != 0 || page.Meta.Page.NextCursor == nil || !page.Meta.Page.HasMore {
		t.Fatalf("first=%#v page=%#v", first, page.Meta.Page)
	}
	second := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--limit", "1", "--cursor", *page.Meta.Page.NextCursor)}, deps)
	if second.ExitCode != 0 || resultPage(t, second).Data.Items[0].EventID != "two" || feedCalls != 2 {
		t.Fatalf("second=%#v calls=%d", second, feedCalls)
	}
	mismatch := app.Execute(context.Background(), app.Request{Argv: []string{"explore", "list", "--city", "Portland", "--state", "WA", "--country", "US", "--limit", "1", "--cursor", *page.Meta.Page.NextCursor}}, deps)
	if mismatch.ExitCode != 2 || !strings.Contains(mismatch.Stdout, `CURSOR_FILTER_MISMATCH`) {
		t.Fatalf("mismatch=%#v", mismatch)
	}
}

func TestExecuteExploreListDateFilterUnreadAndTerminalCompleteness(t *testing.T) {
	calls := 0
	client := scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/getDiscoverEventItemDecorators" {
			return jsonResponse(200, exploreDecorators()), nil
		}
		calls++
		if calls == 1 {
			return jsonResponse(200, exploreFeed([]string{exploreEvent("today", "2026-09-05T23:00:00-07:00")}, "next")), nil
		}
		return jsonResponse(200, exploreFeed([]string{exploreEvent("tomorrow", "2026-09-06T00:00:00-07:00")}, nil)), nil
	}}
	deps := exploreDependencies(client)
	first := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--date-filter", "today", "--limit", "1")}, deps)
	p1 := resultPage(t, first)
	if first.ExitCode != 0 || p1.Meta.Page.FilterComplete == nil || *p1.Meta.Page.FilterComplete || p1.Meta.Page.NextCursor == nil {
		t.Fatalf("first=%#v page=%#v", first, p1.Meta.Page)
	}
	second := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--date-filter", "today", "--limit", "1", "--cursor", *p1.Meta.Page.NextCursor)}, deps)
	p2 := resultPage(t, second)
	if second.ExitCode != 0 || p2.Meta.Page.FilterComplete == nil || !*p2.Meta.Page.FilterComplete || len(p2.Data.Items) != 0 {
		t.Fatalf("second=%#v page=%#v", second, p2)
	}
}
