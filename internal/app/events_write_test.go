package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
)

const (
	eventWriteCredentialsPath = "/config/partiful/credentials.json"
	eventWriteMutationPath    = "/config/partiful/mutation-plans.json"
	eventWriteCredentials     = `{"accessToken":"private-access-token","userId":"private-account","expiresAt":"2026-08-12T02:00:00Z"}`
)

func eventWriteDependencies(files *memoryFilesystem) app.Dependencies {
	return app.Dependencies{
		Files:           files,
		CredentialsPath: eventWriteCredentialsPath,
		MutationPath:    eventWriteMutationPath,
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader(strings.Repeat("0123456789abcdef", 8)),
	}
}

func eventResponse(t *testing.T, event map[string]any) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"result": map[string]any{
			"data": map[string]any{
				"event": event,
			},
		},
	})
	if err != nil {
		t.Fatalf("encode event response: %v", err)
	}
	return string(body)
}

func compatibleUpdateEvent() map[string]any {
	return map[string]any{
		"id":                    "event-example",
		"title":                 "Current title",
		"description":           "Current description",
		"startDate":             "2026-09-12T19:00:00Z",
		"endDate":               "2026-09-12T22:00:00Z",
		"timezone":              "America/Los_Angeles",
		"status":                "PUBLISHED",
		"ownerIds":              []string{"private-account"},
		"rsvpsEnabled":          true,
		"atCapacity":            false,
		"plusOneNamesRequired":  false,
		"questionnaireVersions": nil,
		"hasGuests":             false,
		"ticketing":             nil,
	}
}

func compatibleCancelEvent() map[string]any {
	event := compatibleUpdateEvent()
	event["guestCount"] = 12
	event["startDate"] = "2026-09-12T19:00:00Z"
	return event
}

func assertEventCallableRequest(t *testing.T, request *http.Request, operation, body string) {
	t.Helper()
	if request.Method != http.MethodPost || request.URL.String() != "https://api.partiful.com/"+operation {
		t.Fatalf("request = %s %s, want %s", request.Method, request.URL, operation)
	}
	document, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if string(document) != body {
		t.Fatalf("request body = %s, want %s", document, body)
	}
}

