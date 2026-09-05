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
	"github.com/KalebCole/partiful-cli/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestExecuteMCPUsesCanonicalNestedEventInput(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	requests := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Method != http.MethodGet || request.URL.String() != "https://assets.getpartiful.com/posters.json" {
			t.Fatalf("request = %s %s, want poster catalog only", request.Method, request.URL)
		}
		return jsonResponse(http.StatusOK, `[{"id":"Let's Party","url":"https://assets.getpartiful.com/posters/party","name":"Let's Party","tags":[],"categories":[],"contentType":"image/jpeg","height":1200,"width":800}]`), nil
	}}

	result := app.ExecuteMCP(context.Background(), "events_create", map[string]any{
		"title":       "Example event",
		"start":       "2026-09-12T19:00:00Z",
		"timezone":    "UTC",
		"description": nil,
		"guestLimit":  75,
		"links": []any{
			map[string]any{"label": "Tickets", "url": "https://example.test/tickets"},
		},
		"dryRun": true,
	}, dependencies)

	if result.ExitCode != 0 {
		t.Fatalf("result = %#v, want schema-valid create preview", result)
	}
	var envelope struct {
		Data struct {
			Input struct {
				Description *string `json:"description"`
				GuestLimit  int     `json:"guestLimit"`
				Links       []struct {
					Label string `json:"label"`
					URL   string `json:"url"`
				} `json:"links"`
			} `json:"input"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if envelope.Data.Input.Description != nil ||
		envelope.Data.Input.GuestLimit != 75 ||
		!reflect.DeepEqual(envelope.Data.Input.Links, []struct {
			Label string `json:"label"`
			URL   string `json:"url"`
		}{{Label: "Tickets", URL: "https://example.test/tickets"}}) {
		t.Fatalf("normalized input = %#v", envelope.Data.Input)
	}
	if requests != 1 || files.atomicWrites != 0 {
		t.Fatalf("requests = %d, credential writes = %d; want one preflight and no writes", requests, files.atomicWrites)
	}
}

func TestExecuteMCPAndCLIConvergeOnCanonicalEventOperation(t *testing.T) {
	dependencies := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{}})
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != "https://assets.getpartiful.com/posters.json" {
			t.Fatalf("request = %s %s, want poster catalog only", request.Method, request.URL)
		}
		return jsonResponse(http.StatusOK, `[{"id":"Let's Party","url":"https://assets.getpartiful.com/posters/party","name":"Let's Party","tags":[],"categories":[],"contentType":"image/jpeg","height":1200,"width":800}]`), nil
	}}
	input := `{"title":"Example event","start":"2026-09-12T19:00:00Z","timezone":"UTC","description":null,"guestLimit":75,"links":[{"label":"Tickets","url":"https://example.test/tickets"}]}`

	cli := app.Execute(context.Background(), app.Request{
		Argv:  []string{"events", "create", "--input", "-", "--dry-run"},
		Stdin: strings.NewReader(input),
	}, dependencies)
	mcp := app.ExecuteMCP(context.Background(), "events_create", map[string]any{
		"title":       "Example event",
		"start":       "2026-09-12T19:00:00Z",
		"timezone":    "UTC",
		"description": nil,
		"guestLimit":  75,
		"links": []any{
			map[string]any{"label": "Tickets", "url": "https://example.test/tickets"},
		},
		"dryRun": true,
	}, dependencies)

	var cliEnvelope, mcpEnvelope any
	if err := json.Unmarshal([]byte(cli.Stdout), &cliEnvelope); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if err := json.Unmarshal([]byte(mcp.Stdout), &mcpEnvelope); err != nil {
		t.Fatalf("decode MCP result: %v", err)
	}
	if !reflect.DeepEqual(mcpEnvelope, cliEnvelope) {
		t.Fatalf("MCP envelope = %#v, want CLI envelope %#v", mcpEnvelope, cliEnvelope)
	}
}

func TestCLIAndRealMCPServerReturnSemanticallyEquivalentResults(t *testing.T) {
	const (
		createInput  = `{"title":"Example event","start":"2026-09-12T19:00:00Z","timezone":"UTC","description":null,"guestLimit":75,"links":[{"label":"Tickets","url":"https://example.test/tickets"}]}`
		updateInput  = `{"description":null,"guestLimit":75,"links":[{"label":"Tickets","url":"https://example.test/tickets"}]}`
		cancelInput  = `{"message":"Cancelled","notifyGuests":false}`
		goingInput   = `{"status":"going","displayName":"Example Attendee","partySize":2,"plusOnes":["Guest One"],"message":null,"timezone":"UTC","questionnaireResponse":null}`
		declineInput = `{"status":"not-going","displayName":"Example Attendee","partySize":1,"plusOnes":[],"message":null,"timezone":"UTC","questionnaireResponse":null}`
	)
	tests := []struct {
		name            string
		tool            string
		cliRequest      func() app.Request
		mcpArguments    map[string]any
		newDependencies func(*testing.T) app.Dependencies
	}{
		{
			name: "public read",
			tool: "posters_list",
			cliRequest: func() app.Request {
				return app.Request{Argv: []string{"posters", "list", "--limit", "1"}}
			},
			mcpArguments:    map[string]any{"limit": 1},
			newDependencies: newMCPParityPosterDependencies,
		},
		{
			name: "protected read",
			tool: "contacts_list",
			cliRequest: func() app.Request {
				return app.Request{Argv: []string{"contacts", "list"}}
			},
			mcpArguments:    map[string]any{},
			newDependencies: newMCPParityContactDependencies,
		},
		{
			name: "standard nested write",
			tool: "events_create",
			cliRequest: func() app.Request {
				return app.Request{
					Argv:  []string{"events", "create", "--input", "-", "--dry-run"},
					Stdin: strings.NewReader(createInput),
				}
			},
			mcpArguments: map[string]any{
				"title": "Example event", "start": "2026-09-12T19:00:00Z", "timezone": "UTC",
				"description": nil, "guestLimit": 75,
				"links":  []any{map[string]any{"label": "Tickets", "url": "https://example.test/tickets"}},
				"dryRun": true,
			},
			newDependencies: newMCPParityEventCreateDependencies,
		},
		{
			name: "nullable update",
			tool: "events_update",
			cliRequest: func() app.Request {
				return app.Request{
					Argv:  []string{"events", "update", "event-example", "--input", "-", "--dry-run"},
					Stdin: strings.NewReader(updateInput),
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "description": nil, "guestLimit": 75,
				"links":  []any{map[string]any{"label": "Tickets", "url": "https://example.test/tickets"}},
				"dryRun": true,
			},
			newDependencies: newMCPParityEventUpdateDependencies,
		},
		{
			name: "destructive write",
			tool: "events_cancel",
			cliRequest: func() app.Request {
				return app.Request{
					Argv:  []string{"events", "cancel", "event-example", "--input", "-", "--dry-run"},
					Stdin: strings.NewReader(cancelInput),
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "message": "Cancelled", "notifyGuests": false, "dryRun": true,
			},
			newDependencies: newMCPParityEventCancelDependencies,
		},
		{
			name: "consequential human contact write",
			tool: "blasts_send",
			cliRequest: func() app.Request {
				return app.Request{
					Argv: []string{
						"blasts", "send", "event-example", "--audience", "all-guests",
						"--message-file", "-", "--show-on-event-page", "--dry-run",
					},
					Stdin: strings.NewReader("Hello"),
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "audience": "all-guests", "message": "Hello",
				"showOnEventPage": true, "dryRun": true,
			},
			newDependencies: newMCPParityBlastDependencies,
		},
		{
			name: "guest invite",
			tool: "guests_invite",
			cliRequest: func() app.Request {
				return app.Request{
					Argv: []string{
						"guests", "invite", "event-example",
						"--contact", "Example Contact", "--dry-run",
					},
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
			},
			newDependencies: newMCPParityGuestInviteDependencies,
		},
		{
			name: "cohost invite",
			tool: "cohosts_invite",
			cliRequest: func() app.Request {
				return app.Request{
					Argv: []string{
						"cohosts", "invite", "event-example",
						"--contact", "Example Contact", "--dry-run",
					},
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
			},
			newDependencies: newMCPParityCohostInviteDependencies,
		},
		{
			name: "cohost revoke invite",
			tool: "cohosts_revoke_invite",
			cliRequest: func() app.Request {
				return app.Request{
					Argv: []string{
						"cohosts", "revoke-invite", "event-example",
						"--contact", "Example Contact", "--dry-run",
					},
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
			},
			newDependencies: newMCPParityCohostRevokeInviteDependencies,
		},
		{
			name: "cohost remove",
			tool: "cohosts_remove",
			cliRequest: func() app.Request {
				return app.Request{
					Argv: []string{
						"cohosts", "remove", "event-example",
						"--contact", "Example Contact", "--dry-run",
					},
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
			},
			newDependencies: newMCPParityCohostRemoveDependencies,
		},
		{
			name: "cohost link create",
			tool: "cohosts_link_create",
			cliRequest: func() app.Request {
				return app.Request{
					Argv: []string{"cohosts", "link", "create", "event-example", "--dry-run"},
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "dryRun": true,
			},
			newDependencies: newMCPParityCohostLinkCreateDependencies,
		},
		{
			name: "cohost link revoke",
			tool: "cohosts_link_revoke",
			cliRequest: func() app.Request {
				return app.Request{
					Argv: []string{"cohosts", "link", "revoke", "event-example", "--dry-run"},
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "dryRun": true,
			},
			newDependencies: newMCPParityCohostLinkRevokeDependencies,
		},
		{
			name: "RSVP interested branch",
			tool: "rsvp_set",
			cliRequest: func() app.Request {
				return app.Request{
					Argv: []string{
						"rsvp", "set", "event-example", "--status", "interested", "--dry-run",
					},
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "status": "interested", "dryRun": true,
			},
			newDependencies: newMCPParityRSVPDependencies,
		},
		{
			name: "RSVP going branch",
			tool: "rsvp_set",
			cliRequest: func() app.Request {
				return app.Request{
					Argv:  []string{"rsvp", "set", "event-example", "--input", "-", "--dry-run"},
					Stdin: strings.NewReader(goingInput),
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "status": "going", "displayName": "Example Attendee",
				"partySize": 2, "plusOnes": []any{"Guest One"}, "message": nil, "timezone": "UTC",
				"questionnaireResponse": nil, "dryRun": true,
			},
			newDependencies: newMCPParityRSVPDependencies,
		},
		{
			name: "RSVP not-going branch",
			tool: "rsvp_set",
			cliRequest: func() app.Request {
				return app.Request{
					Argv:  []string{"rsvp", "set", "event-example", "--input", "-", "--dry-run"},
					Stdin: strings.NewReader(declineInput),
				}
			},
			mcpArguments: map[string]any{
				"eventId": "event-example", "status": "not-going", "displayName": "Example Attendee",
				"partySize": 1, "plusOnes": []any{}, "message": nil, "timezone": "UTC",
				"questionnaireResponse": nil, "dryRun": true,
			},
			newDependencies: newMCPParityRSVPDependencies,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cli := app.Execute(context.Background(), test.cliRequest(), test.newDependencies(t))
			if cli.ExitCode != 0 || cli.Stderr != "" {
				t.Fatalf("CLI result = %#v, want success", cli)
			}

			server, err := mcpserver.New(
				test.newDependencies(t),
				mcpserver.Options{RequestInterval: time.Nanosecond},
			)
			if err != nil {
				t.Fatalf("new MCP server: %v", err)
			}
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			if _, err := server.Connect(context.Background(), serverTransport, nil); err != nil {
				t.Fatalf("connect MCP server: %v", err)
			}
			client := mcp.NewClient(&mcp.Implementation{Name: "parity-test", Version: "1"}, nil)
			session, err := client.Connect(context.Background(), clientTransport, nil)
			if err != nil {
				t.Fatalf("connect MCP client: %v", err)
			}
			defer session.Close()

			callContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := session.CallTool(callContext, &mcp.CallToolParams{
				Name:      test.tool,
				Arguments: test.mcpArguments,
			})
			if err != nil || result == nil || result.IsError {
				t.Fatalf("MCP result = %#v, error = %v; want success", result, err)
			}

			var cliEnvelope any
			if err := json.Unmarshal([]byte(cli.Stdout), &cliEnvelope); err != nil {
				t.Fatalf("decode CLI result: %v", err)
			}
			if !reflect.DeepEqual(result.StructuredContent, cliEnvelope) {
				t.Fatalf("MCP structured result = %#v, want CLI envelope %#v", result.StructuredContent, cliEnvelope)
			}
		})
	}
}

func newMCPParityPosterDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	return withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `[
				{"id":"first","name":"First","url":"https://example.test/first","contentType":"image/png","width":1200,"height":800,"tags":["party"],"categories":["fun"]}
			]`), nil
		}},
	})
}

func newMCPParityContactDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	dependencies := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}})
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1:
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":[{"id":"private-contact-id","name":"Example Contact","sharedEventCount":2}],"paging":{"nextCursor":"private-cursor"}}}`,
			), nil
		case 2:
			return jsonResponse(http.StatusOK, `{"result":{"data":[],"paging":{}}}`), nil
		default:
			return nil, errors.New("unexpected request")
		}
	}}
	return dependencies
}

func newMCPParityEventCreateDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	dependencies := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{}})
	dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		return jsonResponse(
			http.StatusOK,
			`[{"id":"Let's Party","url":"https://assets.getpartiful.com/posters/party","name":"Let's Party","tags":[],"categories":[],"contentType":"image/jpeg","height":1200,"width":800}]`,
		), nil
	}}
	return dependencies
}

func newMCPParityEventUpdateDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	return newMCPParityEventMutationDependencies(t, compatibleUpdateEvent())
}

func newMCPParityEventCancelDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	return newMCPParityEventMutationDependencies(t, compatibleCancelEvent())
}

func newMCPParityEventMutationDependencies(t *testing.T, event map[string]any) app.Dependencies {
	t.Helper()
	dependencies := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}})
	dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
	}}
	return dependencies
}

func newMCPParityBlastDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	dependencies := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}})
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
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
			return jsonResponse(http.StatusOK, firestoreListResponse(t)), nil
		default:
			return nil, errors.New("unexpected request")
		}
	}}
	return dependencies
}

func newMCPParityGuestInviteDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	dependencies := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}})
	calls, contactCalls := 0, 0
	cursor := "contacts-cursor"
	eventBody := eventResponse(t, compatibleUpdateEvent())
	firstContactPage := contactPageResponse([]map[string]any{{
		"id":               "private-contact-id",
		"name":             "Example Contact",
		"sharedEventCount": 3,
	}}, &cursor)
	terminalContactPage := contactPageResponse([]map[string]any{}, nil)
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		calls++
		switch request.URL.String() {
		case "https://api.partiful.com/getEventInfo":
			if request.Method != http.MethodPost {
				return nil, errors.New("unexpected guest invite event method")
			}
			return jsonResponse(http.StatusOK, eventBody), nil
		case "https://api.partiful.com/getContacts":
			if request.Method != http.MethodPost {
				return nil, errors.New("unexpected guest invite contacts method")
			}
			contactCalls++
			switch contactCalls {
			case 1:
				return jsonResponse(http.StatusOK, firstContactPage), nil
			case 2:
				return jsonResponse(http.StatusOK, terminalContactPage), nil
			default:
				return nil, errors.New("unexpected guest invite contacts page")
			}
		default:
			return nil, errors.New("unexpected guest invite request")
		}
	}}
	t.Cleanup(func() {
		if calls != 3 {
			t.Errorf("guest invite requests = %d, want three preflight reads", calls)
		}
	})
	return dependencies
}

func newMCPParityCohostInviteDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	return newMCPParityCohostContactDependencies(t, "")
}

func newMCPParityCohostRevokeInviteDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	return newMCPParityCohostContactDependencies(t, "PENDING")
}

func newMCPParityCohostRemoveDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	return newMCPParityCohostContactDependencies(t, "ACCEPTED")
}

func newMCPParityCohostContactDependencies(t *testing.T, status string) app.Dependencies {
	t.Helper()
	dependencies := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}})
	calls, contactCalls := 0, 0
	cursor := "contacts-cursor"
	eventBody := eventResponse(t, compatibleUpdateEvent())
	firstContactPage := contactPageResponse([]map[string]any{{
		"id":               "private-contact-id",
		"name":             "Example Contact",
		"sharedEventCount": 3,
	}}, &cursor)
	terminalContactPage := contactPageResponse([]map[string]any{}, nil)
	cohostRequests := `{}`
	if status != "" {
		cohostRequests = `{"documents":[` + cohostRequestDocument("private-contact-id", status) + `]}`
	}
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		calls++
		switch request.URL.String() {
		case "https://api.partiful.com/getEventInfo":
			if request.Method != http.MethodPost {
				return nil, errors.New("unexpected cohost event method")
			}
			return jsonResponse(http.StatusOK, eventBody), nil
		case "https://api.partiful.com/getContacts":
			if request.Method != http.MethodPost {
				return nil, errors.New("unexpected cohost contacts method")
			}
			contactCalls++
			switch contactCalls {
			case 1:
				return jsonResponse(http.StatusOK, firstContactPage), nil
			case 2:
				return jsonResponse(http.StatusOK, terminalContactPage), nil
			default:
				return nil, errors.New("unexpected cohost contacts page")
			}
		case "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/events/event-example/cohostRequests?pageSize=100":
			if request.Method != http.MethodGet {
				return nil, errors.New("unexpected cohost request-list method")
			}
			return jsonResponse(http.StatusOK, cohostRequests), nil
		default:
			return nil, errors.New("unexpected cohost contact request")
		}
	}}
	t.Cleanup(func() {
		if calls != 4 {
			t.Errorf("cohost contact requests = %d, want four preflight reads", calls)
		}
	})
	return dependencies
}

func newMCPParityCohostLinkCreateDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	return newMCPParityCohostLinkDependencies(t, false)
}

func newMCPParityCohostLinkRevokeDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	return newMCPParityCohostLinkDependencies(t, true)
}

func newMCPParityCohostLinkDependencies(t *testing.T, linkPresent bool) app.Dependencies {
	t.Helper()
	dependencies := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}})
	calls := 0
	eventBody := eventResponse(t, compatibleUpdateEvent())
	linkBody := firestoreNotFound()
	linkStatus := http.StatusNotFound
	if linkPresent {
		linkBody = cohostLinkDocument("/e/event-example?accept-cohost=private-token")
		linkStatus = http.StatusOK
	}
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		calls++
		switch request.URL.String() {
		case "https://api.partiful.com/getEventInfo":
			if request.Method != http.MethodPost {
				return nil, errors.New("unexpected cohost link event method")
			}
			return jsonResponse(http.StatusOK, eventBody), nil
		case "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/events/event-example/private/cohostSecret":
			if request.Method != http.MethodGet {
				return nil, errors.New("unexpected cohost link-state method")
			}
			return jsonResponse(linkStatus, linkBody), nil
		default:
			return nil, errors.New("unexpected cohost link request")
		}
	}}
	t.Cleanup(func() {
		if calls != 2 {
			t.Errorf("cohost link requests = %d, want two preflight reads", calls)
		}
	})
	return dependencies
}

