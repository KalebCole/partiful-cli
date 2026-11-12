package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

var updateProductGoldens = flag.Bool("update-product-goldens", false, "deliberately update reviewed schema and error mapping snapshots")
var errorGoldenMutex sync.Mutex
var observedErrorMappings = map[string]errorMapping{}

// executeChecked applies the product invariants to every existing command fixture,
// including failures after reads and writes. Help is the human discovery exception.
func executeChecked(t *testing.T, ctx context.Context, request app.Request, deps app.Dependencies) app.Result {
	t.Helper()
	result := app.Execute(ctx, request, deps)
	if result.ExitCode == 0 && !strings.HasPrefix(result.Stdout, "{") {
		for _, arg := range request.Argv {
			if arg == "--help" || arg == "-h" || arg == "help" {
				return result
			}
		}
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("stdout must be exactly one complete JSON document: %v", err)
	}
	var ok *bool
	if err := json.Unmarshal(envelope["ok"], &ok); err != nil || ok == nil || *ok != (result.ExitCode == 0) {
		t.Fatal("ok must agree with exit code")
	}
	if len(envelope) != 3 {
		t.Fatalf("envelope keys = %v", reflect.ValueOf(envelope).MapKeys())
	}
	var meta map[string]json.RawMessage
	if json.Unmarshal(envelope["meta"], &meta) != nil {
		t.Fatal("missing meta object")
	}
	for key, want := range map[string]string{"cliVersion": app.Version, "productContractRevision": app.ProductContractRevision, "remoteContractRevision": app.RemoteContractRevision} {
		var got string
		if json.Unmarshal(meta[key], &got) != nil || got != want {
			t.Fatalf("meta.%s does not match build info", key)
		}
	}
	var command string
	if json.Unmarshal(meta["command"], &command) != nil || command == "" {
		t.Fatal("missing meta.command")
	}
	if *ok {
		if envelope["data"] == nil || envelope["error"] != nil || result.Stderr != "" {
			t.Fatal("invalid success envelope or diagnostics")
		}
		var warnings []string
		if json.Unmarshal(meta["warnings"], &warnings) != nil || warnings == nil {
			t.Fatal("success warnings must be an array")
		}
	} else {
		if envelope["data"] != nil {
			t.Fatal("failure contains partial success data")
		}
		var body struct {
			Type      string         `json:"type"`
			Code      string         `json:"code"`
			Message   string         `json:"message"`
			Retryable *bool          `json:"retryable"`
			Details   map[string]any `json:"details"`
		}
		if json.Unmarshal(envelope["error"], &body) != nil || body.Type == "" || body.Code == "" || body.Message == "" || body.Retryable == nil || body.Details == nil {
			t.Fatal("incomplete error envelope")
		}
		exits := map[string]int{"usage.invalid": 2, "input.invalid": 2, "match.ambiguous": 2, "auth.required": 3, "auth.expired": 3, "auth.human_required": 3, "permission.denied": 4, "resource.not_found": 5, "state.conflict": 6, "safety.confirmation_required": 7, "remote.unavailable": 8, "remote.rate_limited": 8, "contract.protocol_changed": 9, "internal.failure": 10}
		if exits[body.Type] != result.ExitCode {
			t.Fatalf("unstable exit mapping: %s => %d", body.Type, result.ExitCode)
		}
		checkErrorGolden(t, body.Code, errorMapping{body.Type, result.ExitCode})
	}
	// Fake secrets shared by remote/auth fixtures must never escape in either stream.
	for _, secret := range []string{"private-access-token", "private-refresh-token", "private-account", "private-contact-id", "private-current-account", "private-other-owner", "private-host", "fake-person@example.invalid", "+12025550173", "fake-secret-token", "fake-partiful-user-id"} {
		if strings.Contains(result.Stdout+result.Stderr, secret) {
			t.Fatal("output leaked a fake private fixture value")
		}
	}
	return result
}

type errorMapping struct {
	Type string `json:"type"`
	Exit int    `json:"exit"`
}

func checkErrorGolden(t *testing.T, code string, got errorMapping) {
	t.Helper()
	errorGoldenMutex.Lock()
	defer errorGoldenMutex.Unlock()
	path := filepath.Join("testdata", "error-mappings.golden.json")
	mappings := map[string]errorMapping{}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(raw, &mappings); err != nil {
			t.Fatal(err)
		}
	} else if !*updateProductGoldens {
		t.Fatal(err)
	}
	if *updateProductGoldens {
		if previous, exists := observedErrorMappings[code]; exists && previous != got {
			t.Fatalf("error code %s has inconsistent mappings", code)
		}
		observedErrorMappings[code] = got
		mappings[code] = got
		raw, err := json.MarshalIndent(mappings, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	} else if want, exists := mappings[code]; !exists || want != got {
		t.Fatalf("error mapping %s changed: got %+v, reviewed %+v; review and update product goldens", code, got, want)
	}
}

