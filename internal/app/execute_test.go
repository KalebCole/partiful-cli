package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
	"github.com/KalebCole/partiful-cli/internal/auth"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

type scriptedHTTP struct {
	do func(*http.Request) (*http.Response, error)
}

type synchronizedReader struct {
	mutex sync.Mutex
	value byte
}

func (reader *synchronizedReader) Read(buffer []byte) (int, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	for index := range buffer {
		buffer[index] = reader.value
	}
	return len(buffer), nil
}

type failingReadCloser struct {
	err error
}

func (reader failingReadCloser) Read([]byte) (int, error) {
	return 0, reader.err
}

func (failingReadCloser) Close() error {
	return nil
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

func TestExecuteEventsListProjectsReviewedUpcomingEventWithoutPrompting(t *testing.T) {
	const (
		currentAccount = "private-current-account"
		otherOwner     = "private-other-owner"
		credentials    = `{"accessToken":"private-access-token","userId":"` +
			currentAccount +
			`","expiresAt":"2026-08-12T02:00:00Z"}`
	)
	terminal := &scriptedPrivateTerminal{values: []string{"must-not-be-read"}}
	requestCount := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"events", "list", "--when", "upcoming"},
		Stdin: strings.NewReader("must-not-be-read"),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		Terminal:   terminal,
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			requestCount++
			if request.Method != http.MethodPost ||
				request.URL.String() != "https://api.partiful.com/getMyUpcomingEventsForHomePage" {
				t.Fatalf("request = %s %s, want reviewed upcoming operation", request.Method, request.URL)
			}
			if got := request.Header.Get("Authorization"); got != "Bearer private-access-token" {
				t.Fatalf("authorization = %q, want bearer credential", got)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			const wantBody = `{"data":{"params":{},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`
			if string(body) != wantBody {
				t.Fatalf("request body = %s, want %s", body, wantBody)
			}
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"upcomingEvents":[{"id":"event-example","title":"Example event","startDate":"2026-09-12T19:00:00-07:00","endDate":null,"timezone":"America/Los_Angeles","status":"PUBLISHED","ownerIds":["`+otherOwner+`","`+currentAccount+`"],"guest":{"status":"GOING"}}]}}}`,
			), nil
		}},
	})

	const want = `{"ok":true,"data":{"items":[{"eventId":"event-example","title":"Example event","start":"2026-09-12T19:00:00-07:00","end":null,"timezone":"America/Los_Angeles","state":"active","userRole":"host","myRsvp":"going"}]},"meta":{"command":"events.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[],"page":{"limit":25,"nextCursor":null,"hasMore":false}}}` + "\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result = %#v, want reviewed upcoming event projection", result)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want one complete list call", requestCount)
	}
	if len(terminal.prompts) != 0 {
		t.Fatalf("protected command prompted: %#v", terminal.prompts)
	}
	for _, privateValue := range []string{currentAccount, otherOwner, "private-access-token"} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("output exposed private value %q", privateValue)
		}
	}
}

func TestExecuteEventsListRefreshesAndUsesTokenIdentityWithoutPrompting(t *testing.T) {
	const (
		currentAccount = "private-current-account"
		newAccessToken = "eyJhbGciOiJub25lIn0." +
			"eyJzdWIiOiJwcml2YXRlLWN1cnJlbnQtYWNjb3VudCJ9." +
			"private-signature"
		credentialsPath = "/config/partiful/credentials.json"
	)
	files := &memoryFilesystem{files: map[string][]byte{
		credentialsPath: []byte(
			`{"accessToken":"private-expired-access","refreshToken":"private-refresh-token","expiresAt":"2026-08-11T23:59:00Z"}`,
		),
	}}
	terminal := &scriptedPrivateTerminal{values: []string{"must-not-be-read"}}
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"events", "list", "--when", "upcoming"},
	}, app.Dependencies{
		Files:           files,
		CredentialsPath: credentialsPath,
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		Terminal:   terminal,
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				if request.URL.Host != "securetoken.googleapis.com" {
					t.Fatalf("first request host = %q, want refresh", request.URL.Host)
				}
				return jsonResponse(
					http.StatusOK,
					`{"access_token":"private-access-alias","id_token":"`+newAccessToken+`","refresh_token":"private-new-refresh","expires_in":"3600","token_type":"Bearer","project_id":"private-project"}`,
				), nil
			case 2:
				if request.URL.String() !=
					"https://api.partiful.com/getMyUpcomingEventsForHomePage" {
					t.Fatalf("second request = %s, want upcoming events", request.URL)
				}
				if got := request.Header.Get("Authorization"); got != "Bearer "+newAccessToken {
					t.Fatalf("authorization = %q, want refreshed bearer", got)
				}
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":{"upcomingEvents":[{"id":"event-example","ownerIds":["`+currentAccount+`"]}]}}}`,
				), nil
			default:
				t.Fatalf("unexpected request %d: %s", call, request.URL)
				return nil, nil
			}
		}},
	})

	const want = `{"ok":true,"data":{"items":[{"eventId":"event-example","title":null,"start":null,"end":null,"timezone":null,"state":null,"userRole":"host","myRsvp":null}]},"meta":{"command":"events.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[],"page":{"limit":25,"nextCursor":null,"hasMore":false}}}` + "\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result = %#v, want refreshed identity projection", result)
	}
	if call != 2 || files.atomicWrites != 1 {
		t.Fatalf("requests = %d, writes = %d, want one refresh and one event read", call, files.atomicWrites)
	}
	if len(terminal.prompts) != 0 {
		t.Fatalf("protected refresh prompted: %#v", terminal.prompts)
	}
	for _, privateValue := range []string{
		currentAccount,
		newAccessToken,
		"private-refresh-token",
		"private-new-refresh",
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("output exposed private value %q", privateValue)
		}
	}
}

func TestExecuteProtectedEventSessionSerializesRefresh(t *testing.T) {
	const credentialsPath = "/config/partiful/credentials.json"
	files := &memoryFilesystem{files: map[string][]byte{
		credentialsPath: []byte(
			`{"accessToken":"private-expired-access","refreshToken":"private-refresh-token","expiresAt":"2026-08-11T23:59:00Z"}`,
		),
	}}
	terminal := &scriptedPrivateTerminal{values: []string{"must-not-be-read"}}
	random := &synchronizedReader{value: 'x'}
	var httpMutex sync.Mutex
	refreshCalls := 0
	eventCalls := 0
	dependencies := app.Dependencies{
		Files:           files,
		CredentialsPath: credentialsPath,
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: random,
		Terminal:   terminal,
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			httpMutex.Lock()
			defer httpMutex.Unlock()
			switch request.URL.Host {
			case "securetoken.googleapis.com":
				refreshCalls++
				return jsonResponse(
					http.StatusOK,
					`{"access_token":"private-access-alias","id_token":"private-new-access","refresh_token":"private-new-refresh","expires_in":"3600","token_type":"Bearer"}`,
				), nil
			case "api.partiful.com":
				eventCalls++
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":{"upcomingEvents":[]}}}`,
				), nil
			default:
				t.Fatalf("unexpected request %s", request.URL)
				return nil, nil
			}
		}},
	}

	results := make(chan app.Result, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- app.Execute(context.Background(), app.Request{
				Argv: []string{"events", "list", "--when", "upcoming"},
			}, dependencies)
		}()
	}
	group.Wait()
	close(results)
	for result := range results {
		if result.ExitCode != 0 ||
			!strings.Contains(result.Stdout, `"items":[]`) ||
			result.Stderr != "" {
			t.Fatalf("result = %#v, want serialized protected read", result)
		}
	}
	if refreshCalls != 1 || eventCalls != 2 || files.atomicWrites != 1 {
		t.Fatalf(
			"refreshes = %d, events = %d, writes = %d, want 1/2/1",
			refreshCalls,
			eventCalls,
			files.atomicWrites,
		)
	}
	if len(terminal.prompts) != 0 {
		t.Fatalf("protected commands prompted: %#v", terminal.prompts)
	}
}

func TestExecuteEventsListMapsInvalidCredentialStorageWithoutPrompting(t *testing.T) {
	const privateCredential = "private-invalid-credential"
	terminal := &scriptedPrivateTerminal{values: []string{"must-not-be-read"}}
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"events", "list", "--when", "upcoming"},
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(privateCredential), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        terminal,
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			t.Fatal("remote must not be called")
			return nil, nil
		}},
	})

	if result.ExitCode != 10 ||
		!strings.Contains(result.Stdout, `"type":"internal.failure"`) ||
		!strings.Contains(result.Stdout, `"code":"CREDENTIALS_INVALID"`) {
		t.Fatalf("result = %#v, want invalid credential storage mapping", result)
	}
	if len(terminal.prompts) != 0 {
		t.Fatalf("protected command prompted: %#v", terminal.prompts)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateCredential) {
		t.Fatal("invalid storage failure exposed private content")
	}
}

func TestExecuteProtectedEventReadsRequireAuthenticationWithoutPrompting(t *testing.T) {
	terminal := &scriptedPrivateTerminal{values: []string{"must-not-be-read"}}
	for _, argv := range [][]string{
		{"events", "list", "--when", "upcoming"},
		{"events", "get", "event-example"},
	} {
		result := app.Execute(context.Background(), app.Request{
			Argv: argv,
		}, app.Dependencies{
			Files: fakeFilesystem{
				readFile: func(string) ([]byte, error) {
					return nil, fs.ErrNotExist
				},
			},
			CredentialsPath: "/config/partiful/credentials.json",
			Terminal:        terminal,
			AuthRandom:      strings.NewReader("must-not-be-read"),
			HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
				t.Fatal("remote must not be called")
				return nil, nil
			}},
		})

		if result.ExitCode != 3 ||
			!strings.Contains(result.Stdout, `"type":"auth.required"`) ||
			!strings.Contains(result.Stdout, `"code":"AUTHENTICATION_REQUIRED"`) {
			t.Fatalf("%v result = %#v, want authentication requirement", argv, result)
		}
	}
	if len(terminal.prompts) != 0 {
		t.Fatalf("protected commands prompted: %#v", terminal.prompts)
	}
}

