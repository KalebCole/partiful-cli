package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func contactPageResponse(items []map[string]any, nextCursor *string) string {
	page := map[string]any{
		"result": map[string]any{
			"data":   items,
			"paging": map[string]any{},
		},
	}
	if nextCursor != nil {
		page["result"].(map[string]any)["paging"] = map[string]any{"nextCursor": *nextCursor}
	}
	body, _ := json.Marshal(page)
	return string(body)
}

func cohostRequestDocument(targetUserID, status string) string {
	return `{"name":"projects/getpartiful/databases/(default)/documents/events/event-example/cohostRequests/req-1","fields":{"target":{"referenceValue":"projects/getpartiful/databases/(default)/documents/users/` + targetUserID + `"},"status":{"stringValue":"` + status + `"}}}`
}

func cohostLinkDocument(path string) string {
	return `{"name":"projects/getpartiful/databases/(default)/documents/events/event-example/private/cohostSecret","fields":{"path":{"stringValue":"` + path + `"}}}`
}

func firestoreNotFound() string {
	return `{"error":{"status":"NOT_FOUND"}}`
}

func assertContactsPageRequest(t *testing.T, request *http.Request, cursor *string) {
	t.Helper()
	if request.Method != http.MethodPost || request.URL.String() != "https://api.partiful.com/getContacts" {
		t.Fatalf("request = %s %s, want getContacts", request.Method, request.URL)
	}
	want := `{"data":{"params":{},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","paging":{"maxResults":1000,"cursor":null}}}`
	if cursor != nil {
		want = `{"data":{"params":{},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","paging":{"maxResults":1000,"cursor":"` + *cursor + `"}}}`
	}
	assertEventCallableRequest(t, request, "getContacts", want)
}

func TestExecuteCohostInviteDryRunsAndDispatchesOnce(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	cursor := "cursor-1"
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 5:
			assertEventCallableRequest(
				t,
				request,
				"getEventInfo",
				`{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`,
			)
			event := compatibleUpdateEvent()
			event["ownerIds"] = []string{"private-account"}
			return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
		case 2, 6:
			assertContactsPageRequest(t, request, nil)
			return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{{
				"id":               "private-contact-id",
				"name":             "Example Contact",
				"sharedEventCount": 3,
			}}, &cursor)), nil
		case 3, 7:
			assertContactsPageRequest(t, request, &cursor)
			return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
		case 4, 8:
			if request.Method != http.MethodGet || request.URL.String() != "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/events/event-example/cohostRequests?pageSize=100" {
				t.Fatalf("request = %s %s, want cohost request list", request.Method, request.URL)
			}
			return jsonResponse(http.StatusOK, `{}`), nil
		case 9:
			assertEventCallableRequest(
				t,
				request,
				"createCohostRequest",
				`{"data":{"params":{"eventId":"event-example","targetUserId":"private-contact-id"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","userId":"private-account"}}`,
			)
			return jsonResponse(http.StatusOK, `{"result":{"data":{}}}`), nil
		default:
			t.Fatalf("unexpected request %d: %s", call, request.URL)
			return nil, nil
		}
	}}

	argv := []string{"cohosts", "invite", "event-example", "--contact", "  Example Contact  "}
	preview := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--dry-run")}, dependencies)
	if preview.ExitCode != 0 || preview.Stderr != "" {
		t.Fatalf("preview = %#v, want cohost invite preview", preview)
	}
	if !strings.Contains(preview.Stdout, `"operation":"createCohostRequest"`) ||
		!strings.Contains(preview.Stdout, `"displayName":"Example Contact"`) ||
		!strings.Contains(preview.Stdout, `"cohostState":"bound"`) {
		t.Fatalf("preview stdout = %s, want redacted cohost invite preview", preview.Stdout)
	}
	applied := app.Execute(context.Background(), app.Request{
		Argv: argv,
	}, dependencies)
	if applied.ExitCode != 0 ||
		!strings.Contains(applied.Stdout, `"status":"invited"`) ||
		applied.Stderr != "" {
		t.Fatalf("applied = %#v, want invite success", applied)
	}
	for _, privateValue := range []string{"private-contact-id", "private-account", "private-access-token"} {
		if strings.Contains(preview.Stdout+applied.Stdout+preview.Stderr+applied.Stderr, privateValue) {
			t.Fatalf("output exposed private value %q", privateValue)
		}
	}
	if call != 9 {
		t.Fatalf("request count = %d, want four preview reads and one four-read/one-write execution", call)
	}
}