func TestProductSchemaGolden(t *testing.T) {
	snapshot := map[string]any{}
	catalog := executeChecked(t, context.Background(), app.Request{Argv: []string{"schema"}}, app.Dependencies{})
	var value any
	if err := json.Unmarshal([]byte(catalog.Stdout), &value); err != nil {
		t.Fatal(err)
	}
	snapshot["catalog"] = value
	definitions := map[string]any{}
	for _, command := range approvedCommandCatalog {
		result := executeChecked(t, context.Background(), app.Request{Argv: []string{"schema", command}}, app.Dependencies{})
		if err := json.Unmarshal([]byte(result.Stdout), &value); err != nil {
			t.Fatal(err)
		}
		definitions[command] = value
	}
	snapshot["definitions"] = definitions
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	path := filepath.Join("testdata", "schema.golden.json")
	if *updateProductGoldens {
		if err := os.WriteFile(path, raw, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(raw) {
		t.Fatal("schema changed; inspect the diff and deliberately update with go test ./internal/app -update-product-goldens")
	}
}

func TestEveryPublicCommandRejectsUnknownFlagsWithCompleteEnvelope(t *testing.T) {
	for _, command := range approvedCommandCatalog {
		t.Run(command, func(t *testing.T) {
			argv := strings.Split(command, ".")
			if command == "version" {
				argv = []string{"--version"}
			}
			if strings.HasPrefix(command, "cohosts.") || strings.HasPrefix(command, "guests.") || strings.HasPrefix(command, "rsvp.") || command == "events.get" || command == "events.update" || command == "events.cancel" || command == "blasts.send" {
				argv = append(argv, "event-example")
			}
			argv = append(argv, "--unknown-private-value")
			result := executeChecked(t, context.Background(), app.Request{Argv: argv}, app.Dependencies{})
			if result.ExitCode != 2 || strings.Contains(result.Stdout+result.Stderr, "unknown-private-value") {
				t.Fatal("unknown flags must fail safely before dependencies")
			}
		})
	}
}

func TestStructuredInputAndTimeRules(t *testing.T) {
	for _, command := range []string{"events.create", "events.update", "events.cancel", "rsvp.set"} {
		for _, input := range []string{`{"unknown":"fake-person@example.invalid"}`, `{} {}`, `[]`, `null`} {
			t.Run(command+input, func(t *testing.T) {
				argv := strings.Split(command, ".")
				if command != "events.create" {
					argv = append(argv, "event-example")
				}
				argv = append(argv, "--input", "-")
				result := executeChecked(t, context.Background(), app.Request{Argv: argv, Stdin: strings.NewReader(input)}, app.Dependencies{})
				if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"type":"input.invalid"`) {
					t.Fatal("invalid structured input must fail before authentication")
				}
			})
		}
	}
	for _, command := range []string{"events.create", "events.update"} {
		cases := []struct{ name, input, code string }{
			{"zone-free", `{"title":"Example","start":"2026-09-12T19:00:00","timezone":"America/Los_Angeles"}`, "START_INVALID"},
			{"natural date", `{"title":"Example","start":"tomorrow","timezone":"America/Los_Angeles"}`, "START_INVALID"},
			{"invalid end", `{"title":"Example","end":"tomorrow","start":"2026-09-12T19:00:00Z","timezone":"America/Los_Angeles"}`, "END_INVALID"},
			{"unknown zone", `{"title":"Example","start":"2026-09-12T19:00:00Z","timezone":"Not/AZone"}`, "TIMEZONE_INVALID"},
			{"implicit local zone", `{"title":"Example","start":"2026-09-12T19:00:00Z","timezone":"Local"}`, "TIMEZONE_INVALID"},
			{"offset zone", `{"title":"Example","start":"2026-09-12T19:00:00Z","timezone":"-07:00"}`, "TIMEZONE_INVALID"},
		}
		if command == "events.create" {
			cases = append(cases, struct{ name, input, code string }{"missing zone", `{"title":"Example","start":"2026-09-12T19:00:00Z"}`, "TIMEZONE_INVALID"})
		}
		for _, tc := range cases {
			t.Run(command+tc.name, func(t *testing.T) {
				argv := strings.Split(command, ".")
				if command == "events.update" {
					argv = append(argv, "event-example")
				}
				argv = append(argv, "--input", "-")
				result := executeChecked(t, context.Background(), app.Request{Argv: argv, Stdin: strings.NewReader(tc.input)}, app.Dependencies{})
				if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"code":"`+tc.code+`"`) {
					t.Fatalf("expected %s, got %s", tc.code, result.Stdout)
				}
			})
		}
	}
}

func TestScalarFlagsCannotRepeatOrMixWithStructuredInput(t *testing.T) {
	for _, command := range approvedCommandCatalog {
		schema := executeChecked(t, context.Background(), app.Request{Argv: []string{"schema", command}}, app.Dependencies{})
		var envelope struct {
			Data struct {
				Flags []struct {
					Name       string
					TakesValue bool
				}
				Positionals []struct{ Required bool }
			}
		}
		if err := json.Unmarshal([]byte(schema.Stdout), &envelope); err != nil {
			t.Fatal(err)
		}
		for _, f := range envelope.Data.Flags {
			if !f.TakesValue || f.Name == "--link" || f.Name == "--plus-one" {
				continue
			}
			t.Run(command+f.Name, func(t *testing.T) {
				argv := strings.Split(command, ".")
				for _, p := range envelope.Data.Positionals {
					if p.Required {
						argv = append(argv, "event-example")
					}
				}
				argv = append(argv, f.Name, "value", f.Name, "value")
				result := executeChecked(t, context.Background(), app.Request{Argv: argv}, app.Dependencies{})
				if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"code":"FLAG_REPEATED"`) {
					t.Fatalf("scalar repetition accepted: %s", result.Stdout)
				}
			})
		}
	}
	for _, command := range []string{"events.create", "events.update", "events.cancel", "rsvp.set"} {
		t.Run(command+" mixed input", func(t *testing.T) {
			argv := strings.Split(command, ".")
			field := "--title"
			if command != "events.create" {
				argv = append(argv, "event-example")
			}
			if command == "events.cancel" {
				field = "--message"
			}
			if command == "rsvp.set" {
				field = "--status"
			}
			argv = append(argv, "--input", "-", field, "value")
			result := executeChecked(t, context.Background(), app.Request{Argv: argv, Stdin: strings.NewReader(`{}`)}, app.Dependencies{})
			if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"type":"input.invalid"`) {
				t.Fatal("mixed input must fail before dependencies")
			}
		})
	}
}

func TestEveryDestructiveCommandRequiresAllowedConfirmationPath(t *testing.T) {
	for _, argv := range [][]string{{"events", "cancel", "event-example"}, {"cohosts", "remove", "event-example", "--contact", "Example Contact"}, {"cohosts", "revoke-invite", "event-example", "--contact", "Example Contact"}, {"cohosts", "link", "revoke", "event-example"}} {
		for _, mode := range []string{"non-tty", "--no-input", "--non-interactive", "decline", "input-error", "confirm", "--force", "--dry-run"} {
			t.Run(strings.Join(argv[:2], ".")+mode, func(t *testing.T) {
				writes := 0
				deps := destructiveCommandDependencies(t, argv, "Example", &writes)
				confirmer := &recordingConfirmer{terminal: mode != "non-tty", answer: mode != "decline"}
				if mode == "input-error" {
					confirmer.err = errors.New("private input error")
				}
				deps.Confirmer = confirmer
				args := append([]string{}, argv...)
				if strings.HasPrefix(mode, "--") {
					args = append(args, mode)
				}
				result := executeChecked(t, context.Background(), app.Request{Argv: args}, deps)
				wantExit, wantWrites := 7, 0
				if mode == "confirm" || mode == "--force" {
					wantExit, wantWrites = 0, 1
				}
				if mode == "--dry-run" {
					wantExit = 0
				}
				if result.ExitCode != wantExit || writes != wantWrites {
					t.Fatalf("exit/writes = %d/%d, want %d/%d", result.ExitCode, writes, wantExit, wantWrites)
				}
				if (mode == "--no-input" || mode == "--non-interactive" || mode == "non-tty" || mode == "--force" || mode == "--dry-run") && confirmer.calls != 0 {
					t.Fatal("unexpected prompt")
				}
			})
		}
	}
}

func TestPrivateRemoteAndStorageValuesNeverEnterOutput(t *testing.T) {
	const private = `fake-person@example.invalid +12025550173 fake-secret-token fake-partiful-user-id`
	// Malformed remote bodies and transport errors must be replaced with safe errors,
	// including failures after a collection has already accumulated valid items.
	for _, argv := range [][]string{{"events", "list", "--when", "upcoming"}, {"events", "get", "event-example"}, {"contacts", "list"}, {"guests", "list", "event-example"}, {"rsvp", "get", "event-example"}, {"posters", "list"}, {"posters", "search", "--query", "Example"}} {
		for _, mode := range []string{"body", "transport", "storage"} {
			t.Run(strings.Join(argv[:2], ".")+mode, func(t *testing.T) {
				deps := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{eventWriteCredentialsPath: []byte(eventWriteCredentials)}})
				deps.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
					if mode == "transport" {
						return nil, errors.New(private)
					}
					return jsonResponse(500, `{"error":"`+private+`"}`), nil
				}}
				if mode == "storage" {
					deps.CredentialsPathError = errors.New(private)
				}
				result := executeChecked(t, context.Background(), app.Request{Argv: argv}, deps)
				if result.ExitCode == 0 {
					t.Fatal("expected failure")
				}
				for _, value := range strings.Fields(private) {
					if strings.Contains(result.Stdout+result.Stderr, value) {
						t.Fatal("private value escaped into output")
					}
				}
			})
		}
	}
	for _, command := range []string{"auth.status", "auth.logout", "doctor"} {
		t.Run(command, func(t *testing.T) {
			deps := app.Dependencies{CredentialsPathError: errors.New(private)}
			result := executeChecked(t, context.Background(), app.Request{Argv: strings.Split(command, ".")}, deps)
			for _, value := range strings.Fields(private) {
				if strings.Contains(result.Stdout+result.Stderr, value) {
					t.Fatal("private storage value escaped into output")
				}
			}
		})
	}
	// Open remote records may contain private identity/contact fields; public
	// projections must keep only the reviewed domain fields.
	for _, command := range []string{"events.get", "events.list", "contacts.list", "guests.list", "rsvp.get"} {
		t.Run(command+" success", func(t *testing.T) {
			deps := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{eventWriteCredentialsPath: []byte(eventWriteCredentials)}})
			calls := 0
			deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				calls++
				privateFields := map[string]any{"phoneNumber": "+12025550173", "email": "fake-person@example.invalid", "accessToken": "fake-secret-token", "userId": "fake-partiful-user-id"}
				switch request.URL.Path {
				case "/getEventInfo":
					event := compatibleCancelEvent()
					for k, v := range privateFields {
						event[k] = v
					}
					return jsonResponse(200, eventResponse(t, event)), nil
				case "/getMyUpcomingEventsForHomePage":
					event := compatibleCancelEvent()
					event["id"] = "event-example"
					for k, v := range privateFields {
						event[k] = v
					}
					raw, _ := json.Marshal(map[string]any{"result": map[string]any{"data": map[string]any{"upcomingEvents": []any{event}}}})
					return jsonResponse(200, string(raw)), nil
				case "/getContacts":
					if calls > 1 {
						return jsonResponse(200, contactPageResponse([]map[string]any{}, nil)), nil
					}
					privateFields["id"] = "fake-partiful-user-id"
					privateFields["name"] = "Example Contact"
					privateFields["sharedEventCount"] = 2
					cursor := "next"
					return jsonResponse(200, contactPageResponse([]map[string]any{privateFields}, &cursor)), nil
				case "/getGuests":
					privateFields["id"] = "fake-private-guest"
					privateFields["name"] = "Example Guest"
					privateFields["status"] = "GOING"
					privateFields["count"] = 1
					privateFields["anchorGuestId"] = nil
					raw, _ := json.Marshal(map[string]any{"result": map[string]any{"data": []any{privateFields}, "paging": map[string]any{}}})
					return jsonResponse(200, string(raw)), nil
				case "/getCurrentGuest":
					privateFields["id"] = "fake-private-guest"
					privateFields["status"] = "GOING"
					raw, _ := json.Marshal(map[string]any{"result": map[string]any{"data": map[string]any{"currentGuest": privateFields}}})
					return jsonResponse(200, string(raw)), nil
				default:
					t.Fatalf("unexpected fixture request %s", request.URL.Path)
					return nil, nil
				}
			}}
			argv := strings.Split(command, ".")
			if command == "events.list" {
				argv = append(argv, "--when", "upcoming")
			} else if command != "contacts.list" {
				argv = append(argv, "event-example")
			}
			result := executeChecked(t, context.Background(), app.Request{Argv: argv}, deps)
			if result.ExitCode != 0 {
				t.Fatalf("expected successful public projection: %s", result.Stdout)
			}
			for _, value := range append(strings.Fields(private), "fake-private-guest") {
				if strings.Contains(result.Stdout+result.Stderr, value) {
					t.Fatal("public projection leaked private record field")
				}
			}
		})
	}
}