func newMCPParityRSVPDependencies(t *testing.T) app.Dependencies {
	t.Helper()
	dependencies := rsvpTestDependencies(&memoryFilesystem{files: map[string][]byte{
		rsvpCredentialsPath: []byte(rsvpCredentials),
	}})
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1:
			return jsonResponse(http.StatusOK, compatibleRSVPEventResponse), nil
		case 2:
			return jsonResponse(http.StatusOK, `{"result":{"data":{"currentGuest":null}}}`), nil
		default:
			return nil, errors.New("unexpected request")
		}
	}}
	return dependencies
}

func TestExecuteMCPAcceptsEveryRSVPUnionBranch(t *testing.T) {
	tests := map[string]map[string]any{
		"interested": {
			"eventId": "event-example",
			"status":  "interested",
			"dryRun":  true,
		},
		"going": {
			"eventId":     "event-example",
			"status":      "going",
			"displayName": "Example Attendee",
			"partySize":   2,
			"plusOnes":    []any{"Guest One"},
			"message":     nil,
			"timezone":    "UTC",
			"questionnaireResponse": map[string]any{
				"questionnaireVersion": 0,
				"answers":              map[string]any{"question-example": "Answer"},
			},
			"dryRun": true,
		},
		"not-going": {
			"eventId":               "event-example",
			"status":                "not-going",
			"displayName":           "Example Attendee",
			"partySize":             1,
			"plusOnes":              []any{},
			"message":               nil,
			"timezone":              "UTC",
			"questionnaireResponse": nil,
			"dryRun":                true,
		},
	}

	for name, arguments := range tests {
		t.Run(name, func(t *testing.T) {
			httpCalled := false
			result := app.ExecuteMCP(context.Background(), "rsvp_set", arguments, app.Dependencies{
				Files:           &memoryFilesystem{files: map[string][]byte{}},
				CredentialsPath: eventWriteCredentialsPath,
				HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
					httpCalled = true
					return nil, nil
				}},
			})
			if result.ExitCode != 3 || !strings.Contains(result.Stdout, `"type":"auth.required"`) || httpCalled {
				t.Fatalf("result = %#v, HTTP called = %t; want canonical input to reach authentication", result, httpCalled)
			}
		})
	}
}

