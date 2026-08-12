package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

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

func TestExecuteCohostInvitePlansAndAppliesConfirmedAction(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("h", 32))
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
	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 || plan.Stderr != "" {
		t.Fatalf("plan = %#v, want cohost invite plan", plan)
	}
	if !strings.Contains(plan.Stdout, `"operation":"createCohostRequest"`) ||
		!strings.Contains(plan.Stdout, `"displayName":"Example Contact"`) ||
		!strings.Contains(plan.Stdout, `"cohostState":"bound"`) {
		t.Fatalf("plan stdout = %s, want redacted cohost invite plan", plan.Stdout)
	}
	token := rsvpPlanToken(t, plan)
	applied := app.Execute(context.Background(), app.Request{
		Argv: append(append([]string{}, argv...), "--apply", "--confirm", token),
	}, dependencies)
	if applied.ExitCode != 0 ||
		!strings.Contains(applied.Stdout, `"status":"invited"`) ||
		applied.Stderr != "" {
		t.Fatalf("applied = %#v, want invite success", applied)
	}
	for _, privateValue := range []string{"private-contact-id", "private-account", "private-access-token"} {
		if strings.Contains(plan.Stdout+applied.Stdout+plan.Stderr+applied.Stderr, privateValue) {
			t.Fatalf("output exposed private value %q", privateValue)
		}
	}
	reused := app.Execute(context.Background(), app.Request{
		Argv: append(append([]string{}, argv...), "--apply", "--confirm", token),
	}, dependencies)
	if reused.ExitCode != 7 || !strings.Contains(reused.Stdout, `"type":"safety.plan_stale"`) {
		t.Fatalf("reused = %#v, want stale consumed plan", reused)
	}
}