const productPoster = `{"id":"Let's Party","url":"https://example.invalid/poster.jpg","name":"Let's Party","tags":[],"categories":[],"contentType":"image/jpeg","height":1200,"width":630}`

func TestCreateOptionalNullsAndExactPosterResolution(t *testing.T) {
	for _, tc := range []struct {
		name, catalog, input string
		exit                 int
	}{
		{"optional nulls", "[" + productPoster + "]", `{"title":"Example","start":"2026-09-12T19:00:00-07:00","timezone":"America/Los_Angeles","end":null,"description":null,"location":null,"visibility":null,"guestLimit":null,"links":null,"posterId":null}`, 0},
		{"missing exact poster", "[]", `{"title":"Example","start":"2026-09-12T19:00:00Z","timezone":"UTC"}`, 5},
		{"duplicate exact poster", "[" + productPoster + "," + productPoster + "]", `{"title":"Example","start":"2026-09-12T19:00:00Z","timezone":"UTC"}`, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			deps := app.Dependencies{HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				calls++
				if request.Method != "GET" || request.URL.Path != "/posters.json" {
					t.Fatal("create dry-run must only resolve posters")
				}
				return jsonResponse(200, tc.catalog), nil
			}}}
			result := executeChecked(t, context.Background(), app.Request{Argv: []string{"events", "create", "--input", "-", "--dry-run"}, Stdin: strings.NewReader(tc.input)}, deps)
			if result.ExitCode != tc.exit || calls != 1 {
				t.Fatalf("exit/calls=%d/%d, want %d/1", result.ExitCode, calls, tc.exit)
			}
			if tc.exit == 0 {
				var envelope struct {
					Data struct {
						Request struct{ Event map[string]any }
						Input   struct{ Visibility string }
					}
				}
				if json.Unmarshal([]byte(result.Stdout), &envelope) != nil {
					t.Fatal("invalid preview")
				}
				if envelope.Data.Input.Visibility != "private" || envelope.Data.Request.Event["startDate"] != "2026-09-13T02:00:00Z" {
					t.Fatal("default visibility or UTC projection changed")
				}
				for _, field := range []string{"endDate", "description", "locationInfo", "isPublic", "maxCapacity", "customFields"} {
					if _, exists := envelope.Data.Request.Event[field]; exists {
						t.Fatalf("optional null leaked into request: %s", field)
					}
				}
			}
		})
	}
}

