package app_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func decodeExploreOutput(t *testing.T, result app.Result) map[string]any {
	t.Helper()
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d output=%s", result.ExitCode, result.Stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func exploreItems(t *testing.T, output map[string]any) []any {
	t.Helper()
	return output["data"].(map[string]any)["items"].([]any)
}
func exploreErrorCode(t *testing.T, result app.Result, exit int, code string) {
	t.Helper()
	if result.ExitCode != exit || !strings.Contains(result.Stdout, `"code":"`+code+`"`) {
		t.Fatalf("exit=%d output=%s; want %d/%s", result.ExitCode, result.Stdout, exit, code)
	}
}
func exploreSection(id string, events []string) string {
	return `{"result":{"data":{"sections":[{"id":"` + id + `","items":[` + strings.Join(events, ",") + `]}]}}}`
}
func exploreNext(t *testing.T, output map[string]any) string {
	t.Helper()
	v := output["meta"].(map[string]any)["page"].(map[string]any)["nextCursor"]
	if v == nil {
		t.Fatalf("missing next cursor: %#v", output)
	}
	return v.(string)
}

func TestTask3ExploreSchemaAndOutputHaveExactPublicItemContract(t *testing.T) {
	const wantKeys = "end,eventId,goingCount,interestedCount,location,mapsUrl,maybeCount,mutuals,mutualsPreviewComplete,start,timezone,title,url"
	schema := app.Execute(context.Background(), app.Request{Argv: []string{"schema", "explore.list"}}, app.Dependencies{})
	var document map[string]any
	if schema.ExitCode != 0 || json.Unmarshal([]byte(schema.Stdout), &document) != nil {
		t.Fatalf("schema=%#v", schema)
	}
	data := document["data"].(map[string]any)
	item := data["successSchema"].(map[string]any)["properties"].(map[string]any)["items"].(map[string]any)["items"].(map[string]any)
	if got := strings.Join(stringKeys(item["properties"].(map[string]any)), ","); got != wantKeys {
		t.Fatalf("schema properties=%s want=%s", got, wantKeys)
	}
	if got := strings.Join(stringKeysFromSlice(anyStrings(item["required"].([]any))), ","); got != wantKeys || item["additionalProperties"] != false {
		t.Fatalf("schema item=%#v", item)
	}
	event := `{"event":{"id":"public","endDate":null,"locationInfo":null,"maybeGuestCount":3,"interestedGuestCount":4}}`
	output := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs()}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/getDiscoverFeedV2" {
			return jsonResponse(200, exploreFeed([]string{event}, nil)), nil
		}
		return jsonResponse(200, exploreDecorators()), nil
	}})))
	itemOut := exploreItems(t, output)[0].(map[string]any)
	if got := strings.Join(stringKeys(itemOut), ","); got != wantKeys {
		t.Fatalf("output keys=%s want=%s", got, wantKeys)
	}
	for _, key := range []string{"title", "start", "end", "timezone", "mapsUrl", "goingCount"} {
		if itemOut[key] != nil {
			t.Fatalf("%s=%#v want null", key, itemOut[key])
		}
	}
	if itemOut["maybeCount"] != float64(3) || itemOut["interestedCount"] != float64(4) || !reflect.DeepEqual(itemOut["location"], []any{}) || !reflect.DeepEqual(itemOut["mutuals"], []any{}) || itemOut["mutualsPreviewComplete"] != nil {
		t.Fatalf("output=%#v", itemOut)
	}
	valued := strings.Replace(exploreEvent("values", "2026-09-05T19:00:00-07:00"), `"endDate":null`, `"endDate":"2026-09-05T21:00:00-07:00","maybeGuestCount":7,"interestedGuestCount":9`, 1)
	valueOutput := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs()}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/getDiscoverFeedV2" {
			return jsonResponse(200, exploreFeed([]string{valued}, nil)), nil
		}
		return jsonResponse(200, exploreDecorators()), nil
	}})))
	values := exploreItems(t, valueOutput)[0].(map[string]any)
	if values["title"] != "values" || values["start"] != "2026-09-05T19:00:00-07:00" || values["end"] != "2026-09-05T21:00:00-07:00" || values["timezone"] != "America/Los_Angeles" || values["maybeCount"] != float64(7) || values["interestedCount"] != float64(9) {
		t.Fatalf("valued output=%#v", values)
	}
}

