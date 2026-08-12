package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func TestExecuteGuestsListPaginatesHostGuestsAndKeepsOutputPrivacySafe(t *testing.T) {
	const (
		currentUserID  = "private-host"
		cohostUserID   = "private-cohost"
		guestCursor    = "private-guest-cursor"
		credentials    = `{"accessToken":"private-access-token","userId":"` + currentUserID + `","expiresAt":"2026-08-12T02:00:00Z"}`
		deviceIDBase64 = "MDEyMzQ1Njc4OWFiY2RlZg"
	)
	call := 0
	dependencies := withTestCursorCrypto(app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) { return []byte(credentials), nil },
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader(strings.Repeat("0123456789abcdef", 4)),
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1, 4:
				if request.URL.String() != "https://api.partiful.com/getEventInfo" {
					t.Fatalf("request %d = %s, want getEventInfo", call, request.URL)
				}
				body, _ := io.ReadAll(request.Body)
				const want = `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"` + deviceIDBase64 + `"}}`
				if string(body) != want {
					t.Fatalf("event request body = %s, want %s", body, want)
				}
				return jsonResponse(http.StatusOK, `{"result":{"data":{"event":{"id":"event-example","ownerIds":["`+currentUserID+`","`+cohostUserID+`"],"rsvpsEnabled":true,"atCapacity":false,"plusOneNamesRequired":false,"questionnaireVersions":null,"hasGuests":true}}}}`), nil
			case 2, 5:
				if request.URL.String() != "https://api.partiful.com/getGuests" {
					t.Fatalf("request %d = %s, want getGuests", call, request.URL)
				}
				body, _ := io.ReadAll(request.Body)
				const want = `{"data":{"params":{"eventId":"event-example","includeInvitedGuests":true},"amplitudeDeviceId":"` + deviceIDBase64 + `","paging":{"cursor":null,"maxResults":500}}}`
				if string(body) != want {
					t.Fatalf("first guest page body = %s, want %s", body, want)
				}
				return jsonResponse(http.StatusOK, `{"result":{"data":[{"id":"private-guest-1","userId":"private-attendee","name":"Alice Example","status":"GOING","count":2,"anchorGuestId":null},{"id":"private-plus-one","name":"Plus One","status":"GOING","count":1,"anchorGuestId":"private-guest-1"}],"paging":{"nextCursor":"`+guestCursor+`"}}}`), nil
			case 3, 6:
				body, _ := io.ReadAll(request.Body)
				const want = `{"data":{"params":{"eventId":"event-example","includeInvitedGuests":true},"amplitudeDeviceId":"` + deviceIDBase64 + `","paging":{"cursor":"` + guestCursor + `","maxResults":500}}}`
				if string(body) != want {
					t.Fatalf("second guest page body = %s, want %s", body, want)
				}
				return jsonResponse(http.StatusOK, `{"result":{"data":[{"id":"private-guest-2","userId":"`+cohostUserID+`","name":"Cohost Example","status":"SENT","count":1,"anchorGuestId":null}],"paging":{}}}`), nil
			default:
				return nil, errors.New("unexpected request")
			}
		}},
	})

	first := app.Execute(context.Background(), app.Request{
		Argv: []string{"guests", "list", "event-example", "--limit", "1"},
	}, dependencies)
	if first.ExitCode != 0 || first.Stderr != "" {
		t.Fatalf("first = %#v, want first guest page", first)
	}
	var firstEnvelope struct {
		Data struct {
			Items []guestOutput `json:"items"`
		} `json:"data"`
		Meta struct {
			Page struct {
				Limit      int     `json:"limit"`
				NextCursor *string `json:"nextCursor"`
				HasMore    bool    `json:"hasMore"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(first.Stdout), &firstEnvelope); err != nil {
		t.Fatalf("decode first result: %v", err)
	}
	if !reflect.DeepEqual(firstEnvelope.Data.Items, []guestOutput{{
		DisplayName: "Alice Example",
		RSVPStatus:  "going",
		PartySize:   2,
		Cohost:      false,
	}}) {
		t.Fatalf("first items = %#v", firstEnvelope.Data.Items)
	}
	if firstEnvelope.Meta.Page.Limit != 1 ||
		firstEnvelope.Meta.Page.NextCursor == nil ||
		*firstEnvelope.Meta.Page.NextCursor == "" ||
		!firstEnvelope.Meta.Page.HasMore {
		t.Fatalf("first page = %#v, want resumable first page", firstEnvelope.Meta.Page)
	}
	for _, privateValue := range []string{"private-attendee", cohostUserID, guestCursor, "private-guest-1", "private-plus-one"} {
		if strings.Contains(first.Stdout+first.Stderr, privateValue) {
			t.Fatalf("first page exposed private value %q", privateValue)
		}
	}

	second := app.Execute(context.Background(), app.Request{
		Argv: []string{"guests", "list", "event-example", "--cursor", *firstEnvelope.Meta.Page.NextCursor},
	}, dependencies)
	if second.ExitCode != 0 || second.Stderr != "" {
		t.Fatalf("second = %#v, want resumed guest page", second)
	}
	var secondEnvelope struct {
		Data struct {
			Items []guestOutput `json:"items"`
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
	if !reflect.DeepEqual(secondEnvelope.Data.Items, []guestOutput{{
		DisplayName: "Cohost Example",
		RSVPStatus:  "sent",
		PartySize:   1,
		Cohost:      true,
	}}) || secondEnvelope.Meta.Page.NextCursor != nil || secondEnvelope.Meta.Page.HasMore {
		t.Fatalf("second result = %#v envelope = %#v", second, secondEnvelope)
	}
	if call != 6 {
		t.Fatalf("request count = %d, want two complete guest traversals", call)
	}
	for _, privateValue := range []string{cohostUserID, guestCursor, "private-guest-2"} {
		if strings.Contains(second.Stdout+second.Stderr, privateValue) {
			t.Fatalf("second page exposed private value %q", privateValue)
		}
	}
}

func TestExecuteGuestsListReturnsPermissionDeniedForAttendee(t *testing.T) {
	const (
		currentUserID = "private-attendee"
		credentials   = `{"accessToken":"private-access-token","userId":"` + currentUserID + `","expiresAt":"2026-08-12T02:00:00Z"}`
	)
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"guests", "list", "event-example"},
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) { return []byte(credentials), nil },
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader(strings.Repeat("0123456789abcdef", 4)),
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			if request.URL.String() != "https://api.partiful.com/getEventInfo" {
				t.Fatalf("request = %s, want getEventInfo", request.URL)
			}
			return jsonResponse(http.StatusOK, `{"result":{"data":{"event":{"id":"event-example","ownerIds":["private-host"],"rsvpsEnabled":true,"atCapacity":false,"plusOneNamesRequired":false,"questionnaireVersions":null,"hasGuests":true}}}}`), nil
		}},
	})

	const want = `{"ok":false,"error":{"type":"permission.denied","code":"HOST_PERMISSION_REQUIRED","message":"This command requires host access to the event.","retryable":false,"details":{"requiredRole":"host"}},"meta":{"command":"guests.list","cliVersion":"3.0.0","productContractRevision":"2026-08-12.7","remoteContractRevision":"2026-08-12.7"}}` + "\n"
	if result.ExitCode != 4 || result.Stdout != want || result.Stderr != "partiful: host access required\n" {
		t.Fatalf("result = %#v, want host permission denial", result)
	}
	if call != 1 {
		t.Fatalf("request count = %d, want no guest traversal", call)
	}
	for _, privateValue := range []string{"private-host", currentUserID} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("permission denial exposed private value %q", privateValue)
		}
	}
}

func TestExecuteGuestsListRejectsMissingEventID(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"guests", "list"},
	}, app.Dependencies{})
	if result.ExitCode != 2 ||
		!strings.Contains(result.Stdout, `"type":"input.invalid"`) ||
		!strings.Contains(result.Stdout, `"code":"EVENT_ID_REQUIRED"`) {
		t.Fatalf("result = %#v, want missing event ID input failure", result)
	}
}

func TestExecuteGuestsListReturnsEmptyCollectionForHost(t *testing.T) {
	const credentials = `{"accessToken":"private-access-token","userId":"private-host","expiresAt":"2026-08-12T02:00:00Z"}`
	call := 0
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"guests", "list", "event-example"},
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) { return []byte(credentials), nil },
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader(strings.Repeat("0123456789abcdef", 4)),
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(http.StatusOK, `{"result":{"data":{"event":{"id":"event-example","ownerIds":["private-host"],"rsvpsEnabled":true,"atCapacity":false,"plusOneNamesRequired":false,"questionnaireVersions":null,"hasGuests":false}}}}`), nil
			case 2:
				return jsonResponse(http.StatusOK, `{"result":{"data":[],"paging":{}}}`), nil
			default:
				return nil, errors.New("unexpected request")
			}
		}},
	})

	const want = `{"ok":true,"data":{"items":[]},"meta":{"command":"guests.list","cliVersion":"3.0.0","productContractRevision":"2026-08-12.7","remoteContractRevision":"2026-08-12.7","warnings":[],"page":{"limit":25,"nextCursor":null,"hasMore":false}}}` + "\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result = %#v, want empty host guest list", result)
	}
}

func TestExecuteGuestsInviteBindsResolvedContactAndAppliesWithoutReResolvingName(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(`{"accessToken":"private-access-token","userId":"private-host","expiresAt":"2026-08-12T02:00:00Z"}`),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("v", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 4:
			assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
			return jsonResponse(http.StatusOK, `{"result":{"data":{"event":{"id":"event-example","ownerIds":["private-host"],"rsvpsEnabled":true,"atCapacity":false,"plusOneNamesRequired":false,"questionnaireVersions":null,"hasGuests":true}}}}`), nil
		case 2:
			assertEventCallableRequest(t, request, "getContacts", `{"data":{"params":{},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","paging":{"maxResults":1000,"cursor":null}}}`)
			return jsonResponse(http.StatusOK, `{"result":{"data":[{"id":"private-contact-1","name":"Alex Example","sharedEventCount":2}],"paging":{"nextCursor":"private-contacts-cursor"}}}`), nil
		case 3:
			return jsonResponse(http.StatusOK, `{"result":{"data":[],"paging":{}}}`), nil
		case 5:
			return jsonResponse(http.StatusOK, `{"result":{"data":[{"id":"private-contact-1","name":"Alex Example","sharedEventCount":2},{"id":"private-contact-2","name":"Alex Example","sharedEventCount":5}],"paging":{"nextCursor":"private-contacts-cursor"}}}`), nil
		case 6:
			return jsonResponse(http.StatusOK, `{"result":{"data":[],"paging":{}}}`), nil
		case 7:
			assertEventCallableRequest(t, request, "addInvitedGuestsAsHost", `{"data":{"params":{"eventId":"event-example","userIdsToInvite":["private-contact-1"],"invitationMessage":"","otherMutualsCount":0,"phoneContactsToInvite":[],"emailsToInvite":[]},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","userId":"private-host"}}`)
			return jsonResponse(http.StatusOK, `{"result":{"ok":true}}`), nil
		default:
			return nil, errors.New("unexpected request")
		}
	}}

	argv := []string{"guests", "invite", "event-example", "--contact", "Alex Example"}
	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 || strings.Contains(plan.Stdout, "private-contact-1") {
		t.Fatalf("plan = %#v, want redacted guest invite plan", plan)
	}
	if !strings.Contains(plan.Stdout, `"operation":"addInvitedGuestsAsHost"`) ||
		!strings.Contains(plan.Stdout, `\u003credacted\u003e`) ||
		!strings.Contains(plan.Stdout, `"contact":"Alex Example"`) {
		t.Fatalf("plan = %s, want reviewed guest invite plan", plan.Stdout)
	}
	token := rsvpPlanToken(t, plan)

	applied := app.Execute(context.Background(), app.Request{
		Argv: append(append([]string{}, argv...), "--apply", "--confirm", token),
	}, dependencies)
	const want = `{"ok":true,"data":{"eventId":"event-example","submitted":true},"meta":{"command":"guests.invite","cliVersion":"3.0.0","productContractRevision":"2026-08-12.7","remoteContractRevision":"2026-08-12.7","warnings":[]}}` + "\n"
	if applied.ExitCode != 0 || applied.Stdout != want || applied.Stderr != "" {
		t.Fatalf("applied = %#v, want submitted-only guest invite result", applied)
	}
	if call != 7 {
		t.Fatalf("request count = %d, want plan reads, apply reads, and one mutation", call)
	}
	for _, privateValue := range []string{"private-contact-1", "private-contact-2", "private-contacts-cursor"} {
		if strings.Contains(plan.Stdout+plan.Stderr+applied.Stdout+applied.Stderr, privateValue) {
			t.Fatalf("guest invite flow exposed private value %q", privateValue)
		}
	}
}

func TestExecuteGuestsInviteReturnsPublicAmbiguityAndStalesChangedContact(t *testing.T) {
	t.Run("ambiguous planning", func(t *testing.T) {
		dependencies := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{
			eventWriteCredentialsPath: []byte(`{"accessToken":"private-access-token","userId":"private-host","expiresAt":"2026-08-12T02:00:00Z"}`),
		}})
		call := 0
		dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return jsonResponse(http.StatusOK, `{"result":{"data":{"event":{"id":"event-example","ownerIds":["private-host"],"rsvpsEnabled":true,"atCapacity":false,"plusOneNamesRequired":false,"questionnaireVersions":null,"hasGuests":true}}}}`), nil
			case 2:
				return jsonResponse(http.StatusOK, `{"result":{"data":[{"id":"private-contact-1","name":"Alex Example","sharedEventCount":2},{"id":"private-contact-2","name":"Alex Example","sharedEventCount":5}],"paging":{"nextCursor":"private-cursor"}}}`), nil
			case 3:
				return jsonResponse(http.StatusOK, `{"result":{"data":[],"paging":{}}}`), nil
			default:
				return nil, errors.New("unexpected request")
			}
		}}

		result := app.Execute(context.Background(), app.Request{
			Argv: []string{"guests", "invite", "event-example", "--contact", "Alex Example"},
		}, dependencies)
		if result.ExitCode != 2 ||
			!strings.Contains(result.Stdout, `"type":"match.ambiguous"`) ||
			!strings.Contains(result.Stdout, `"displayName":"Alex Example"`) ||
			!strings.Contains(result.Stdout, `"sharedEventCount":2`) ||
			!strings.Contains(result.Stdout, `"sharedEventCount":5`) {
			t.Fatalf("result = %#v, want public ambiguity", result)
		}
		for _, privateValue := range []string{"private-contact-1", "private-contact-2", "private-cursor"} {
			if strings.Contains(result.Stdout+result.Stderr, privateValue) {
				t.Fatalf("ambiguity exposed private value %q", privateValue)
			}
		}
	})

	t.Run("stale contact", func(t *testing.T) {
		files := &memoryFilesystem{files: map[string][]byte{
			eventWriteCredentialsPath: []byte(`{"accessToken":"private-access-token","userId":"private-host","expiresAt":"2026-08-12T02:00:00Z"}`),
		}}
		dependencies := eventWriteDependencies(files)
		dependencies.MutationRandom = strings.NewReader(strings.Repeat("w", 32))
		call := 0
		dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1, 4:
				return jsonResponse(http.StatusOK, `{"result":{"data":{"event":{"id":"event-example","ownerIds":["private-host"],"rsvpsEnabled":true,"atCapacity":false,"plusOneNamesRequired":false,"questionnaireVersions":null,"hasGuests":true}}}}`), nil
			case 2:
				return jsonResponse(http.StatusOK, `{"result":{"data":[{"id":"private-contact-1","name":"Alex Example","sharedEventCount":2}],"paging":{"nextCursor":"private-cursor"}}}`), nil
			case 3:
				return jsonResponse(http.StatusOK, `{"result":{"data":[],"paging":{}}}`), nil
			case 5:
				return jsonResponse(http.StatusOK, `{"result":{"data":[{"id":"private-contact-1","name":"Alex Example","sharedEventCount":3}],"paging":{"nextCursor":"private-cursor"}}}`), nil
			case 6:
				return jsonResponse(http.StatusOK, `{"result":{"data":[],"paging":{}}}`), nil
			default:
				return nil, errors.New("unexpected request")
			}
		}}

		argv := []string{"guests", "invite", "event-example", "--contact", "Alex Example"}
		plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
		if plan.ExitCode != 0 {
			t.Fatalf("plan = %#v", plan)
		}
		token := rsvpPlanToken(t, plan)
		applied := app.Execute(context.Background(), app.Request{
			Argv: append(append([]string{}, argv...), "--apply", "--confirm", token),
		}, dependencies)
		if applied.ExitCode != 7 || !strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) {
			t.Fatalf("applied = %#v, want stale changed-contact plan", applied)
		}
		if call != 6 {
			t.Fatalf("request count = %d, want no mutation on stale contact", call)
		}
	})
}