func TestEventMutationPreconditionsFailBeforeWritesEvenWithForce(t *testing.T) {
	cases := []struct {
		name, command, input string
		change               func(map[string]any)
		exit                 int
	}{
		{"cancel ownership", "cancel", `{}`, func(e map[string]any) { e["ownerIds"] = []string{"other"} }, 4},
		{"cancel published", "cancel", `{}`, func(e map[string]any) { e["status"] = "CANCELED" }, 6},
		{"cancel positive guests", "cancel", `{}`, func(e map[string]any) { e["guestCount"] = 0 }, 6},
		{"cancel invalid count", "cancel", `{}`, func(e map[string]any) { e["guestCount"] = nil }, 9},
		{"cancel missing status", "cancel", `{}`, func(e map[string]any) { delete(e, "status") }, 9},
		{"update ownership", "update", `{"title":"New"}`, func(e map[string]any) { delete(e, "ownerIds") }, 4},
		{"update ticketing dates", "update", `{"timezone":"UTC"}`, func(e map[string]any) { e["ticketing"] = map[string]any{} }, 6},
		{"update missing guest fact", "update", `{"timezone":"UTC"}`, func(e map[string]any) { delete(e, "hasGuests") }, 9},
		{"update past event with guests", "update", `{"timezone":"UTC"}`, func(e map[string]any) {
			e["hasGuests"] = true
			e["startDate"] = "2026-08-01T00:00:00Z"
			e["endDate"] = "2026-08-01T01:00:00Z"
		}, 6},
		{"update past event without end", "update", `{"timezone":"UTC"}`, func(e map[string]any) {
			e["hasGuests"] = true
			e["startDate"] = "2026-08-01T00:00:00Z"
			delete(e, "endDate")
		}, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{eventWriteCredentialsPath: []byte(eventWriteCredentials)}})
			calls := 0
			deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				calls++
				if request.URL.Path != "/getEventInfo" {
					t.Fatal("precondition failure dispatched write")
				}
				event := compatibleCancelEvent()
				tc.change(event)
				return jsonResponse(200, eventResponse(t, event)), nil
			}}
			result := executeChecked(t, context.Background(), app.Request{Argv: []string{"events", tc.command, "event-example", "--input", "-", "--force"}, Stdin: strings.NewReader(tc.input)}, deps)
			if result.ExitCode != tc.exit || calls != 1 {
				t.Fatalf("exit/calls=%d/%d, want %d/1", result.ExitCode, calls, tc.exit)
			}
		})
	}
}

