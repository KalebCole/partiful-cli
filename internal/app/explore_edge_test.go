package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func TestExecuteExploreListEmptyFeedAndSelectedSections(t *testing.T) {
	t.Run("empty feed does not decorate", func(t *testing.T) {
		calls := 0
		result := app.Execute(context.Background(), app.Request{Argv: exploreArgs()}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Path != "/getDiscoverFeedV2" {
				t.Fatalf("unexpected decorator request")
			}
			return jsonResponse(200, exploreFeed([]string{}, nil)), nil
		}}))
		if result.ExitCode != 0 || len(resultPage(t, result).Data.Items) != 0 || calls != 1 {
			t.Fatalf("result=%#v calls=%d", result, calls)
		}
	})
	aliases := map[string]string{"trending": "area-dynamic-trending-events", "friends": "area-mutual-events", "open-invite": "area-open-invite", "followed": "area-followed-events"}
	for alias, id := range aliases {
		t.Run(alias, func(t *testing.T) {
			calls := []string{}
			result := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--section", alias)}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
				calls = append(calls, r.URL.Path)
				switch r.URL.Path {
				case "/getDiscoverSectionsV2":
					return jsonResponse(200, `{"result":{"data":{"sections":[{"id":"`+id+`","items":[`+exploreEvent(alias, "2026-09-05T19:00:00-07:00")+`]}]}}}`), nil
				case "/getDiscoverEventItemDecorators":
					return jsonResponse(200, exploreDecorators()), nil
				}
				t.Fatalf("unexpected %s", r.URL.Path)
				return nil, nil
			}}))
			page := resultPage(t, result)
			if result.ExitCode != 0 || len(page.Data.Items) != 1 || page.Data.Items[0].EventID != alias || strings.Join(calls, ",") != "/getDiscoverSectionsV2,/getDiscoverEventItemDecorators" {
				t.Fatalf("result=%#v calls=%v", result, calls)
			}
		})
	}
}

func TestExecuteExploreListDateFacetsUseEventLocalInclusiveOverlap(t *testing.T) {
	cases := []struct{ name, facet, start, end, want string }{
		{"today inclusive", "today", "2026-09-05T00:00:00-07:00", "2026-09-05T23:59:00-07:00", "edge"},
		{"tomorrow", "tomorrow", "2026-09-06T00:00:00-07:00", "2026-09-06T01:00:00-07:00", "edge"},
		{"week sunday", "this-week", "2026-09-06T20:00:00-07:00", "2026-09-06T21:00:00-07:00", "edge"},
		{"weekend sunday", "this-weekend", "2026-09-06T20:00:00-07:00", "2026-09-06T21:00:00-07:00", "edge"},
		{"overnight overlap", "today", "2026-09-04T23:00:00-07:00", "2026-09-05T01:00:00-07:00", "edge"},
		{"multi day overlap", "today", "2026-09-03T23:00:00-07:00", "2026-09-07T01:00:00-07:00", "edge"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := strings.Replace(exploreEvent("edge", tc.start), `"endDate":null`, `"endDate":"`+tc.end+`"`, 1)
			result := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--date-filter", tc.facet)}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/getDiscoverFeedV2" {
					return jsonResponse(200, exploreFeed([]string{event}, nil)), nil
				}
				return jsonResponse(200, exploreDecorators()), nil
			}}))
			page := resultPage(t, result)
			if result.ExitCode != 0 || len(page.Data.Items) != 1 || page.Data.Items[0].EventID != tc.want {
				t.Fatalf("result=%#v page=%#v", result, page)
			}
		})
	}
}

func TestExecuteExploreListDecoratorProtocolAndNullLocation(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{Argv: exploreArgs()}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/getDiscoverFeedV2" {
			return jsonResponse(200, exploreFeed([]string{exploreEvent("event", "2026-09-05T19:00:00-07:00")}, nil)), nil
		}
		return jsonResponse(200, `{"result":{"data":{"decoratorsByEventId":{"event":{"mutualGuests":[{"name":"x","status":"NOT_REVIEWED"}]}}}}}`), nil
	}}))
	if result.ExitCode != 9 || !strings.Contains(result.Stdout, "EXPLORE_PROTOCOL_CHANGED") {
		t.Fatalf("result=%#v", result)
	}
}

var _ = time.UTC