func TestExecuteEventsListProjectsPastRolesAndEveryReviewedRSVP(t *testing.T) {
	const credentials = `{"accessToken":"private-access-token","userId":"private-current-account","expiresAt":"2026-08-12T02:00:00Z"}`
	statuses := []string{
		"READY_TO_SEND",
		"SENDING",
		"SEND_ERROR",
		"DELIVERY_ERROR",
		"SENT",
		"INTERESTED",
		"WAITLIST",
		"MAYBE",
		"DECLINED",
		"GOING",
		"PENDING_APPROVAL",
		"APPROVED",
		"WITHDRAWN",
		"WAITLISTED_FOR_APPROVAL",
		"REJECTED",
		"RESPONDED_TO_FIND_A_TIME",
	}
	readValues := []string{
		"ready-to-send",
		"sending",
		"send-error",
		"delivery-error",
		"sent",
		"interested",
		"waitlist",
		"maybe",
		"declined",
		"going",
		"pending-approval",
		"approved",
		"withdrawn",
		"waitlisted-for-approval",
		"rejected",
		"responded-to-find-a-time",
	}
	items := make([]string, 0, len(statuses))
	for index, status := range statuses {
		owners := `["private-other-owner"]`
		switch index {
		case 0:
			owners = `["private-current-account"]`
		case 2:
			owners = `[]`
		case 3:
			owners = ""
		}
		ownerProperty := ""
		if owners != "" {
			ownerProperty = `,"ownerIds":` + owners
		}
		items = append(items, fmt.Sprintf(
			`{"id":"event-%02d","status":"CANCELED"%s,"guest":{"status":"%s"}}`,
			index,
			ownerProperty,
			status,
		))
	}
	items = append(
		items,
		`{"id":"event-none","ownerIds":["private-other-owner"]}`,
	)
	terminal := &scriptedPrivateTerminal{values: []string{"must-not-be-read"}}
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"events",
			"list",
			"--when",
			"past",
			"--all",
			"--max-items",
			"1000",
		},
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		Terminal:   terminal,
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			if request.URL.String() !=
				"https://api.partiful.com/getMyPastEventsForHomePage" {
				t.Fatalf("request = %s, want reviewed past operation", request.URL)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			const wantBody = `{"data":{"params":{},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`
			if string(body) != wantBody {
				t.Fatalf("request body = %s, want %s", body, wantBody)
			}
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"pastEvents":[`+strings.Join(items, ",")+`]}}}`,
			), nil
		}},
	})

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Items []struct {
				EventID  string  `json:"eventId"`
				State    *string `json:"state"`
				UserRole *string `json:"userRole"`
				MyRSVP   *string `json:"myRsvp"`
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
		t.Fatalf("decode output: %v", err)
	}
	if result.ExitCode != 0 ||
		!envelope.OK ||
		len(envelope.Data.Items) != len(statuses)+1 {
		t.Fatalf("result = %#v, output = %#v, want all reviewed RSVP projections", result, envelope)
	}
	wantRoles := []*string{
		stringPointer("host"),
		stringPointer("attendee"),
		stringPointer("attendee"),
		nil,
	}
	for index, item := range envelope.Data.Items[:len(statuses)] {
		if item.EventID != fmt.Sprintf("event-%02d", index) ||
			item.State == nil ||
			*item.State != "cancelled" ||
			item.MyRSVP == nil ||
			*item.MyRSVP != readValues[index] {
			t.Fatalf("item %d = %#v, want exact lossless mapping", index, item)
		}
		if index < len(wantRoles) && !reflect.DeepEqual(item.UserRole, wantRoles[index]) {
			t.Fatalf("item %d role = %#v, want %#v", index, item.UserRole, wantRoles[index])
		}
	}
	noneItem := envelope.Data.Items[len(statuses)]
	if noneItem.EventID != "event-none" ||
		noneItem.State != nil ||
		noneItem.UserRole == nil ||
		*noneItem.UserRole != "none" ||
		noneItem.MyRSVP != nil {
		t.Fatalf("non-attendee item = %#v, want none role and null RSVP", noneItem)
	}
	if envelope.Meta.Page.Limit != 1000 ||
		envelope.Meta.Page.NextCursor != nil ||
		envelope.Meta.Page.HasMore {
		t.Fatalf("page = %#v, want complete local representation", envelope.Meta.Page)
	}
	if len(terminal.prompts) != 0 {
		t.Fatalf("protected command prompted: %#v", terminal.prompts)
	}
}

func TestExecuteEventsListRejectsUnmappedStatusOutsideRequestedPage(t *testing.T) {
	const privateDraftTitle = "private draft event"
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"events", "list", "--when", "upcoming", "--limit", "1"},
	}, withTestCursorCrypto(app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"private-access-token","expiresAt":"2026-08-12T02:00:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"upcomingEvents":[{"id":"event-visible","status":"PUBLISHED"},{"id":"event-hidden","title":"`+privateDraftTitle+`","status":"UNSAVED"}]}}}`,
			), nil
		}},
	}))

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) ||
		!strings.Contains(result.Stdout, `"code":"EVENT_LIST_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want unmapped complete-response status failure", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateDraftTitle) {
		t.Fatal("protocol failure exposed hidden event data")
	}
}

func TestExecuteEventsListUsesDigestAndFilterBoundLocalCursor(t *testing.T) {
	const (
		credentials = `{"accessToken":"private-access-token","expiresAt":"2026-08-12T02:00:00Z"}`
		firstBody   = `{"result":{"data":{"upcomingEvents":[{"id":"event-a"},{"id":"event-b"}]}}}`
		changedBody = `{"result":{"data":{"upcomingEvents":[{"id":"event-replacement"},{"id":"event-b"}]}}}`
	)
	call := 0
	dependencies := withTestCursorCrypto(app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader(strings.Repeat("0123456789abcdef", 4)),
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			if request.URL.String() !=
				"https://api.partiful.com/getMyUpcomingEventsForHomePage" {
				t.Fatalf("request = %s, want upcoming refetch", request.URL)
			}
			if call <= 2 {
				return jsonResponse(http.StatusOK, firstBody), nil
			}
			return jsonResponse(http.StatusOK, changedBody), nil
		}},
	})
	first := app.Execute(context.Background(), app.Request{
		Argv: []string{"events", "list", "--when", "upcoming", "--limit", "1"},
	}, dependencies)
	var firstEnvelope struct {
		Data struct {
			Items []struct {
				EventID string `json:"eventId"`
			} `json:"items"`
		} `json:"data"`
		Meta struct {
			Page struct {
				NextCursor *string `json:"nextCursor"`
				HasMore    bool    `json:"hasMore"`
			} `json:"page"`
		} `json:"meta"`
	}
	if json.Unmarshal([]byte(first.Stdout), &firstEnvelope) != nil ||
		first.ExitCode != 0 ||
		len(firstEnvelope.Data.Items) != 1 ||
		firstEnvelope.Data.Items[0].EventID != "event-a" ||
		firstEnvelope.Meta.Page.NextCursor == nil ||
		!firstEnvelope.Meta.Page.HasMore {
		t.Fatalf("first result = %#v, output = %#v, want resumable local page", first, firstEnvelope)
	}
	cursor := *firstEnvelope.Meta.Page.NextCursor

	second := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"events", "list", "--when", "upcoming", "--limit", "1", "--cursor", cursor,
		},
	}, dependencies)
	if second.ExitCode != 0 ||
		!strings.Contains(second.Stdout, `"eventId":"event-b"`) ||
		!strings.Contains(second.Stdout, `"nextCursor":null`) {
		t.Fatalf("second result = %#v, want exact local resume", second)
	}

	filterMismatch := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"events", "list", "--when", "past", "--limit", "1", "--cursor", cursor,
		},
	}, dependencies)
	if filterMismatch.ExitCode != 2 ||
		!strings.Contains(filterMismatch.Stdout, `"code":"CURSOR_FILTER_MISMATCH"`) {
		t.Fatalf("filter mismatch = %#v, want cursor filter rejection", filterMismatch)
	}
	if call != 2 {
		t.Fatalf("request count after filter mismatch = %d, want no mismatched refetch", call)
	}

	changed := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"events", "list", "--when", "upcoming", "--limit", "1", "--cursor", cursor,
		},
	}, dependencies)
	if changed.ExitCode != 6 ||
		!strings.Contains(changed.Stdout, `"type":"state.conflict"`) ||
		!strings.Contains(changed.Stdout, `"code":"CURSOR_SNAPSHOT_CHANGED"`) {
		t.Fatalf("changed result = %#v, want digest conflict", changed)
	}
	for _, privateValue := range []string{"event-a", "event-b", "event-replacement"} {
		if strings.Contains(changed.Stdout+changed.Stderr, privateValue) {
			t.Fatalf("snapshot conflict exposed event value %q", privateValue)
		}
	}
}

func TestExecuteEventsListFailsClosedInsteadOfTruncatingLocalBounds(t *testing.T) {
	itemDocuments := make([]string, 1001)
	for index := range itemDocuments {
		itemDocuments[index] = fmt.Sprintf(`{"id":"event-%04d"}`, index)
	}
	cases := []struct {
		name string
		body string
	}{
		{
			name: "item count",
			body: `{"result":{"data":{"upcomingEvents":[` +
				strings.Join(itemDocuments, ",") +
				`]}}}`,
		},
		{
			name: "body bytes",
			body: `{"result":{"data":{"upcomingEvents":[{"id":"event-large","title":"` +
				strings.Repeat("private-large-value", (8<<20)/len("private-large-value")+1) +
				`"}]}}}`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := app.Execute(context.Background(), app.Request{
				Argv: []string{"events", "list", "--when", "upcoming", "--all", "--max-items", "1000"},
			}, app.Dependencies{
				Files: fakeFilesystem{
					readFile: func(string) ([]byte, error) {
						return []byte(
							`{"accessToken":"private-access-token","expiresAt":"2026-08-12T02:00:00Z"}`,
						), nil
					},
				},
				CredentialsPath: "/config/partiful/credentials.json",
				Now: func() time.Time {
					return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
				},
				AuthRandom: strings.NewReader("0123456789abcdef"),
				HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
					return jsonResponse(http.StatusOK, testCase.body), nil
				}},
			})

			if result.ExitCode != 9 ||
				!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) ||
				!strings.Contains(result.Stdout, `"code":"EVENT_LIST_BOUND_EXCEEDED"`) ||
				strings.Contains(result.Stdout, `"ok":true`) {
				t.Fatalf("result = %#v, want closed bound failure without truncation", result)
			}
		})
	}
}

func TestExecuteEventsGetProjectsReviewedDetailAndNullUnavailableFields(t *testing.T) {
	const (
		credentials        = `{"accessToken":"private-access-token","expiresAt":"2026-08-12T02:00:00Z"}`
		privateRemoteID    = "private-different-remote-event"
		privateDescription = "private unprojected description"
	)
	terminal := &scriptedPrivateTerminal{values: []string{"must-not-be-read"}}
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"events", "get", "event-example"},
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		Terminal:   terminal,
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodPost ||
				request.URL.String() != "https://api.partiful.com/getEventInfo" {
				t.Fatalf("request = %s %s, want reviewed event detail operation", request.Method, request.URL)
			}
			if got := request.Header.Get("Authorization"); got != "Bearer private-access-token" {
				t.Fatalf("authorization = %q, want bearer credential", got)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			const wantBody = `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`
			if string(body) != wantBody {
				t.Fatalf("request body = %s, want %s", body, wantBody)
			}
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"event":{"id":"`+privateRemoteID+`","title":"Example event","startDate":"2026-09-12T19:00:00-07:00","endDate":"2026-09-12T22:00:00-07:00","timezone":"America/Los_Angeles","status":"CANCELED","description":"`+privateDescription+`","location":"private unreviewed location","links":["private unreviewed link"]}}}}`,
			), nil
		}},
	})

	const want = `{"ok":true,"data":{"eventId":"event-example","title":"Example event","start":"2026-09-12T19:00:00-07:00","end":"2026-09-12T22:00:00-07:00","timezone":"America/Los_Angeles","state":"cancelled","userRole":null,"myRsvp":null,"description":null,"location":null,"address":null,"visibility":null,"guestLimit":null,"poster":null,"links":null},"meta":{"command":"events.get","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result = %#v, want reviewed nullable event detail", result)
	}
	if len(terminal.prompts) != 0 {
		t.Fatalf("protected command prompted: %#v", terminal.prompts)
	}
	for _, privateValue := range []string{
		privateRemoteID,
		privateDescription,
		"private unreviewed location",
		"private unreviewed link",
		"private-access-token",
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("output exposed unavailable or private value %q", privateValue)
		}
	}
}

func TestExecuteEventsGetMapsOnlyReviewedFailuresAndObjectVariants(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		remoteErr   error
		exitCode    int
		failureType string
		code        string
	}{
		{
			name:        "reviewed not found",
			status:      http.StatusNotFound,
			body:        `{"error":{"message":"private missing event detail","status":"NOT_FOUND"}}`,
			exitCode:    5,
			failureType: "resource.not_found",
			code:        "EVENT_NOT_FOUND",
		},
		{
			name:        "unobserved forbidden",
			status:      http.StatusForbidden,
			body:        `{"error":{"message":"private forbidden detail","status":"PERMISSION_DENIED"}}`,
			exitCode:    9,
			failureType: "contract.protocol_changed",
			code:        "EVENT_PROTOCOL_CHANGED",
		},
		{
			name:        "null event",
			status:      http.StatusOK,
			body:        `{"result":{"data":{"event":null}}}`,
			exitCode:    9,
			failureType: "contract.protocol_changed",
			code:        "EVENT_PROTOCOL_CHANGED",
		},
		{
			name:        "scalar event",
			status:      http.StatusOK,
			body:        `{"result":{"data":{"event":"private scalar"}}}`,
			exitCode:    9,
			failureType: "contract.protocol_changed",
			code:        "EVENT_PROTOCOL_CHANGED",
		},
		{
			name:        "array event",
			status:      http.StatusOK,
			body:        `{"result":{"data":{"event":[]}}}`,
			exitCode:    9,
			failureType: "contract.protocol_changed",
			code:        "EVENT_PROTOCOL_CHANGED",
		},
		{
			name:        "unmapped draft",
			status:      http.StatusOK,
			body:        `{"result":{"data":{"event":{"status":"UNSAVED","title":"private draft"}}}}`,
			exitCode:    9,
			failureType: "contract.protocol_changed",
			code:        "EVENT_PROTOCOL_CHANGED",
		},
		{
			name:        "transport unavailable",
			remoteErr:   errors.New("private transport failure"),
			exitCode:    8,
			failureType: "remote.unavailable",
			code:        "EVENT_UNAVAILABLE",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := app.Execute(context.Background(), app.Request{
				Argv: []string{"events", "get", "event-example"},
			}, app.Dependencies{
				Files: fakeFilesystem{
					readFile: func(string) ([]byte, error) {
						return []byte(
							`{"accessToken":"private-access-token","expiresAt":"2026-08-12T02:00:00Z"}`,
						), nil
					},
				},
				CredentialsPath: "/config/partiful/credentials.json",
				Now: func() time.Time {
					return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
				},
				AuthRandom: strings.NewReader("0123456789abcdef"),
				HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
					if strings.Contains(request.URL.Path, "firestore") ||
						strings.Contains(request.URL.Path, "getCurrentGuest") ||
						strings.Contains(request.URL.Path, "firestoreGetGuest") {
						t.Fatalf("called forbidden S3 operation %s", request.URL)
					}
					if testCase.remoteErr != nil {
						return nil, testCase.remoteErr
					}
					return jsonResponse(testCase.status, testCase.body), nil
				}},
			})

			if result.ExitCode != testCase.exitCode ||
				!strings.Contains(result.Stdout, `"type":"`+testCase.failureType+`"`) ||
				!strings.Contains(result.Stdout, `"code":"`+testCase.code+`"`) {
				t.Fatalf("result = %#v, want %s/%s", result, testCase.failureType, testCase.code)
			}
			for _, privateValue := range []string{
				"private missing event detail",
				"private forbidden detail",
				"private scalar",
				"private draft",
				"private transport failure",
			} {
				if strings.Contains(result.Stdout+result.Stderr, privateValue) {
					t.Fatalf("failure exposed private value %q", privateValue)
				}
			}
		})
	}
}

