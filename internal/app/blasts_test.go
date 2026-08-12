package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func firestoreListResponse(t *testing.T, documents ...map[string]any) string {
	t.Helper()
	entries := make([]map[string]any, 0, len(documents))
	entries = append(entries, documents...)
	body, err := json.Marshal(map[string]any{"documents": entries})
	if err != nil {
		t.Fatalf("encode firestore list response: %v", err)
	}
	return string(body)
}

func firestoreDocument(name string, fields map[string]any) map[string]any {
	return map[string]any{
		"name":   name,
		"fields": fields,
	}
}

func firestoreString(value string) map[string]any {
	return map[string]any{"stringValue": value}
}

func firestoreTimestamp(value string) map[string]any {
	return map[string]any{"timestampValue": value}
}

func assertFirestoreListRequest(t *testing.T, request *http.Request, collection string) {
	t.Helper()
	if request.Method != http.MethodGet {
		t.Fatalf("request method = %s, want GET", request.Method)
	}
	want := "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/" + collection + "?pageSize=1000"
	if request.URL.String() != want {
		t.Fatalf("request URL = %s, want %s", request.URL.String(), want)
	}
	if auth := request.Header.Get("Authorization"); auth != "Bearer private-access-token" {
		t.Fatalf("authorization = %q, want bearer token", auth)
	}
}

func compatibleBlastEvent() map[string]any {
	event := compatibleCancelEvent()
	event["findATime"] = nil
	delete(event, "guestAction")
	delete(event, "enableWaitlist")
	return event
}

