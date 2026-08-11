package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

type scriptedHTTP struct {
	do func(*http.Request) (*http.Response, error)
}

type scriptedRoundTripper struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (script scriptedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return script.roundTrip(request)
}

func (script scriptedHTTP) Do(request *http.Request) (*http.Response, error) {
	return script.do(request)
}

type staticCursorKeyProvider struct{}

func (staticCursorKeyProvider) Key() ([]byte, error) {
	return []byte("0123456789abcdef0123456789abcdef"), nil
}

func withTestCursorCrypto(dependencies app.Dependencies) app.Dependencies {
	dependencies.CursorKeys = staticCursorKeyProvider{}
	dependencies.CursorRandom = strings.NewReader(strings.Repeat("deterministic-nonce", 100))
	return dependencies
}

func TestExecutePostersListReturnsFirstLocalPage(t *testing.T) {
	const catalog = `[
		{"id":"first","name":"First","url":"https://example.invalid/first.png","contentType":"image/png","width":1200,"height":630,"tags":["party"],"categories":["fun"]},
		{"id":"duplicate","name":"Duplicate","url":"https://example.invalid/duplicate.gif","contentType":"image/gif","width":null,"height":null,"tags":["dance"],"categories":[]},
		{"id":"duplicate","name":"Duplicate Again","url":"https://example.invalid/duplicate-2.gif","contentType":"image/gif","width":640,"height":480,"tags":[],"categories":["night"]}
	]`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list"},
		Stdin: strings.NewReader(""),
	}, withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(catalog)),
			}, nil
		}},
	}))

	const want = `{"ok":true,"data":{"items":[{"posterId":"first","name":"First","url":"https://example.invalid/first.png","contentType":"image/png","width":1200,"height":630,"tags":["party"],"categories":["fun"]},{"posterId":"duplicate","name":"Duplicate","url":"https://example.invalid/duplicate.gif","contentType":"image/gif","width":null,"height":null,"tags":["dance"],"categories":[]},{"posterId":"duplicate","name":"Duplicate Again","url":"https://example.invalid/duplicate-2.gif","contentType":"image/gif","width":640,"height":480,"tags":[],"categories":["night"]}]},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[],"page":{"limit":25,"nextCursor":null,"hasMore":false}}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecutePostersListHonorsLimitAndReturnsOpaqueCursor(t *testing.T) {
	const catalog = `[
		{"id":"first","name":"First","url":"https://example.invalid/first.png","contentType":"image/png","width":1200,"height":630,"tags":[],"categories":[]},
		{"id":"second","name":"Second","url":"https://example.invalid/second.png","contentType":"image/png","width":1200,"height":630,"tags":[],"categories":[]},
		{"id":"third","name":"Third","url":"https://example.invalid/third.png","contentType":"image/png","width":1200,"height":630,"tags":[],"categories":[]}
	]`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--limit", "2"},
		Stdin: strings.NewReader(""),
	}, withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(catalog)),
			}, nil
		}},
	}))

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Items []struct {
				PosterID string `json:"posterId"`
			} `json:"items"`
		} `json:"data"`
		Meta struct {
			Page struct {
				Limit      int     `json:"limit"`
				NextCursor *string `json:"nextCursor"`
				HasMore    bool    `json:"hasMore"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if result.ExitCode != 0 || !envelope.OK {
		t.Fatalf("result = %#v, want success", result)
	}
	if len(envelope.Data.Items) != 2 {
		t.Fatalf("items = %#v, want two", envelope.Data.Items)
	}
	if got := []string{envelope.Data.Items[0].PosterID, envelope.Data.Items[1].PosterID}; !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("poster IDs = %v, want first page", got)
	}
	if envelope.Meta.Page.Limit != 2 || !envelope.Meta.Page.HasMore {
		t.Fatalf("page = %#v, want limit 2 with more", envelope.Meta.Page)
	}
	if envelope.Meta.Page.NextCursor == nil || *envelope.Meta.Page.NextCursor == "" {
		t.Fatal("next cursor is empty")
	}
	if strings.Contains(*envelope.Meta.Page.NextCursor, "first") {
		t.Fatal("next cursor exposes catalog contents")
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecutePostersListResumesFromOpaqueCursor(t *testing.T) {
	const catalog = `[
		{"id":"first","name":"First","url":"https://example.invalid/first.png","contentType":"image/png","width":1200,"height":630,"tags":[],"categories":[]},
		{"id":"second","name":"Second","url":"https://example.invalid/second.png","contentType":"image/png","width":1200,"height":630,"tags":[],"categories":[]},
		{"id":"third","name":"Third","url":"https://example.invalid/third.png","contentType":"image/png","width":1200,"height":630,"tags":[],"categories":[]}
	]`
	dependencies := withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(catalog)),
			}, nil
		}},
	})
	first := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--limit", "2"},
		Stdin: strings.NewReader(""),
	}, dependencies)
	var firstEnvelope struct {
		Meta struct {
			Page struct {
				NextCursor *string `json:"nextCursor"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(first.Stdout), &firstEnvelope); err != nil {
		t.Fatalf("decode first stdout: %v", err)
	}
	if firstEnvelope.Meta.Page.NextCursor == nil {
		t.Fatal("first page cursor is nil")
	}

	second := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"posters", "list",
			"--limit", "2",
			"--cursor", *firstEnvelope.Meta.Page.NextCursor,
		},
		Stdin: strings.NewReader(""),
	}, dependencies)
	var secondEnvelope struct {
		Data struct {
			Items []struct {
				PosterID string `json:"posterId"`
			} `json:"items"`
		} `json:"data"`
		Meta struct {
			Page struct {
				NextCursor *string `json:"nextCursor"`
				HasMore    bool    `json:"hasMore"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(second.Stdout), &secondEnvelope); err != nil {
		t.Fatalf("decode second stdout: %v", err)
	}
	if second.ExitCode != 0 {
		t.Fatalf("second result = %#v, want success", second)
	}
	if len(secondEnvelope.Data.Items) != 1 || secondEnvelope.Data.Items[0].PosterID != "third" {
		t.Fatalf("second items = %#v, want third poster", secondEnvelope.Data.Items)
	}
	if secondEnvelope.Meta.Page.HasMore || secondEnvelope.Meta.Page.NextCursor != nil {
		t.Fatalf("second page = %#v, want completed collection", secondEnvelope.Meta.Page)
	}
}

func TestExecutePostersListRejectsTamperedAuthenticatedCursor(t *testing.T) {
	const catalog = `[
		{"id":"one","name":"One","url":"https://example.invalid/one.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"two","name":"Two","url":"https://example.invalid/two.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"three","name":"Three","url":"https://example.invalid/three.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]}
	]`
	dependencies := withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(catalog)),
			}, nil
		}},
	})
	first := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--limit", "1"},
		Stdin: strings.NewReader(""),
	}, dependencies)
	var firstEnvelope struct {
		Meta struct {
			Page struct {
				NextCursor string `json:"nextCursor"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(first.Stdout), &firstEnvelope); err != nil {
		t.Fatalf("decode first stdout: %v", err)
	}

	tamperedCursor := []byte(firstEnvelope.Meta.Page.NextCursor)
	last := len(tamperedCursor) - 1
	if tamperedCursor[last] == 'A' {
		tamperedCursor[last] = 'B'
	} else {
		tamperedCursor[last] = 'A'
	}

	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--cursor", string(tamperedCursor)},
		Stdin: strings.NewReader(""),
	}, dependencies)

	if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"code":"CURSOR_INVALID"`) {
		t.Fatalf("result = %#v, want forged cursor rejection", result)
	}
}

func TestExecutePostersSearchFiltersCaseInsensitivelyInCatalogOrder(t *testing.T) {
	const catalog = `[
		{"id":"tag-match","name":"First","url":"https://example.invalid/first.png","contentType":"image/png","width":1200,"height":630,"tags":["Dance Floor"],"categories":[]},
		{"id":"not-a-match","name":"Second","url":"https://example.invalid/second.png","contentType":"image/png","width":1200,"height":630,"tags":[],"categories":[]},
		{"id":"name-match","name":"DANCE Party","url":"https://example.invalid/third.png","contentType":"image/png","width":1200,"height":630,"tags":[],"categories":[]},
		{"id":"category-match","name":"Fourth","url":"https://example.invalid/fourth.png","contentType":"image/png","width":1200,"height":630,"tags":[],"categories":["social dance"]}
	]`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "search", "--query", "  dance  "},
		Stdin: strings.NewReader(""),
	}, withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(catalog)),
			}, nil
		}},
	}))

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Items []struct {
				PosterID string `json:"posterId"`
			} `json:"items"`
		} `json:"data"`
		Meta struct {
			Command string `json:"command"`
			Page    struct {
				Limit      int     `json:"limit"`
				NextCursor *string `json:"nextCursor"`
				HasMore    bool    `json:"hasMore"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if result.ExitCode != 0 || !envelope.OK {
		t.Fatalf("result = %#v, want success", result)
	}
	var got []string
	for _, item := range envelope.Data.Items {
		got = append(got, item.PosterID)
	}
	if !reflect.DeepEqual(got, []string{"tag-match", "name-match", "category-match"}) {
		t.Fatalf("poster IDs = %v, want filtered catalog order", got)
	}
	if envelope.Meta.Command != "posters.search" ||
		envelope.Meta.Page.Limit != 25 ||
		envelope.Meta.Page.HasMore ||
		envelope.Meta.Page.NextCursor != nil {
		t.Fatalf("meta = %#v, want completed search page", envelope.Meta)
	}
}

func TestExecutePostersListAllStopsAtMaxItems(t *testing.T) {
	const catalog = `[
		{"id":"one","name":"One","url":"https://example.invalid/one.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"two","name":"Two","url":"https://example.invalid/two.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"three","name":"Three","url":"https://example.invalid/three.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"four","name":"Four","url":"https://example.invalid/four.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"five","name":"Five","url":"https://example.invalid/five.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]}
	]`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--all", "--max-items", "3"},
		Stdin: strings.NewReader(""),
	}, withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(catalog)),
			}, nil
		}},
	}))

	var envelope struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
		Meta struct {
			Page struct {
				Limit      int     `json:"limit"`
				NextCursor *string `json:"nextCursor"`
				HasMore    bool    `json:"hasMore"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v, want success", result)
	}
	if len(envelope.Data.Items) != 3 {
		t.Fatalf("item count = %d, want 3", len(envelope.Data.Items))
	}
	if envelope.Meta.Page.Limit != 3 ||
		!envelope.Meta.Page.HasMore ||
		envelope.Meta.Page.NextCursor == nil {
		t.Fatalf("page = %#v, want resumable max-items boundary", envelope.Meta.Page)
	}
}

func TestExecutePostersListRejectsCursorWhenPayloadChanges(t *testing.T) {
	const firstCatalog = `[
		{"id":"one","name":"One","url":"https://example.invalid/one.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"two","name":"Two","url":"https://example.invalid/two.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"three","name":"Three","url":"https://example.invalid/three.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]}
	]`
	const changedCatalog = `[
		{"id":"replacement","name":"Replacement","url":"https://example.invalid/replacement.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]}
	]`
	call := 0
	dependencies := withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			body := firstCatalog
			if call == 2 {
				body = changedCatalog
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}},
	})
	first := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--limit", "2"},
		Stdin: strings.NewReader(""),
	}, dependencies)
	var firstEnvelope struct {
		Meta struct {
			Page struct {
				NextCursor string `json:"nextCursor"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(first.Stdout), &firstEnvelope); err != nil {
		t.Fatalf("decode first stdout: %v", err)
	}

	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--cursor", firstEnvelope.Meta.Page.NextCursor},
		Stdin: strings.NewReader(""),
	}, dependencies)

	const want = `{"ok":false,"error":{"type":"state.conflict","code":"CURSOR_SNAPSHOT_CHANGED","message":"The poster catalog changed after this cursor was issued.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 6 {
		t.Fatalf("exit code = %d, want 6", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteSchemaProjectsPosterSearchDefinition(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "posters.search"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	var envelope struct {
		Data struct {
			Command string `json:"command"`
			Flags   []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"flags"`
			InputSchema struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					Minimum   *int `json:"minimum"`
					Maximum   *int `json:"maximum"`
					MinLength *int `json:"minLength"`
				} `json:"properties"`
			} `json:"inputSchema"`
			SuccessSchema struct {
				Properties map[string]struct {
					Items struct {
						Required []string `json:"required"`
					} `json:"items"`
				} `json:"properties"`
			} `json:"successSchema"`
			FailureTypes []string `json:"failureTypes"`
			Safety       struct {
				Kind                 string `json:"kind"`
				PlanRequired         bool   `json:"planRequired"`
				ConfirmationRequired bool   `json:"confirmationRequired"`
			} `json:"safety"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v, want success", result)
	}
	if envelope.Data.Command != "posters.search" {
		t.Fatalf("command = %q, want posters.search", envelope.Data.Command)
	}
	var gotFlags []string
	for _, flag := range envelope.Data.Flags {
		gotFlags = append(gotFlags, fmt.Sprintf("%s:%t", flag.Name, flag.Required))
	}
	wantFlags := []string{"--query:true", "--limit:false", "--cursor:false", "--all:false", "--max-items:false"}
	if !reflect.DeepEqual(gotFlags, wantFlags) {
		t.Fatalf("flags = %#v, want %#v", gotFlags, wantFlags)
	}
	if !reflect.DeepEqual(envelope.Data.InputSchema.Required, []string{"query"}) {
		t.Fatalf("required inputs = %v, want query", envelope.Data.InputSchema.Required)
	}
	limit := envelope.Data.InputSchema.Properties["limit"]
	maxItems := envelope.Data.InputSchema.Properties["maxItems"]
	query := envelope.Data.InputSchema.Properties["query"]
	if limit.Minimum == nil || *limit.Minimum != 1 ||
		limit.Maximum == nil || *limit.Maximum != 100 ||
		maxItems.Minimum == nil || *maxItems.Minimum != 1 ||
		maxItems.Maximum == nil || *maxItems.Maximum != 1000 ||
		query.MinLength == nil || *query.MinLength != 1 {
		t.Fatalf("input constraints = %#v, want documented collection bounds", envelope.Data.InputSchema.Properties)
	}
	wantPosterFields := []string{"posterId", "name", "url", "contentType", "width", "height", "tags", "categories"}
	if got := envelope.Data.SuccessSchema.Properties["items"].Items.Required; !reflect.DeepEqual(got, wantPosterFields) {
		t.Fatalf("poster fields = %v, want %v", got, wantPosterFields)
	}
	wantFailures := []string{"input.invalid", "state.conflict", "remote.unavailable", "contract.protocol_changed", "internal.failure"}
	if !reflect.DeepEqual(envelope.Data.FailureTypes, wantFailures) {
		t.Fatalf("failure types = %v, want %v", envelope.Data.FailureTypes, wantFailures)
	}
	if envelope.Data.Safety.Kind != "read-only" ||
		envelope.Data.Safety.PlanRequired ||
		envelope.Data.Safety.ConfirmationRequired {
		t.Fatalf("safety = %#v, want read-only", envelope.Data.Safety)
	}
}

func TestExecutePostersListRejectsUnexpectedSuccessContentType(t *testing.T) {
	const privateBody = `[{"id":"private-value","name":"Private","url":"https://example.invalid/private.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]}]`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/html"}},
				Body:       io.NopCloser(strings.NewReader(privateBody)),
			}, nil
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"contract.protocol_changed","code":"POSTER_CATALOG_PROTOCOL_CHANGED","message":"The poster catalog no longer matches the reviewed remote contract.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	const wantStderr = "partiful: poster catalog protocol changed\n"
	if result.ExitCode != 9 {
		t.Fatalf("exit code = %d, want 9", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if result.Stderr != wantStderr {
		t.Fatalf("stderr = %q, want %q", result.Stderr, wantStderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, "private-value") {
		t.Fatal("protocol failure exposed response body")
	}
}

func TestExecutePostersListRejectsMalformedCursorBeforeRemoteFailure(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--cursor", "not-an-opaque-cursor"},
		Stdin: strings.NewReader(""),
	}, withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("private network failure")
		}},
	}))

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"CURSOR_INVALID","message":"The cursor is malformed.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecutePostersSearchRejectsCursorWithDifferentNormalizedFilter(t *testing.T) {
	const catalog = `[
		{"id":"dance","name":"Dance","url":"https://example.invalid/dance.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"party","name":"Party","url":"https://example.invalid/party.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]}
	]`
	call := 0
	dependencies := withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call > 1 {
				return nil, errors.New("private network failure")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(catalog)),
			}, nil
		}},
	})
	first := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "search", "--query", "a", "--limit", "1"},
		Stdin: strings.NewReader(""),
	}, dependencies)
	var firstEnvelope struct {
		Meta struct {
			Page struct {
				NextCursor string `json:"nextCursor"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(first.Stdout), &firstEnvelope); err != nil {
		t.Fatalf("decode first stdout: %v", err)
	}

	result := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"posters", "search",
			"--query", "different",
			"--cursor", firstEnvelope.Meta.Page.NextCursor,
		},
		Stdin: strings.NewReader(""),
	}, dependencies)

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"CURSOR_FILTER_MISMATCH","message":"The cursor does not match this command and filters.","retryable":false,"details":{}},"meta":{"command":"posters.search","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecutePostersListMapsNetworkFailureToRemoteUnavailable(t *testing.T) {
	const privateError = "private endpoint failure details"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return nil, errors.New(privateError)
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"remote.unavailable","code":"POSTER_CATALOG_UNAVAILABLE","message":"The poster catalog is unavailable.","retryable":true,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	const wantStderr = "partiful: poster catalog unavailable\n"
	if result.ExitCode != 8 {
		t.Fatalf("exit code = %d, want 8", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if result.Stderr != wantStderr {
		t.Fatalf("stderr = %q, want %q", result.Stderr, wantStderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateError) {
		t.Fatal("remote failure exposed private transport details")
	}
}

func TestExecutePostersListFailsClosedOnReceivedNon200(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"private":"body"}`)),
			}, nil
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"contract.protocol_changed","code":"POSTER_CATALOG_PROTOCOL_CHANGED","message":"The poster catalog no longer matches the reviewed remote contract.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	const wantStderr = "partiful: poster catalog protocol changed\n"
	if result.ExitCode != 9 {
		t.Fatalf("exit code = %d, want 9", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if result.Stderr != wantStderr {
		t.Fatalf("stderr = %q, want %q", result.Stderr, wantStderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, "remote.rate_limited") ||
		strings.Contains(result.Stdout+result.Stderr, "private") {
		t.Fatal("non-200 failure claimed rate limiting or exposed response body")
	}
}

func TestExecutePostersListProductionHTTPDoesNotFollowRedirects(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		HTTP: remote.NewHTTPClient(scriptedRoundTripper{
			roundTrip: func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusFound,
					Header: http.Header{
						"Location": {"https://assets.getpartiful.com/unreviewed.json"},
					},
					Body: io.NopCloser(strings.NewReader("")),
				}, nil
			},
		}),
	})

	if result.ExitCode != 9 || !strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("result = %#v, want original redirect to fail closed", result)
	}
}