func TestExecuteRSVPGetProjectsReviewedGuestWithoutPrivateIdentifiers(t *testing.T) {
	const (
		privateGuestID = "private-guest-id"
		privateUserID  = "private-user-id"
		privateName    = "Private Guest Name"
	)
	requestCount := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"rsvp", "get", "event-example"},
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"private-access-token","userId":"private-account","expiresAt":"2026-08-12T02:00:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			requestCount++
			if request.Method != http.MethodPost ||
				request.URL.String() != "https://api.partiful.com/getCurrentGuest" {
				t.Fatalf("request = %s %s, want reviewed current-guest operation", request.Method, request.URL)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			const wantBody = `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`
			if string(body) != wantBody {
				t.Fatalf("request body = %s, want %s", body, wantBody)
			}
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"currentGuest":{"id":"`+privateGuestID+`","count":2,"status":"APPROVED","name":"`+privateName+`","plusOnes":null,"userId":"`+privateUserID+`"}}}}`,
			), nil
		}},
	})

	if result.ExitCode != 0 ||
		!strings.Contains(result.Stdout, `"data":{"eventId":"event-example","status":"approved"}`) ||
		result.Stderr != "" {
		t.Fatalf("result = %#v, want reviewed RSVP projection", result)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want one current-guest read", requestCount)
	}
	for _, privateValue := range []string{
		privateGuestID,
		privateUserID,
		privateName,
		"private-account",
		"private-access-token",
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("output exposed private value %q", privateValue)
		}
	}
}

func TestExecuteSchemaProjectsCompleteEventReadDefinitions(t *testing.T) {
	type schemaEnvelope struct {
		Data struct {
			Command     string `json:"command"`
			Positionals []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"positionals"`
			Flags []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"flags"`
			InputSchema struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					Enum      []string `json:"enum"`
					MinLength *int     `json:"minLength"`
				} `json:"properties"`
			} `json:"inputSchema"`
			SuccessSchema struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					Type  any `json:"type"`
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

	listResult := app.Execute(context.Background(), app.Request{
		Argv: []string{"schema", "events.list"},
	}, app.Dependencies{})
	var list schemaEnvelope
	if json.Unmarshal([]byte(listResult.Stdout), &list) != nil || listResult.ExitCode != 0 {
		t.Fatalf("list schema result = %#v", listResult)
	}
	if list.Data.Command != "events.list" ||
		!reflect.DeepEqual(list.Data.InputSchema.Required, []string{"when"}) ||
		!reflect.DeepEqual(
			list.Data.InputSchema.Properties["when"].Enum,
			[]string{"upcoming", "past"},
		) {
		t.Fatalf("list input schema = %#v, want required closed when", list.Data)
	}
	var listFlags []string
	for _, flag := range list.Data.Flags {
		listFlags = append(listFlags, fmt.Sprintf("%s:%t", flag.Name, flag.Required))
	}
	if !reflect.DeepEqual(listFlags, []string{
		"--when:true",
		"--limit:false",
		"--cursor:false",
		"--all:false",
		"--max-items:false",
	}) {
		t.Fatalf("list flags = %v, want event collection flags", listFlags)
	}
	wantSummaryFields := []string{
		"eventId", "title", "start", "end", "timezone", "state", "userRole", "myRsvp",
	}
	if got := list.Data.SuccessSchema.Properties["items"].Items.Required; !reflect.DeepEqual(got, wantSummaryFields) {
		t.Fatalf("summary fields = %v, want %v", got, wantSummaryFields)
	}

	getResult := app.Execute(context.Background(), app.Request{
		Argv: []string{"schema", "events.get"},
	}, app.Dependencies{})
	var get schemaEnvelope
	if json.Unmarshal([]byte(getResult.Stdout), &get) != nil || getResult.ExitCode != 0 {
		t.Fatalf("get schema result = %#v", getResult)
	}
	if get.Data.Command != "events.get" ||
		len(get.Data.Positionals) != 1 ||
		get.Data.Positionals[0].Name != "event-id" ||
		!get.Data.Positionals[0].Required ||
		len(get.Data.Flags) != 0 ||
		!reflect.DeepEqual(get.Data.InputSchema.Required, []string{"eventId"}) ||
		get.Data.InputSchema.Properties["eventId"].MinLength == nil ||
		*get.Data.InputSchema.Properties["eventId"].MinLength != 1 {
		t.Fatalf("get input schema = %#v, want required event ID", get.Data)
	}
	wantEventFields := []string{
		"eventId",
		"title",
		"start",
		"end",
		"timezone",
		"state",
		"userRole",
		"myRsvp",
		"description",
		"location",
		"address",
		"visibility",
		"guestLimit",
		"poster",
		"links",
	}
	if !reflect.DeepEqual(get.Data.SuccessSchema.Required, wantEventFields) ||
		get.Data.SuccessSchema.Properties["userRole"].Type != "null" ||
		get.Data.SuccessSchema.Properties["myRsvp"].Type != "null" ||
		get.Data.SuccessSchema.Properties["links"].Type != "null" {
		t.Fatalf("event success schema = %#v, want nullable S3 event", get.Data.SuccessSchema)
	}
	for _, definition := range []schemaEnvelope{list, get} {
		if definition.Data.Safety.Kind != "read-only" ||
			definition.Data.Safety.PlanRequired ||
			definition.Data.Safety.ConfirmationRequired {
			t.Fatalf("safety = %#v, want read-only event command", definition.Data.Safety)
		}
		wantFailures := []string{
			"usage.invalid",
			"input.invalid",
			"auth.required",
			"auth.expired",
		}
		if !reflect.DeepEqual(
			definition.Data.FailureTypes[:len(wantFailures)],
			wantFailures,
		) {
			t.Fatalf("failure types = %v, want auth contract prefix", definition.Data.FailureTypes)
		}
	}
}

func stringPointer(value string) *string {
	return &value
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

	const want = `{"ok":true,"data":{"items":[{"posterId":"first","name":"First","url":"https://example.invalid/first.png","contentType":"image/png","width":1200,"height":630,"tags":["party"],"categories":["fun"]},{"posterId":"duplicate","name":"Duplicate","url":"https://example.invalid/duplicate.gif","contentType":"image/gif","width":null,"height":null,"tags":["dance"],"categories":[]},{"posterId":"duplicate","name":"Duplicate Again","url":"https://example.invalid/duplicate-2.gif","contentType":"image/gif","width":640,"height":480,"tags":[],"categories":["night"]}]},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[],"page":{"limit":25,"nextCursor":null,"hasMore":false}}}` + "\n"
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

func TestExecutePostersListAllResumesFromReturnedCursor(t *testing.T) {
	const catalog = `[
		{"id":"one","name":"One","url":"https://example.invalid/one.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"two","name":"Two","url":"https://example.invalid/two.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"three","name":"Three","url":"https://example.invalid/three.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"four","name":"Four","url":"https://example.invalid/four.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]},
		{"id":"five","name":"Five","url":"https://example.invalid/five.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]}
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
		Argv:  []string{"posters", "list", "--all", "--max-items", "2"},
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

	second := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"posters", "list",
			"--all", "--max-items", "2",
			"--cursor", firstEnvelope.Meta.Page.NextCursor,
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
	var got []string
	for _, item := range secondEnvelope.Data.Items {
		got = append(got, item.PosterID)
	}
	if second.ExitCode != 0 || !reflect.DeepEqual(got, []string{"three", "four"}) {
		t.Fatalf("second result = %#v, IDs = %v, want resumed all page", second, got)
	}
	if !secondEnvelope.Meta.Page.HasMore || secondEnvelope.Meta.Page.NextCursor == nil {
		t.Fatalf("second page = %#v, want another resumable boundary", secondEnvelope.Meta.Page)
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

	const want = `{"ok":false,"error":{"type":"state.conflict","code":"CURSOR_SNAPSHOT_CHANGED","message":"The poster catalog changed after this cursor was issued.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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
				Required          []string            `json:"required"`
				DependentRequired map[string][]string `json:"dependentRequired"`
				AllOf             []struct {
					Not struct {
						Required []string `json:"required"`
					} `json:"not"`
				} `json:"allOf"`
				Properties map[string]struct {
					Minimum   *int   `json:"minimum"`
					Maximum   *int   `json:"maximum"`
					MinLength *int   `json:"minLength"`
					Pattern   string `json:"pattern"`
				} `json:"properties"`
			} `json:"inputSchema"`
			SuccessSchema struct {
				Properties map[string]struct {
					Items struct {
						Required   []string `json:"required"`
						Properties map[string]struct {
							Minimum *int `json:"minimum"`
						} `json:"properties"`
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
	wantDependencies := map[string][]string{
		"all":      {"maxItems"},
		"maxItems": {"all"},
	}
	if !reflect.DeepEqual(envelope.Data.InputSchema.DependentRequired, wantDependencies) {
		t.Fatalf(
			"input dependencies = %#v, want %#v",
			envelope.Data.InputSchema.DependentRequired,
			wantDependencies,
		)
	}
	if len(envelope.Data.InputSchema.AllOf) != 1 ||
		!reflect.DeepEqual(
			envelope.Data.InputSchema.AllOf[0].Not.Required,
			[]string{"all", "limit"},
		) {
		t.Fatalf(
			"input exclusions = %#v, want --all and --limit to be mutually exclusive",
			envelope.Data.InputSchema.AllOf,
		)
	}
	limit := envelope.Data.InputSchema.Properties["limit"]
	maxItems := envelope.Data.InputSchema.Properties["maxItems"]
	query := envelope.Data.InputSchema.Properties["query"]
	cursor := envelope.Data.InputSchema.Properties["cursor"]
	if limit.Minimum == nil || *limit.Minimum != 1 ||
		limit.Maximum == nil || *limit.Maximum != 100 ||
		maxItems.Minimum == nil || *maxItems.Minimum != 1 ||
		maxItems.Maximum == nil || *maxItems.Maximum != 1000 ||
		query.MinLength == nil || *query.MinLength != 1 ||
		query.Pattern != `\S` ||
		cursor.MinLength == nil || *cursor.MinLength != 1 {
		t.Fatalf("input constraints = %#v, want documented collection bounds", envelope.Data.InputSchema.Properties)
	}
	if strings.Contains(result.Stdout, `"type":null`) {
		t.Fatalf("schema contains invalid null type: %s", result.Stdout)
	}
	wantPosterFields := []string{"posterId", "name", "url", "contentType", "width", "height", "tags", "categories"}
	if got := envelope.Data.SuccessSchema.Properties["items"].Items.Required; !reflect.DeepEqual(got, wantPosterFields) {
		t.Fatalf("poster fields = %v, want %v", got, wantPosterFields)
	}
	wantFailures := []string{"usage.invalid", "input.invalid", "state.conflict", "remote.unavailable", "contract.protocol_changed", "internal.failure"}
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

	const wantStdout = `{"ok":false,"error":{"type":"contract.protocol_changed","code":"POSTER_CATALOG_PROTOCOL_CHANGED","message":"The poster catalog no longer matches the reviewed remote contract.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"CURSOR_INVALID","message":"The cursor is malformed.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"CURSOR_FILTER_MISMATCH","message":"The cursor does not match this command and filters.","retryable":false,"details":{}},"meta":{"command":"posters.search","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const wantStdout = `{"ok":false,"error":{"type":"remote.unavailable","code":"POSTER_CATALOG_UNAVAILABLE","message":"The poster catalog is unavailable.","retryable":true,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const wantStdout = `{"ok":false,"error":{"type":"contract.protocol_changed","code":"POSTER_CATALOG_PROTOCOL_CHANGED","message":"The poster catalog no longer matches the reviewed remote contract.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const wantStdout = `{"ok":false,"error":{"type":"contract.protocol_changed","code":"POSTER_CATALOG_PROTOCOL_CHANGED","message":"The poster catalog no longer matches the reviewed remote contract.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

func TestExecutePostersListRejectsMalformedUTF8(t *testing.T) {
	body := append(
		[]byte(`[{"id":"poster","name":"`),
		0xff,
	)
	body = append(
		body,
		[]byte(`","url":"https://example.invalid/poster.png","contentType":"image/png","width":1,"height":1,"tags":[],"categories":[]}]`)...,
	)
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(body)),
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

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"MAX_ITEMS_REQUIRED","message":"--all requires --max-items.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

func TestExecutePostersListRejectsLimitWithAll(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"posters", "list",
			"--limit", "10",
			"--all", "--max-items", "100",
		},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"LIMIT_WITH_ALL","message":"--limit cannot be combined with --all.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"LIMIT_INVALID","message":"Limit must be an integer from 1 to 100.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
}

func TestExecutePostersListRejectsEmptyCursor(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "list", "--cursor", ""},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"CURSOR_INVALID","message":"The cursor is malformed.","retryable":false,"details":{}},"meta":{"command":"posters.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

func TestExecutePostersSearchRequiresNonEmptyQuery(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"posters", "search", "--query", "   "},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"QUERY_REQUIRED","message":"Search query must not be empty.","retryable":false,"details":{}},"meta":{"command":"posters.search","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const want = `{"ok":true,"data":{"version":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"},"meta":{"command":"version","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

func TestExecuteEventsListRequiresWhen(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"events", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"WHEN_INVALID","message":"--when must be upcoming or past.","retryable":false,"details":{}},"meta":{"command":"events.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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
    "productContractRevision": "2026-08-12.5",
    "remoteContractRevision": "2026-08-12.5"
  },
  "meta": {
    "command": "version",
    "cliVersion": "1.0.0",
    "productContractRevision": "2026-08-12.5",
    "remoteContractRevision": "2026-08-12.5",
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

	const want = `{"ok":true,"data":{"version":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"},"meta":{"command":"version","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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
    "productContractRevision": "2026-08-12.5",
    "remoteContractRevision": "2026-08-12.5"
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

	const want = `{"ok":true,"data":{"commands":["auth.login","auth.logout","auth.status","contacts.list","doctor","events.cancel","events.create","events.get","events.list","events.update","guests.invite","guests.list","posters.list","posters.search","rsvp.get","rsvp.set","schema","version"]},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

	const want = `{"ok":true,"data":{"command":"auth.status","positionals":[],"flags":[],"inputSchema":{"type":"object","additionalProperties":false},"successSchema":{"type":"object","additionalProperties":false,"required":["authenticated","tokenState","expiresAt"],"properties":{"authenticated":{"type":"boolean"},"expiresAt":{"type":["string","null"],"format":"date-time"},"tokenState":{"type":"string","enum":["healthy","expiring","expired","missing"]}}},"failureTypes":["usage.invalid","input.invalid","auth.expired","remote.unavailable","contract.protocol_changed","internal.failure"],"safety":{"kind":"local-mutation","planRequired":false,"confirmationRequired":false}},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

func TestExecuteSchemaProjectsCompleteAuthLoginDefinition(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"schema", "auth.login"},
	}, app.Dependencies{})

	var envelope struct {
		Data struct {
			Command      string   `json:"command"`
			FailureTypes []string `json:"failureTypes"`
			Safety       struct {
				Kind                 string `json:"kind"`
				PlanRequired         bool   `json:"planRequired"`
				ConfirmationRequired bool   `json:"confirmationRequired"`
			} `json:"safety"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	wantFailures := []string{
		"usage.invalid",
		"input.invalid",
		"auth.expired",
		"auth.human_required",
		"remote.unavailable",
		"contract.protocol_changed",
		"internal.failure",
	}
	if result.ExitCode != 0 ||
		envelope.Data.Command != "auth.login" ||
		!reflect.DeepEqual(envelope.Data.FailureTypes, wantFailures) {
		t.Fatalf("schema = %#v, want complete auth login definition", envelope.Data)
	}
	if envelope.Data.Safety.Kind != "local-mutation" ||
		envelope.Data.Safety.PlanRequired ||
		envelope.Data.Safety.ConfirmationRequired {
		t.Fatalf("safety = %#v, want local credential mutation", envelope.Data.Safety)
	}
}

func TestExecuteRejectsUnknownSchemaPathWithoutEchoingInput(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "secret-private-value"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{})

	const want = `{"ok":false,"error":{"type":"usage.invalid","code":"COMMAND_SCHEMA_NOT_FOUND","message":"No completed command has that schema path.","retryable":false,"details":{}},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const want = `{"ok":true,"data":{"authenticated":false,"tokenState":"missing","expiresAt":null},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

func TestExecuteAuthLoginRequiresPrivateTerminal(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "login"},
		Stdin: strings.NewReader("private-stdin-value"),
	}, app.Dependencies{})

	const wantStdout = `{"ok":false,"error":{"type":"auth.human_required","code":"PRIVATE_TERMINAL_REQUIRED","message":"Authentication login requires a private terminal.","retryable":false,"details":{}},"meta":{"command":"auth.login","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	const wantStderr = "partiful: private terminal required\n"
	if result.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if result.Stderr != wantStderr {
		t.Fatalf("stderr = %q, want %q", result.Stderr, wantStderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, "private-stdin-value") {
		t.Fatal("login read ordinary stdin")
	}
}

func TestExecuteAuthLoginNonInteractiveNeverReadsPrivateTerminal(t *testing.T) {
	terminal := &scriptedPrivateTerminal{values: []string{"+15555550123"}}
	httpCalled := false
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login", "--non-interactive"},
	}, app.Dependencies{
		Terminal: terminal,
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			httpCalled = true
			return nil, errors.New("unexpected request")
		}},
	})

	if result.ExitCode != 3 ||
		!strings.Contains(result.Stdout, `"type":"auth.human_required"`) {
		t.Fatalf("result = %#v, want human-required failure", result)
	}
	if len(terminal.prompts) != 0 {
		t.Fatalf("private terminal prompts = %#v, want none", terminal.prompts)
	}
	if httpCalled {
		t.Fatal("non-interactive login reached HTTP")
	}
}

func TestExecuteAuthLoginRejectsEmptyPrivateInputBeforeHTTP(t *testing.T) {
	httpCalled := false
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        &scriptedPrivateTerminal{values: []string{"   "}},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			httpCalled = true
			return nil, errors.New("unexpected request")
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"input.invalid","code":"AUTH_INPUT_INVALID","message":"Authentication input is invalid.","retryable":false,"details":{}},"meta":{"command":"auth.login","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	if result.ExitCode != 2 || result.Stdout != wantStdout {
		t.Fatalf("result = %#v, want invalid private input failure", result)
	}
	if httpCalled {
		t.Fatal("invalid private input reached HTTP")
	}
}

type scriptedPrivateTerminal struct {
	values  []string
	prompts []string
}

func (terminal *scriptedPrivateTerminal) ReadSecret(prompt string) (string, error) {
	terminal.prompts = append(terminal.prompts, prompt)
	if len(terminal.values) == 0 {
		return "", errors.New("no scripted terminal value")
	}
	value := terminal.values[0]
	terminal.values = terminal.values[1:]
	return value, nil
}

type failingPrivateTerminal struct {
	err error
}

func (terminal failingPrivateTerminal) ReadSecret(string) (string, error) {
	return "", terminal.err
}

func TestExecuteAuthLoginPreservesPrivateTerminalValidationFailure(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        failingPrivateTerminal{err: auth.ErrInputInvalid},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})
	if result.ExitCode != 2 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_INPUT_INVALID"`) {
		t.Fatalf("result = %#v, want private terminal validation failure", result)
	}
}

func TestExecuteAuthLoginPersistsReviewedSessionWithoutRevealingPrivateValues(t *testing.T) {
	const (
		phone        = "+15555550123"
		code         = "123456"
		customValue  = "custom-private-token"
		accessValue  = "access-private-token"
		refreshValue = "refresh-private-token"
	)
	terminal := &scriptedPrivateTerminal{values: []string{phone, code}}
	files := &memoryFilesystem{files: map[string][]byte{}}
	call := 0
	clockCalls := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "login"},
		Stdin: strings.NewReader("ordinary-private-input"),
	}, app.Dependencies{
		Files:           files,
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			clockCalls++
			if clockCalls > 1 {
				return time.Date(2026, time.August, 11, 0, 10, 0, 0, time.UTC)
			}
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		Terminal:   terminal,
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if request.Method != http.MethodPost {
				t.Fatalf("request method = %q, want POST", request.Method)
			}
			switch call {
			case 1:
				if request.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("callable Content-Type = %q, want application/json", request.Header.Get("Content-Type"))
				}
				if request.Header.Get("Origin") != "" || request.Header.Get("Referer") != "" {
					t.Fatalf("callable headers = %#v, want no uncontracted Origin or Referer", request.Header)
				}
				if request.URL.String() != "https://api.partiful.com/sendAuthCodeTrusted" {
					t.Fatalf("send URL = %q", request.URL)
				}
				const want = `{"data":{"params":{"displayName":"","phoneNumber":"+15555550123","silent":false,"channelPreference":"sms","captchaToken":null,"useAppleBusinessUpdates":false},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","amplitudeSessionId":1786406400000}}`
				if string(body) != want {
					t.Fatalf("send body = %s, want %s", body, want)
				}
				return jsonResponse(http.StatusOK, `{}`), nil
			case 2:
				if request.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("callable Content-Type = %q, want application/json", request.Header.Get("Content-Type"))
				}
				if request.Header.Get("Origin") != "" || request.Header.Get("Referer") != "" {
					t.Fatalf("callable headers = %#v, want no uncontracted Origin or Referer", request.Header)
				}
				if request.URL.String() != "https://api.partiful.com/getLoginToken" {
					t.Fatalf("login token URL = %q", request.URL)
				}
				const want = `{"data":{"params":{"phoneNumber":"+15555550123","authCode":"123456","affiliateId":null,"utms":{}},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","amplitudeSessionId":1786406400000}}`
				if string(body) != want {
					t.Fatalf("login token body = %s, want %s", body, want)
				}
				return jsonResponse(http.StatusOK, `{"result":{"data":{"token":"`+customValue+`"}}}`), nil
			case 3:
				if request.URL.Scheme != "https" ||
					request.URL.Host != "identitytoolkit.googleapis.com" ||
					request.URL.Path != "/v1/accounts:signInWithCustomToken" ||
					request.URL.Query().Get("key") == "" {
					t.Fatalf("custom-token URL = %q, want reviewed Firebase endpoint", request.URL)
				}
				if request.Header.Get("Referer") != "https://partiful.com/" {
					t.Fatalf("custom-token Referer = %q, want contracted value", request.Header.Get("Referer"))
				}
				if request.Header.Get("Origin") != "" {
					t.Fatalf("custom-token Origin = %q, want absent (not contracted)", request.Header.Get("Origin"))
				}
				const want = `{"token":"custom-private-token","returnSecureToken":true}`
				if string(body) != want {
					t.Fatalf("custom-token body = %s, want %s", body, want)
				}
				return jsonResponse(http.StatusOK, `{"idToken":"`+accessValue+`","refreshToken":"`+refreshValue+`","expiresIn":"3600","kind":"identitytoolkit#VerifyCustomTokenResponse"}`), nil
			default:
				t.Fatalf("unexpected HTTP call %d", call)
				return nil, nil
			}
		}},
	})

	const want = `{"ok":true,"data":{"authenticated":true,"tokenState":"healthy","expiresAt":"2026-08-11T01:10:00Z"},"meta":{"command":"auth.login","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result = %#v, want redacted login success", result)
	}
	if call != 3 {
		t.Fatalf("HTTP calls = %d, want 3", call)
	}
	if clockCalls != 2 {
		t.Fatalf("clock calls = %d, want request and completion timestamps", clockCalls)
	}
	if files.atomicWrites != 1 {
		t.Fatalf("atomic writes = %d, want 1", files.atomicWrites)
	}
	if !reflect.DeepEqual(
		terminal.prompts,
		[]string{"Phone number: ", "Verification code: "},
	) {
		t.Fatalf("private prompts = %#v, want safe prompts", terminal.prompts)
	}
	for _, privateValue := range []string{
		phone,
		code,
		customValue,
		accessValue,
		refreshValue,
		"ordinary-private-input",
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("output contains private value %q", privateValue)
		}
	}

	status := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files:           files,
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 1, 0, 0, time.UTC)
		},
	})
	if !strings.Contains(status.Stdout, `"authenticated":true,"tokenState":"healthy"`) {
		t.Fatalf("status after login = %q, want persisted healthy credentials", status.Stdout)
	}
}

func TestExecuteAuthLoginMapsReviewedWrongCodeWithoutEchoingIt(t *testing.T) {
	const privateCode = "654321"
	terminal := &scriptedPrivateTerminal{values: []string{"+15555550123", privateCode}}
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        terminal,
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call == 1 {
				return jsonResponse(http.StatusOK, `{}`), nil
			}
			return jsonResponse(
				http.StatusForbidden,
				`{"error":{"message":"private remote rejection","status":"PERMISSION_DENIED","details":{"authErrorCode":"PRIVATE_REMOTE_CODE"}}}`,
			), nil
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"input.invalid","code":"AUTH_CODE_REJECTED","message":"The verification code was rejected.","retryable":false,"details":{}},"meta":{"command":"auth.login","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	const wantStderr = "partiful: authentication code rejected\n"
	if result.ExitCode != 2 || result.Stdout != wantStdout || result.Stderr != wantStderr {
		t.Fatalf("result = %#v, want reviewed wrong-code failure", result)
	}
	for _, privateValue := range []string{
		privateCode,
		"private remote rejection",
		"PRIVATE_REMOTE_CODE",
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("failure output contains private value %q", privateValue)
		}
	}
}

func TestExecuteAuthLoginPreservesErrorResponseReadFailure(t *testing.T) {
	const privateReadError = "private response read failure"
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call == 1 {
				return jsonResponse(http.StatusOK, `{}`), nil
			}
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       failingReadCloser{err: errors.New(privateReadError)},
			}, nil
		}},
	})
	if result.ExitCode != 8 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_SERVICE_UNAVAILABLE"`) {
		t.Fatalf("result = %#v, want response read failure to stay unavailable", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateReadError) {
		t.Fatal("response read failure leaked private detail")
	}
}

