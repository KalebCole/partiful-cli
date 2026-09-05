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

func TestMutationSchemaUsesDryRunFlagsWithoutPlanLifecycle(t *testing.T) {
	commands := map[string]bool{
		"blasts.send":           false,
		"cohosts.invite":        false,
		"cohosts.link.create":   false,
		"cohosts.link.revoke":   true,
		"cohosts.remove":        true,
		"cohosts.revoke-invite": true,
		"events.cancel":         true,
		"events.create":         false,
		"events.update":         false,
		"guests.invite":         false,
		"rsvp.set":              false,
	}
	for command, destructive := range commands {
		t.Run(command, func(t *testing.T) {
			result := app.Execute(context.Background(), app.Request{Argv: []string{"schema", command}}, app.Dependencies{})
			if result.ExitCode != 0 {
				t.Fatalf("schema result = %#v", result)
			}
			var envelope struct {
				Data struct {
					Flags []struct {
						Name string `json:"name"`
					} `json:"flags"`
					Safety map[string]any `json:"safety"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
				t.Fatal(err)
			}
			flags := make(map[string]bool)
			for _, flag := range envelope.Data.Flags {
				flags[flag.Name] = true
			}
			for _, want := range []string{"--dry-run", "--force", "--no-input"} {
				if !flags[want] {
					t.Fatalf("flags = %v, missing %s", flags, want)
				}
			}
			for _, obsolete := range []string{"--apply", "--plan", "--confirm"} {
				if flags[obsolete] {
					t.Fatalf("flags = %v, unexpectedly contains %s", flags, obsolete)
				}
			}
			if strings.Contains(result.Stdout, "planRequired") || strings.Contains(result.Stdout, "confirmationRequired") {
				t.Fatalf("schema retains plan lifecycle: %s", result.Stdout)
			}
			for _, obsolete := range []string{`"apply"`, `"plan"`, `"confirm"`, `"planToken"`, `"expiresInSeconds"`} {
				if strings.Contains(result.Stdout, obsolete) {
					t.Fatalf("schema retains obsolete plan field %s: %s", obsolete, result.Stdout)
				}
			}
			if got, ok := envelope.Data.Safety["destructive"].(bool); !ok || got != destructive {
				t.Fatalf("safety = %v, want destructive=%t", envelope.Data.Safety, destructive)
			}
		})
	}
}

func TestMutationHelpFindsSafetyFlagsAnywhere(t *testing.T) {
	commands := [][]string{
		{"blasts", "send"}, {"cohosts", "invite"}, {"cohosts", "link", "create"},
		{"cohosts", "link", "revoke"}, {"cohosts", "remove"}, {"cohosts", "revoke-invite"},
		{"events", "cancel"}, {"events", "create"}, {"events", "update"},
		{"guests", "invite"}, {"rsvp", "set"},
	}
	for _, command := range commands {
		name := strings.Join(command, ".")
		t.Run(name, func(t *testing.T) {
			argv := append([]string{"--dry-run"}, command...)
			argv = append(argv, "--force", "--help", "--no-input")
			result := app.Execute(context.Background(), app.Request{Argv: argv}, app.Dependencies{})
			if result.ExitCode != 0 {
				t.Fatalf("help result = %#v", result)
			}
			for _, want := range []string{"--dry-run", "--force", "--no-input"} {
				if !strings.Contains(result.Stdout, want) {
					t.Fatalf("help missing %s: %s", want, result.Stdout)
				}
			}
			for _, obsolete := range []string{"--apply", "--plan", "--confirm"} {
				if strings.Contains(result.Stdout, obsolete) {
					t.Fatalf("help retains %s: %s", obsolete, result.Stdout)
				}
			}
		})
	}
}

type recordingConfirmer struct {
	terminal bool
	answer   bool
	err      error
	calls    int
	prompts  []string
}

func (confirmer *recordingConfirmer) IsTerminal() bool {
	return confirmer.terminal
}

func (confirmer *recordingConfirmer) Confirm(prompt string) (bool, error) {
	confirmer.calls++
	confirmer.prompts = append(confirmer.prompts, prompt)
	return confirmer.answer, confirmer.err
}

func TestDestructiveMutationConfirmationSafety(t *testing.T) {
	tests := []struct {
		name             string
		flags            []string
		confirmer        *recordingConfirmer
		wantExitCode     int
		wantConfirmCalls int
		wantWrites       int
	}{
		{
			name:         "non-terminal refuses",
			confirmer:    &recordingConfirmer{},
			wantExitCode: 7,
		},
		{
			name:             "no-input refuses without prompting",
			flags:            []string{"--no-input"},
			confirmer:        &recordingConfirmer{terminal: true, answer: true},
			wantExitCode:     7,
			wantConfirmCalls: 0,
		},
		{
			name:             "terminal decline refuses",
			confirmer:        &recordingConfirmer{terminal: true},
			wantExitCode:     7,
			wantConfirmCalls: 1,
		},
		{
			name:             "confirmation error refuses",
			confirmer:        &recordingConfirmer{terminal: true, err: errors.New("input failed")},
			wantExitCode:     7,
			wantConfirmCalls: 1,
		},
		{
			name:             "terminal confirmation dispatches once",
			confirmer:        &recordingConfirmer{terminal: true, answer: true},
			wantExitCode:     0,
			wantConfirmCalls: 1,
			wantWrites:       1,
		},
		{
			name:         "force before command bypasses only prompt",
			flags:        []string{"--force"},
			confirmer:    &recordingConfirmer{},
			wantExitCode: 0,
			wantWrites:   1,
		},
		{
			name:             "dry-run after command skips prompt and write",
			flags:            []string{"--dry-run"},
			confirmer:        &recordingConfirmer{terminal: true, answer: true},
			wantExitCode:     0,
			wantConfirmCalls: 0,
			wantWrites:       0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := &memoryFilesystem{files: map[string][]byte{
				eventWriteCredentialsPath: []byte(eventWriteCredentials),
			}}
			dependencies := eventWriteDependencies(files)
			dependencies.Confirmer = test.confirmer
			reads := 0
			writes := 0
			dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				switch request.URL.String() {
				case "https://api.partiful.com/getEventInfo":
					reads++
					return jsonResponse(http.StatusOK, eventResponse(t, compatibleCancelEvent())), nil
				case "https://api.partiful.com/cancelEvent":
					writes++
					return jsonResponse(http.StatusOK, `{"result":true}`), nil
				default:
					t.Fatalf("unexpected request: %s", request.URL)
					return nil, nil
				}
			}}

			argv := append([]string{}, test.flags...)
			argv = append(argv, "events", "cancel", "event-example")
			result := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
			if result.ExitCode != test.wantExitCode {
				t.Fatalf("result = %#v, want exit %d", result, test.wantExitCode)
			}
			if test.wantExitCode == 7 && !strings.Contains(result.Stdout, `"type":"safety.confirmation_required"`) {
				t.Fatalf("result = %#v, want typed confirmation failure", result)
			}
			if test.confirmer.calls != test.wantConfirmCalls {
				t.Fatalf("confirm calls = %d, want %d", test.confirmer.calls, test.wantConfirmCalls)
			}
			if writes != test.wantWrites {
				t.Fatalf("mutation writes = %d, want %d", writes, test.wantWrites)
			}
			if test.wantWrites == 1 && reads != 1 {
				t.Fatalf("read checks = %d, want 1", reads)
			}
		})
	}
}

func TestDestructiveCommandsPromptWithCurrentRemoteTitleAndRefuseWithoutMutation(t *testing.T) {
	const title = `Remote "title" \ name`
	tests := []struct {
		name   string
		argv   []string
		action string
	}{
		{"cancel", []string{"events", "cancel", "event-example"}, "Cancel event"},
		{"remove cohost", []string{"cohosts", "remove", "event-example", "--contact", "Example Contact"}, "Remove a cohost from"},
		{"revoke cohost invite", []string{"cohosts", "revoke-invite", "event-example", "--contact", "Example Contact"}, "Revoke a cohost invite for"},
		{"revoke cohost link", []string{"cohosts", "link", "revoke", "event-example"}, "Revoke the cohost invite link for"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writes := 0
			dependencies := destructiveCommandDependencies(t, test.argv, title, &writes)
			declining := &recordingConfirmer{terminal: true}
			dependencies.Confirmer = declining

			result := app.Execute(context.Background(), app.Request{Argv: test.argv}, dependencies)
			if result.ExitCode != 7 || declining.calls != 1 {
				t.Fatalf("declined result = %#v, confirmations = %d", result, declining.calls)
			}
			wantPrompt := test.action + ` "Remote \"title\" \\ name"? [y/N] `
			if len(declining.prompts) != 1 || declining.prompts[0] != wantPrompt {
				t.Fatalf("confirmation prompts = %#v, want %q", declining.prompts, wantPrompt)
			}
			if writes != 0 {
				t.Fatalf("declined command made %d mutation requests", writes)
			}

			nonTerminalDependencies := destructiveCommandDependencies(t, test.argv, title, &writes)
			nonTerminal := &recordingConfirmer{}
			nonTerminalDependencies.Confirmer = nonTerminal
			result = app.Execute(context.Background(), app.Request{Argv: test.argv}, nonTerminalDependencies)
			if result.ExitCode != 7 || nonTerminal.calls != 0 || writes != 0 {
				t.Fatalf("non-terminal result = %#v, confirmations = %d, mutations = %d", result, nonTerminal.calls, writes)
			}
		})
	}
}

func destructiveCommandDependencies(t *testing.T, argv []string, title string, writes *int) app.Dependencies {
	t.Helper()
	files := &memoryFilesystem{files: map[string][]byte{
		eventWriteCredentialsPath: []byte(eventWriteCredentials),
	}}
	dependencies := eventWriteDependencies(files)
	contactCalls := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case "https://api.partiful.com/getEventInfo":
			event := compatibleUpdateEvent()
			event["title"] = title
			if strings.Join(argv[:2], " ") == "events cancel" {
				event["guestCount"] = 1
			}
			return jsonResponse(http.StatusOK, eventResponse(t, event)), nil
		case "https://api.partiful.com/getContacts":
			contactCalls++
			if contactCalls%2 == 1 {
				cursor := "cursor-1"
				return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{{"id": "private-contact-id", "name": "Example Contact", "sharedEventCount": 1}}, &cursor)), nil
			}
			return jsonResponse(http.StatusOK, contactPageResponse([]map[string]any{}, nil)), nil
		case "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/events/event-example/cohostRequests?pageSize=100":
			status := "ACCEPTED"
			if strings.Join(argv[:2], " ") == "cohosts revoke-invite" {
				status = "DECLINED"
			}
			return jsonResponse(http.StatusOK, `{"documents":[`+cohostRequestDocument("private-contact-id", status)+`]}`), nil
		case "https://firestore.googleapis.com/v1/projects/getpartiful/databases/(default)/documents/events/event-example/private/cohostSecret":
			return jsonResponse(http.StatusOK, cohostLinkDocument("/e/event-example?accept-cohost=existing-token")), nil
		case "https://api.partiful.com/cancelEvent", "https://api.partiful.com/removeCohost", "https://api.partiful.com/deleteCohostRequest", "https://api.partiful.com/revokeEventCohostLink":
			*writes++
			return jsonResponse(http.StatusOK, `{"result":true}`), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL)
			return nil, nil
		}
	}}
	return dependencies
}
