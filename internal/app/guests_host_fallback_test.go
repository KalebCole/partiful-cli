package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func hostFallbackDependencies(t *testing.T, firestoreBody string, calls *int) app.Dependencies {
	t.Helper()
	const credentials = `{"accessToken":"private-access-token","userId":"private-host","expiresAt":"2026-08-12T02:00:00Z"}`
	return withTestCursorCrypto(app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) { return []byte(credentials), nil },
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader(strings.Repeat("0123456789abcdef", 4)),
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			*calls++
			switch *calls {
			case 1:
				return jsonResponse(http.StatusOK, `{"result":{"data":{"event":{"id":"event-example","ownerIds":["private-host","private-cohost"],"rsvpsEnabled":true,"atCapacity":false,"plusOneNamesRequired":false,"questionnaireVersions":null,"hasGuests":true}}}}`), nil
			case 2:
				if request.URL.String() != "https://api.partiful.com/getGuests" {
					t.Fatalf("request 2 = %s, want getGuests", request.URL)
				}
				// Sanitized rejection observed live on 2026-09-29 when the caller hosts the event.
				return jsonResponse(http.StatusBadRequest, `{"error":{"message":"User is host","status":"FAILED_PRECONDITION"}}`), nil
			case 3:
				const wantPrefix = "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/events/event-example/guests?"
				if request.Method != http.MethodGet || !strings.HasPrefix(request.URL.String(), wantPrefix) {
					t.Fatalf("request 3 = %s %s, want guests subcollection list", request.Method, request.URL)
				}
				if got := request.Header.Get("Authorization"); got != "Bearer private-access-token" {
					t.Fatalf("authorization = %q, want bearer credential", got)
				}
				return jsonResponse(http.StatusOK, firestoreBody), nil
			default:
				return nil, errors.New("unexpected request")
			}
		}},
	})
}

func TestExecuteGuestsListFallsBackToGuestsSubcollectionForHost(t *testing.T) {
	const documents = `{"documents":[` +
		`{"name":"projects/getpartiful/databases/(default)/documents/events/event-example/guests/private-guest-1","fields":{"name":{"stringValue":"Alice Example"},"status":{"stringValue":"GOING"},"count":{"integerValue":"2"},"userId":{"stringValue":"private-attendee"},"anchorGuestId":{"nullValue":null}}},` +
		`{"name":"projects/getpartiful/databases/(default)/documents/events/event-example/guests/private-guest-2","fields":{"name":{"stringValue":"Cohost Example"},"status":{"stringValue":"SENT"},"count":{"integerValue":"1"},"userId":{"stringValue":"private-cohost"}}}` +
		`]}`
	calls := 0
	result := executeChecked(t, context.Background(), app.Request{
		Argv: []string{"guests", "list", "event-example"},
	}, hostFallbackDependencies(t, documents, &calls))
	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result = %#v, want host guest list", result)
	}
	var envelope struct {
		Data struct {
			Items []guestOutput `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	want := []guestOutput{
		{DisplayName: "Alice Example", RSVPStatus: "going", PartySize: 2, Cohost: false},
		{DisplayName: "Cohost Example", RSVPStatus: "sent", PartySize: 1, Cohost: true},
	}
	if !reflect.DeepEqual(envelope.Data.Items, want) {
		t.Fatalf("items = %#v, want %#v", envelope.Data.Items, want)
	}
	for _, privateValue := range []string{"private-attendee", "private-cohost", "private-guest-1", "private-guest-2"} {
		if strings.Contains(result.Stdout, privateValue) {
			t.Fatalf("output exposed private value %q", privateValue)
		}
	}
	if calls != 3 {
		t.Fatalf("request count = %d, want event, rejected getGuests, subcollection", calls)
	}
}

func TestExecuteGuestsListHostFallbackReturnsEmptyCollection(t *testing.T) {
	calls := 0
	result := executeChecked(t, context.Background(), app.Request{
		Argv: []string{"guests", "list", "event-example"},
	}, hostFallbackDependencies(t, `{}`, &calls))
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, `"data":{"items":[]}`) || result.Stderr != "" {
		t.Fatalf("result = %#v, want empty host guest list", result)
	}
}

func TestExecuteGuestsListHostFallbackFailsClosedOnUnknownGuestDocument(t *testing.T) {
	const documents = `{"documents":[{"name":"projects/getpartiful/databases/(default)/documents/events/event-example/guests/private-guest-1","fields":{"displayName":{"stringValue":"Alice Example"},"status":{"stringValue":"GOING"}}}]}`
	calls := 0
	result := executeChecked(t, context.Background(), app.Request{
		Argv: []string{"guests", "list", "event-example"},
	}, hostFallbackDependencies(t, documents, &calls))
	if result.ExitCode == 0 || !strings.Contains(result.Stdout, "GUESTS_PROTOCOL_CHANGED") {
		t.Fatalf("result = %#v, want fail-closed protocol change", result)
	}
}