func TestStructuredInputFileMatchesStdin(t *testing.T) {
	for _, tc := range []struct{ command, input string }{
		{"events.create", `{"title":"Example","start":"2026-09-12T19:00:00Z","timezone":"America/Los_Angeles"}`},
		{"events.update", `{"title":"Example"}`},
		{"events.cancel", `{"notifyGuests":false}`},
		{"rsvp.set", `{"status":"going","displayName":"Example","partySize":1,"timezone":"America/Los_Angeles"}`},
	} {
		t.Run(tc.command, func(t *testing.T) {
			outputs := []string{}
			for _, source := range []string{"-", "input.json"} {
				files := &memoryFilesystem{files: map[string][]byte{eventWriteCredentialsPath: []byte(eventWriteCredentials), "input.json": []byte(tc.input)}}
				deps := eventWriteDependencies(files)
				deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
					switch request.URL.Path {
					case "/posters.json":
						return jsonResponse(200, "["+productPoster+"]"), nil
					case "/getEventInfo":
						return jsonResponse(200, eventResponse(t, compatibleCancelEvent())), nil
					case "/getCurrentGuest":
						return jsonResponse(200, `{"result":{"data":{"currentGuest":null}}}`), nil
					default:
						t.Fatal("dry-run dispatched an unexpected request")
						return nil, nil
					}
				}}
				argv := strings.Split(tc.command, ".")
				if tc.command != "events.create" {
					argv = append(argv, "event-example")
				}
				argv = append(argv, "--input", source, "--dry-run")
				result := executeChecked(t, context.Background(), app.Request{Argv: argv, Stdin: strings.NewReader(tc.input)}, deps)
				if result.ExitCode != 0 {
					t.Fatalf("structured input failed: %s", result.Stdout)
				}
				if string(files.files[eventWriteCredentialsPath]) != eventWriteCredentials {
					t.Fatal("dry-run changed stored credentials")
				}
				outputs = append(outputs, result.Stdout)
			}
			if outputs[0] != outputs[1] {
				t.Fatal("file and stdin normalized differently")
			}
		})
	}
}