type guestOutput struct {
	DisplayName string `json:"displayName"`
	RSVPStatus  string `json:"rsvpStatus"`
	PartySize   int    `json:"partySize"`
	Cohost      bool   `json:"cohost"`
}

func TestExecuteSchemaProjectsGuestCommands(t *testing.T) {
	list := app.Execute(context.Background(), app.Request{
		Argv: []string{"schema", "guests.list"},
	}, app.Dependencies{})
	if list.ExitCode != 0 ||
		!strings.Contains(list.Stdout, `"command":"guests.list"`) ||
		!strings.Contains(list.Stdout, `"rsvpStatus"`) ||
		!strings.Contains(list.Stdout, `"cohost"`) ||
		!strings.Contains(list.Stdout, `"kind":"read-only"`) {
		t.Fatalf("list schema = %#v, want guests.list projection", list)
	}

	invite := app.Execute(context.Background(), app.Request{
		Argv: []string{"schema", "guests.invite"},
	}, app.Dependencies{})
	if invite.ExitCode != 0 ||
		!strings.Contains(invite.Stdout, `"command":"guests.invite"`) ||
		!strings.Contains(invite.Stdout, `"--contact"`) ||
		!strings.Contains(invite.Stdout, `"kind":"consequential-action"`) ||
		!strings.Contains(invite.Stdout, `"confirmationRequired":true`) {
		t.Fatalf("invite schema = %#v, want guests.invite projection", invite)
	}
}