func TestExecuteAuthLoginRejectsInvalidOptionalCallableErrorField(t *testing.T) {
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call == 1 {
				return jsonResponse(http.StatusOK, `{}`), nil
			}
			return jsonResponse(
				http.StatusForbidden,
				`{"error":{"message":"rejected","status":"PERMISSION_DENIED","details":{"authErrorCode":123}}}`,
			), nil
		}},
	})
	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want invalid optional callable field to fail closed", result)
	}
}

func TestExecuteAuthLoginRejectsOversizedSendAcknowledgement(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        &scriptedPrivateTerminal{values: []string{"+15555550123"}},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body: io.NopCloser(
					io.MultiReader(
						strings.NewReader("{}"),
						strings.NewReader(strings.Repeat(" ", (64<<10)-1)),
					),
				),
			}, nil
		}},
	})
	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want oversized send acknowledgement to fail closed", result)
	}
}

func TestExecuteAuthLoginMapsRejectedCustomTokenToExpired(t *testing.T) {
	terminal := &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}}
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        terminal,
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(http.StatusOK, `{}`), nil
			case 2:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":{"token":"custom-private-token"}}}`,
				), nil
			default:
				return jsonResponse(
					http.StatusBadRequest,
					`{"error":{"code":400,"message":"private invalid token","errors":[{"domain":"private","message":"private","reason":"private"}]}}`,
				), nil
			}
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"auth.expired","code":"INVALID_CUSTOM_TOKEN","message":"Authentication expired during login. Start login again.","retryable":false,"details":{}},"meta":{"command":"auth.login","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	const wantStderr = "partiful: authentication expired\n"
	if result.ExitCode != 3 || result.Stdout != wantStdout || result.Stderr != wantStderr {
		t.Fatalf("result = %#v, want reviewed custom-token failure", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, "private") {
		t.Fatal("custom-token failure exposed a remote response value")
	}
}

func TestExecuteAuthLoginRejectsInvalidOptionalFirebaseSuccessField(t *testing.T) {
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(http.StatusOK, `{}`), nil
			case 2:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":{"token":"custom-private-token"}}}`,
				), nil
			default:
				return jsonResponse(
					http.StatusOK,
					`{"idToken":"private-id-token","refreshToken":"private-refresh","expiresIn":"3600","kind":123}`,
				), nil
			}
		}},
	})
	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want invalid optional Firebase field to fail closed", result)
	}
}