func TestInvitationNameResolutionAndCohostStates(t *testing.T) {
	for _, command := range []string{"guests.invite", "cohosts.invite", "cohosts.revoke-invite", "cohosts.remove"} {
		for _, tc := range []struct {
			name, query string
			names       []string
			status      string
			exit        int
		}{
			{"no match", "Nobody", []string{"Example Contact"}, "", 5},
			{"ambiguous partial", "Example", []string{"Example A", "Example B"}, "", 2},
			{"duplicate exact", "Example", []string{"Example", "Example"}, "", 2},
			{"unique partial", "Contact", []string{"Example Contact"}, "", 0},
			{"exact before partial", "Example", []string{"Example", "Example Contact"}, "", 0},
			{"wrong state", "Example", []string{"Example"}, "PENDING", 6},
		} {
			if tc.name == "wrong state" && (command == "guests.invite" || command == "cohosts.revoke-invite") {
				continue
			}
			t.Run(command+tc.name, func(t *testing.T) {
				deps := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{eventWriteCredentialsPath: []byte(eventWriteCredentials)}})
				contactCalls := 0
				deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
					switch request.URL.Path {
					case "/getEventInfo":
						return jsonResponse(200, eventResponse(t, compatibleCancelEvent())), nil
					case "/getContacts":
						contactCalls++
						if contactCalls > 1 {
							return jsonResponse(200, contactPageResponse([]map[string]any{}, nil)), nil
						}
						items := []map[string]any{}
						for i, name := range tc.names {
							items = append(items, map[string]any{"id": fmt.Sprintf("private-contact-%d", i), "name": name, "sharedEventCount": 1})
						}
						cursor := "next"
						return jsonResponse(200, contactPageResponse(items, &cursor)), nil
					case "/v1/projects/getpartiful/databases/(default)/documents/events/event-example/cohostRequests":
						return jsonResponse(200, `{"documents":[`+cohostRequestDocument("private-contact-0", func() string {
							if tc.status == "" {
								if tc.exit == 0 && command == "cohosts.remove" {
									return "ACCEPTED"
								}
								return "DECLINED"
							}
							return tc.status
						}())+`]}`), nil
					default:
						t.Fatal("failed resolution or precondition dispatched mutation")
						return nil, nil
					}
				}}
				argv := append(strings.Split(command, "."), "event-example", "--contact", tc.query, "--force", "--dry-run")
				result := executeChecked(t, context.Background(), app.Request{Argv: argv}, deps)
				if result.ExitCode != tc.exit {
					t.Fatalf("exit=%d, want %d: %s", result.ExitCode, tc.exit, result.Stdout)
				}
			})
		}
	}
}

func TestBlastAudienceAndAgeSafeguards(t *testing.T) {
	for _, tc := range []struct {
		name        string
		eventChange func(map[string]any)
		statuses    []string
		want        []string
		exit        int
	}{
		{"application", func(e map[string]any) { e["guestAction"] = "APPLY" }, []string{"REJECTED", "WITHDRAWN", "WAITLISTED_FOR_APPROVAL", "PENDING_APPROVAL", "APPROVED"}, []string{"APPROVED", "PENDING_APPROVAL", "WAITLISTED_FOR_APPROVAL", "WITHDRAWN", "REJECTED"}, 0},
		{"find a time", func(e map[string]any) {
			e["findATime"] = map[string]any{"enabled": true, "optionMap": map[string]any{}}
		}, []string{"GOING", "RESPONDED_TO_FIND_A_TIME"}, []string{"RESPONDED_TO_FIND_A_TIME"}, 0},
		{"ordinary waitlist", func(e map[string]any) { e["enableWaitlist"] = true }, []string{"WAITLIST", "DECLINED", "MAYBE", "GOING"}, []string{"GOING", "MAYBE", "DECLINED", "WAITLIST"}, 0},
		{"invited over limit", func(e map[string]any) {}, append([]string{"GOING"}, strings.Split(strings.Repeat("SENT,", 100)+"SENT", ",")...), []string{"GOING"}, 0},
		{"empty recipient set", func(e map[string]any) {}, []string{}, nil, 6},
		{"old end", func(e map[string]any) { e["startDate"] = "2026-01-01T00:00:00Z"; e["endDate"] = "2026-01-02T00:00:00Z" }, []string{"GOING"}, nil, 6},
		{"old start without end", func(e map[string]any) { e["startDate"] = "2026-01-01T00:00:00Z"; delete(e, "endDate") }, []string{"GOING"}, nil, 6},
		{"host required", func(e map[string]any) { delete(e, "ownerIds") }, []string{"GOING"}, nil, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{eventWriteCredentialsPath: []byte(eventWriteCredentials)}})
			reads := 0
			deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				reads++
				switch request.URL.Path {
				case "/getEventInfo":
					event := compatibleBlastEvent()
					tc.eventChange(event)
					return jsonResponse(200, eventResponse(t, event)), nil
				case "/v1/projects/getpartiful/databases/(default)/documents/events/event-example/guests":
					docs := []map[string]any{}
					for i, status := range tc.statuses {
						docs = append(docs, firestoreDocument(fmt.Sprintf("projects/getpartiful/databases/(default)/documents/events/event-example/guests/g%d", i), map[string]any{"status": firestoreString(status)}))
					}
					return jsonResponse(200, firestoreListResponse(t, docs...)), nil
				case "/v1/projects/getpartiful/databases/(default)/documents/events/event-example/hostMessages":
					return jsonResponse(200, `{}`), nil
				default:
					t.Fatal("dry-run or rejected blast dispatched mutation")
					return nil, nil
				}
			}}
			result := executeChecked(t, context.Background(), app.Request{Argv: []string{"blasts", "send", "event-example", "--message-file", "-", "--audience", "all-guests", "--dry-run"}, Stdin: strings.NewReader("fake-person@example.invalid +12025550173")}, deps)
			if result.ExitCode != tc.exit {
				t.Fatalf("exit=%d, want %d: %s", result.ExitCode, tc.exit, result.Stdout)
			}
			if strings.Contains(result.Stdout+result.Stderr, "fake-person@example.invalid") || strings.Contains(result.Stdout+result.Stderr, "+12025550173") {
				t.Fatal("private message escaped")
			}
			if tc.exit == 0 {
				var envelope struct {
					Data struct {
						Request struct{ Message struct{ To []string } }
					}
				}
				if json.Unmarshal([]byte(result.Stdout), &envelope) != nil || !reflect.DeepEqual(envelope.Data.Request.Message.To, tc.want) {
					t.Fatalf("recipient groups differ: %s", result.Stdout)
				}
				if reads != 3 {
					t.Fatal("blast must read each precondition once")
				}
			}
		})
	}
}