func TestExecuteEventCreatePlansExactNormalizedRequestAndDefaultPoster(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("c", 32))
	requestCount := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		requestCount++
		if request.Method != http.MethodGet || request.URL.String() != "https://assets.getpartiful.com/posters.json" {
			t.Fatalf("request = %s %s, want poster catalog", request.Method, request.URL)
		}
		return jsonResponse(http.StatusOK, `[{"id":"Let's Party","url":"https://assets.getpartiful.com/posters/Let's%20Party","name":"Let's Party","tags":["party"],"categories":["party"],"contentType":"image/jpeg","height":2000,"width":2000}]`), nil
	}}

	result := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"events", "create",
			"--title", "  Example event  ",
			"--start", "2026-09-12T19:00:00-07:00",
			"--end", "2026-09-12T22:00:00-07:00",
			"--timezone", "America/Los_Angeles",
			"--description", "  Bring snacks  ",
			"--location", "  Sunset roof  ",
			"--guest-limit", "75",
			"--link", "Tickets=https://example.test/tickets",
		},
	}, dependencies)

	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result = %#v, want create plan", result)
	}
	var envelope struct {
		Data struct {
			Operation string `json:"operation"`
			Input     struct {
				Title       string `json:"title"`
				Start       string `json:"start"`
				End         string `json:"end"`
				Timezone    string `json:"timezone"`
				Description string `json:"description"`
				Location    string `json:"location"`
				Visibility  string `json:"visibility"`
				GuestLimit  int    `json:"guestLimit"`
				PosterID    string `json:"posterId"`
				Links       []struct {
					Label string `json:"label"`
					URL   string `json:"url"`
				} `json:"links"`
			} `json:"input"`
			Request struct {
				Event     map[string]any `json:"event"`
				CohostIDs []string       `json:"cohostIds"`
			} `json:"request"`
			Preconditions struct {
				Poster string `json:"poster"`
			} `json:"preconditions"`
			ExpiresInSeconds int    `json:"expiresInSeconds"`
			PlanToken        string `json:"planToken"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(result.Stdout), &envelope) != nil {
		t.Fatalf("decode result: %s", result.Stdout)
	}
	if envelope.Data.Operation != "createEvent" ||
		envelope.Data.Input.Title != "Example event" ||
		envelope.Data.Input.Start != "2026-09-12T19:00:00-07:00" ||
		envelope.Data.Input.End != "2026-09-12T22:00:00-07:00" ||
		envelope.Data.Input.Timezone != "America/Los_Angeles" ||
		envelope.Data.Input.Description != "Bring snacks" ||
		envelope.Data.Input.Location != "Sunset roof" ||
		envelope.Data.Input.Visibility != "private" ||
		envelope.Data.Input.GuestLimit != 75 ||
		envelope.Data.Input.PosterID != "Let's Party" ||
		!reflect.DeepEqual(envelope.Data.Input.Links, []struct {
			Label string `json:"label"`
			URL   string `json:"url"`
		}{{Label: "Tickets", URL: "https://example.test/tickets"}}) ||
		envelope.Data.Preconditions.Poster != "bound" ||
		envelope.Data.ExpiresInSeconds != 300 ||
		envelope.Data.PlanToken == "" {
		t.Fatalf("plan = %#v, want normalized create plan", envelope.Data)
	}
	if !reflect.DeepEqual(envelope.Data.Request.CohostIDs, []string{}) {
		t.Fatalf("cohost ids = %#v, want empty list", envelope.Data.Request.CohostIDs)
	}
	if event := envelope.Data.Request.Event; event["status"] != "UNSAVED" || event["visibility"] != "public" || event["rsvpButtonGlyphType"] != "emojis" {
		t.Fatalf("request event = %#v, want reviewed defaults", event)
	}
	for _, privateValue := range []string{"private-access-token", "private-account"} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("output exposed private value %q", privateValue)
		}
	}
	if requestCount != 1 || files.atomicWrites != 1 {
		t.Fatalf("request count = %d atomic writes = %d, want one catalog read and one plan write", requestCount, files.atomicWrites)
	}
}

func TestExecuteEventCreateConsumesPlanBeforeOneAttemptAndReturnsSubmittedOnly(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("a", 32))
	call := 0
	catalogBody := `[{
		"id":"birthdaycake.png",
		"url":"https://assets.getpartiful.com/posters/birthdaycake.png",
		"name":"Birthday Cake",
		"tags":["birthday"],
		"categories":["birthday"],
		"blurHash":"LKO2?U%2Tw=w]~RBVZRi};RPxuwH",
		"contentType":"image/png",
		"height":1200,
		"width":800
	}]`
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 2:
			if request.Method != http.MethodGet || request.URL.String() != "https://assets.getpartiful.com/posters.json" {
				t.Fatalf("request = %s %s, want poster catalog", request.Method, request.URL)
			}
			return jsonResponse(http.StatusOK, catalogBody), nil
		case 3:
			assertEventCallableRequest(t, request, "createEvent", `{"data":{"params":{"event":{"title":"Example event","startDate":"2026-09-13T02:00:00Z","timezone":"America/Los_Angeles","guestStatusCounts":{"APPROVED":0,"DECLINED":0,"DELIVERY_ERROR":0,"GOING":0,"INTERESTED":0,"MAYBE":0,"PENDING_APPROVAL":0,"READY_TO_SEND":0,"REJECTED":0,"RESPONDED_TO_FIND_A_TIME":0,"SENDING":0,"SEND_ERROR":0,"SENT":0,"WAITLIST":0,"WAITLISTED_FOR_APPROVAL":0,"WITHDRAWN":0},"displaySettings":{"theme":"cloudflow","effect":"fireflies","titleFont":"display"},"status":"UNSAVED","rsvpButtonGlyphType":"emojis","image":{"source":"partiful_posters","poster":{"id":"birthdaycake.png","name":"Birthday Cake","url":"https://assets.getpartiful.com/posters/birthdaycake.png","blurHash":"LKO2?U%2Tw=w]~RBVZRi};RPxuwH","contentType":"image/png","height":1200,"width":800,"tags":["birthday"],"categories":["birthday"]},"url":"https://assets.getpartiful.com/posters/birthdaycake.png","blurHash":"LKO2?U%2Tw=w]~RBVZRi};RPxuwH","contentType":"image/png","name":"Birthday Cake","height":1200,"width":800},"showHostList":true,"showGuestCount":true,"showGuestList":true,"showActivityTimestamps":true,"displayInviteButton":true,"visibility":"public","allowGuestPhotoUpload":true,"enableGuestReminders":true,"rsvpsEnabled":true,"allowGuestsToInviteMutuals":true},"cohostIds":[]},"userId":"private-account"}}`)
			return jsonResponse(http.StatusOK, `{"data":"private-event-id"}`), nil
		default:
			t.Fatalf("unexpected request %d: %s", call, request.URL)
			return nil, nil
		}
	}}
	argv := []string{
		"events", "create",
		"--title", "Example event",
		"--start", "2026-09-12T19:00:00-07:00",
		"--timezone", "America/Los_Angeles",
		"--poster-id", "birthdaycake.png",
	}
	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 {
		t.Fatalf("plan = %#v", plan)
	}
	token := rsvpPlanToken(t, plan)
	applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--plan", token)}, dependencies)
	if applied.ExitCode != 0 ||
		!strings.Contains(applied.Stdout, `"data":{"submitted":true}`) ||
		strings.Contains(applied.Stdout, "private-event-id") ||
		applied.Stderr != "" {
		t.Fatalf("applied = %#v, want submitted-only create result", applied)
	}
	reused := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--plan", token)}, dependencies)
	if reused.ExitCode != 7 || !strings.Contains(reused.Stdout, `"type":"safety.plan_stale"`) {
		t.Fatalf("reused = %#v, want stale plan after one attempt", reused)
	}
}

func TestExecuteEventCreateFailsClosedOnMalformedCompletionAndExpiredPlan(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("e", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 2:
			if request.Method != http.MethodGet || request.URL.String() != "https://assets.getpartiful.com/posters.json" {
				t.Fatalf("request = %s %s, want poster catalog", request.Method, request.URL)
			}
			return jsonResponse(http.StatusOK, `[{"id":"birthdaycake.png","url":"https://assets.getpartiful.com/posters/birthdaycake.png","name":"Birthday Cake","tags":["birthday"],"categories":["birthday"],"contentType":"image/png","height":1200,"width":800}]`), nil
		case 3:
			assertEventCallableRequest(t, request, "createEvent", `{"data":{"params":{"event":{"title":"Example event","startDate":"2026-09-13T02:00:00Z","timezone":"America/Los_Angeles","guestStatusCounts":{"APPROVED":0,"DECLINED":0,"DELIVERY_ERROR":0,"GOING":0,"INTERESTED":0,"MAYBE":0,"PENDING_APPROVAL":0,"READY_TO_SEND":0,"REJECTED":0,"RESPONDED_TO_FIND_A_TIME":0,"SENDING":0,"SEND_ERROR":0,"SENT":0,"WAITLIST":0,"WAITLISTED_FOR_APPROVAL":0,"WITHDRAWN":0},"displaySettings":{"theme":"cloudflow","effect":"fireflies","titleFont":"display"},"status":"UNSAVED","rsvpButtonGlyphType":"emojis","image":{"source":"partiful_posters","poster":{"id":"birthdaycake.png","name":"Birthday Cake","url":"https://assets.getpartiful.com/posters/birthdaycake.png","contentType":"image/png","height":1200,"width":800,"tags":["birthday"],"categories":["birthday"]},"url":"https://assets.getpartiful.com/posters/birthdaycake.png","blurHash":null,"contentType":"image/png","name":"Birthday Cake","height":1200,"width":800},"showHostList":true,"showGuestCount":true,"showGuestList":true,"showActivityTimestamps":true,"displayInviteButton":true,"visibility":"public","allowGuestPhotoUpload":true,"enableGuestReminders":true,"rsvpsEnabled":true,"allowGuestsToInviteMutuals":true},"cohostIds":[]},"userId":"private-account"}}`)
			return jsonResponse(http.StatusOK, `{"unexpected":true}`), nil
		default:
			t.Fatalf("unexpected request %d", call)
			return nil, nil
		}
	}}
	argv := []string{"events", "create", "--title", "Example event", "--start", "2026-09-12T19:00:00-07:00", "--timezone", "America/Los_Angeles", "--poster-id", "birthdaycake.png"}
	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 {
		t.Fatalf("plan = %#v", plan)
	}
	token := rsvpPlanToken(t, plan)
	applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--plan", token)}, dependencies)
	if applied.ExitCode != 9 || !strings.Contains(applied.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("applied = %#v, want protocol-changed failure", applied)
	}

	dependencies.Now = func() time.Time {
		return time.Date(2026, time.August, 12, 0, 6, 0, 0, time.UTC)
	}
	expired := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--plan", token)}, dependencies)
	if expired.ExitCode != 7 || !strings.Contains(expired.Stdout, `"type":"safety.plan_stale"`) {
		t.Fatalf("expired = %#v, want stale expired plan", expired)
	}
}

func TestExecuteEventUpdatePlansSortedFirestorePatchAndStalesOnChangedPreconditions(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("u", 32))
	call := 0
	first := compatibleUpdateEvent()
	second := compatibleUpdateEvent()
	second["title"] = "Changed elsewhere"
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1:
			assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
			return jsonResponse(http.StatusOK, eventResponse(t, first)), nil
		case 2:
			assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
			return jsonResponse(http.StatusOK, eventResponse(t, second)), nil
		default:
			t.Fatalf("unexpected request %d: %s", call, request.URL)
			return nil, nil
		}
	}}
	argv := []string{
		"events", "update", "event-example",
		"--title", "Updated title",
		"--start", "2026-09-12T20:00:00Z",
		"--timezone", "America/Los_Angeles",
	}
	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 || !strings.Contains(plan.Stdout, `"fields":["start","timezone","title"]`) {
		t.Fatalf("plan = %#v, want sorted update fields", plan)
	}
	token := rsvpPlanToken(t, plan)
	applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--plan", token)}, dependencies)
	if applied.ExitCode != 7 || !strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) {
		t.Fatalf("applied = %#v, want stale update plan", applied)
	}
	if call != 2 {
		t.Fatalf("request count = %d, want one plan read and one apply read", call)
	}
}

func TestExecuteEventUpdateAppliesExactFirestorePatchAndReturnsSubmittedFields(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("p", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 2:
			assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
			return jsonResponse(http.StatusOK, eventResponse(t, compatibleUpdateEvent())), nil
		case 3:
			if request.Method != http.MethodPatch || request.URL.String() != "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/events/event-example?currentDocument.exists=true&updateMask.fieldPaths=description&updateMask.fieldPaths=title&updateMask.fieldPaths=updatedBy" {
				t.Fatalf("request = %s %s, want reviewed patch request", request.Method, request.URL)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			const wantBody = `{"fields":{"description":{"stringValue":"Updated description"},"title":{"stringValue":"Updated title"},"updatedBy":{"referenceValue":"projects/getpartiful/databases/(default)/documents/users/private-account"}}}`
			if string(body) != wantBody {
				t.Fatalf("request body = %s, want %s", body, wantBody)
			}
			return jsonResponse(http.StatusOK, `{"name":"projects/getpartiful/databases/(default)/documents/events/event-example","fields":{"title":{"stringValue":"Updated title"}}}`), nil
		default:
			t.Fatalf("unexpected request %d: %s", call, request.URL)
			return nil, nil
		}
	}}
	argv := []string{"events", "update", "event-example", "--title", "Updated title", "--description", "Updated description"}
	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 || strings.Contains(plan.Stdout, "private-account") {
		t.Fatalf("plan = %#v, want redacted update plan", plan)
	}
	token := rsvpPlanToken(t, plan)
	applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--plan", token)}, dependencies)
	if applied.ExitCode != 0 ||
		!strings.Contains(applied.Stdout, `"data":{"eventId":"event-example","fields":["description","title"],"submitted":true}`) ||
		applied.Stderr != "" {
		t.Fatalf("applied = %#v, want submitted-only update result", applied)
	}
}

func TestExecuteEventUpdateFailsClosedOnMalformedPatchCompletion(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("q", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 2:
			assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
			return jsonResponse(http.StatusOK, eventResponse(t, compatibleUpdateEvent())), nil
		case 3:
			return jsonResponse(http.StatusOK, `{"unexpected":true}`), nil
		default:
			t.Fatalf("unexpected request %d", call)
			return nil, nil
		}
	}}
	argv := []string{"events", "update", "event-example", "--title", "Updated title"}
	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 {
		t.Fatalf("plan = %#v", plan)
	}
	token := rsvpPlanToken(t, plan)
	applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--plan", token)}, dependencies)
	if applied.ExitCode != 9 || !strings.Contains(applied.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("applied = %#v, want protocol-changed failure", applied)
	}
}

func TestExecuteEventUpdateRejectsDisallowedFieldsAndInvertedMergedRange(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		return nil, errors.New("must not call HTTP")
	}}
	badFlag := app.Execute(context.Background(), app.Request{Argv: []string{"events", "update", "event-example", "--location", "Elsewhere"}}, dependencies)
	if badFlag.ExitCode != 2 || !strings.Contains(badFlag.Stdout, `"type":"input.invalid"`) {
		t.Fatalf("bad flag = %#v, want input error", badFlag)
	}

	dependencies.AuthRandom = strings.NewReader("0123456789abcdef")
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
		return jsonResponse(http.StatusOK, eventResponse(t, compatibleUpdateEvent())), nil
	}}
	inverted := app.Execute(context.Background(), app.Request{Argv: []string{"events", "update", "event-example", "--end", "2026-09-12T18:00:00Z"}}, dependencies)
	if inverted.ExitCode != 2 || !strings.Contains(inverted.Stdout, `"code":"EVENT_RANGE_INVALID"`) {
		t.Fatalf("inverted = %#v, want merged range failure", inverted)
	}
}

func TestExecuteEventWriteRejectsMalformedLinkFlag(t *testing.T) {
	dependencies := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{}})
	dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		return nil, errors.New("must not call HTTP")
	}}

	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"events", "create", "--link", "missing-separator"},
	}, dependencies)
	if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"code":"LINKS_INVALID"`) {
		t.Fatalf("result = %#v, want malformed link input failure", result)
	}
}