func TestExecuteAuthLoginRejectsTokenInsideRefreshWindow(t *testing.T) {
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(http.StatusOK, `{}`), nil
			case 2:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":{"token":"custom-private-token"}}}`,
				), nil
			default:
				return jsonResponse(
					http.StatusOK,
					`{"idToken":"private-id-token","refreshToken":"private-refresh","expiresIn":"300"}`,
				), nil
			}
		}},
	})
	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want short token lifetime to fail closed", result)
	}
}

func TestExecuteAuthLoginAllowsUndeclaredFirebaseErrorStatus(t *testing.T) {
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(http.StatusOK, `{}`), nil
			case 2:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":{"token":"custom-private-token"}}}`,
				), nil
			default:
				return jsonResponse(
					http.StatusBadRequest,
					`{"error":{"code":400,"message":"invalid token","status":123}}`,
				), nil
			}
		}},
	})
	if result.ExitCode != 3 ||
		!strings.Contains(result.Stdout, `"code":"INVALID_CUSTOM_TOKEN"`) {
		t.Fatalf("result = %#v, want undeclared Firebase error field to remain allowed", result)
	}
}

func TestExecuteAuthLoginRejectsInvalidOptionalFirebaseValidationErrors(t *testing.T) {
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(http.StatusOK, `{}`), nil
			case 2:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":{"token":"custom-private-token"}}}`,
				), nil
			default:
				return jsonResponse(
					http.StatusBadRequest,
					`{"error":{"code":400,"message":"invalid token","errors":"invalid"}}`,
				), nil
			}
		}},
	})
	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want invalid declared Firebase errors field to fail closed", result)
	}
}

func TestExecuteAuthLoginFailsClosedOnMalformedReviewedSuccess(t *testing.T) {
	const privateBodyValue = "private-access-token"
	terminal := &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}}
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        terminal,
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(http.StatusOK, `{}`), nil
			case 2:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":{"token":"custom-private-token"}}}`,
				), nil
			default:
				return jsonResponse(
					http.StatusOK,
					`{"idToken":"`+privateBodyValue+`","expiresIn":"3600"}`,
				), nil
			}
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"contract.protocol_changed","code":"AUTH_PROTOCOL_CHANGED","message":"Authentication no longer matches the reviewed remote contract.","retryable":false,"details":{}},"meta":{"command":"auth.login","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	const wantStderr = "partiful: authentication protocol changed\n"
	if result.ExitCode != 9 || result.Stdout != wantStdout || result.Stderr != wantStderr {
		t.Fatalf("result = %#v, want protocol drift failure", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateBodyValue) {
		t.Fatal("protocol drift failure exposed a response value")
	}
}

func TestExecuteAuthLoginRejectsMissingFirebaseResponse(t *testing.T) {
	terminal := &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}}
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        terminal,
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(http.StatusOK, `{}`), nil
			case 2:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":{"token":"custom-private-token"}}}`,
				), nil
			default:
				return nil, nil
			}
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want missing response to fail closed", result)
	}
}

func TestExecuteAuthLoginRejectsMalformedWrongCodeMapping(t *testing.T) {
	terminal := &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}}
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        terminal,
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call == 1 {
				return jsonResponse(http.StatusOK, `{}`), nil
			}
			return jsonResponse(
				http.StatusForbidden,
				`{"error":{"message":"private malformed error"}}`,
			), nil
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) ||
		!strings.Contains(result.Stdout, `"code":"AUTH_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want malformed error to fail closed", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, "private malformed error") {
		t.Fatal("malformed error failure exposed a response value")
	}
}

func TestExecuteAuthLoginMapsNoResponseWithoutLeakingTransportError(t *testing.T) {
	const privateTransportValue = "private transport detail"
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, app.Dependencies{
		Files:           &memoryFilesystem{files: map[string][]byte{}},
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        &scriptedPrivateTerminal{values: []string{"+15555550123"}},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return nil, errors.New(privateTransportValue)
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"remote.unavailable","code":"AUTH_SERVICE_UNAVAILABLE","message":"The authentication service is unavailable.","retryable":true,"details":{}},"meta":{"command":"auth.login","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	const wantStderr = "partiful: authentication service unavailable\n"
	if result.ExitCode != 8 || result.Stdout != wantStdout || result.Stderr != wantStderr {
		t.Fatalf("result = %#v, want auth unavailable failure", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateTransportValue) {
		t.Fatal("transport failure exposed a private error value")
	}
}

func TestExecuteAuthLoginAtomicPersistenceFailurePreservesExistingCredentials(t *testing.T) {
	const (
		oldCredentials = `{"accessToken":"old-private-token","refreshToken":"old-private-refresh","expiresAt":"2026-08-11T02:00:00Z"}`
		privateError   = "private atomic storage detail"
	)
	files := fakeFilesystem{
		readFile: func(string) ([]byte, error) {
			return []byte(oldCredentials), nil
		},
		writeFileAtomic: func(string, []byte) error {
			return errors.New(privateError)
		},
	}
	call := 0
	dependencies := app.Dependencies{
		Files:           files,
		CredentialsPath: "/config/partiful/credentials.json",
		Terminal:        &scriptedPrivateTerminal{values: []string{"+15555550123", "123456"}},
		AuthRandom:      strings.NewReader("0123456789abcdef"),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(http.StatusOK, `{}`), nil
			case 2:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":{"token":"custom-private-token"}}}`,
				), nil
			default:
				return jsonResponse(
					http.StatusOK,
					`{"idToken":"new-private-token","refreshToken":"new-private-refresh","expiresIn":"3600"}`,
				), nil
			}
		}},
	}
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "login"},
	}, dependencies)

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CREDENTIAL_STORE_UNAVAILABLE","message":"Local credential storage is unavailable.","retryable":false,"details":{}},"meta":{"command":"auth.login","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	if result.ExitCode != 10 || result.Stdout != wantStdout {
		t.Fatalf("result = %#v, want atomic persistence failure", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateError) {
		t.Fatal("persistence failure exposed a private filesystem value")
	}

	status := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "status"},
	}, dependencies)
	if !strings.Contains(status.Stdout, `"authenticated":true,"tokenState":"healthy"`) {
		t.Fatalf("status after failed persistence = %q, want old credentials intact", status.Stdout)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type fakeFilesystem struct {
	readFile        func(string) ([]byte, error)
	remove          func(string) error
	writeFileAtomic func(string, []byte) error
	withLock        func(string, func()) error
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

func (filesystem fakeFilesystem) WriteFileAtomic(path string, document []byte) error {
	if filesystem.writeFileAtomic != nil {
		return filesystem.writeFileAtomic(path, document)
	}
	return nil
}

func (filesystem fakeFilesystem) WithLock(path string, operation func()) error {
	if filesystem.withLock != nil {
		return filesystem.withLock(path, operation)
	}
	operation()
	return nil
}

func TestExecuteContactsListTraversesReviewedPagesAndReturnsOnlyPublicFields(t *testing.T) {
	const (
		credentials   = `{"accessToken":"private-access-token","refreshToken":"private-refresh-token","expiresAt":"2026-08-11T02:00:00Z"}`
		privateID     = "private-contact-identity"
		privateCursor = "private-remote-cursor"
	)
	requestCount := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			requestCount++
			if request.Method != http.MethodPost ||
				request.URL.String() != "https://api.partiful.com/getContacts" {
				t.Fatalf("request = %s %s, want reviewed getContacts call", request.Method, request.URL)
			}
			if got := request.Header.Get("Authorization"); got != "Bearer private-access-token" {
				t.Fatalf("authorization = %q, want bearer credential", got)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if requestCount == 1 {
				const want = `{"data":{"params":{},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","paging":{"maxResults":1000,"cursor":null}}}`
				if string(body) != want {
					t.Fatalf("first request body = %s, want %s", body, want)
				}
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":[{"id":"`+privateID+`","name":"Alice Example","sharedEventCount":2}],"paging":{"nextCursor":"`+privateCursor+`"}}}`,
				), nil
			}
			const want = `{"data":{"params":{},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","paging":{"maxResults":1000,"cursor":"private-remote-cursor"}}}`
			if string(body) != want {
				t.Fatalf("terminal request body = %s, want %s", body, want)
			}
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":[],"paging":{}}}`,
			), nil
		}},
	})

	const want = `{"ok":true,"data":{"items":[{"displayName":"Alice Example","sharedEventCount":2}]},"meta":{"command":"contacts.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[],"page":{"limit":25,"nextCursor":null,"hasMore":false}}}` + "\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result = %#v, want reviewed public contact collection", result)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want data page and terminal sentinel", requestCount)
	}
	for _, privateValue := range []string{
		privateID,
		privateCursor,
		"private-access-token",
		"private-refresh-token",
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("result exposed private value %q", privateValue)
		}
	}
}