func TestExecuteBlastSendPlansAndAppliesPrivacySafeAllGuests(t *testing.T) {
	const messageText = "Keep this private"
	messageDigest := sha256.Sum256([]byte(messageText))
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
		"message.txt":             []byte(messageText),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("b", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 4:
			assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
			return jsonResponse(http.StatusOK, eventResponse(t, compatibleBlastEvent())), nil
		case 2, 5:
			assertFirestoreListRequest(t, request, "events/event-example/guests")
			return jsonResponse(http.StatusOK, firestoreListResponse(t,
				firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/guests/g1", map[string]any{"status": firestoreString("GOING"), "checkIn": firestoreTimestamp("2026-09-12T19:10:00Z")}),
				firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/guests/g2", map[string]any{"status": firestoreString("MAYBE")}),
				firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/guests/g3", map[string]any{"status": firestoreString("READY_TO_SEND")}),
			)), nil
		case 3, 6:
			assertFirestoreListRequest(t, request, "events/event-example/hostMessages")
			return jsonResponse(http.StatusOK, firestoreListResponse(t,
				firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/hostMessages/h1", map[string]any{}),
				firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/hostMessages/h2", map[string]any{"type": firestoreString("TEXT_BLAST")}),
				firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/hostMessages/h3", map[string]any{"type": firestoreString("CANCELLATION_MESSAGE")}),
			)), nil
		case 7:
			assertEventCallableRequest(t, request, "createTextBlast", `{"data":{"params":{"eventId":"event-example","message":{"text":"Keep this private","to":["invited","checkedIn","GOING","MAYBE"],"showOnEventPage":true}},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","userId":"private-account"}}`)
			return jsonResponse(http.StatusOK, `{"result":true}`), nil
		default:
			t.Fatalf("unexpected request %d: %s %s", call, request.Method, request.URL)
			return nil, nil
		}
	}}

	argv := []string{"blasts", "send", "event-example", "--audience", "all-guests", "--message-file", "message.txt", "--show-on-event-page"}
	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 || plan.Stderr != "" {
		t.Fatalf("plan = %#v, want success", plan)
	}
	var envelope struct {
		Data struct {
			Operation string `json:"operation"`
			EventID   string `json:"eventId"`
			Input     struct {
				Audience        string `json:"audience"`
				ShowOnEventPage bool   `json:"showOnEventPage"`
				Message         struct {
					SHA256 string `json:"sha256"`
					Length int    `json:"length"`
				} `json:"message"`
			} `json:"input"`
			Request struct {
				Message struct {
					TextSHA256      string   `json:"textSha256"`
					TextLength      int      `json:"textLength"`
					To              []string `json:"to"`
					ShowOnEventPage bool     `json:"showOnEventPage"`
				} `json:"message"`
			} `json:"request"`
			PlanToken string `json:"planToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(plan.Stdout), &envelope); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, plan.Stdout)
	}
	if envelope.Data.Operation != "createTextBlast" || envelope.Data.EventID != "event-example" || envelope.Data.Input.Audience != "all-guests" || !envelope.Data.Input.ShowOnEventPage || envelope.Data.Input.Message.SHA256 != hex.EncodeToString(messageDigest[:]) || envelope.Data.Input.Message.Length != len([]rune(messageText)) {
		t.Fatalf("plan input = %#v, want blast plan summary", envelope.Data)
	}
	if got := envelope.Data.Request.Message.To; fmt.Sprint(got) != fmt.Sprint([]string{"invited", "checkedIn", "GOING", "MAYBE"}) || !envelope.Data.Request.Message.ShowOnEventPage || envelope.Data.Request.Message.TextSHA256 != hex.EncodeToString(messageDigest[:]) || envelope.Data.Request.Message.TextLength != len([]rune(messageText)) {
		t.Fatalf("plan request = %#v, want exact reviewed audience mapping", envelope.Data.Request.Message)
	}
	if strings.Contains(plan.Stdout+plan.Stderr, messageText) {
		t.Fatal("plan output exposed private message text")
	}
	if strings.Contains(string(files.files[eventWriteMutationPath]), messageText) {
		t.Fatal("stored mutation plan exposed private message text")
	}

	applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", envelope.Data.PlanToken)}, dependencies)
	if applied.ExitCode != 0 || applied.Stderr != "" || strings.Contains(applied.Stdout, messageText) {
		t.Fatalf("applied = %#v, want private submitted result", applied)
	}
	if !strings.Contains(applied.Stdout, `"recipientStatus":"not-reported"`) || !strings.Contains(applied.Stdout, `"audience":"all-guests"`) || !strings.Contains(applied.Stdout, `"showOnEventPage":true`) {
		t.Fatalf("applied stdout = %s, want submitted blast result", applied.Stdout)
	}
	if reused := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", envelope.Data.PlanToken)}, dependencies); reused.ExitCode != 7 {
		t.Fatalf("reused = %#v, want stale consumed plan", reused)
	}
}

func TestExecuteBlastSendSupportsStdinAndRejectsArgvMessage(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{eventWriteCredentialsPath: []byte(eventWriteCredentials)}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("c", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1:
			assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
			event := compatibleBlastEvent()
			event["guestAction"] = "APPLY"
			return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
		case 2:
			assertFirestoreListRequest(t, request, "events/event-example/guests")
			return jsonResponse(http.StatusOK, firestoreListResponse(t,
				firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/guests/g1", map[string]any{"status": firestoreString("APPROVED")}),
				firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/guests/g2", map[string]any{"status": firestoreString("READY_TO_SEND")}),
			)), nil
		case 3:
			assertFirestoreListRequest(t, request, "events/event-example/hostMessages")
			return jsonResponse(http.StatusOK, firestoreListResponse(t)), nil
		default:
			t.Fatalf("unexpected request %d", call)
			return nil, nil
		}
	}}

	result := app.Execute(context.Background(), app.Request{Argv: []string{"blasts", "send", "event-example", "--message", "bad"}}, dependencies)
	if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"code":"FLAG_UNKNOWN"`) {
		t.Fatalf("argv message result = %#v, want unknown flag", result)
	}

	stdinPlan := app.Execute(context.Background(), app.Request{Argv: []string{"blasts", "send", "event-example", "--audience", "all-guests", "--message-file", "-"}, Stdin: strings.NewReader("stdin secret")}, dependencies)
	if stdinPlan.ExitCode != 0 || strings.Contains(stdinPlan.Stdout, "stdin secret") || !strings.Contains(stdinPlan.Stdout, `"to":["invited","APPROVED"]`) {
		t.Fatalf("stdin plan = %#v, want stdin-based blast plan", stdinPlan)
	}
}

func TestExecuteBlastSendRejectsTheEleventhBlast(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
		"message.txt":             []byte("Private body"),
	}}
	dependencies := eventWriteDependencies(files)
	hostMessages := make([]map[string]any, 0, 10)
	for index := range 10 {
		hostMessages = append(hostMessages, firestoreDocument(
			fmt.Sprintf("projects/getpartiful/databases/(default)/documents/events/event-example/hostMessages/m%d", index),
			map[string]any{},
		))
	}
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1:
			return jsonResponse(http.StatusOK, eventResponse(t, compatibleBlastEvent())), nil
		case 2:
			return jsonResponse(http.StatusOK, firestoreListResponse(t,
				firestoreDocument(
					"projects/getpartiful/databases/(default)/documents/events/event-example/guests/g1",
					map[string]any{"status": firestoreString("GOING")},
				),
			)), nil
		case 3:
			return jsonResponse(http.StatusOK, firestoreListResponse(t, hostMessages...)), nil
		default:
			return nil, errors.New("must not submit an eleventh blast")
		}
	}}

	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"blasts", "send", "event-example", "--audience", "all-guests", "--message-file", "message.txt"},
	}, dependencies)
	if result.ExitCode != 6 || !strings.Contains(result.Stdout, `"code":"EVENT_PRECONDITION_FAILED"`) {
		t.Fatalf("result = %#v, want blast limit precondition failure", result)
	}
	if call != 3 {
		t.Fatalf("request count = %d, want read-only precondition checks", call)
	}
}