func TestExecuteMCPRejectsUndocumentedInputWrapper(t *testing.T) {
	httpCalled := false
	result := app.ExecuteMCP(context.Background(), "rsvp_set", map[string]any{
		"eventId": "event-example",
		"input":   map[string]any{"status": "interested"},
		"dryRun":  true,
	}, app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			httpCalled = true
			return nil, nil
		}},
	})
	if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"type":"input.invalid"`) || httpCalled {
		t.Fatalf("result = %#v, HTTP called = %t; want pre-dispatch input rejection", result, httpCalled)
	}
}

func TestExecuteMCPRejectsCollectionInputsOutsideAdvertisedSchema(t *testing.T) {
	tests := []map[string]any{
		{"when": "upcoming", "all": false},
		{"when": "upcoming", "all": true, "maxItems": 10, "limit": 1},
	}
	for _, arguments := range tests {
		result := app.ExecuteMCP(context.Background(), "events_list", arguments, app.Dependencies{})
		if result.ExitCode != 2 {
			t.Fatalf("arguments %#v exit code = %d, want 2", arguments, result.ExitCode)
		}
		if !strings.Contains(result.Stdout, `"code":"MCP_ARGUMENTS_INVALID"`) {
			t.Fatalf("arguments %#v result = %#v", arguments, result)
		}
	}
}

func TestExecuteMCPAppliesItemBoundWithTruthfulContinuation(t *testing.T) {
	dependencies := withTestCursorCrypto(app.Dependencies{
		HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `[
				{"id":"first","name":"First","url":"https://example.test/first","contentType":"image/png","width":1200,"height":800,"tags":[],"categories":[]},
				{"id":"second","name":"Second","url":"https://example.test/second","contentType":"image/png","width":1200,"height":800,"tags":[],"categories":[]},
				{"id":"third","name":"Third","url":"https://example.test/third","contentType":"image/png","width":1200,"height":800,"tags":[],"categories":[]}
			]`), nil
		}},
	})

	result := app.ExecuteMCP(
		context.Background(),
		"posters_list",
		map[string]any{"limit": 3},
		dependencies,
		app.MCPExecutionOptions{MaxItems: 1},
	)
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v, want bounded success", result)
	}
	var envelope struct {
		Data struct {
			Items []any `json:"items"`
		} `json:"data"`
		Meta struct {
			Page struct {
				Limit            int     `json:"limit"`
				NextCursor       *string `json:"nextCursor"`
				HasMore          bool    `json:"hasMore"`
				Truncated        bool    `json:"truncated"`
				TruncationReason string  `json:"truncationReason"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(envelope.Data.Items) != 1 ||
		envelope.Meta.Page.Limit != 1 ||
		!envelope.Meta.Page.HasMore ||
		envelope.Meta.Page.NextCursor == nil ||
		!envelope.Meta.Page.Truncated ||
		envelope.Meta.Page.TruncationReason != "server_item_limit" {
		t.Fatalf("bounded envelope = %#v", envelope)
	}
}

func TestExecuteMCPDestructiveMutationNeverPromptsAndDispatchesOnce(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	confirmer := &recordingConfirmer{terminal: true, answer: false}
	dependencies.Confirmer = confirmer
	requests := 0
	dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			return jsonResponse(http.StatusOK, eventResponse(t, compatibleCancelEvent())), nil
		case 2:
			return nil, errors.New("ambiguous transport failure")
		default:
			t.Fatalf("unexpected retry %d", requests)
			return nil, nil
		}
	}}

	result := app.ExecuteMCP(context.Background(), "events_cancel", map[string]any{
		"eventId": "event-example", "message": "Cancelled", "notifyGuests": false,
	}, dependencies)

	if result.ExitCode != 8 || !strings.Contains(result.Stdout, `"code":"EVENT_SUBMISSION_UNCERTAIN"`) {
		t.Fatalf("result = %#v, want ambiguous one-attempt mutation failure", result)
	}
	if confirmer.calls != 0 {
		t.Fatalf("confirmation calls = %d, want zero", confirmer.calls)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want one preflight and one mutation attempt", requests)
	}
	if files.atomicWrites != 0 {
		t.Fatalf("credential writes = %d, want zero", files.atomicWrites)
	}
}