func TestExecuteContactsListFiltersLocallyAfterFirstIdentityOccurrenceWins(t *testing.T) {
	const credentials = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
	responses := []string{
		`{"result":{"data":[{"id":"private-duplicate","name":"Alice First","sharedEventCount":1},{"id":"private-other","name":"Bob","sharedEventCount":2}],"paging":{"nextCursor":"cursor-one"}}}`,
		`{"result":{"data":[{"id":"private-duplicate","name":"Alice Later","sharedEventCount":99},{"id":"private-second","name":"Second ALICE","sharedEventCount":3}],"paging":{"nextCursor":"cursor-two"}}}`,
		`{"result":{"data":[],"paging":{}}}`,
	}
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list", "--query", "  ALICE  "},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if bytes.Contains(body, []byte("ALICE")) || bytes.Contains(body, []byte("alice")) {
				t.Fatalf("request sent local query to remote: %s", body)
			}
			response := jsonResponse(http.StatusOK, responses[call])
			call++
			return response, nil
		}},
	})

	const want = `{"ok":true,"data":{"items":[{"displayName":"Alice First","sharedEventCount":1},{"displayName":"Second ALICE","sharedEventCount":3}]},"meta":{"command":"contacts.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[],"page":{"limit":25,"nextCursor":null,"hasMore":false}}}` + "\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result = %#v, want locally filtered first-occurrence contacts", result)
	}
	if call != len(responses) {
		t.Fatalf("request count = %d, want %d", call, len(responses))
	}
	for _, privateValue := range []string{
		"private-duplicate",
		"private-other",
		"private-second",
		"Alice Later",
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("result exposed non-public value %q", privateValue)
		}
	}
}

func TestExecuteContactsListReturnsOpaqueResumableLocalCursor(t *testing.T) {
	const (
		credentials = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
		dataPage    = `{"result":{"data":[{"id":"private-one","name":"One","sharedEventCount":1},{"id":"private-two","name":"Two","sharedEventCount":2},{"id":"private-three","name":"Three","sharedEventCount":3}],"paging":{"nextCursor":"private-remote-cursor"}}}`
		terminal    = `{"result":{"data":[],"paging":{}}}`
	)
	call := 0
	dependencies := withTestCursorCrypto(app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdefFEDCBA9876543210"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call%2 == 1 {
				return jsonResponse(http.StatusOK, dataPage), nil
			}
			return jsonResponse(http.StatusOK, terminal), nil
		}},
	})
	first := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list", "--limit", "2"},
		Stdin: strings.NewReader(""),
	}, dependencies)
	var firstEnvelope struct {
		Data struct {
			Items []contactOutput `json:"items"`
		} `json:"data"`
		Meta struct {
			Page struct {
				NextCursor *string `json:"nextCursor"`
				HasMore    bool    `json:"hasMore"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(first.Stdout), &firstEnvelope); err != nil {
		t.Fatalf("decode first result: %v", err)
	}
	if first.ExitCode != 0 ||
		!reflect.DeepEqual(firstEnvelope.Data.Items, []contactOutput{{"One", 1}, {"Two", 2}}) ||
		!firstEnvelope.Meta.Page.HasMore ||
		firstEnvelope.Meta.Page.NextCursor == nil {
		t.Fatalf("first result = %#v, envelope = %#v, want resumable first page", first, firstEnvelope)
	}
	cursor := *firstEnvelope.Meta.Page.NextCursor
	for _, privateValue := range []string{"private-one", "private-two", "private-three"} {
		if strings.Contains(cursor, privateValue) {
			t.Fatalf("local cursor exposed private identity %q", privateValue)
		}
	}

	second := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list", "--limit", "2", "--cursor", cursor},
		Stdin: strings.NewReader(""),
	}, dependencies)
	var secondEnvelope struct {
		Data struct {
			Items []contactOutput `json:"items"`
		} `json:"data"`
		Meta struct {
			Page struct {
				NextCursor *string `json:"nextCursor"`
				HasMore    bool    `json:"hasMore"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(second.Stdout), &secondEnvelope); err != nil {
		t.Fatalf("decode second result: %v", err)
	}
	if second.ExitCode != 0 ||
		!reflect.DeepEqual(secondEnvelope.Data.Items, []contactOutput{{"Three", 3}}) ||
		secondEnvelope.Meta.Page.HasMore ||
		secondEnvelope.Meta.Page.NextCursor != nil {
		t.Fatalf("second result = %#v, envelope = %#v, want completed second page", second, secondEnvelope)
	}
	if call != 4 {
		t.Fatalf("request count = %d, want two complete traversals", call)
	}
}

type contactOutput struct {
	DisplayName      string `json:"displayName"`
	SharedEventCount int    `json:"sharedEventCount"`
}

func TestExecuteContactsListFailsClosedOnRepeatedRemoteCursor(t *testing.T) {
	const (
		credentials = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
		privateLoop = "private-repeated-cursor"
	)
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call > 2 {
				return nil, errors.New("traversal continued after repeated cursor")
			}
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":[{"id":"private-id","name":"Private Name","sharedEventCount":1}],"paging":{"nextCursor":"`+privateLoop+`"}}}`,
			), nil
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) ||
		!strings.Contains(result.Stdout, `"code":"CONTACTS_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want repeated cursor protocol failure", result)
	}
	if call != 2 {
		t.Fatalf("request count = %d, want stop on repeated cursor", call)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateLoop) ||
		strings.Contains(result.Stdout+result.Stderr, "private-id") {
		t.Fatal("repeated cursor failure exposed private transport data")
	}
}

func TestExecuteContactsListFailsClosedBeyondReviewedTraversalBound(t *testing.T) {
	const credentials = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list", "--all", "--max-items", "1000"},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call > 4 {
				return nil, errors.New("traversal exceeded finite request bound")
			}
			return jsonResponse(
				http.StatusOK,
				fmt.Sprintf(
					`{"result":{"data":[{"id":"private-%d","name":"Private","sharedEventCount":0}],"paging":{"nextCursor":"cursor-%d"}}}`,
					call,
					call,
				),
			), nil
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("result = %#v, want finite traversal failure", result)
	}
	if call != 4 {
		t.Fatalf("request count = %d, want stop at first page beyond reviewed bound", call)
	}
	if strings.Contains(result.Stdout+result.Stderr, "private-") ||
		strings.Contains(result.Stdout+result.Stderr, "cursor-") {
		t.Fatal("traversal bound failure exposed private transport data")
	}
}

func TestExecuteContactsListFailsClosedWhenPrivateIdentityIsMissing(t *testing.T) {
	const (
		credentials = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
		privateName = "Private Name"
	)
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call > 1 {
				return nil, errors.New("malformed contact was accepted")
			}
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":[{"name":"`+privateName+`","sharedEventCount":1}],"paging":{"nextCursor":"cursor"}}}`,
			), nil
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("result = %#v, want required identity protocol failure", result)
	}
	if call != 1 {
		t.Fatalf("request count = %d, want immediate shape failure", call)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateName) {
		t.Fatal("malformed contact failure exposed a private name")
	}
}

func TestExecuteContactsListFailsClosedWhenTerminalPagingIsMissing(t *testing.T) {
	const credentials = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"result":{"data":[]}}`), nil
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("result = %#v, want required paging protocol failure", result)
	}
}

func TestExecuteContactsListAcceptsOpenReviewedResponseFields(t *testing.T) {
	const (
		credentials          = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
		privateContactValue  = "private-unknown-contact-value"
		privateNestedValue   = "private-unknown-nested-value"
		privateEnvelopeValue = "private-unknown-envelope-value"
	)
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call == 1 {
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":[{"id":"private-contact-id","name":"Open Contact","sharedEventCount":4,"unknownContact":"`+privateContactValue+`","unknownObject":{"nested":"`+privateNestedValue+`"}}],"paging":{"nextCursor":"private-remote-cursor","unknownPageInteger":11},"unknownResult":true},"unknownEnvelope":"`+privateEnvelopeValue+`"}`,
				), nil
			}
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":[],"paging":{"unknownTerminalInteger":7}},"unknownEnvelopeInteger":9}`,
			), nil
		}},
	})

	const want = `{"ok":true,"data":{"items":[{"displayName":"Open Contact","sharedEventCount":4}]},"meta":{"command":"contacts.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[],"page":{"limit":25,"nextCursor":null,"hasMore":false}}}` + "\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result = %#v, want reviewed open response traversal", result)
	}
	if call != 2 {
		t.Fatalf("request count = %d, want data and terminal pages", call)
	}
	for _, privateValue := range []string{
		"private-contact-id",
		"private-remote-cursor",
		privateContactValue,
		privateNestedValue,
		privateEnvelopeValue,
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("open response exposed private transport value %q", privateValue)
		}
	}
}

func TestExecuteContactsListRejectsTrailingSuccessJSON(t *testing.T) {
	const (
		credentials  = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
		privateValue = "private-trailing-json-value"
	)
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":[],"paging":{}}}{"trailing":"`+privateValue+`"}`,
			), nil
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("result = %#v, want trailing JSON protocol failure", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateValue) {
		t.Fatal("trailing JSON failure exposed private transport content")
	}
}

func TestExecuteContactsListFailsClosedOnNullRemoteCursor(t *testing.T) {
	const credentials = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":[],"paging":{"nextCursor":null}}}`,
			), nil
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("result = %#v, want invalid cursor shape protocol failure", result)
	}
}

func TestExecuteContactsListAcceptsUnknownUnauthenticatedErrorFields(t *testing.T) {
	const (
		credentials  = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
		privateValue = "private-remote-detail"
	)
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusUnauthorized,
				`{"error":{"message":"Unauthenticated","status":"UNAUTHENTICATED","detail":"`+privateValue+`"}}`,
			), nil
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"auth.expired","code":"REMOTE_SESSION_UNAUTHENTICATED","message":"Stored authentication is no longer accepted. Log in again.","retryable":false,"details":{}},"meta":{"command":"contacts.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	if result.ExitCode != 3 || result.Stdout != wantStdout {
		t.Fatalf("result = %#v, want reviewed unauthenticated mapping", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateValue) {
		t.Fatal("unauthenticated failure exposed remote response content")
	}
}

func TestExecuteContactsListMapsReviewedUnauthenticatedResponseToExpired(t *testing.T) {
	const credentials = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
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
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusUnauthorized,
				`{"error":{"message":"Unauthenticated","status":"UNAUTHENTICATED"}}`,
			), nil
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"auth.expired","code":"REMOTE_SESSION_UNAUTHENTICATED","message":"Stored authentication is no longer accepted. Log in again.","retryable":false,"details":{}},"meta":{"command":"contacts.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	if result.ExitCode != 3 || result.Stdout != wantStdout {
		t.Fatalf("result = %#v, want reviewed unauthenticated mapping", result)
	}
}