func TestExecuteBlastSendFailsClosedOnProtocolAndSubmissionUncertainty(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
		"message.txt":             []byte("Private body"),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("d", 32))

	cases := []struct {
		name     string
		event    map[string]any
		hostBody string
		postErr  error
		postBody string
		exitCode int
		contains string
	}{
		{name: "missing find a time", event: func() map[string]any { e := compatibleBlastEvent(); delete(e, "findATime"); return e }(), exitCode: 9, contains: `"type":"contract.protocol_changed"`},
		{name: "old event", event: func() map[string]any { e := compatibleBlastEvent(); e["endDate"] = "2026-05-01T01:00:00Z"; return e }(), exitCode: 6, contains: `"code":"EVENT_PRECONDITION_FAILED"`},
		{name: "malformed completion", event: compatibleBlastEvent(), hostBody: firestoreListResponse(t), postBody: `{"result":null}`, exitCode: 9, contains: `"type":"contract.protocol_changed"`},
		{name: "submission uncertain", event: compatibleBlastEvent(), hostBody: firestoreListResponse(t), postErr: errors.New("network failure"), exitCode: 8, contains: `"code":"TEXT_BLAST_SUBMISSION_UNCERTAIN"`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			deps := dependencies
			deps.MutationRandom = strings.NewReader(strings.Repeat("d", 32))
			call := 0
			deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				call++
				switch call {
				case 1:
					assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
					return jsonResponse(http.StatusOK, eventResponse(t, testCase.event)), nil
				case 2:
					assertFirestoreListRequest(t, request, "events/event-example/guests")
					return jsonResponse(http.StatusOK, firestoreListResponse(t, firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/guests/g1", map[string]any{"status": firestoreString("GOING")}))), nil
				case 3:
					assertFirestoreListRequest(t, request, "events/event-example/hostMessages")
					return jsonResponse(http.StatusOK, firestoreListResponse(t)), nil
				case 4:
					assertEventCallableRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
					return jsonResponse(http.StatusOK, eventResponse(t, compatibleBlastEvent())), nil
				case 5:
					assertFirestoreListRequest(t, request, "events/event-example/guests")
					return jsonResponse(http.StatusOK, firestoreListResponse(t, firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/guests/g1", map[string]any{"status": firestoreString("GOING")}))), nil
				case 6:
					assertFirestoreListRequest(t, request, "events/event-example/hostMessages")
					return jsonResponse(http.StatusOK, testCase.hostBody), nil
				case 7:
					if testCase.postErr != nil {
						return nil, testCase.postErr
					}
					return jsonResponse(http.StatusOK, testCase.postBody), nil
				default:
					t.Fatalf("unexpected request %d", call)
					return nil, nil
				}
			}}
			argv := []string{"blasts", "send", "event-example", "--audience", "all-guests", "--message-file", "message.txt"}
			if testCase.postBody == "" && testCase.postErr == nil {
				result := app.Execute(context.Background(), app.Request{Argv: argv}, deps)
				if result.ExitCode != testCase.exitCode || !strings.Contains(result.Stdout, testCase.contains) {
					t.Fatalf("result = %#v, want exit %d containing %q", result, testCase.exitCode, testCase.contains)
				}
				return
			}
			plan := app.Execute(context.Background(), app.Request{Argv: argv}, deps)
			if plan.ExitCode != 0 {
				t.Fatalf("plan = %#v", plan)
			}
			token := rsvpPlanToken(t, plan)
			applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", token)}, deps)
			if applied.ExitCode != testCase.exitCode || !strings.Contains(applied.Stdout, testCase.contains) {
				t.Fatalf("applied = %#v, want exit %d containing %q", applied, testCase.exitCode, testCase.contains)
			}
			if reused := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", token)}, deps); reused.ExitCode != 7 {
				t.Fatalf("reused = %#v, want stale consumed plan", reused)
			}
		})
	}
}