func TestCollectionBoundsApplyToEveryCollectionCommand(t *testing.T) {
	for _, argv := range [][]string{{"events", "list", "--when", "upcoming"}, {"contacts", "list"}, {"guests", "list", "event-example"}, {"posters", "list"}, {"posters", "search", "--query", "Example"}} {
		for _, flags := range [][]string{{"--limit", "0"}, {"--limit", "101"}, {"--limit", "bad"}, {"--all"}, {"--all", "--max-items", "1001"}, {"--all", "--max-items", "0"}, {"--all", "--max-items", "10", "--limit", "1"}, {"--cursor", ""}} {
			t.Run(strings.Join(append(append([]string{}, argv...), flags...), " "), func(t *testing.T) {
				result := executeChecked(t, context.Background(), app.Request{Argv: append(append([]string{}, argv...), flags...)}, app.Dependencies{})
				if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"type":"input.invalid"`) {
					t.Fatalf("invalid collection bounds accepted: %s", result.Stdout)
				}
			})
		}
	}
}

func TestEveryRemoteMutationDryRunsAndNeverRetries(t *testing.T) {
	commands := [][]string{
		{"events", "create", "--title", "Example", "--start", "2026-09-12T19:00:00Z", "--timezone", "America/Los_Angeles"},
		{"events", "update", "event-example", "--title", "Example"},
		{"events", "cancel", "event-example"},
		{"guests", "invite", "event-example", "--contact", "Example Contact"},
		{"rsvp", "set", "event-example", "--status", "going", "--display-name", "Example", "--party-size", "1", "--timezone", "America/Los_Angeles"},
		{"blasts", "send", "event-example", "--audience", "all-guests", "--message-file", "message.txt"},
		{"cohosts", "invite", "event-example", "--contact", "Example Contact"},
		{"cohosts", "remove", "event-example", "--contact", "Example Contact"},
		{"cohosts", "revoke-invite", "event-example", "--contact", "Example Contact"},
		{"cohosts", "link", "create", "event-example"},
		{"cohosts", "link", "revoke", "event-example"},
	}
	for _, argv := range commands {
		for _, mode := range []string{"dry-run", "uncertain", "malformed completion"} {
			command := strings.Join(argv[:2], ".")
			if argv[0] == "cohosts" && argv[1] == "link" {
				command = strings.Join(argv[:3], ".")
			}
			t.Run(command+"/"+mode, func(t *testing.T) {
				files := &memoryFilesystem{files: map[string][]byte{eventWriteCredentialsPath: []byte(eventWriteCredentials), "message.txt": []byte("fake-person@example.invalid +12025550173")}}
				deps := eventWriteDependencies(files)
				writes := 0
				reads := map[string]int{}
				deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
					path := request.URL.Path
					switch path {
					case "/posters.json":
						reads[path]++
						return jsonResponse(200, "["+productPoster+"]"), nil
					case "/getEventInfo":
						reads[path]++
						return jsonResponse(200, eventResponse(t, compatibleBlastEvent())), nil
					case "/getCurrentGuest":
						reads[path]++
						return jsonResponse(200, `{"result":{"data":{"currentGuest":null}}}`), nil
					case "/getContacts":
						reads[path]++
						if reads[path] == 1 {
							cursor := "next"
							return jsonResponse(200, contactPageResponse([]map[string]any{{"id": "private-contact-id", "name": "Example Contact", "sharedEventCount": 1}}, &cursor)), nil
						}
						return jsonResponse(200, contactPageResponse([]map[string]any{}, nil)), nil
					case "/v1/projects/getpartiful/databases/(default)/documents/events/event-example/cohostRequests":
						reads[path]++
						if argv[1] == "invite" {
							return jsonResponse(200, `{}`), nil
						}
						status := "ACCEPTED"
						if argv[1] == "revoke-invite" {
							status = "PENDING"
						}
						return jsonResponse(200, `{"documents":[`+cohostRequestDocument("private-contact-id", status)+`]}`), nil
					case "/v1/projects/getpartiful/databases/(default)/documents/events/event-example/private/cohostSecret":
						reads[path]++
						if argv[2] == "create" {
							return jsonResponse(404, firestoreNotFound()), nil
						}
						return jsonResponse(200, cohostLinkDocument("/e/event-example?accept-cohost=existing-token")), nil
					case "/v1/projects/getpartiful/databases/(default)/documents/events/event-example/guests":
						reads[path]++
						return jsonResponse(200, firestoreListResponse(t, firestoreDocument("projects/getpartiful/databases/(default)/documents/events/event-example/guests/g1", map[string]any{"status": firestoreString("GOING")}))), nil
					case "/v1/projects/getpartiful/databases/(default)/documents/events/event-example/hostMessages":
						reads[path]++
						return jsonResponse(200, `{}`), nil
					default:
						writes++
						if mode == "dry-run" {
							t.Fatal("dry-run dispatched remote mutation")
						}
						if mode == "uncertain" {
							return nil, errors.New("private-access-token +12025550173 fake-person@example.invalid")
						}
						return jsonResponse(200, `{"unexpected":"private-access-token"}`), nil
					}
				}}
				args := append(append([]string{}, argv...), "--force")
				if mode == "dry-run" {
					args = append(args, "--dry-run")
				}
				result := executeChecked(t, context.Background(), app.Request{Argv: args}, deps)
				wantExit, wantWrites := 8, 1
				if mode == "dry-run" {
					wantExit, wantWrites = 0, 0
				}
				if mode == "malformed completion" {
					wantExit = 9
				}
				if result.ExitCode != wantExit || writes != wantWrites {
					t.Fatalf("exit/writes=%d/%d, want %d/%d: %s", result.ExitCode, writes, wantExit, wantWrites, result.Stdout)
				}
				if result.ExitCode != 0 && !strings.Contains(result.Stdout, `"retryable":false`) {
					t.Fatal("uncertain or malformed mutation must not invite automatic retry")
				}
				for path, count := range reads {
					want := 1
					if path == "/getContacts" {
						want = 2
					}
					if count != want {
						t.Fatalf("precondition %s read %d times, want %d", path, count, want)
					}
				}
				if string(files.files[eventWriteCredentialsPath]) != eventWriteCredentials {
					t.Fatal("mutation fixture unexpectedly persisted credentials")
				}
			})
		}
	}
}

func TestSchemaPublishesGlobalFlagsForEveryCommand(t *testing.T) {
	for _, command := range approvedCommandCatalog {
		t.Run(command, func(t *testing.T) {
			result := executeChecked(t, context.Background(), app.Request{Argv: []string{"schema", command}}, app.Dependencies{})
			var envelope struct {
				Data struct{ Flags []struct{ Name string } }
			}
			if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{}
			for _, f := range envelope.Data.Flags {
				counts[f.Name]++
			}
			for _, name := range []string{"--pretty", "--non-interactive", "--no-input"} {
				if counts[name] != 1 {
					t.Fatalf("global flag %s appears %d times", name, counts[name])
				}
			}
		})
	}
	for _, name := range []string{"--pretty", "--non-interactive", "--no-input"} {
		for _, argv := range [][]string{{name, "--version"}, {"--version", name}} {
			result := executeChecked(t, context.Background(), app.Request{Argv: argv}, app.Dependencies{})
			if result.ExitCode != 0 {
				t.Fatal("global flag rejected")
			}
			repeated := executeChecked(t, context.Background(), app.Request{Argv: append(argv, name)}, app.Dependencies{})
			var failure struct{ Error struct{ Code string } }
			if err := json.Unmarshal([]byte(repeated.Stdout), &failure); err != nil {
				t.Fatal(err)
			}
			if repeated.ExitCode != 2 || failure.Error.Code != "FLAG_REPEATED" {
				t.Fatal("repeated global flag accepted")
			}
		}
	}
}

func TestCohostPreconditionStateMatrix(t *testing.T) {
	for _, action := range []string{"invite", "revoke-invite", "remove", "link.create", "link.revoke"} {
		states := []string{"absent", "PENDING", "DECLINED", "ACCEPTED"}
		if strings.HasPrefix(action, "link.") {
			states = []string{"absent", "present"}
		}
		for _, state := range states {
			t.Run(action+"/"+state, func(t *testing.T) {
				deps := eventWriteDependencies(&memoryFilesystem{files: map[string][]byte{eventWriteCredentialsPath: []byte(eventWriteCredentials)}})
				contacts := 0
				deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
					switch request.URL.Path {
					case "/getEventInfo":
						return jsonResponse(200, eventResponse(t, compatibleCancelEvent())), nil
					case "/getContacts":
						contacts++
						if contacts > 1 {
							return jsonResponse(200, contactPageResponse([]map[string]any{}, nil)), nil
						}
						cursor := "next"
						return jsonResponse(200, contactPageResponse([]map[string]any{{"id": "private-contact-id", "name": "Example Contact", "sharedEventCount": 1}}, &cursor)), nil
					case "/v1/projects/getpartiful/databases/(default)/documents/events/event-example/cohostRequests":
						if state == "absent" {
							return jsonResponse(200, `{}`), nil
						}
						return jsonResponse(200, `{"documents":[`+cohostRequestDocument("private-contact-id", state)+`]}`), nil
					case "/v1/projects/getpartiful/databases/(default)/documents/events/event-example/private/cohostSecret":
						if state == "absent" {
							return jsonResponse(404, firestoreNotFound()), nil
						}
						return jsonResponse(200, cohostLinkDocument("/e/event-example?accept-cohost=existing-token")), nil
					default:
						t.Fatal("cohost state check dispatched mutation")
						return nil, nil
					}
				}}
				argv := append([]string{"cohosts"}, strings.Split(action, ".")...)
				argv = append(argv, "event-example", "--dry-run")
				if !strings.HasPrefix(action, "link.") {
					argv = append(argv, "--contact", "Example Contact")
				}
				result := executeChecked(t, context.Background(), app.Request{Argv: argv}, deps)
				allowed := (action == "invite" && (state == "absent" || state == "DECLINED")) || (action == "revoke-invite" && (state == "PENDING" || state == "DECLINED")) || (action == "remove" && state == "ACCEPTED") || (action == "link.create" && state == "absent") || (action == "link.revoke" && state == "present")
				want := 6
				if allowed {
					want = 0
				}
				if result.ExitCode != want {
					t.Fatalf("exit=%d, want %d: %s", result.ExitCode, want, result.Stdout)
				}
			})
		}
	}
}