func TestExecuteEventUpdateFailsClosedWhenMergedRangeNeedsMissingStart(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	event := compatibleUpdateEvent()
	delete(event, "startDate")
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
		return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
	}}

	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"events", "update", "event-example", "--end", "2026-09-12T23:00:00Z"},
	}, dependencies)
	if result.ExitCode != 9 || !strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
		t.Fatalf("result = %#v, want missing current start protocol failure", result)
	}
}

func TestExecuteEventUpdateBindsAbsentCustomFields(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("l", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		event := compatibleUpdateEvent()
		if call == 2 {
			event["customFields"] = []any{}
		}
		assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
		return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
	}}
	argv := []string{"events", "update", "event-example", "--link", "Tickets=https://example.test/tickets"}

	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 {
		t.Fatalf("plan = %#v, want absent target field bound", plan)
	}
	token := rsvpPlanToken(t, plan)
	applied := app.Execute(context.Background(), app.Request{
		Argv: append(append([]string{}, argv...), "--apply", "--plan", token),
	}, dependencies)
	if applied.ExitCode != 7 || !strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) {
		t.Fatalf("applied = %#v, want changed target state stale failure", applied)
	}
}

func TestExecuteEventCancelRequiresConfirmAndAppliesConsequentialPlan(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("k", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 2:
			assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
			return jsonResponse(http.StatusOK, eventResponse(t, compatibleCancelEvent())), nil
		case 3:
			assertEventCallableRequest(t, request, "cancelEvent", `{"data":{"params":{"eventId":"event-example","cancellationMessage":"See you next time","shouldSkipNotifyGuests":true},"userId":"private-account"}}`)
			return jsonResponse(http.StatusOK, `{"result":true}`), nil
		default:
			t.Fatalf("unexpected request %d: %s", call, request.URL)
			return nil, nil
		}
	}}
	argv := []string{"events", "cancel", "event-example", "--message", "See you next time", "--notify-guests", "false"}
	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 ||
		!strings.Contains(plan.Stdout, `"eventId":"event-example"`) ||
		!strings.Contains(plan.Stdout, `"notifyGuests":false`) ||
		!strings.Contains(plan.Stdout, `"effects"`) {
		t.Fatalf("plan = %#v, want consequential cancel plan", plan)
	}
	missingConfirm := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply")}, dependencies)
	if missingConfirm.ExitCode != 7 || !strings.Contains(missingConfirm.Stdout, `"type":"safety.confirmation_required"`) {
		t.Fatalf("missing confirm = %#v, want confirmation-required failure", missingConfirm)
	}
	token := rsvpPlanToken(t, plan)
	applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", token)}, dependencies)
	if applied.ExitCode != 0 ||
		!strings.Contains(applied.Stdout, `"data":{"eventId":"event-example","notifyGuests":false,"submitted":true}`) ||
		applied.Stderr != "" {
		t.Fatalf("applied = %#v, want submitted-only cancel result", applied)
	}
}