func TestExecuteContactsListRefreshesExpiringSessionBeforeProtectedRead(t *testing.T) {
	const (
		credentialsPath = "/config/partiful/credentials.json"
		oldAccess       = "private-old-access"
		newAccess       = "private-new-access"
		newRefresh      = "private-new-refresh"
	)
	document := []byte(
		`{"accessToken":"` + oldAccess +
			`","refreshToken":"private-old-refresh","expiresAt":"2026-08-11T00:04:00Z"}`,
	)
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return append([]byte(nil), document...), nil
			},
			writeFileAtomic: func(_ string, replacement []byte) error {
				document = append([]byte(nil), replacement...)
				return nil
			},
		},
		CredentialsPath: credentialsPath,
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				if request.URL.Host != "securetoken.googleapis.com" {
					t.Fatalf("first request host = %q, want session refresh", request.URL.Host)
				}
				return jsonResponse(
					http.StatusOK,
					`{"access_token":"private-alias","id_token":"`+newAccess+`","refresh_token":"`+newRefresh+`","expires_in":"3600","token_type":"Bearer"}`,
				), nil
			case 2:
				if got := request.Header.Get("Authorization"); got != "Bearer "+newAccess {
					t.Fatalf("contacts authorization = %q, want refreshed session", got)
				}
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":[{"id":"private-contact","name":"Refreshed Contact","sharedEventCount":4}],"paging":{"nextCursor":"remote-cursor"}}}`,
				), nil
			case 3:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":[],"paging":{}}}`,
				), nil
			default:
				return nil, errors.New("unexpected request")
			}
		}},
	})

	if result.ExitCode != 0 ||
		!strings.Contains(result.Stdout, `"displayName":"Refreshed Contact"`) {
		t.Fatalf("result = %#v, want protected read after deterministic refresh", result)
	}
	if !bytes.Contains(document, []byte(newAccess)) ||
		!bytes.Contains(document, []byte(newRefresh)) ||
		bytes.Contains(document, []byte(oldAccess)) {
		t.Fatal("credentials were not atomically replaced with refreshed session")
	}
	for _, privateValue := range []string{oldAccess, newAccess, newRefresh, "private-contact"} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("result exposed private session value %q", privateValue)
		}
	}
}

func TestExecuteContactsListRejectsCursorWhenContactCatalogChanges(t *testing.T) {
	const credentials = `{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`
	call := 0
	dependencies := withTestCursorCrypto(app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdefFEDCBA9876543210"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":[{"id":"private-one","name":"One","sharedEventCount":1},{"id":"private-two","name":"Two","sharedEventCount":2}],"paging":{"nextCursor":"cursor-one"}}}`,
				), nil
			case 2, 4:
				return jsonResponse(http.StatusOK, `{"result":{"data":[],"paging":{}}}`), nil
			case 3:
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":[{"id":"private-replacement","name":"Replacement","sharedEventCount":3}],"paging":{"nextCursor":"cursor-two"}}}`,
				), nil
			default:
				return nil, errors.New("unexpected request")
			}
		}},
	})
	first := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list", "--limit", "1"},
		Stdin: strings.NewReader(""),
	}, dependencies)
	var envelope struct {
		Meta struct {
			Page struct {
				NextCursor string `json:"nextCursor"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(first.Stdout), &envelope); err != nil {
		t.Fatalf("decode first result: %v", err)
	}

	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list", "--cursor", envelope.Meta.Page.NextCursor},
		Stdin: strings.NewReader(""),
	}, dependencies)

	const want = `{"ok":false,"error":{"type":"state.conflict","code":"CURSOR_SNAPSHOT_CHANGED","message":"The contact catalog changed after this cursor was issued.","retryable":false,"details":{}},"meta":{"command":"contacts.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	if result.ExitCode != 6 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result = %#v, want contact snapshot conflict", result)
	}
	for _, privateValue := range []string{"private-one", "private-two", "private-replacement"} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("snapshot failure exposed private value %q", privateValue)
		}
	}
}

func TestExecuteSchemaProjectsCompleteContactsListDefinition(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "contacts.list"},
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
				Properties map[string]struct {
					Minimum   *int   `json:"minimum"`
					Maximum   *int   `json:"maximum"`
					MinLength *int   `json:"minLength"`
					Pattern   string `json:"pattern"`
				} `json:"properties"`
			} `json:"inputSchema"`
			SuccessSchema struct {
				Properties map[string]struct {
					Items struct {
						Required   []string `json:"required"`
						Properties map[string]struct {
							Minimum *int `json:"minimum"`
						} `json:"properties"`
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
		t.Fatalf("decode schema: %v", err)
	}
	if result.ExitCode != 0 || envelope.Data.Command != "contacts.list" {
		t.Fatalf("result = %#v, schema = %#v, want contacts definition", result, envelope.Data)
	}
	var flags []string
	for _, flag := range envelope.Data.Flags {
		flags = append(flags, fmt.Sprintf("%s:%t", flag.Name, flag.Required))
	}
	wantFlags := []string{
		"--query:false",
		"--limit:false",
		"--cursor:false",
		"--all:false",
		"--max-items:false",
	}
	if !reflect.DeepEqual(flags, wantFlags) {
		t.Fatalf("flags = %v, want %v", flags, wantFlags)
	}
	query := envelope.Data.InputSchema.Properties["query"]
	limit := envelope.Data.InputSchema.Properties["limit"]
	maxItems := envelope.Data.InputSchema.Properties["maxItems"]
	if query.MinLength == nil || *query.MinLength != 1 || query.Pattern != `\S` ||
		limit.Minimum == nil || *limit.Minimum != 1 ||
		limit.Maximum == nil || *limit.Maximum != 100 ||
		maxItems.Minimum == nil || *maxItems.Minimum != 1 ||
		maxItems.Maximum == nil || *maxItems.Maximum != 1000 {
		t.Fatalf("input constraints = %#v, want product collection bounds", envelope.Data.InputSchema.Properties)
	}
	wantFields := []string{"displayName", "sharedEventCount"}
	if got := envelope.Data.SuccessSchema.Properties["items"].Items.Required; !reflect.DeepEqual(got, wantFields) {
		t.Fatalf("contact fields = %v, want only %v", got, wantFields)
	}
	sharedEventCount := envelope.Data.SuccessSchema.Properties["items"].Items.Properties["sharedEventCount"]
	if sharedEventCount.Minimum == nil || *sharedEventCount.Minimum != 0 {
		t.Fatalf("sharedEventCount schema = %#v, want nonnegative integer", sharedEventCount)
	}
	wantFailures := []string{
		"usage.invalid",
		"input.invalid",
		"auth.required",
		"auth.expired",
		"state.conflict",
		"remote.unavailable",
		"contract.protocol_changed",
		"internal.failure",
	}
	if !reflect.DeepEqual(envelope.Data.FailureTypes, wantFailures) {
		t.Fatalf("failure types = %v, want %v", envelope.Data.FailureTypes, wantFailures)
	}
	if envelope.Data.Safety.Kind != "read-only" ||
		envelope.Data.Safety.PlanRequired ||
		envelope.Data.Safety.ConfirmationRequired {
		t.Fatalf("safety = %#v, want read-only", envelope.Data.Safety)
	}
	if strings.Contains(result.Stdout, `"id"`) ||
		strings.Contains(result.Stdout, `"phone"`) ||
		strings.Contains(result.Stdout, `"email"`) {
		t.Fatal("public contact schema exposed a private field")
	}
}

func TestExecuteContactsListMapsRejectedRefreshWithoutAttemptingRead(t *testing.T) {
	const privateRefresh = "private-rejected-refresh"
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"private-expired-access","refreshToken":"` +
						privateRefresh +
						`","expiresAt":"2026-08-10T23:59:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("must-not-be-read"),
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			if request.URL.Host != "securetoken.googleapis.com" {
				t.Fatalf("request host = %q, want only refresh attempt", request.URL.Host)
			}
			return jsonResponse(
				http.StatusBadRequest,
				`{"error":{"code":400,"message":"INVALID_REFRESH_TOKEN","status":"INVALID_ARGUMENT"}}`,
			), nil
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"auth.expired","code":"INVALID_REFRESH_TOKEN","message":"Stored authentication has expired. Log in again.","retryable":false,"details":{}},"meta":{"command":"contacts.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	if result.ExitCode != 3 || result.Stdout != wantStdout {
		t.Fatalf("result = %#v, want rejected refresh failure", result)
	}
	if call != 1 {
		t.Fatalf("request count = %d, want refresh only", call)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateRefresh) {
		t.Fatal("rejected refresh failure exposed private token")
	}
}

func TestExecuteContactsListPreservesUnauthenticatedBodyReadFailureAsUnavailable(t *testing.T) {
	const privateReadError = "private unauthenticated response read failure"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       failingReadCloser{err: errors.New(privateReadError)},
			}, nil
		}},
	})

	if result.ExitCode != 8 ||
		!strings.Contains(result.Stdout, `"type":"remote.unavailable"`) {
		t.Fatalf("result = %#v, want response read failure to remain unavailable", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateReadError) {
		t.Fatal("response read failure exposed private transport details")
	}
}

func TestExecuteContactsListReportsUnavailableConfigurationBeforeProtectedRead(t *testing.T) {
	const privateConfigurationError = "private configuration path failure"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		CredentialsPathError: errors.New(privateConfigurationError),
		AuthRandom:           strings.NewReader("must-not-be-read"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("remote must not be called")
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CONFIG_DIRECTORY_UNAVAILABLE","message":"Local configuration directory is unavailable.","retryable":false,"details":{}},"meta":{"command":"contacts.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	if result.ExitCode != 10 || result.Stdout != wantStdout {
		t.Fatalf("result = %#v, want unavailable configuration failure", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateConfigurationError) {
		t.Fatal("configuration failure exposed private local details")
	}
}

func TestExecuteContactsListRequiresAuthenticationWithoutRemoteCall(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
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
		AuthRandom: strings.NewReader("must-not-be-read"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("remote must not be called")
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"auth.required","code":"AUTHENTICATION_REQUIRED","message":"Authentication is required. Log in and try again.","retryable":false,"details":{}},"meta":{"command":"contacts.list","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	if result.ExitCode != 3 || result.Stdout != wantStdout {
		t.Fatalf("result = %#v, want authentication requirement", result)
	}
}

func TestExecuteContactsListPreservesAmbiguousDisplayNamesWithoutIdentity(t *testing.T) {
	const ambiguousName = "Same Display Name"
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list", "--query", "same display"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			call++
			if call == 1 {
				return jsonResponse(
					http.StatusOK,
					`{"result":{"data":[{"id":"private-first","name":"`+ambiguousName+`","sharedEventCount":1},{"id":"private-second","name":"`+ambiguousName+`","sharedEventCount":2}],"paging":{"nextCursor":"private-cursor"}}}`,
				), nil
			}
			return jsonResponse(http.StatusOK, `{"result":{"data":[],"paging":{}}}`), nil
		}},
	})

	var envelope struct {
		Data struct {
			Items []contactOutput `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	want := []contactOutput{
		{DisplayName: ambiguousName, SharedEventCount: 1},
		{DisplayName: ambiguousName, SharedEventCount: 2},
	}
	if result.ExitCode != 0 || !reflect.DeepEqual(envelope.Data.Items, want) {
		t.Fatalf("result = %#v, items = %#v, want both ambiguous public matches", result, envelope.Data.Items)
	}
	for _, privateValue := range []string{"private-first", "private-second", "private-cursor"} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("ambiguous result exposed private identity %q", privateValue)
		}
	}
}

func TestExecuteContactsListFailsClosedOnUnsupportedStatus(t *testing.T) {
	const privateBody = "private unsupported response"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusTooManyRequests,
				`{"error":"`+privateBody+`"}`,
			), nil
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) ||
		strings.Contains(result.Stdout, `"type":"remote.rate_limited"`) {
		t.Fatalf("result = %#v, want unsupported status protocol failure", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateBody) {
		t.Fatal("unsupported status exposed remote response body")
	}
}