func TestExecuteCohostRevokeInviteAndRemoveDryRunThenForce(t *testing.T) {
	tests := []struct {
		name           string
		argv           []string
		status         string
		operation      string
		successStatus  string
		mutationBody   string
		mutationResult string
	}{
		{
			name:           "revoke invite",
			argv:           []string{"cohosts", "revoke-invite", "event-example", "--contact", "Example Contact"},
			status:         "DECLINED",
			operation:      "deleteCohostRequest",
			successStatus:  "revoked",
			mutationBody:   `{"data":{"params":{"eventId":"event-example","targetUserId":"private-contact-id"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","userId":"private-account"}}`,
			mutationResult: `{"result":null}`,
		},
		{
			name:           "remove cohost",
			argv:           []string{"cohosts", "remove", "event-example", "--contact", "Example Contact"},
			status:         "ACCEPTED",
			operation:      "removeCohost",
			successStatus:  "removed",
			mutationBody:   `{"data":{"params":{"eventId":"event-example","targetUserId":"private-contact-id"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","userId":"private-account"}}`,
			mutationResult: `{"data":true}`,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			files := &memoryFilesystem{files: map[string][]byte{
				eventWriteCredentialsPath: []byte(eventWriteCredentials),
			}}
			dependencies := eventWriteDependencies(files)
			cursor := "cursor-1"
			call := 0
			dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				call++
				switch call {
				case 1, 5:
					assertEventCallableRequest(
						t,
						request,
						"getEventInfo",
						`{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`,
					)
					event := compatibleUpdateEvent()
					event["ownerIds"] = []string{"private-account"}
					return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
				case 2, 6:
					assertContactsPageRequest(t, request, nil)
					return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{{
						"id":               "private-contact-id",
						"name":             "Example Contact",
						"sharedEventCount": 3,
					}}, &cursor)), nil
				case 3, 7:
					assertContactsPageRequest(t, request, &cursor)
					return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
				case 4, 8:
					if request.Method != http.MethodGet || request.URL.String() != "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/events/event-example/cohostRequests?pageSize=100" {
						t.Fatalf("request = %s %s, want cohost request list", request.Method, request.URL)
					}
					return jsonResponse(http.StatusOK, `{"documents":[`+cohostRequestDocument("private-contact-id", testCase.status)+`]}`), nil
				case 9:
					assertEventCallableRequest(t, request, testCase.operation, testCase.mutationBody)
					return jsonResponse(http.StatusOK, testCase.mutationResult), nil
				default:
					t.Fatalf("unexpected request %d: %s", call, request.URL)
					return nil, nil
				}
			}}

			preview := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, testCase.argv...), "--dry-run")}, dependencies)
			if preview.ExitCode != 0 || !strings.Contains(preview.Stdout, `"operation":"`+testCase.operation+`"`) {
				t.Fatalf("preview = %#v, want %s preview", preview, testCase.operation)
			}
			applied := app.Execute(context.Background(), app.Request{
				Argv: append(append([]string{}, testCase.argv...), "--force"),
			}, dependencies)
			if applied.ExitCode != 0 || !strings.Contains(applied.Stdout, `"status":"`+testCase.successStatus+`"`) {
				t.Fatalf("applied = %#v, want %s success", applied, testCase.successStatus)
			}
			if call != 9 {
				t.Fatalf("request count = %d, want one dry-run and one single-attempt execution", call)
			}
		})
	}
}