func TestTask3ExploreExpiredCredentialRefreshesOnceBeforeCallable(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{"/credentials": []byte(`{"accessToken":"expired","refreshToken":"refresh","expiresAt":"2026-09-04T00:00:00Z"}`)}}
	calls := 0
	result := app.Execute(context.Background(), app.Request{Argv: exploreArgs()}, app.Dependencies{Files: files, CredentialsPath: "/credentials", Now: func() time.Time { return time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC) }, AuthRandom: strings.NewReader("0123456789abcdef"), CursorKeys: exploreCursorKeys{}, CursorRandom: strings.NewReader(strings.Repeat("x", 32)), HTTP: scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if r.URL.Host != "securetoken.googleapis.com" {
				t.Fatalf("first=%s", r.URL)
			}
			return jsonResponse(200, `{"access_token":"access-alias","id_token":"fresh","refresh_token":"new-refresh","expires_in":"3600","token_type":"Bearer","project_id":"project"}`), nil
		case 2:
			if r.URL.Path != "/getDiscoverFeedV2" || r.Header.Get("Authorization") != "Bearer fresh" {
				t.Fatalf("callable=%s auth=%s", r.URL, r.Header.Get("Authorization"))
			}
			return jsonResponse(200, exploreFeed(nil, nil)), nil
		}
		t.Fatalf("unexpected call %d", calls)
		return nil, nil
	}}})
	decodeExploreOutput(t, result)
	if calls != 2 || files.atomicWrites != 1 {
		t.Fatalf("calls=%d writes=%d", calls, files.atomicWrites)
	}
}

func TestTask3ExploreSectionsEmptyAndNoMutualProjection(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"absent", exploreSection("area-followed-events", nil)}, {"selected empty", exploreSection("area-dynamic-trending-events", nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			out := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs("--section", "trending")}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path != "/getDiscoverSectionsV2" {
					t.Fatalf("unexpected=%s", r.URL.Path)
				}
				return jsonResponse(200, tc.body), nil
			}})))
			if len(exploreItems(t, out)) != 0 || calls != 1 {
				t.Fatalf("calls=%d output=%#v", calls, out)
			}
		})
	}
	event := exploreEvent("no-mutual", "2026-09-05T19:00:00-07:00")
	out := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs()}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/getDiscoverFeedV2" {
			return jsonResponse(200, exploreFeed([]string{event}, nil)), nil
		}
		return jsonResponse(200, exploreDecorators()), nil
	}})))
	item := exploreItems(t, out)[0].(map[string]any)
	if !reflect.DeepEqual(item["mutuals"], []any{}) || item["mutualsPreviewComplete"] != nil {
		t.Fatalf("item=%#v", item)
	}
}

