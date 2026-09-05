package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestCLIAndMCPAdaptersProduceSameTypedInvocationForEveryTool(t *testing.T) {
	const (
		eventCreateInput = `{"title":" Example event ","start":"2026-09-12T19:00:00Z","timezone":"UTC","description":null,"guestLimit":75,"links":[{"label":"Tickets","url":"https://example.test/tickets"}]}`
		eventUpdateInput = `{"description":null,"guestLimit":75,"links":[{"label":"Tickets","url":"https://example.test/tickets"}]}`
		eventCancelInput = `{"message":"Cancelled","notifyGuests":false}`
		rsvpInput        = `{"status":"going","displayName":" Example Attendee ","partySize":2,"plusOnes":[" Guest One "],"message":null,"timezone":"UTC","questionnaireResponse":{"questionnaireVersion":0,"answers":{"question-example":"Answer"}}}`
	)
	tests := []struct {
		tool      string
		path      string
		argv      []string
		stdin     string
		arguments map[string]any
		dryRun    bool
	}{
		{
			tool: "posters_list", path: "posters.list",
			argv: []string{"posters", "list", "--limit", "7"}, arguments: map[string]any{"limit": 7},
		},
		{
			tool: "posters_search", path: "posters.search",
			argv:      []string{"posters", "search", "--query", " Party ", "--limit", "7"},
			arguments: map[string]any{"query": " Party ", "limit": 7},
		},
		{
			tool: "contacts_list", path: "contacts.list",
			argv:      []string{"contacts", "list", "--query", " Example "},
			arguments: map[string]any{"query": " Example "},
		},
		{
			tool: "guests_list", path: "guests.list",
			argv:      []string{"guests", "list", "event-example", "--limit", "7"},
			arguments: map[string]any{"eventId": "event-example", "limit": 7},
		},
		{
			tool: "guests_invite", path: "guests.invite",
			argv:      []string{"guests", "invite", "event-example", "--contact", " Example Contact "},
			arguments: map[string]any{"eventId": "event-example", "contact": " Example Contact ", "dryRun": true},
			dryRun:    true,
		},
		{
			tool: "events_list", path: "events.list",
			argv:      []string{"events", "list", "--when", "UPCOMING", "--limit", "7"},
			arguments: map[string]any{"when": "UPCOMING", "limit": 7},
		},
		{
			tool: "events_get", path: "events.get",
			argv:      []string{"events", "get", "event-example"},
			arguments: map[string]any{"eventId": "event-example"},
		},
		{
			tool: "events_create", path: "events.create",
			argv: []string{"events", "create", "--input", "-"}, stdin: eventCreateInput,
			arguments: map[string]any{
				"title": " Example event ", "start": "2026-09-12T19:00:00Z", "timezone": "UTC",
				"description": nil, "guestLimit": 75,
				"links":  []any{map[string]any{"label": "Tickets", "url": "https://example.test/tickets"}},
				"dryRun": true,
			},
			dryRun: true,
		},
		{
			tool: "events_update", path: "events.update",
			argv: []string{"events", "update", "event-example", "--input", "-"}, stdin: eventUpdateInput,
			arguments: map[string]any{
				"eventId": "event-example", "description": nil, "guestLimit": 75,
				"links":  []any{map[string]any{"label": "Tickets", "url": "https://example.test/tickets"}},
				"dryRun": true,
			},
			dryRun: true,
		},
		{
			tool: "events_cancel", path: "events.cancel",
			argv: []string{"events", "cancel", "event-example", "--input", "-"}, stdin: eventCancelInput,
			arguments: map[string]any{
				"eventId": "event-example", "message": "Cancelled", "notifyGuests": false, "dryRun": true,
			},
			dryRun: true,
		},
		{
			tool: "blasts_send", path: "blasts.send",
			argv: []string{
				"blasts", "send", "event-example", "--audience", "all-guests",
				"--message-file", "-", "--show-on-event-page",
			},
			stdin: "Hello",
			arguments: map[string]any{
				"eventId": "event-example", "audience": "all-guests", "message": "Hello",
				"showOnEventPage": true, "dryRun": true,
			},
			dryRun: true,
		},
		{
			tool: "rsvp_get", path: "rsvp.get",
			argv:      []string{"rsvp", "get", "event-example"},
			arguments: map[string]any{"eventId": "event-example"},
		},
		{
			tool: "rsvp_set", path: "rsvp.set",
			argv: []string{"rsvp", "set", "event-example", "--input", "-"}, stdin: rsvpInput,
			arguments: map[string]any{
				"eventId": "event-example", "status": "going", "displayName": " Example Attendee ",
				"partySize": 2, "plusOnes": []any{" Guest One "}, "message": nil, "timezone": "UTC",
				"questionnaireResponse": map[string]any{
					"questionnaireVersion": 0,
					"answers":              map[string]any{"question-example": "Answer"},
				},
				"dryRun": true,
			},
			dryRun: true,
		},
		{
			tool: "cohosts_invite", path: "cohosts.invite",
			argv:      []string{"cohosts", "invite", "event-example", "--contact", " Example Contact "},
			arguments: map[string]any{"eventId": "event-example", "contact": " Example Contact ", "dryRun": true},
			dryRun:    true,
		},
		{
			tool: "cohosts_revoke_invite", path: "cohosts.revoke-invite",
			argv:      []string{"cohosts", "revoke-invite", "event-example", "--contact", " Example Contact "},
			arguments: map[string]any{"eventId": "event-example", "contact": " Example Contact ", "dryRun": true},
			dryRun:    true,
		},
		{
			tool: "cohosts_remove", path: "cohosts.remove",
			argv:      []string{"cohosts", "remove", "event-example", "--contact", " Example Contact "},
			arguments: map[string]any{"eventId": "event-example", "contact": " Example Contact ", "dryRun": true},
			dryRun:    true,
		},
		{
			tool: "cohosts_link_create", path: "cohosts.link.create",
			argv:      []string{"cohosts", "link", "create", "event-example"},
			arguments: map[string]any{"eventId": "event-example", "dryRun": true},
			dryRun:    true,
		},
		{
			tool: "cohosts_link_revoke", path: "cohosts.link.revoke",
			argv:      []string{"cohosts", "link", "revoke", "event-example"},
			arguments: map[string]any{"eventId": "event-example", "dryRun": true},
			dryRun:    true,
		},
	}
	if len(tests) != 18 {
		t.Fatalf("test cases = %d, want all 18 exposed tools", len(tests))
	}

	for _, test := range tests {
		t.Run(test.tool, func(t *testing.T) {
			definition, ok := mcpCommandDefinition(test.tool)
			if !ok {
				t.Fatalf("tool %q is not exposed", test.tool)
			}
			if definition.path != test.path {
				t.Fatalf("tool %q path = %q, want %q", test.tool, definition.path, test.path)
			}

			cliInvocation, cliError := parseCLIProductInvocation(
				Request{Argv: test.argv, Stdin: strings.NewReader(test.stdin)},
				definition,
				test.argv,
				Dependencies{},
				mutationExecution{DryRun: test.dryRun},
			)
			if cliError != nil {
				t.Fatalf("parse CLI invocation: %#v", cliError)
			}
			document, documentError := mcpArgumentDocument(test.arguments)
			if documentError != nil {
				t.Fatalf("decode MCP arguments: %#v", documentError)
			}
			mcpInvocation, mcpError := parseMCPProductInvocation(definition, document, MCPExecutionOptions{})
			if mcpError != nil {
				t.Fatalf("parse MCP invocation: %#v", mcpError)
			}
			if !reflect.DeepEqual(mcpInvocation, cliInvocation) {
				t.Fatalf("MCP invocation = %#v, want CLI invocation %#v", mcpInvocation, cliInvocation)
			}
		})
	}
}