func TestExecuteCohostRevokeInviteAndRemoveApplyExactReviewedOperations(t *testing.T) {
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
			dependencies.MutationRandom = strings.NewReader(strings.Repeat("r", 32))
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

			plan := app.Execute(context.Background(), app.Request{Argv: testCase.argv}, dependencies)
			if plan.ExitCode != 0 || !strings.Contains(plan.Stdout, `"operation":"`+testCase.operation+`"`) {
				t.Fatalf("plan = %#v, want %s plan", plan, testCase.operation)
			}
			token := rsvpPlanToken(t, plan)
			applied := app.Execute(context.Background(), app.Request{
				Argv: append(append([]string{}, testCase.argv...), "--apply", "--confirm", token),
			}, dependencies)
			if applied.ExitCode != 0 || !strings.Contains(applied.Stdout, `"status":"`+testCase.successStatus+`"`) {
				t.Fatalf("applied = %#v, want %s success", applied, testCase.successStatus)
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
			dependencies.MutationRandom = strings.NewReader(strings.Repeat("l", 32))
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

			plan := app.Execute(context.Background(), app.Request{Argv: testCase.argv}, dependencies)
			if plan.ExitCode != 0 || !strings.Contains(plan.Stdout, `"operation":"`+testCase.operation+`"`) {
				t.Fatalf("plan = %#v, want %s plan", plan, testCase.operation)
			}
			token := rsvpPlanToken(t, plan)
			applied := app.Execute(context.Background(), app.Request{
				Argv: append(append([]string{}, testCase.argv...), "--apply", "--confirm", token),
			}, dependencies)
			if applied.ExitCode != 0 || !strings.Contains(applied.Stdout, testCase.successContains) {
				t.Fatalf("applied = %#v, want %s", applied, testCase.successContains)
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
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("i", 32))
	cursor := "cursor-1"
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 5:
			event := compatibleUpdateEvent()
			event["ownerIds"] = []string{"private-account"}
			return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
		case 2, 6:
			return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{{
				"id": "private-contact-id", "name": "Alex Example", "sharedEventCount": 2,
			}}, &cursor)), nil
		case 3, 7:
			return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
		case 4, 8:
			return jsonResponse(http.StatusOK, `{}`), nil
		case 9:
			return jsonResponse(http.StatusOK, `{"result":null}`), nil
		default:
			return nil, errors.New("unexpected request")
		}
	}}
	argv := []string{"cohosts", "invite", "event-example", "--contact", "Alex Example"}

	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 {
		t.Fatalf("plan = %#v", plan)
	}
	token := rsvpPlanToken(t, plan)
	applied := app.Execute(context.Background(), app.Request{
		Argv: append(append([]string{}, argv...), "--apply", "--confirm", token),
	}, dependencies)
	if applied.ExitCode != 0 || !strings.Contains(applied.Stdout, `"status":"invited"`) {
		t.Fatalf("applied = %#v, want submitted invite", applied)
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

func TestExecuteCohostPlansStaleOnChangedRoleContactLinkAccountAndExpiry(t *testing.T) {
	t.Run("changed role", func(t *testing.T) {
		files := &memoryFilesystem{files: map[string][]byte{
			eventWriteCredentialsPath: []byte(eventWriteCredentials),
		}}
		dependencies := eventWriteDependencies(files)
		dependencies.MutationRandom = strings.NewReader(strings.Repeat("s", 32))
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
					"id":               "private-contact-id",
					"name":             "Example Contact",
					"sharedEventCount": 3,
				}}, &cursor)), nil
			case 3:
				return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
			case 4:
				return jsonResponse(http.StatusOK, `{}`), nil
			case 5:
				event := compatibleUpdateEvent()
				event["ownerIds"] = []string{"different-host"}
				return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
			case 6:
				return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{{
					"id":               "private-contact-id",
					"name":             "Example Contact",
					"sharedEventCount": 3,
				}}, &cursor)), nil
			case 7:
				return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
			case 8:
				return jsonResponse(http.StatusOK, `{}`), nil
			default:
				t.Fatalf("unexpected request %d", call)
				return nil, nil
			}
		}}
		argv := []string{"cohosts", "invite", "event-example", "--contact", "Example Contact"}
		plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
		token := rsvpPlanToken(t, plan)
		applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", token)}, dependencies)
		if applied.ExitCode != 7 || !strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) {
			t.Fatalf("applied = %#v, want stale plan on role change", applied)
		}
	})

	t.Run("changed contact", func(t *testing.T) {
		files := &memoryFilesystem{files: map[string][]byte{
			eventWriteCredentialsPath: []byte(eventWriteCredentials),
		}}
		dependencies := eventWriteDependencies(files)
		dependencies.MutationRandom = strings.NewReader(strings.Repeat("c", 32))
		cursor := "cursor-1"
		call := 0
		dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1, 5:
				event := compatibleUpdateEvent()
				event["ownerIds"] = []string{"private-account"}
				return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
			case 2:
				return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{{
					"id":               "private-contact-id",
					"name":             "Example Contact",
					"sharedEventCount": 3,
				}}, &cursor)), nil
			case 3:
				return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
			case 4, 8:
				return jsonResponse(http.StatusOK, `{}`), nil
			case 6:
				return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{{
					"id":               "other-contact-id",
					"name":             "Example Contact",
					"sharedEventCount": 3,
				}}, &cursor)), nil
			case 7:
				return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
			default:
				t.Fatalf("unexpected request %d", call)
				return nil, nil
			}
		}}
		argv := []string{"cohosts", "invite", "event-example", "--contact", "Example Contact"}
		plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
		token := rsvpPlanToken(t, plan)
		applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", token)}, dependencies)
		if applied.ExitCode != 7 || !strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) {
			t.Fatalf("applied = %#v, want stale plan on contact change", applied)
		}
	})

	t.Run("changed link state", func(t *testing.T) {
		files := &memoryFilesystem{files: map[string][]byte{
			eventWriteCredentialsPath: []byte(eventWriteCredentials),
		}}
		dependencies := eventWriteDependencies(files)
		dependencies.MutationRandom = strings.NewReader(strings.Repeat("k", 32))
		call := 0
		dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1, 3:
				event := compatibleUpdateEvent()
				event["ownerIds"] = []string{"private-account"}
				return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
			case 2:
				return jsonResponse(http.StatusNotFound, firestoreNotFound()), nil
			case 4:
				return jsonResponse(http.StatusOK, cohostLinkDocument(`/e/event-example?accept-cohost=unexpected`)), nil
			default:
				t.Fatalf("unexpected request %d", call)
				return nil, nil
			}
		}}
		argv := []string{"cohosts", "link", "create", "event-example"}
		plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
		token := rsvpPlanToken(t, plan)
		applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", token)}, dependencies)
		if applied.ExitCode != 7 || !strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) {
			t.Fatalf("applied = %#v, want stale plan on link change", applied)
		}
	})

	t.Run("changed account", func(t *testing.T) {
		files := &memoryFilesystem{files: map[string][]byte{
			eventWriteCredentialsPath: []byte(eventWriteCredentials),
		}}
		dependencies := eventWriteDependencies(files)
		dependencies.MutationRandom = strings.NewReader(strings.Repeat("a", 32))
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
					"id":               "private-contact-id",
					"name":             "Example Contact",
					"sharedEventCount": 3,
				}}, &cursor)), nil
			case 3:
				return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
			case 4:
				return jsonResponse(http.StatusOK, `{}`), nil
			default:
				t.Fatalf("unexpected request %d", call)
				return nil, nil
			}
		}}
		argv := []string{"cohosts", "invite", "event-example", "--contact", "Example Contact"}
		plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
		files.files[eventWriteCredentialsPath] = []byte(`{"accessToken":"private-access-token","userId":"other-account","expiresAt":"2026-08-12T02:00:00Z"}`)
		token := rsvpPlanToken(t, plan)
		applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", token)}, dependencies)
		if applied.ExitCode != 7 || !strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) {
			t.Fatalf("applied = %#v, want stale plan on account change", applied)
		}
		if call != 4 {
			t.Fatalf("HTTP calls = %d, want no apply reads after account mismatch", call)
		}
	})

	t.Run("expired plan", func(t *testing.T) {
		files := &memoryFilesystem{files: map[string][]byte{
			eventWriteCredentialsPath: []byte(eventWriteCredentials),
		}}
		dependencies := eventWriteDependencies(files)
		dependencies.MutationRandom = strings.NewReader(strings.Repeat("e", 32))
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
					"id":               "private-contact-id",
					"name":             "Example Contact",
					"sharedEventCount": 3,
				}}, &cursor)), nil
			case 3:
				return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
			case 4:
				return jsonResponse(http.StatusOK, `{}`), nil
			default:
				t.Fatalf("unexpected request %d", call)
				return nil, nil
			}
		}}
		argv := []string{"cohosts", "invite", "event-example", "--contact", "Example Contact"}
		plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
		dependencies.Now = func() time.Time {
			return time.Date(2026, time.August, 12, 0, 6, 0, 0, time.UTC)
		}
		token := rsvpPlanToken(t, plan)
		applied := app.Execute(context.Background(), app.Request{Argv: append(append([]string{}, argv...), "--apply", "--confirm", token)}, dependencies)
		if applied.ExitCode != 7 || !strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) {
			t.Fatalf("applied = %#v, want stale expired plan", applied)
		}
	})
}