func TestExecutePostersListRejectsBodyAboveFiniteCeiling(t *testing.T) {
	oversizedBody := io.MultiReader(
		strings.NewReader("[]"),
		strings.NewReader(strings.Repeat(" ", 8<<20)),
	)
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(oversizedBody),
			}, nil
		}},
	})

	if result.ExitCode != 9 || !strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("result = %#v, want oversized body protocol change", result)
	}
}

func TestExecutePostersListFailsClosedOnMalformed200Body(t *testing.T) {
	const malformedBody = `[{"id":"private-id","name":"Missing categories","url":"https://example.invalid/poster.png","contentType":"image/png","width":1,"height":1,"tags":[]}]`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(malformedBody)),
			}, nil
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"contract.protocol_changed","code":"POSTER_CATALOG_PROTOCOL_CHANGED","message":"The poster catalog no longer matches the reviewed remote contract.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 9 {
		t.Fatalf("exit code = %d, want 9", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if strings.Contains(result.Stdout+result.Stderr, "private-id") {
		t.Fatal("malformed response failure exposed response body")
	}
}

func TestExecutePostersListRejectsMalformedOptionalBlurHash(t *testing.T) {
	const malformedBody = `[{"id":"poster","name":"Poster","url":"https://example.invalid/poster.png","blurHash":42,"contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]}]`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(malformedBody)),
			}, nil
		}},
	})

	if result.ExitCode != 9 || !strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("result = %#v, want protocol change", result)
	}
}