func TestTask3ExploreFeedCursorFailuresAndHardStopResume(t *testing.T) {
	invalid := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--cursor", "not-a-cursor")}, exploreDependencies(scriptedHTTP{do: func(*http.Request) (*http.Response, error) { t.Fatal("network"); return nil, nil }}))
	exploreErrorCode(t, invalid, 2, "CURSOR_INVALID")
	// A section cursor presented to feed must fail before either callable is invoked.
	section := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs("--section", "trending", "--limit", "1")}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/getDiscoverSectionsV2" {
			return jsonResponse(200, exploreSection("area-dynamic-trending-events", []string{exploreEvent("one", "2026-09-05T19:00:00-07:00"), exploreEvent("two", "2026-09-05T20:00:00-07:00")})), nil
		}
		return jsonResponse(200, exploreDecorators()), nil
	}})))
	mode := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--cursor", exploreNext(t, section))}, exploreDependencies(scriptedHTTP{do: func(*http.Request) (*http.Response, error) { t.Fatal("network"); return nil, nil }}))
	exploreErrorCode(t, mode, 2, "CURSOR_FILTER_MISMATCH")
	calls := 0
	deps := exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/getDiscoverEventItemDecorators" {
			return jsonResponse(200, exploreDecorators()), nil
		}
		calls++
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"cursor":"remote-1"`) {
			return jsonResponse(200, exploreFeed([]string{exploreEvent("two", "2026-09-05T20:00:00-07:00")}, nil)), nil
		}
		return jsonResponse(200, exploreFeed([]string{exploreEvent("one", "2026-09-05T19:00:00-07:00")}, "remote-1")), nil
	}})
	first := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs("--all", "--max-items", "1")}, deps))
	if first["meta"].(map[string]any)["page"].(map[string]any)["hasMore"] != true {
		t.Fatalf("first=%#v", first)
	}
	second := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs("--all", "--max-items", "1", "--cursor", exploreNext(t, first))}, deps))
	if exploreItems(t, second)[0].(map[string]any)["eventId"] != "two" || calls != 2 {
		t.Fatalf("second=%#v calls=%d", second, calls)
	}
	duplicates := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs("--all", "--max-items", "2")}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/getDiscoverFeedV2" {
			return jsonResponse(200, exploreFeed([]string{exploreEvent("first", "2026-09-05T19:00:00-07:00"), exploreEvent("first", "2026-09-05T19:00:00-07:00"), exploreEvent("second", "2026-09-05T20:00:00-07:00")}, nil)), nil
		}
		return jsonResponse(200, exploreDecorators()), nil
	}})))
	duplicateItems := exploreItems(t, duplicates)
	if len(duplicateItems) != 2 || duplicateItems[0].(map[string]any)["eventId"] != "first" || duplicateItems[1].(map[string]any)["eventId"] != "second" {
		t.Fatalf("first-occurrence dedupe output=%#v", duplicates)
	}
	for _, tc := range []struct {
		name  string
		pages []string
	}{
		{"repeated remote cursor", []string{exploreFeed([]string{exploreEvent("one", "2026-09-05T19:00:00-07:00")}, "same"), exploreFeed([]string{exploreEvent("two", "2026-09-05T20:00:00-07:00")}, "same")}},
		{"no progress", []string{exploreFeed([]string{exploreEvent("one", "2026-09-05T19:00:00-07:00")}, "next"), exploreFeed([]string{exploreEvent("one", "2026-09-05T19:00:00-07:00")}, "last")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := 0
			r := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--all", "--max-items", "2")}, exploreDependencies(scriptedHTTP{do: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/getDiscoverEventItemDecorators" {
					return jsonResponse(200, exploreDecorators()), nil
				}
				p := tc.pages[n]
				n++
				return jsonResponse(200, p), nil
			}}))
			exploreErrorCode(t, r, 9, "EXPLORE_PROTOCOL_CHANGED")
			if n != 2 {
				t.Fatalf("feed calls=%d", n)
			}
		})
	}
}

func TestTask3ExploreSectionSnapshotAndDateAndLocations(t *testing.T) {
	responses := []string{exploreSection("area-dynamic-trending-events", []string{exploreEvent("one", "2026-09-05T19:00:00-07:00"), exploreEvent("two", "2026-09-05T20:00:00-07:00")}), exploreSection("area-dynamic-trending-events", []string{exploreEvent("one", "2026-09-05T19:00:00-07:00"), exploreEvent("two", "2026-09-05T20:00:00-07:00")}), exploreSection("area-dynamic-trending-events", []string{exploreEvent("changed", "2026-09-05T19:00:00-07:00")})}
	n := 0
	deps := exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/getDiscoverEventItemDecorators" {
			return jsonResponse(200, exploreDecorators()), nil
		}
		v := responses[n]
		n++
		return jsonResponse(200, v), nil
	}})
	first := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs("--section", "trending", "--limit", "1")}, deps))
	cursor := exploreNext(t, first)
	second := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs("--section", "trending", "--limit", "1", "--cursor", cursor)}, deps))
	if exploreItems(t, second)[0].(map[string]any)["eventId"] != "two" {
		t.Fatalf("second=%#v", second)
	}
	changed := app.Execute(context.Background(), app.Request{Argv: exploreArgs("--section", "trending", "--limit", "1", "--cursor", cursor)}, deps)
	exploreErrorCode(t, changed, 6, "CURSOR_SNAPSHOT_CHANGED")
	// Monday/Saturday inclusions and outside-range exclusions, evaluated by the public command.
	for _, tc := range []struct {
		name, facet, start, end string
		want                    int
	}{
		{"monday", "this-week", "2026-09-07T19:00:00-07:00", "2026-09-07T20:00:00-07:00", 1}, {"saturday", "this-weekend", "2026-09-12T19:00:00-07:00", "2026-09-12T20:00:00-07:00", 1}, {"today outside", "today", "2026-09-06T00:00:00-07:00", "2026-09-06T01:00:00-07:00", 0}, {"tomorrow outside", "tomorrow", "2026-09-07T00:00:00-07:00", "2026-09-07T01:00:00-07:00", 0}, {"week outside", "this-week", "2026-09-14T00:00:00-07:00", "2026-09-14T01:00:00-07:00", 0}, {"weekend outside", "this-weekend", "2026-09-14T00:00:00-07:00", "2026-09-14T01:00:00-07:00", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := strings.Replace(exploreEvent("edge", tc.start), `"endDate":null`, `"endDate":"`+tc.end+`"`, 1)
			deps := exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/getDiscoverFeedV2" {
					return jsonResponse(200, exploreFeed([]string{e}, nil)), nil
				}
				return jsonResponse(200, exploreDecorators()), nil
			}})
			if tc.name == "monday" {
				deps.Now = func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) }
			}
			if tc.name == "saturday" {
				deps.Now = func() time.Time { return time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC) }
			}
			if tc.name == "monday" || tc.name == "saturday" {
				deps.Files = fakeFilesystem{readFile: func(string) ([]byte, error) {
					return []byte(`{"accessToken":"token","userId":"user","expiresAt":"2026-10-01T00:00:00Z"}`), nil
				}}
			}
			o := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs("--date-filter", tc.facet)}, deps))
			if len(exploreItems(t, o)) != tc.want {
				t.Fatalf("output=%#v", o)
			}
		})
	}
	for _, tc := range []struct {
		location string
		want     []any
	}{
		{`{"displayAddressLines":["Hidden location"],"mapsInfo":null}`, []any{"Hidden location"}},
		{`{"displayAddressLines":["Near Pike Place","Seattle"],"mapsInfo":null}`, []any{"Near Pike Place", "Seattle"}},
	} {
		e := strings.Replace(exploreEvent("place", "2026-09-05T19:00:00-07:00"), `"locationInfo":null`, `"locationInfo":`+tc.location, 1)
		o := decodeExploreOutput(t, app.Execute(context.Background(), app.Request{Argv: exploreArgs()}, exploreDependencies(scriptedHTTP{do: func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/getDiscoverFeedV2" {
				return jsonResponse(200, exploreFeed([]string{e}, nil)), nil
			}
			return jsonResponse(200, exploreDecorators()), nil
		}})))
		item := exploreItems(t, o)[0].(map[string]any)
		if item["mapsUrl"] != nil || !reflect.DeepEqual(item["location"], tc.want) || item["mutualsPreviewComplete"] != nil {
			t.Fatalf("location=%s output=%#v", tc.location, o)
		}
	}
}
func stringKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
func anyStrings(values []any) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = v.(string)
	}
	return out
}

func stringKeysFromSlice(values []string) []string {
	set := make(map[string]any, len(values))
	for _, value := range values {
		set[value] = nil
	}
	return stringKeys(set)
}