func TestRealMCPServerCredentialRefreshPersistencePolicy(t *testing.T) {
	const (
		currentAccount = "private-current-account"
		newAccessToken = "e30." +
			"eyJzdWIiOiJwcml2YXRlLWN1cnJlbnQtYWNjb3VudCJ9." +
			"private-signature"
		expiredCredentials = `{"accessToken":"private-expired-access","refreshToken":"private-refresh-token","expiresAt":"2026-08-11T23:59:00Z"}`
	)
	tests := []struct {
		name       string
		options    mcpserver.Options
		wantWrites int
	}{
		{
			name:       "read-only server refreshes in memory",
			options:    mcpserver.Options{ReadOnly: true},
			wantWrites: 0,
		},
		{
			name:       "default server persists refresh",
			options:    mcpserver.Options{},
			wantWrites: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := &memoryFilesystem{files: map[string][]byte{
				eventWriteCredentialsPath: []byte(expiredCredentials),
			}}
			dependencies := eventWriteDependencies(files)
			requests := 0
			dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				requests++
				switch requests {
				case 1:
					if request.URL.Host != "securetoken.googleapis.com" {
						t.Fatalf("first request host = %q, want refresh", request.URL.Host)
					}
					return jsonResponse(
						http.StatusOK,
						`{"access_token":"private-access-alias","id_token":"`+newAccessToken+`","refresh_token":"private-new-refresh","expires_in":"3600","token_type":"Bearer"}`,
					), nil
				case 2:
					if request.URL.String() != "https://api.partiful.com/getMyUpcomingEventsForHomePage" {
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
					t.Fatalf("unexpected request %d: %s", requests, request.URL)
					return nil, nil
				}
			}}
			test.options.RequestInterval = time.Nanosecond
			server, err := mcpserver.New(dependencies, test.options)
			if err != nil {
				t.Fatalf("new MCP server: %v", err)
			}
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			if _, err := server.Connect(context.Background(), serverTransport, nil); err != nil {
				t.Fatalf("connect MCP server: %v", err)
			}
			client := mcp.NewClient(&mcp.Implementation{Name: "credential-policy-test", Version: "1"}, nil)
			session, err := client.Connect(context.Background(), clientTransport, nil)
			if err != nil {
				t.Fatalf("connect MCP client: %v", err)
			}
			t.Cleanup(func() { _ = session.Close() })

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "events_list",
				Arguments: map[string]any{"when": "upcoming"},
			})
			if err != nil || result == nil || result.IsError {
				t.Fatalf("result = %#v, error = %v; want refreshed protected read", result, err)
			}
			if requests != 2 || files.atomicWrites != test.wantWrites {
				t.Fatalf(
					"requests = %d, credential writes = %d; want two requests and %d writes",
					requests,
					files.atomicWrites,
					test.wantWrites,
				)
			}
		})
	}
}

func TestExecuteMCPMutationHonorsCredentialPersistencePolicy(t *testing.T) {
	const newAccessToken = "e30." +
		"eyJzdWIiOiJwcml2YXRlLWFjY291bnQifQ." +
		"private-signature"
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(`{"accessToken":"private-expired-access","refreshToken":"private-refresh-token","expiresAt":"2026-08-11T23:59:00Z"}`),
	}}
	dependencies := eventWriteDependencies(files)
	requests := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			return jsonResponse(
				http.StatusOK,
				`{"access_token":"private-access-alias","id_token":"`+newAccessToken+`","refresh_token":"private-new-refresh","expires_in":"3600","token_type":"Bearer"}`,
			), nil
		case 2:
			return jsonResponse(http.StatusOK, eventResponse(t, compatibleCancelEvent())), nil
		case 3:
			return jsonResponse(http.StatusOK, `{"result":true}`), nil
		default:
			t.Fatalf("unexpected request %d: %s", requests, request.URL)
			return nil, nil
		}
	}}

	result := app.ExecuteMCP(
		context.Background(),
		"events_cancel",
		map[string]any{
			"eventId": "event-example", "message": "Cancelled", "notifyGuests": false,
		},
		dependencies,
		app.MCPExecutionOptions{DisableCredentialPersistence: true},
	)

	if result.ExitCode != 0 || requests != 3 {
		t.Fatalf("result = %#v, requests = %d; want refreshed single-attempt mutation", result, requests)
	}
	if files.atomicWrites != 0 {
		t.Fatalf("credential writes = %d, want zero", files.atomicWrites)
	}
}