func TestExecutePostersListRequiresMaxItemsWithAll(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--all"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"MAX_ITEMS_REQUIRED","message":"--all requires --max-items.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecutePostersListRejectsLimitAboveMaximum(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--limit", "101"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"LIMIT_INVALID","message":"Limit must be an integer from 1 to 100.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
}

func TestExecutePostersSearchRequiresNonEmptyQuery(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "search", "--query", "   "},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"QUERY_REQUIRED","message":"Search query must not be empty.","retryable":false,"details":{}},"meta":{"command":"posters.search","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
}

func TestExecuteVersion(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"--version"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":true,"data":{"version":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"},"meta":{"command":"version","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteRejectsUnknownCommand(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"events", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"usage.invalid","code":"COMMAND_NOT_FOUND","message":"Unknown command.","retryable":false,"details":{}},"meta":{"command":"unknown","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecutePrettyPrintsOneCompleteEnvelope(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"--pretty", "--version"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{
  "ok": true,
  "data": {
    "version": "1.0.0",
    "productContractRevision": "2026-08-10.1",
    "remoteContractRevision": "2026-08-11.1"
  },
  "meta": {
    "command": "version",
    "cliVersion": "1.0.0",
    "productContractRevision": "2026-08-10.1",
    "remoteContractRevision": "2026-08-11.1",
    "warnings": []
  }
}
`
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteAcceptsNonInteractiveGlobalFlag(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"--version", "--non-interactive"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":true,"data":{"version":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"},"meta":{"command":"version","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteRejectsRepeatedScalarFlag(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"--pretty", "--version", "--pretty"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{
  "ok": false,
  "error": {
    "type": "input.invalid",
    "code": "FLAG_REPEATED",
    "message": "A scalar flag cannot be repeated.",
    "retryable": false,
    "details": {
      "flag": "--pretty"
    }
  },
  "meta": {
    "command": "version",
    "cliVersion": "1.0.0",
    "productContractRevision": "2026-08-10.1",
    "remoteContractRevision": "2026-08-11.1"
  }
}
`
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteSchemaListsOnlyCompletedCatalog(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":true,"data":{"commands":["auth.logout","auth.status","doctor","posters.list","posters.search","schema","version"]},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteSchemaProjectsExecutableDefinition(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "auth.status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":true,"data":{"command":"auth.status","positionals":[],"flags":[],"inputSchema":{"type":"object","additionalProperties":false},"successSchema":{"type":"object","additionalProperties":false,"required":["authenticated","tokenState","expiresAt"],"properties":{"authenticated":{"type":"boolean"},"expiresAt":{"type":["string","null"],"format":"date-time"},"tokenState":{"type":"string","enum":["healthy","expiring","expired","missing"]}}},"failureTypes":["internal.failure"],"safety":{"kind":"read-only","planRequired":false,"confirmationRequired":false}},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteRejectsUnknownSchemaPathWithoutEchoingInput(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "secret-private-value"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"usage.invalid","code":"COMMAND_SCHEMA_NOT_FOUND","message":"No completed command has that schema path.","retryable":false,"details":{}},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, "secret-private-value") {
		t.Fatal("output echoed untrusted command input")
	}
}

func TestExecuteAuthStatusWhenCredentialsAreMissing(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return nil, fs.ErrNotExist
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"authenticated":false,"tokenState":"missing","expiresAt":null},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

type fakeFilesystem struct {
	readFile func(string) ([]byte, error)
	remove   func(string) error
}

func (filesystem fakeFilesystem) ReadFile(path string) ([]byte, error) {
	return filesystem.readFile(path)
}

func (filesystem fakeFilesystem) Remove(path string) error {
	if filesystem.remove != nil {
		return filesystem.remove(path)
	}
	return nil
}

func TestExecuteAuthStatusRedactsHealthyCredentials(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","refreshToken":"secret-refresh-value","userId":"private-user-value","expiresAt":"2026-08-11T02:00:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"authenticated":true,"tokenState":"healthy","expiresAt":"2026-08-11T02:00:00Z"},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	for _, privateValue := range []string{"secret-token-value", "secret-refresh-value", "private-user-value"} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("output contains private credential value %q", privateValue)
		}
	}
}