func TestExecuteCohostLinkCreateAndRevokeReturnDocumentedState(t *testing.T) {
	tests := []struct {
		name             string
		argv             []string
		linkResponse     string
		linkPresent      bool
		operation        string
		successContains  string
		mutationBody     string
		mutationResponse string
	}{
		{
			name:             "create link",
			argv:             []string{"cohosts", "link", "create", "event-example"},
			linkPresent:      false,
			linkResponse:     firestoreNotFound(),
			operation:        "generateEventCohostLink",
			successContains:  `"link":{"url":"https://partiful.com/e/event-example?accept-cohost=secret-token","state":"active"}`,
			mutationBody:     `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","userId":"private-account"}}`,
			mutationResponse: `{"result":{"data":{"path":"/e/event-example?accept-cohost=secret-token"}}}`,
		},
		{
			name:             "revoke link",
			argv:             []string{"cohosts", "link", "revoke", "event-example"},
			linkPresent:      true,
			linkResponse:     cohostLinkDocument(`/e/event-example?accept-cohost=existing-token`),
			operation:        "revokeEventCohostLink",
			successContains:  `"link":{"url":null,"state":"revoked"}`,
			mutationBody:     `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg","userId":"private-account"}}`,
			mutationResponse: `{"result":null}`,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			files := &memoryFilesystem{files: map[string][]byte{
				eventWriteCredentialsPath: []byte(eventWriteCredentials),
			}}
			dependencies := eventWriteDependencies(files)
			call := 0
			dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				call++
				switch call {
				case 1, 3:
					assertEventCallableRequest(
						t,
						request,
						"getEventInfo",
						`{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`,
					)
					event := compatibleUpdateEvent()
					event["ownerIds"] = []string{"private-account"}
					return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
				case 2, 4:
					if request.Method != http.MethodGet || request.URL.String() != "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/events/event-example/private/cohostSecret" {
						t.Fatalf("request = %s %s, want cohost secret read", request.Method, request.URL)
					}
					if testCase.linkPresent {
						return jsonResponse(http.StatusOK, testCase.linkResponse), nil
					}
					return jsonResponse(http.StatusNotFound, testCase.linkResponse), nil
				case 5:
					assertEventCallableRequest(t, request, testCase.operation, testCase.mutationBody)
					return jsonResponse(http.StatusOK, testCase.mutationResponse), nil
				default:
					t.Fatalf("unexpected request %d: %s", call, request.URL)
					return nil, nil
				}
			}}

			preview := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, testCase.argv...), "--dry-run")}, dependencies)
			if preview.ExitCode != 0 || !strings.Contains(preview.Stdout, `"operation":"`+testCase.operation+`"`) {
				t.Fatalf("preview = %#v, want %s preview", preview, testCase.operation)
			}
			executionArgv := append([]string{}, testCase.argv...)
			if testCase.operation == "revokeEventCohostLink" {
				executionArgv = append(executionArgv, "--force")
			}
			applied := app.Execute(context.Background(), app.Request{
				Argv: executionArgv,
			}, dependencies)
			if applied.ExitCode != 0 || !strings.Contains(applied.Stdout, testCase.successContains) {
				t.Fatalf("applied = %#v, want %s", applied, testCase.successContains)
			}
			if call != 5 {
				t.Fatalf("request count = %d, want one dry-run and one single-attempt execution", call)
			}
		})
	}
}

func TestExecuteCohostLinkCreateFailsClosedWithoutLeakingExistingURL(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case "https://api.partiful.com/getEventInfo":
			event := compatibleUpdateEvent()
			event["ownerIds"] = []string{"private-account"}
			return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
		case "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/events/event-example/private/cohostSecret":
			return jsonResponse(http.StatusOK, cohostLinkDocument(`/e/event-example?accept-cohost=private-secret-token`)), nil
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	}}

	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"cohosts", "link", "create", "event-example"},
	}, dependencies)
	if result.ExitCode != 6 || !strings.Contains(result.Stdout, `"type":"state.conflict"`) {
		t.Fatalf("result = %#v, want link precondition failure", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, "private-secret-token") {
		t.Fatal("existing cohost link leaked in failure output")
	}
}

func TestExecuteCohostInviteAcceptsNullCompletion(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	cursor := "cursor-1"
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1:
			event := compatibleUpdateEvent()
			event["ownerIds"] = []string{"private-account"}
			return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
		case 2:
			return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{{
				"id": "private-contact-id", "name": "Alex Example", "sharedEventCount": 2,
			}}, &cursor)), nil
		case 3:
			return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
		case 4:
			return jsonResponse(http.StatusOK, `{}`), nil
		case 5:
			return jsonResponse(http.StatusOK, `{"result":null}`), nil
		default:
			return nil, errors.New("unexpected request")
		}
	}}
	argv := []string{"cohosts", "invite", "event-example", "--contact", "Alex Example"}

	applied := app.Execute(context.Background(), app.Request{
		Argv: argv,
	}, dependencies)
	if applied.ExitCode != 0 || !strings.Contains(applied.Stdout, `"status":"invited"`) {
		t.Fatalf("applied = %#v, want submitted invite", applied)
	}
	if call != 5 {
		t.Fatalf("request count = %d, want one mutation attempt", call)
	}
}

func TestExecuteCohostActionsFailClosedWhenOwnerIDsAreMissing(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{name: "contact action", argv: []string{"cohosts", "invite", "event-example", "--contact", "Alex Example"}},
		{name: "link action", argv: []string{"cohosts", "link", "create", "event-example"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			files := &memoryFilesystem{files: map[string][]byte{
				eventWriteCredentialsPath: []byte(eventWriteCredentials),
			}}
			dependencies := eventWriteDependencies(files)
			dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
				event := compatibleUpdateEvent()
				delete(event, "ownerIds")
				return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
			}}

			result := app.Execute(context.Background(), app.Request{Argv: testCase.argv}, dependencies)
			if result.ExitCode != 9 ||
				!strings.Contains(result.Stdout, `"type":"contract.protocol_changed"`) {
				t.Fatalf("result = %#v, want missing owner protocol failure", result)
			}
		})
	}
}