func TestExecuteContactsListFailsClosedOnUnexpectedTerminalData(t *testing.T) {
	const (
		privateID   = "private-terminal-identity"
		privateName = "Private Terminal Name"
	)
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"contacts", "list"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"private-access-token","expiresAt":"2026-08-11T02:00:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader("0123456789abcdef"),
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":[{"id":"`+privateID+`","name":"`+privateName+`","sharedEventCount":1}],"paging":{}}}`,
			), nil
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("result = %#v, want unexpected terminal data failure", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateID) ||
		strings.Contains(result.Stdout+result.Stderr, privateName) {
		t.Fatal("terminal data failure exposed private contact content")
	}
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

	const want = `{"ok":true,"data":{"authenticated":true,"tokenState":"healthy","expiresAt":"2026-08-11T02:00:00Z"},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

	const want = `{"ok":true,"data":{"authenticated":true,"tokenState":"expiring","expiresAt":"2026-08-11T00:04:00Z"},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

func TestExecuteAuthStatusDeterministicallyRefreshesExpiringSession(t *testing.T) {
	const (
		oldAccessValue  = "old-access-private-value"
		oldRefreshValue = "refresh/private+value"
		newAccessValue  = "new-id-private-value"
		newRefreshValue = "new-refresh-private-value"
	)
	const credentialsPath = "/config/partiful/credentials.json"
	files := &memoryFilesystem{files: map[string][]byte{
		credentialsPath: []byte(
			`{"accessToken":"` + oldAccessValue +
				`","refreshToken":"` + oldRefreshValue +
				`","expiresAt":"2026-08-11T00:04:00Z"}`,
		),
	}}
	terminal := &scriptedPrivateTerminal{values: []string{"must-not-be-read"}}
	httpCalls := 0
	clockCalls := 0
	dependencies := app.Dependencies{
		Files:           files,
		CredentialsPath: credentialsPath,
		Terminal:        terminal,
		Now: func() time.Time {
			clockCalls++
			if clockCalls > 1 {
				return time.Date(2026, time.August, 11, 0, 10, 0, 0, time.UTC)
			}
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			httpCalls++
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read refresh request: %v", err)
			}
			if request.Method != http.MethodPost ||
				request.URL.Scheme != "https" ||
				request.URL.Host != "securetoken.googleapis.com" ||
				request.URL.Path != "/v1/token" ||
				request.URL.Query().Get("key") == "" {
				t.Fatalf("refresh request URL = %q, want reviewed endpoint", request.URL)
			}
			if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" ||
				request.Header.Get("Referer") != "https://partiful.com/" {
				t.Fatalf("refresh headers = %#v, want contracted Referer", request.Header)
			}
			if request.Header.Get("Origin") != "" {
				t.Fatalf("refresh Origin = %q, want absent (not contracted)", request.Header.Get("Origin"))
			}
			const wantBody = "grant_type=refresh_token&refresh_token=refresh%2Fprivate%2Bvalue"
			if string(body) != wantBody {
				t.Fatalf("refresh body = %q, want %q", body, wantBody)
			}
			return jsonResponse(
				http.StatusOK,
				`{"access_token":"private-access-alias","id_token":"`+newAccessValue+`","refresh_token":"`+newRefreshValue+`","expires_in":"3600","token_type":"Bearer","project_id":"private-project","user_id":"private-user"}`,
			), nil
		}},
	}

	first := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "status"},
	}, dependencies)
	const want = `{"ok":true,"data":{"authenticated":true,"tokenState":"healthy","expiresAt":"2026-08-11T01:10:00Z"},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
	if first.ExitCode != 0 || first.Stdout != want || first.Stderr != "" {
		t.Fatalf("first status = %#v, want refreshed healthy session", first)
	}
	if httpCalls != 1 || files.atomicWrites != 1 {
		t.Fatalf("refresh calls = %d, atomic writes = %d, want one each", httpCalls, files.atomicWrites)
	}
	if clockCalls != 2 {
		t.Fatalf("clock calls = %d, want state and refresh completion timestamps", clockCalls)
	}
	if len(terminal.prompts) != 0 {
		t.Fatalf("session refresh prompted: %#v", terminal.prompts)
	}
	for _, privateValue := range []string{
		oldAccessValue,
		oldRefreshValue,
		newAccessValue,
		newRefreshValue,
		"private-access-alias",
		"private-project",
		"private-user",
	} {
		if strings.Contains(first.Stdout+first.Stderr, privateValue) {
			t.Fatalf("refresh output contains private value %q", privateValue)
		}
	}

	second := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "status"},
	}, dependencies)
	if second.ExitCode != 0 || second.Stdout != want {
		t.Fatalf("second status = %#v, want persisted healthy session", second)
	}
	if httpCalls != 1 || files.atomicWrites != 1 {
		t.Fatalf("second status repeated refresh: calls = %d, writes = %d", httpCalls, files.atomicWrites)
	}
}

func TestExecuteAuthLogoutCannotBeUndoneByConcurrentRefresh(t *testing.T) {
	const credentialsPath = "/config/partiful/credentials.json"
	files := &memoryFilesystem{files: map[string][]byte{
		credentialsPath: []byte(
			`{"accessToken":"old-private-token","refreshToken":"old-private-refresh","expiresAt":"2026-08-11T00:04:00Z"}`,
		),
	}}
	refreshStarted := make(chan struct{})
	allowRefresh := make(chan struct{})
	dependencies := app.Dependencies{
		Files:           files,
		CredentialsPath: credentialsPath,
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			close(refreshStarted)
			<-allowRefresh
			return jsonResponse(
				http.StatusOK,
				`{"access_token":"private-access","id_token":"new-private-token","refresh_token":"new-private-refresh","expires_in":"3600","token_type":"Bearer","user_id":"private-user"}`,
			), nil
		}},
	}

	statusResult := make(chan app.Result, 1)
	go func() {
		statusResult <- app.Execute(context.Background(), app.Request{
			Argv: []string{"auth", "status"},
		}, dependencies)
	}()
	<-refreshStarted

	logoutResult := make(chan app.Result, 1)
	go func() {
		logoutResult <- app.Execute(context.Background(), app.Request{
			Argv: []string{"auth", "logout"},
		}, dependencies)
	}()

	select {
	case result := <-logoutResult:
		t.Fatalf("logout completed before in-flight refresh committed: %#v", result)
	case <-time.After(20 * time.Millisecond):
	}

	close(allowRefresh)
	if result := <-statusResult; result.ExitCode != 0 {
		t.Fatalf("refreshing status = %#v, want success", result)
	}
	if result := <-logoutResult; result.ExitCode != 0 {
		t.Fatalf("serialized logout = %#v, want success", result)
	}
	final := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "status"},
	}, dependencies)
	if final.ExitCode != 0 ||
		!strings.Contains(final.Stdout, `"authenticated":false,"tokenState":"missing"`) {
		t.Fatalf("final status = %#v, want logout to remain authoritative", final)
	}
}

func TestExecuteAuthStatusAtomicallyReplacesProductionCredentials(t *testing.T) {
	credentialsPath := t.TempDir() + "/credentials.json"
	if err := os.WriteFile(
		credentialsPath,
		[]byte(`{"accessToken":"old-private-token","refreshToken":"old-private-refresh","expiresAt":"2026-08-11T00:04:00Z"}`),
		0o600,
	); err != nil {
		t.Fatalf("write initial credentials: %v", err)
	}

	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "status"},
	}, app.Dependencies{
		Files:           auth.OSFileSystem{},
		CredentialsPath: credentialsPath,
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusOK,
				`{"access_token":"private-access","id_token":"new-private-token","refresh_token":"new-private-refresh","expires_in":"3600","token_type":"Bearer","user_id":"private-user"}`,
			), nil
		}},
	})
	if result.ExitCode != 0 {
		t.Fatalf("status = %#v, want successful production credential replacement", result)
	}
	document, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatalf("read replaced credentials: %v", err)
	}
	if strings.Contains(string(document), "old-private") ||
		!strings.Contains(string(document), "new-private-token") {
		t.Fatalf("replaced credentials do not contain one coherent refreshed record")
	}
	info, err := os.Stat(credentialsPath)
	if err != nil {
		t.Fatalf("stat replaced credentials: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential mode = %o, want 600", info.Mode().Perm())
	}
}

func TestExecuteAuthStatusMapsReviewedInvalidRefreshTokenToExpired(t *testing.T) {
	const (
		privateRefreshValue = "expired-private-refresh-value"
		privateRemoteValue  = "private invalid refresh"
	)
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "status"},
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"expired-private-access","refreshToken":"` +
						privateRefreshValue +
						`","expiresAt":"2026-08-10T23:59:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusBadRequest,
				`{"error":{"code":400,"message":"`+privateRemoteValue+`","status":"INVALID_ARGUMENT"}}`,
			), nil
		}},
	})

	const wantStdout = `{"ok":false,"error":{"type":"auth.expired","code":"INVALID_REFRESH_TOKEN","message":"Stored authentication has expired. Log in again.","retryable":false,"details":{}},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
	const wantStderr = "partiful: authentication expired\n"
	if result.ExitCode != 3 || result.Stdout != wantStdout || result.Stderr != wantStderr {
		t.Fatalf("result = %#v, want reviewed invalid-refresh failure", result)
	}

	for _, privateValue := range []string{privateRefreshValue, privateRemoteValue} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("invalid-refresh output contains private value %q", privateValue)
		}
	}
}

func TestExecuteAuthStatusRejectsInvalidOptionalRefreshField(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "status"},
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"private-access","refreshToken":"private-refresh","expiresAt":"2026-08-11T00:04:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusOK,
				`{"access_token":"private-access","id_token":"private-id","refresh_token":"private-refresh","expires_in":"3600","token_type":"Bearer","user_id":123}`,
			), nil
		}},
	})
	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want invalid optional refresh field to fail closed", result)
	}
}

func TestExecuteAuthStatusRejectsRefreshedTokenInsideRefreshWindow(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "status"},
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"private-access","refreshToken":"private-refresh","expiresAt":"2026-08-11T00:04:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusOK,
				`{"access_token":"private-access","id_token":"private-id","refresh_token":"private-refresh","expires_in":"300","token_type":"Bearer"}`,
			), nil
		}},
	})
	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want short refreshed lifetime to fail closed", result)
	}
}

func TestExecuteAuthStatusFailsClosedOnMalformedRefreshSuccess(t *testing.T) {
	const privateValue = "private-refreshed-id-token"
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"auth", "status"},
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(
					`{"accessToken":"expired-private-access","refreshToken":"refresh-private-value","expiresAt":"2026-08-10T23:59:00Z"}`,
				), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusOK,
				`{"access_token":"private-access","id_token":"`+privateValue+`","refresh_token":"private-refresh","expires_in":"3600"}`,
			), nil
		}},
	})

	if result.ExitCode != 9 ||
		!strings.Contains(result.Stdout, `"code":"AUTH_PROTOCOL_CHANGED"`) {
		t.Fatalf("result = %#v, want malformed refresh to fail closed", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateValue) {
		t.Fatal("malformed refresh failure exposed a credential value")
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

	const want = `{"ok":true,"data":{"authenticated":false,"tokenState":"expired","expiresAt":"2026-08-10T23:59:00Z"},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CREDENTIALS_INVALID","message":"Local credentials are invalid.","retryable":false,"details":{}},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const want = `{"ok":true,"data":{"authenticated":false,"tokenState":"missing","expiresAt":null},"meta":{"command":"auth.logout","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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
	files        map[string][]byte
	atomicWrites int
	mutex        sync.Mutex
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

func (filesystem *memoryFilesystem) WriteFileAtomic(path string, document []byte) error {
	filesystem.atomicWrites++
	filesystem.files[path] = append([]byte(nil), document...)
	return nil
}

func (filesystem *memoryFilesystem) WithLock(_ string, operation func()) error {
	filesystem.mutex.Lock()
	defer filesystem.mutex.Unlock()
	operation()
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

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CREDENTIAL_STORE_UNAVAILABLE","message":"Local credential storage is unavailable.","retryable":false,"details":{}},"meta":{"command":"auth.logout","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const want = `{"ok":true,"data":{"healthy":true,"checks":[{"name":"credentials","status":"pass","message":"Authentication credentials are available.","remediation":null}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Authentication credentials are missing.","remediation":"Establish authentication before using commands that require it."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

	const want = `{"ok":true,"data":{"healthy":true,"checks":[{"name":"credentials","status":"warn","message":"Authentication credentials expire soon.","remediation":"Refresh authentication before the credentials expire."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Authentication credentials have expired.","remediation":"Re-establish authentication."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Authentication credentials are invalid.","remediation":"Remove the invalid credentials and re-establish authentication."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Credential storage is unavailable.","remediation":"Check local credential file permissions."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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
							"kind": {"type": "string", "enum": ["read-only", "local-mutation", "standard-mutation", "consequential-action"]},
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

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"FLAG_REPEATED","message":"A scalar flag cannot be repeated.","retryable":false,"details":{"flag":"--non-interactive"}},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const want = `{"ok":false,"error":{"type":"usage.invalid","code":"COMMAND_NOT_FOUND","message":"Unknown command.","retryable":false,"details":{}},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CONFIG_DIRECTORY_UNAVAILABLE","message":"Local configuration directory is unavailable.","retryable":false,"details":{}},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5"}}` + "\n"
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

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Configuration directory is unavailable.","remediation":"Set a usable user configuration directory."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-12.5","remoteContractRevision":"2026-08-12.5","warnings":[]}}` + "\n"
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