func TestExecuteAuthStatusReportsExpiringToken(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","expiresAt":"2026-08-11T00:04:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"authenticated":true,"tokenState":"expiring","expiresAt":"2026-08-11T00:04:00Z"},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteAuthStatusReportsExpiredToken(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","expiresAt":"2026-08-10T23:59:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"authenticated":false,"tokenState":"expired","expiresAt":"2026-08-10T23:59:00Z"},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteAuthStatusFailureDoesNotRevealCredentialContents(t *testing.T) {
	const privateContents = "secret-token-content private-user-identifier"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(privateContents), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CREDENTIALS_INVALID","message":"Local credentials are invalid.","retryable":false,"details":{}},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	const wantStderr = "partiful: local operation failed\n"
	if result.ExitCode != 10 {
		t.Fatalf("exit code = %d, want 10", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if result.Stderr != wantStderr {
		t.Fatalf("stderr = %q, want %q", result.Stderr, wantStderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateContents) {
		t.Fatal("failure output revealed credential file contents")
	}
}

func TestExecuteAuthLogoutAtomicallyRemovesCredentials(t *testing.T) {
	const credentialsPath = "/config/partiful/credentials.json"
	files := &memoryFilesystem{
		files: map[string][]byte{
			credentialsPath: []byte(`{"accessToken":"secret-token-value","expiresAt":"2026-08-11T02:00:00Z"}`),
		},
	}
	dependencies := app.Dependencies{
		Files:           files,
		CredentialsPath: credentialsPath,
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	}

	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "logout"},
		Stdin: strings.NewReader(""),
	}, dependencies)

	const want = `{"ok":true,"data":{"authenticated":false,"tokenState":"missing","expiresAt":null},"meta":{"command":"auth.logout","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}

	status := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, dependencies)
	if !strings.Contains(status.Stdout, `"authenticated":false,"tokenState":"missing","expiresAt":null`) {
		t.Fatalf("status after logout = %q, want missing credentials", status.Stdout)
	}
}

type memoryFilesystem struct {
	files map[string][]byte
}

func (filesystem *memoryFilesystem) ReadFile(path string) ([]byte, error) {
	document, ok := filesystem.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), document...), nil
}

func (filesystem *memoryFilesystem) Remove(path string) error {
	if _, ok := filesystem.files[path]; !ok {
		return fs.ErrNotExist
	}
	delete(filesystem.files, path)
	return nil
}

func TestExecuteAuthLogoutFailureLeavesCredentialsAvailableAndRedactsError(t *testing.T) {
	const privateValue = "private-user-identifier"
	const credentials = `{"accessToken":"secret-token-value","expiresAt":"2026-08-11T02:00:00Z"}`
	files := fakeFilesystem{
		readFile: func(string) ([]byte, error) {
			return []byte(credentials), nil
		},
		remove: func(string) error {
			return errors.New("filesystem failure involving " + privateValue)
		},
	}
	dependencies := app.Dependencies{
		Files:           files,
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	}

	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "logout"},
		Stdin: strings.NewReader(""),
	}, dependencies)

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CREDENTIAL_STORE_UNAVAILABLE","message":"Local credential storage is unavailable.","retryable":false,"details":{}},"meta":{"command":"auth.logout","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 10 {
		t.Fatalf("exit code = %d, want 10", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateValue) {
		t.Fatal("logout failure output revealed a private identifier")
	}

	status := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, dependencies)
	if !strings.Contains(status.Stdout, `"authenticated":true,"tokenState":"healthy"`) {
		t.Fatalf("status after failed logout = %q, want credentials preserved", status.Stdout)
	}
}

func TestExecuteDoctorReportsHealthyCredentialsWithoutPrivateData(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","userId":"private-user-identifier","expiresAt":"2026-08-11T02:00:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":true,"checks":[{"name":"credentials","status":"pass","message":"Authentication credentials are available.","remediation":null}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	for _, privateValue := range []string{"secret-token-value", "private-user-identifier"} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("doctor output contains private value %q", privateValue)
		}
	}
}

func TestExecuteDoctorReportsMissingCredentialsAsARedactedCheck(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return nil, fs.ErrNotExist
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Authentication credentials are missing.","remediation":"Establish authentication before using commands that require it."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteDoctorWarnsWhenCredentialsExpireSoon(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","expiresAt":"2026-08-11T00:04:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":true,"checks":[{"name":"credentials","status":"warn","message":"Authentication credentials expire soon.","remediation":"Refresh authentication before the credentials expire."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteDoctorFailsExpiredCredentialsCheck(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","expiresAt":"2026-08-10T23:59:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Authentication credentials have expired.","remediation":"Re-establish authentication."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteDoctorRedactsInvalidCredentialFile(t *testing.T) {
	const privateContents = "secret-token-content private-user-identifier"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(privateContents), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Authentication credentials are invalid.","remediation":"Remove the invalid credentials and re-establish authentication."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateContents) {
		t.Fatal("doctor output revealed credential file contents")
	}
}

func TestExecuteDoctorRedactsCredentialStorageFailure(t *testing.T) {
	const privateError = "permission failure for private-user-identifier"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return nil, errors.New(privateError)
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Credential storage is unavailable.","remediation":"Check local credential file permissions."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateError) {
		t.Fatal("doctor output revealed filesystem error contents")
	}
}

func TestExecuteSchemaDescribesBothDiscoveryResultShapes(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "schema"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}

	var envelope struct {
		Data struct {
			SuccessSchema any `json:"successSchema"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}

	const expectedLiteral = `{
		"type": "object",
		"oneOf": [
			{
				"type": "object",
				"additionalProperties": false,
				"required": ["commands"],
				"properties": {
					"commands": {"type": "array", "items": {"type": "string"}}
				}
			},
			{
				"type": "object",
				"additionalProperties": false,
				"required": ["command", "positionals", "flags", "inputSchema", "successSchema", "failureTypes", "safety"],
				"properties": {
					"command": {"type": "string"},
					"positionals": {
						"type": "array",
						"items": {
							"type": "object",
							"additionalProperties": false,
							"required": ["name", "required", "description"],
							"properties": {
								"name": {"type": "string"},
								"required": {"type": "boolean"},
								"description": {"type": "string"}
							}
						}
					},
					"flags": {
						"type": "array",
						"items": {
							"type": "object",
							"additionalProperties": false,
							"required": ["name", "required", "description"],
							"properties": {
								"name": {"type": "string"},
								"required": {"type": "boolean"},
								"description": {"type": "string"}
							}
						}
					},
					"inputSchema": {"type": "object"},
					"successSchema": {"type": "object"},
					"failureTypes": {"type": "array", "items": {"type": "string"}},
					"safety": {
						"type": "object",
						"additionalProperties": false,
						"required": ["kind", "planRequired", "confirmationRequired"],
						"properties": {
							"kind": {"type": "string", "enum": ["read-only", "local-mutation"]},
							"planRequired": {"type": "boolean"},
							"confirmationRequired": {"type": "boolean"}
						}
					}
				}
			}
		]
	}`
	var expected any
	if err := json.Unmarshal([]byte(expectedLiteral), &expected); err != nil {
		t.Fatalf("decode expected schema: %v", err)
	}
	if !reflect.DeepEqual(envelope.Data.SuccessSchema, expected) {
		t.Fatalf("success schema = %#v, want %#v", envelope.Data.SuccessSchema, expected)
	}
}