func TestExecuteMCPDryRunRefreshesWithoutPersistingOrMutating(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		rsvpCredentialsPath: []byte(`{"accessToken":"private-expired-access","refreshToken":"private-refresh-token","expiresAt":"2026-08-11T23:59:00Z"}`),
	}}
	dependencies := rsvpTestDependencies(files)
	requests := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			return jsonResponse(http.StatusOK, `{"access_token":"private-access-alias","id_token":"private-new-access","refresh_token":"private-new-refresh","expires_in":"3600","token_type":"Bearer"}`), nil
		case 2:
			return jsonResponse(http.StatusOK, compatibleRSVPEventResponse), nil
		case 3:
			return jsonResponse(http.StatusOK, `{"result":{"data":{"currentGuest":null}}}`), nil
		default:
			t.Fatalf("unexpected target mutation request %d: %s", requests, request.URL)
			return nil, nil
		}
	}}

	result := app.ExecuteMCP(context.Background(), "rsvp_set", map[string]any{
		"eventId": "event-example", "status": "interested", "dryRun": true,
	}, dependencies)

	if result.ExitCode != 0 || requests != 3 {
		t.Fatalf("result = %#v, requests = %d; want refresh and preflight-only success", result, requests)
	}
	if files.atomicWrites != 0 {
		t.Fatalf("credential writes = %d, want zero", files.atomicWrites)
	}
	for _, privateValue := range []string{
		"private-refresh-token",
		"private-new-refresh",
		"private-new-access",
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("result exposed private value %q", privateValue)
		}
	}
}

func TestExecuteMCPContactProjectionDoesNotExposePrivateFields(t *testing.T) {
	const (
		privateID     = "private-contact-marker"
		privatePhone  = "private-phone-marker"
		privateEmail  = "private-email-marker"
		privateCursor = "private-cursor-marker"
	)
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	requests := 0
	dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return jsonResponse(http.StatusOK,
				`{"result":{"data":[{"id":"`+privateID+`","name":"Example Contact","sharedEventCount":2,"phoneNumber":"`+privatePhone+`","email":"`+privateEmail+`"}],"paging":{"nextCursor":"`+privateCursor+`"}}}`,
			), nil
		}
		return jsonResponse(http.StatusOK, `{"result":{"data":[],"paging":{}}}`), nil
	}}

	result := app.ExecuteMCP(context.Background(), "contacts_list", map[string]any{}, dependencies)

	if result.ExitCode != 0 || !strings.Contains(result.Stdout, `"displayName":"Example Contact"`) {
		t.Fatalf("result = %#v, want public contact projection", result)
	}
	for _, privateValue := range []string{
		privateID,
		privatePhone,
		privateEmail,
		privateCursor,
		"private-access-token",
		"private-refresh-token",
		"private-account",
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("result exposed private value %q", privateValue)
		}
	}
}

func TestExecuteMCPDecodesEveryAdvertisedOperationBeforeRemoteAccess(t *testing.T) {
	validInputs := map[string]map[string]any{
		"blasts_send": {
			"eventId": "event-example", "audience": "all-guests", "message": "Hello", "dryRun": true,
		},
		"cohosts_invite": {
			"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
		},
		"cohosts_link_create": {
			"eventId": "event-example", "dryRun": true,
		},
		"cohosts_link_revoke": {
			"eventId": "event-example", "dryRun": true,
		},
		"cohosts_remove": {
			"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
		},
		"cohosts_revoke_invite": {
			"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
		},
		"contacts_list": {},
		"events_cancel": {
			"eventId": "event-example", "message": "", "notifyGuests": true, "dryRun": true,
		},
		"events_create": {
			"title": "Example", "start": "2026-09-12T19:00:00Z", "timezone": "UTC", "dryRun": true,
		},
		"events_get":  {"eventId": "event-example"},
		"events_list": {"when": "upcoming"},
		"events_update": {
			"eventId": "event-example", "description": nil, "dryRun": true,
		},
		"guests_invite": {
			"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
		},
		"guests_list":    {"eventId": "event-example"},
		"posters_list":   {},
		"posters_search": {"query": "party"},
		"rsvp_get":       {"eventId": "event-example"},
		"rsvp_set": {
			"eventId": "event-example", "status": "interested", "dryRun": true,
		},
	}
	for name, arguments := range validInputs {
		t.Run(name, func(t *testing.T) {
			httpCalled := false
			result := app.ExecuteMCP(context.Background(), name, arguments, app.Dependencies{
				Files:           &memoryFilesystem{files: map[string][]byte{}},
				CredentialsPath: eventWriteCredentialsPath,
				HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
					httpCalled = true
					return nil, errors.New("offline")
				}},
			})
			if result.ExitCode == 2 {
				t.Fatalf("schema-valid input failed before operation dispatch: %#v", result)
			}
			if (name == "posters_list" || name == "posters_search" || name == "events_create") != httpCalled {
				t.Fatalf("HTTP called = %t for %s", httpCalled, name)
			}
		})
	}
}