func TestExecuteEventCancelFailsClosedOnMalformedFactsAndSubmissionUncertainty(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("m", 32))

	cases := []struct {
		name     string
		event    map[string]any
		httpErr  error
		exitCode int
		contains string
	}{
		{name: "missing guest count", event: compatibleUpdateEvent(), exitCode: 9, contains: `"type":"contract.protocol_changed"`},
		{name: "past start", event: func() map[string]any { e := compatibleCancelEvent(); e["startDate"] = "2026-08-11T19:00:00Z"; return e }(), exitCode: 6, contains: `"code":"EVENT_PRECONDITION_FAILED"`},
		{name: "uncertain submission", httpErr: errors.New("private network"), exitCode: 8, contains: `"type":"remote.unavailable"`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			call := 0
			dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				call++
				switch call {
				case 1:
					assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
					if testCase.event == nil {
						return jsonResponse(http.StatusOK, eventResponse(t, compatibleCancelEvent())), nil
					}
					return jsonResponse(http.StatusOK, eventResponse(t, testCase.event)), nil
				case 2:
					assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
					return jsonResponse(http.StatusOK, eventResponse(t, compatibleCancelEvent())), nil
				case 3:
					if testCase.httpErr != nil {
						return nil, testCase.httpErr
					}
					return jsonResponse(http.StatusOK, `{"data":true}`), nil
				default:
					t.Fatalf("unexpected request %d", call)
					return nil, nil
				}
			}}
			argv := []string{"events", "cancel", "event-example"}
			if testCase.httpErr == nil {
				result := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
				if result.ExitCode != testCase.exitCode || !strings.Contains(result.Stdout, testCase.contains) {
					t.Fatalf("result = %#v, want exit %d containing %q", result, testCase.exitCode, testCase.contains)
				}
				return
			}
			plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
			if plan.ExitCode != 0 {
				t.Fatalf("plan = %#v", plan)
			}
			token := rsvpPlanToken(t, plan)
			applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", token)}, dependencies)
			if applied.ExitCode != testCase.exitCode || !strings.Contains(applied.Stdout, testCase.contains) {
				t.Fatalf("applied = %#v, want exit %d containing %q", applied, testCase.exitCode, testCase.contains)
			}
			if reused := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", token)}, dependencies); reused.ExitCode != 7 {
				t.Fatalf("reused = %#v, want stale consumed plan", reused)
			}
		})
	}
}

func TestExecuteSchemaIncludesEventWriteCommands(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{Argv: []string{"schema"}}, app.Dependencies{})
	var envelope struct {
		Data struct {
			Commands []string `json:"commands"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(result.Stdout), &envelope) != nil || result.ExitCode != 0 {
		t.Fatalf("schema = %#v", result)
	}
	for _, command := range []string{"events.create", "events.update", "events.cancel"} {
		if !slices.Contains(envelope.Data.Commands, command) {
			t.Fatalf("commands = %v, want %s", envelope.Data.Commands, command)
		}
	}
}