func TestExecuteUsesDefinitionForFlagFailureCommandMetadata(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status", "--non-interactive", "--non-interactive"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"FLAG_REPEATED","message":"A scalar flag cannot be repeated.","retryable":false,"details":{"flag":"--non-interactive"}},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecutePrettyAppliesWhenItFollowsInvalidGlobalFlag(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"auth",
			"status",
			"--non-interactive",
			"--non-interactive",
			"--pretty",
		},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if !strings.HasPrefix(result.Stdout, "{\n  \"ok\": false,") {
		t.Fatalf("stdout = %q, want indented failure envelope", result.Stdout)
	}
	if !strings.Contains(result.Stdout, `"command": "auth.status"`) {
		t.Fatalf("stdout = %q, want command auth.status", result.Stdout)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteUsesKnownCommandMetadataForInvalidArity(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "auth.status", "extra-private-value"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"usage.invalid","code":"COMMAND_NOT_FOUND","message":"Unknown command.","retryable":false,"details":{}},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if strings.Contains(result.Stdout+result.Stderr, "extra-private-value") {
		t.Fatal("invalid-arity output echoed untrusted input")
	}
}

func TestExecuteAuthStatusReportsConfigurationDirectoryFailure(t *testing.T) {
	const privateError = "configuration error containing private-user-identifier"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files:                fakeFilesystem{},
		CredentialsPathError: errors.New(privateError),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CONFIG_DIRECTORY_UNAVAILABLE","message":"Local configuration directory is unavailable.","retryable":false,"details":{}},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1"}}` + "\n"
	if result.ExitCode != 10 {
		t.Fatalf("exit code = %d, want 10", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateError) {
		t.Fatal("configuration failure output revealed private error contents")
	}
}

func TestExecuteDoctorDiagnosesConfigurationDirectoryFailure(t *testing.T) {
	const privateError = "configuration error containing private-user-identifier"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files:                fakeFilesystem{},
		CredentialsPathError: errors.New(privateError),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Configuration directory is unavailable.","remediation":"Set a usable user configuration directory."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-11.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateError) {
		t.Fatal("doctor output revealed configuration error contents")
	}
}
