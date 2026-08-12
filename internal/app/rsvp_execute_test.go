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

func TestExecuteRSVPGetReturnsExplicitNoRSVPAndFailsClosedOnOtherVariants(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		remoteError error
		exitCode    int
		contains    string
	}{
		{
			name:     "explicit null",
			body:     `{"result":{"data":{"currentGuest":null}}}`,
			exitCode: 0,
			contains: `"data":{"eventId":"event-example","status":null}`,
		},
		{
			name:     "missing current guest",
			body:     `{"result":{"data":{}}}`,
			exitCode: 9,
			contains: `"type":"contract.protocol_changed"`,
		},
		{
			name:     "scalar current guest",
			body:     `{"result":{"data":{"currentGuest":"private scalar"}}}`,
			exitCode: 9,
			contains: `"type":"contract.protocol_changed"`,
		},
		{
			name:     "missing private identity",
			body:     `{"result":{"data":{"currentGuest":{"status":"GOING"}}}}`,
			exitCode: 9,
			contains: `"type":"contract.protocol_changed"`,
		},
		{
			name:     "unreviewed status",
			body:     `{"result":{"data":{"currentGuest":{"id":"private-id","status":"UNKNOWN"}}}}`,
			exitCode: 9,
			contains: `"type":"contract.protocol_changed"`,
		},
		{
			name:        "transport unavailable",
			remoteError: errors.New("private transport failure"),
			exitCode:    8,
			contains:    `"type":"remote.unavailable"`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dependencies := rsvpTestDependencies(&memoryFilesystem{files: map[string][]byte{
				rsvpCredentialsPath: []byte(rsvpCredentials),
			}})
			dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
				if testCase.remoteError != nil {
					return nil, testCase.remoteError
				}
				return jsonResponse(http.StatusOK, testCase.body), nil
			}}

			result := app.Execute(context.Background(), app.Request{
				Argv: []string{"rsvp", "get", "event-example"},
			}, dependencies)

			if result.ExitCode != testCase.exitCode ||
				!strings.Contains(result.Stdout, testCase.contains) {
				t.Fatalf("result = %#v, want exit %d containing %q", result, testCase.exitCode, testCase.contains)
			}
			for _, privateValue := range []string{
				"private scalar",
				"private-id",
				"private transport failure",
				"private-account",
				"private-access-token",
			} {
				if strings.Contains(result.Stdout+result.Stderr, privateValue) {
					t.Fatalf("output exposed private value %q", privateValue)
				}
			}
		})
	}
}

func TestExecuteRSVPCommandsRequireAuthenticationBeforeRemoteAccess(t *testing.T) {
	for _, argv := range [][]string{
		{"rsvp", "get", "event-example"},
		{"rsvp", "set", "event-example", "--status", "interested"},
	} {
		t.Run(strings.Join(argv[:2], " "), func(t *testing.T) {
			httpCalled := false
			result := app.Execute(
				context.Background(),
				app.Request{Argv: argv},
				app.Dependencies{
					Files:           &memoryFilesystem{files: map[string][]byte{}},
					CredentialsPath: rsvpCredentialsPath,
					Now: func() time.Time {
						return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
					},
					HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
						httpCalled = true
						return nil, errors.New("must not call HTTP")
					}},
				},
			)
			if result.ExitCode != 3 ||
				!strings.Contains(result.Stdout, `"type":"auth.required"`) ||
				httpCalled {
				t.Fatalf("result = %#v, HTTP = %t, want protected command", result, httpCalled)
			}
		})
	}
}

func TestExecuteRSVPSetPlansNormalizedGoingCreateWithoutMutation(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		rsvpCredentialsPath: []byte(rsvpCredentials),
	}}
	dependencies := rsvpTestDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("p", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1:
			assertRSVPRequest(
				t,
				request,
				"getEventInfo",
				`{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`,
			)
			return jsonResponse(http.StatusOK, `{"result":{"data":{"event":{
				"rsvpsEnabled":true,
				"atCapacity":false,
				"plusOneNamesRequired":true,
				"questionnaireEnabled":false,
				"questionnaireVersions":null,
				"ticketing":null,
				"guestAction":"RSVP",
				"maxCountPerGuest":4,
				"maxCapacity":50,
				"remainingCapacity":3,
				"enableWaitlist":false
			}}}}`), nil
		case 2:
			assertRSVPRequest(
				t,
				request,
				"getCurrentGuest",
				`{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`,
			)
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"currentGuest":null}}}`,
			), nil
		default:
			t.Fatalf("unexpected mutation request %d: %s", call, request.URL)
			return nil, nil
		}
	}}

	result := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"rsvp", "set", "event-example",
			"--status", "going",
			"--display-name", "  Example Attendee  ",
			"--party-size", "2",
			"--plus-one", "  Guest One  ",
			"--message", "  See you there  ",
			"--timezone", "America/Los_Angeles",
		},
	}, dependencies)

	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result = %#v, want plan success", result)
	}
	var envelope struct {
		Data struct {
			Operation string `json:"operation"`
			Mode      string `json:"mode"`
			Input     struct {
				Status                string   `json:"status"`
				DisplayName           string   `json:"displayName"`
				PartySize             int      `json:"partySize"`
				PlusOnes              []string `json:"plusOnes"`
				Message               *string  `json:"message"`
				Timezone              string   `json:"timezone"`
				QuestionnaireResponse any      `json:"questionnaireResponse"`
			} `json:"input"`
			Request struct {
				EventID string `json:"eventId"`
				RSVP    struct {
					Name             string           `json:"name"`
					Count            int              `json:"count"`
					PlusOnes         []map[string]any `json:"plusOnes"`
					Message          string           `json:"message"`
					Status           string           `json:"status"`
					Timezone         string           `json:"timezone"`
					ShouldFollowOrgs bool             `json:"shouldFollowOrgs"`
					GuestID          any              `json:"guestId"`
				} `json:"rsvp"`
			} `json:"request"`
			Preconditions struct {
				CurrentGuest    string `json:"currentGuest"`
				EventSafeguards string `json:"eventSafeguards"`
			} `json:"preconditions"`
			ExpiresInSeconds int    `json:"expiresInSeconds"`
			PlanToken        string `json:"planToken"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(result.Stdout), &envelope) != nil {
		t.Fatalf("decode result: %s", result.Stdout)
	}
	if envelope.Data.Operation != "addGuest" ||
		envelope.Data.Mode != "create" ||
		envelope.Data.Input.Status != "going" ||
		envelope.Data.Input.DisplayName != "Example Attendee" ||
		envelope.Data.Input.PartySize != 2 ||
		!reflect.DeepEqual(envelope.Data.Input.PlusOnes, []string{"Guest One"}) ||
		envelope.Data.Input.Message == nil ||
		*envelope.Data.Input.Message != "See you there" ||
		envelope.Data.Input.Timezone != "America/Los_Angeles" ||
		envelope.Data.Input.QuestionnaireResponse != nil {
		t.Fatalf("plan input = %#v, want normalized exact input", envelope.Data.Input)
	}
	if envelope.Data.Request.EventID != "event-example" ||
		envelope.Data.Request.RSVP.Name != "Example Attendee" ||
		envelope.Data.Request.RSVP.Count != 2 ||
		!reflect.DeepEqual(
			envelope.Data.Request.RSVP.PlusOnes,
			[]map[string]any{{"name": "Guest One"}},
		) ||
		envelope.Data.Request.RSVP.Message != "See you there" ||
		envelope.Data.Request.RSVP.Status != "GOING" ||
		envelope.Data.Request.RSVP.Timezone != "America/Los_Angeles" ||
		envelope.Data.Request.RSVP.ShouldFollowOrgs ||
		envelope.Data.Request.RSVP.GuestID != nil {
		t.Fatalf("plan request = %#v, want exact redacted create request", envelope.Data.Request)
	}
	if envelope.Data.Preconditions.CurrentGuest != "absent" ||
		envelope.Data.Preconditions.EventSafeguards != "bound" ||
		envelope.Data.ExpiresInSeconds != 300 ||
		envelope.Data.PlanToken == "" {
		t.Fatalf("plan authority = %#v, want bound five-minute plan", envelope.Data)
	}
	if call != 2 {
		t.Fatalf("request count = %d, want only two pre-reads", call)
	}
	if files.atomicWrites != 1 {
		t.Fatalf("atomic writes = %d, want one persistent plan write", files.atomicWrites)
	}
	for _, privateValue := range []string{
		"private-account",
		"private-access-token",
		"private-guest-id",
	} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("output exposed private value %q", privateValue)
		}
	}
}

func TestExecuteRSVPSetUpdatesReviewedGuestOnceAndReturnsOnlySubmittedIntent(t *testing.T) {
	const privateGuestID = "private-existing-guest"
	files := &memoryFilesystem{files: map[string][]byte{
		rsvpCredentialsPath: []byte(rsvpCredentials),
	}}
	dependencies := rsvpTestDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("u", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 3:
			assertRSVPRequest(
				t,
				request,
				"getEventInfo",
				`{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`,
			)
			return jsonResponse(http.StatusOK, `{"result":{"data":{"event":{
				"rsvpsEnabled":true,
				"atCapacity":false,
				"plusOneNamesRequired":true,
				"questionnaireEnabled":true,
				"questionnaireVersions":[{},{}],
				"ticketing":null,
				"maxCountPerGuest":3,
				"maxCapacity":20,
				"remainingCapacity":1,
				"enableWaitlist":null
			}}}}`), nil
		case 2, 4:
			assertRSVPRequest(
				t,
				request,
				"getCurrentGuest",
				`{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`,
			)
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"currentGuest":{"id":"`+privateGuestID+`","status":"GOING","count":1}}}}`,
			), nil
		case 5:
			assertRSVPRequest(
				t,
				request,
				"addGuest",
				`{"data":{"params":{"eventId":"event-example","rsvp":{"name":"Example Attendee","count":2,"plusOnes":[{"name":"Guest One"}],"status":"GOING","guestId":"`+privateGuestID+`","timezone":"America/Los_Angeles","questionnaireResponse":{"questionnaireVersion":1,"answers":{"question-example":"Answer"}},"shouldFollowOrgs":false}},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`,
			)
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"privateUnclaimed":"must-not-be-returned"}}}`,
			), nil
		default:
			t.Fatalf("unexpected post-write request %d: %s", call, request.URL)
			return nil, nil
		}
	}}
	argv := []string{
		"rsvp", "set", "event-example",
		"--status", "going",
		"--display-name", "Example Attendee",
		"--party-size", "2",
		"--plus-one", "Guest One",
		"--timezone", "America/Los_Angeles",
		"--questionnaire-response",
		`{"questionnaireVersion":1,"answers":{"question-example":"Answer"}}`,
	}

	plan := app.Execute(
		context.Background(),
		app.Request{Argv: argv},
		dependencies,
	)
	if plan.ExitCode != 0 ||
		!strings.Contains(plan.Stdout, `"mode":"update"`) ||
		!strings.Contains(plan.Stdout, `"guestId":"\u003credacted\u003e"`) {
		t.Fatalf("plan = %#v, want redacted update plan", plan)
	}
	if strings.Contains(plan.Stdout+plan.Stderr, privateGuestID) {
		t.Fatal("public plan exposed the private guest ID")
	}
	token := rsvpPlanToken(t, plan)
	applyArgv := append(append([]string{}, argv...), "--apply", "--plan", token)
	applied := app.Execute(
		context.Background(),
		app.Request{Argv: applyArgv},
		dependencies,
	)

	if applied.ExitCode != 0 ||
		!strings.Contains(
			applied.Stdout,
			`"data":{"eventId":"event-example","intent":"going","submitted":true}`,
		) ||
		applied.Stderr != "" {
		t.Fatalf("applied = %#v, want minimal submitted result", applied)
	}
	if call != 5 {
		t.Fatalf("request count = %d, want four pre-reads and one mutation", call)
	}
	if files.atomicWrites != 2 {
		t.Fatalf("atomic writes = %d, want plan save and pre-dispatch consume", files.atomicWrites)
	}
	for _, privateValue := range []string{
		privateGuestID,
		"private-account",
		"private-access-token",
		"privateUnclaimed",
		"Example Attendee",
		"Guest One",
		"question-example",
		"Answer",
	} {
		if strings.Contains(applied.Stdout+applied.Stderr, privateValue) {
			t.Fatalf("applied output exposed private submitted value %q", privateValue)
		}
	}
}

func TestExecuteRSVPSetSubmitsInterestWithoutDisplayNameOrSource(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		rsvpCredentialsPath: []byte(rsvpCredentials),
	}}
	dependencies := rsvpTestDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("i", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 3:
			return jsonResponse(http.StatusOK, compatibleRSVPEventResponse), nil
		case 2, 4:
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"currentGuest":null}}}`,
			), nil
		case 5:
			assertRSVPRequest(
				t,
				request,
				"markEventInterest",
				`{"data":{"params":{"eventId":"event-example","interested":true},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`,
			)
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"success":1e999999999,"interested":true,"privateValue":"ignored"}}}`,
			), nil
		default:
			t.Fatalf("unexpected request %d: %s", call, request.URL)
			return nil, nil
		}
	}}
	argv := []string{
		"rsvp", "set", "event-example",
		"--status", "interested",
	}

	plan := app.Execute(context.Background(), app.Request{Argv: argv}, dependencies)
	if plan.ExitCode != 0 ||
		!strings.Contains(plan.Stdout, `"operation":"markEventInterest"`) ||
		!strings.Contains(plan.Stdout, `"input":{"status":"interested"}`) ||
		strings.Contains(plan.Stdout, "displayName") ||
		strings.Contains(plan.Stdout, "source") {
		t.Fatalf("plan = %#v, want narrow direct-interest request", plan)
	}
	token := rsvpPlanToken(t, plan)
	applied := app.Execute(context.Background(), app.Request{
		Argv: append(append([]string{}, argv...), "--apply", "--plan", token),
	}, dependencies)
	if applied.ExitCode != 0 ||
		!strings.Contains(
			applied.Stdout,
			`"data":{"eventId":"event-example","intent":"interested","submitted":true}`,
		) ||
		call != 5 {
		t.Fatalf("applied = %#v, calls = %d, want one accepted interest submission", applied, call)
	}
}

func TestExecuteRSVPSetAcceptsStructuredDeclineAndRequiresNoCompletionFields(t *testing.T) {
	const input = `{
		"status":"not-going",
		"displayName":"  Example  ",
		"partySize":1,
		"plusOnes":[],
		"message":"   ",
		"timezone":"America/Los_Angeles",
		"questionnaireResponse":null
	}`
	files := &memoryFilesystem{files: map[string][]byte{
		rsvpCredentialsPath: []byte(rsvpCredentials),
	}}
	dependencies := rsvpTestDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("d", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1, 3:
			return jsonResponse(http.StatusOK, compatibleRSVPEventResponse), nil
		case 2, 4:
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"currentGuest":null}}}`,
			), nil
		case 5:
			assertRSVPRequest(
				t,
				request,
				"addGuest",
				`{"data":{"params":{"eventId":"event-example","rsvp":{"name":"Example","count":1,"plusOnes":[],"status":"DECLINED","timezone":"America/Los_Angeles","shouldFollowOrgs":false}},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`,
			)
			return jsonResponse(http.StatusOK, `{"result":{"data":null}}`), nil
		default:
			t.Fatalf("unexpected request %d: %s", call, request.URL)
			return nil, nil
		}
	}}
	argv := []string{"rsvp", "set", "event-example", "--input", "-"}

	plan := app.Execute(context.Background(), app.Request{
		Argv:  argv,
		Stdin: strings.NewReader(input),
	}, dependencies)
	if plan.ExitCode != 0 ||
		!strings.Contains(plan.Stdout, `"status":"not-going"`) ||
		!strings.Contains(plan.Stdout, `"message":null`) ||
		strings.Contains(plan.Stdout, "questionnaireVersion") {
		t.Fatalf("plan = %#v, want normalized decline", plan)
	}
	applied := app.Execute(context.Background(), app.Request{
		Argv: append(
			append([]string{}, argv...),
			"--apply",
			"--plan",
			rsvpPlanToken(t, plan),
		),
		Stdin: strings.NewReader(input),
	}, dependencies)
	if applied.ExitCode != 0 ||
		!strings.Contains(
			applied.Stdout,
			`"data":{"eventId":"event-example","intent":"not-going","submitted":true}`,
		) ||
		call != 5 {
		t.Fatalf("applied = %#v, want field-free accepted completion", applied)
	}
}

func TestExecuteRSVPSetRejectsInvalidNormalizedInputBeforeAuthenticationOrReads(t *testing.T) {
	valid := []string{
		"rsvp", "set", "event-example",
		"--status", "going",
		"--display-name", "Example",
		"--party-size", "1",
		"--timezone", "America/Los_Angeles",
	}
	cases := []struct {
		name  string
		argv  []string
		stdin string
		code  string
	}{
		{
			name: "display name required",
			argv: []string{
				"rsvp", "set", "event-example", "--status", "going",
				"--party-size", "1", "--timezone", "America/Los_Angeles",
			},
			code: "DISPLAY_NAME_INVALID",
		},
		{
			name: "display name length",
			argv: []string{
				"rsvp", "set", "event-example", "--status", "going",
				"--display-name", strings.Repeat("n", 51),
				"--party-size", "1", "--timezone", "America/Los_Angeles",
			},
			code: "DISPLAY_NAME_INVALID",
		},
		{
			name: "party count consistency",
			argv: append(append([]string{}, valid...), "--plus-one", "Guest"),
			code: "PARTY_SIZE_MISMATCH",
		},
		{
			name: "empty named plus one",
			argv: []string{
				"rsvp", "set", "event-example", "--status", "going",
				"--display-name", "Example", "--party-size", "2",
				"--plus-one", "  ", "--timezone", "America/Los_Angeles",
			},
			code: "PLUS_ONES_INVALID",
		},
		{
			name: "timezone is exact IANA value",
			argv: []string{
				"rsvp", "set", "event-example", "--status", "going",
				"--display-name", "Example", "--party-size", "1",
				"--timezone", " America/Los_Angeles ",
			},
			code: "TIMEZONE_INVALID",
		},
		{
			name: "message length",
			argv: append(
				append([]string{}, valid...),
				"--message",
				strings.Repeat("m", 401),
			),
			code: "MESSAGE_INVALID",
		},
		{
			name: "declined questionnaire omission",
			argv: []string{
				"rsvp", "set", "event-example", "--status", "not-going",
				"--display-name", "Example", "--party-size", "1",
				"--timezone", "America/Los_Angeles",
				"--questionnaire-response",
				`{"questionnaireVersion":0,"answers":{}}`,
			},
			code: "QUESTIONNAIRE_RESPONSE_INVALID",
		},
		{
			name: "interest accepts no profile input",
			argv: []string{
				"rsvp", "set", "event-example", "--status", "interested",
				"--display-name", "Example",
			},
			code: "INTEREST_INPUT_INVALID",
		},
		{
			name:  "unknown structured field",
			argv:  []string{"rsvp", "set", "event-example", "--input", "-"},
			stdin: `{"status":"interested","privateUnknown":"value"}`,
			code:  "INPUT_FIELD_UNKNOWN",
		},
		{
			name: "input source conflict",
			argv: []string{
				"rsvp", "set", "event-example", "--input", "-",
				"--status", "interested",
			},
			stdin: `{"status":"interested"}`,
			code:  "INPUT_SOURCE_CONFLICT",
		},
		{
			name: "questionnaire answer type",
			argv: append(
				append([]string{}, valid...),
				"--questionnaire-response",
				`{"questionnaireVersion":0,"answers":{"question":7}}`,
			),
			code: "QUESTIONNAIRE_RESPONSE_INVALID",
		},
		{
			name: "apply requires plan",
			argv: append(append([]string{}, valid...), "--apply"),
			code: "PLAN_REQUIRED",
		},
		{
			name: "plan requires apply",
			argv: append(append([]string{}, valid...), "--plan", "opaque"),
			code: "APPLY_REQUIRED",
		},
		{
			name: "repeated scalar",
			argv: append(append([]string{}, valid...), "--status", "going"),
			code: "FLAG_REPEATED",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			httpCalled := false
			result := app.Execute(context.Background(), app.Request{
				Argv:  testCase.argv,
				Stdin: strings.NewReader(testCase.stdin),
			}, app.Dependencies{
				HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
					httpCalled = true
					return nil, errors.New("must not call HTTP")
				}},
			})
			if result.ExitCode != 2 ||
				!strings.Contains(result.Stdout, `"type":"input.invalid"`) ||
				!strings.Contains(result.Stdout, `"code":"`+testCase.code+`"`) {
				t.Fatalf("result = %#v, want input failure %s", result, testCase.code)
			}
			if httpCalled {
				t.Fatal("invalid RSVP input caused a remote read")
			}
		})
	}
}

func TestExecuteRSVPSetEnforcesReviewedEventAndGuestCompatibility(t *testing.T) {
	going := []string{
		"rsvp", "set", "event-example",
		"--status", "going",
		"--display-name", "Example",
		"--party-size", "1",
		"--timezone", "America/Los_Angeles",
	}
	withQuestionnaire := append(
		append([]string{}, going...),
		"--questionnaire-response",
		`{"questionnaireVersion":0,"answers":{}}`,
	)
	partyTwo := []string{
		"rsvp", "set", "event-example",
		"--status", "going",
		"--display-name", "Example",
		"--party-size", "2",
		"--plus-one", "Guest",
		"--timezone", "America/Los_Angeles",
	}
	cases := []struct {
		name        string
		change      func(map[string]any)
		current     string
		argv        []string
		failureType string
		code        string
	}{
		{
			name: "RSVP disabled",
			change: func(event map[string]any) {
				event["rsvpsEnabled"] = false
			},
			failureType: "state.conflict",
			code:        "RSVP_EVENT_UNSUPPORTED",
		},
		{
			name: "required capacity flag missing",
			change: func(event map[string]any) {
				delete(event, "atCapacity")
			},
			failureType: "contract.protocol_changed",
			code:        "RSVP_PROTOCOL_CHANGED",
		},
		{
			name: "application event",
			change: func(event map[string]any) {
				event["guestAction"] = "APPLY"
			},
			failureType: "state.conflict",
			code:        "RSVP_EVENT_UNSUPPORTED",
		},
		{
			name: "ticketed event",
			change: func(event map[string]any) {
				event["ticketing"] = map[string]any{"private": "unprojected"}
			},
			failureType: "state.conflict",
			code:        "RSVP_EVENT_UNSUPPORTED",
		},
		{
			name: "unobserved null password",
			change: func(event map[string]any) {
				event["password"] = nil
			},
			failureType: "contract.protocol_changed",
			code:        "RSVP_PROTOCOL_CHANGED",
		},
		{
			name: "protected event",
			change: func(event map[string]any) {
				event["passwordProtected"] = true
			},
			failureType: "state.conflict",
			code:        "RSVP_EVENT_UNSUPPORTED",
		},
		{
			name: "at capacity does not invent waitlist",
			change: func(event map[string]any) {
				event["atCapacity"] = true
				event["enableWaitlist"] = true
			},
			failureType: "state.conflict",
			code:        "RSVP_EVENT_UNSUPPORTED",
		},
		{
			name: "party maximum",
			change: func(event map[string]any) {
				event["maxCountPerGuest"] = 1
			},
			argv:        partyTwo,
			failureType: "state.conflict",
			code:        "RSVP_EVENT_UNSUPPORTED",
		},
		{
			name: "invalid party maximum evidence",
			change: func(event map[string]any) {
				event["maxCountPerGuest"] = 0
			},
			failureType: "contract.protocol_changed",
			code:        "RSVP_PROTOCOL_CHANGED",
		},
		{
			name: "capacity snapshot incomplete",
			change: func(event map[string]any) {
				event["maxCapacity"] = 20
			},
			failureType: "state.conflict",
			code:        "RSVP_EVENT_UNSUPPORTED",
		},
		{
			name: "insufficient capacity",
			change: func(event map[string]any) {
				event["remainingCapacity"] = 0
			},
			failureType: "state.conflict",
			code:        "RSVP_EVENT_UNSUPPORTED",
		},
		{
			name: "declined current guest does not reduce going capacity delta",
			change: func(event map[string]any) {
				event["remainingCapacity"] = 1
			},
			current: `{"result":{"data":{"currentGuest":{
				"id":"private-guest","status":"DECLINED","count":1
			}}}}`,
			argv:        partyTwo,
			failureType: "state.conflict",
			code:        "RSVP_EVENT_UNSUPPORTED",
		},
		{
			name: "questionnaire required",
			change: func(event map[string]any) {
				event["questionnaireEnabled"] = true
				event["questionnaireVersions"] = []any{map[string]any{}}
			},
			failureType: "input.invalid",
			code:        "QUESTIONNAIRE_RESPONSE_INVALID",
		},
		{
			name: "questionnaire versions unavailable",
			change: func(event map[string]any) {
				event["questionnaireEnabled"] = true
				event["questionnaireVersions"] = nil
			},
			argv:        withQuestionnaire,
			failureType: "state.conflict",
			code:        "RSVP_EVENT_UNSUPPORTED",
		},
		{
			name: "questionnaire latest version",
			change: func(event map[string]any) {
				event["questionnaireEnabled"] = true
				event["questionnaireVersions"] = []any{
					map[string]any{},
					map[string]any{},
				}
			},
			argv:        withQuestionnaire,
			failureType: "input.invalid",
			code:        "QUESTIONNAIRE_RESPONSE_INVALID",
		},
		{
			name: "questionnaire omitted when disabled",
			change: func(event map[string]any) {
				event["questionnaireEnabled"] = false
			},
			argv:        withQuestionnaire,
			failureType: "input.invalid",
			code:        "QUESTIONNAIRE_RESPONSE_INVALID",
		},
		{
			name: "existing guest count required",
			current: `{"result":{"data":{"currentGuest":{
				"id":"private-guest","status":"GOING"
			}}}}`,
			failureType: "contract.protocol_changed",
			code:        "RSVP_PROTOCOL_CHANGED",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			event := compatibleRSVPEvent()
			if testCase.change != nil {
				testCase.change(event)
			}
			current := testCase.current
			if current == "" {
				current = `{"result":{"data":{"currentGuest":null}}}`
			}
			files := &memoryFilesystem{files: map[string][]byte{
				rsvpCredentialsPath: []byte(rsvpCredentials),
			}}
			dependencies := rsvpTestDependencies(files)
			dependencies.MutationRandom = strings.NewReader(strings.Repeat("c", 32))
			call := 0
			dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				call++
				switch call {
				case 1:
					return jsonResponse(
						http.StatusOK,
						rsvpEventResponse(t, event),
					), nil
				case 2:
					return jsonResponse(http.StatusOK, current), nil
				default:
					t.Fatalf("unexpected mutation request %d: %s", call, request.URL)
					return nil, nil
				}
			}}
			argv := testCase.argv
			if argv == nil {
				argv = going
			}

			result := app.Execute(
				context.Background(),
				app.Request{Argv: argv},
				dependencies,
			)

			if result.ExitCode == 0 ||
				!strings.Contains(result.Stdout, `"type":"`+testCase.failureType+`"`) ||
				!strings.Contains(result.Stdout, `"code":"`+testCase.code+`"`) {
				t.Fatalf(
					"result = %#v, want %s/%s",
					result,
					testCase.failureType,
					testCase.code,
				)
			}
			if files.atomicWrites != 0 {
				t.Fatalf("atomic writes = %d, want no plan for incompatible event", files.atomicWrites)
			}
		})
	}
}

func TestExecuteRSVPSetRejectsAnyChangedReviewedPreconditionAsStale(t *testing.T) {
	cases := []struct {
		name       string
		planEvent  map[string]any
		applyEvent map[string]any
		planGuest  string
		applyGuest string
	}{
		{
			name:       "absent to explicit null event field",
			planEvent:  compatibleRSVPEvent(),
			applyEvent: rsvpEventWith("ticketing", nil),
			planGuest:  `{"result":{"data":{"currentGuest":null}}}`,
			applyGuest: `{"result":{"data":{"currentGuest":null}}}`,
		},
		{
			name:       "no guest to existing guest",
			planEvent:  compatibleRSVPEvent(),
			applyEvent: compatibleRSVPEvent(),
			planGuest:  `{"result":{"data":{"currentGuest":null}}}`,
			applyGuest: `{"result":{"data":{"currentGuest":{"id":"private-new-guest","status":"GOING","count":1}}}}`,
		},
		{
			name:       "existing guest count",
			planEvent:  compatibleRSVPEvent(),
			applyEvent: compatibleRSVPEvent(),
			planGuest:  `{"result":{"data":{"currentGuest":{"id":"private-guest","status":"GOING","count":1}}}}`,
			applyGuest: `{"result":{"data":{"currentGuest":{"id":"private-guest","status":"GOING","count":2}}}}`,
		},
		{
			name:       "existing guest count becomes absent",
			planEvent:  compatibleRSVPEvent(),
			applyEvent: compatibleRSVPEvent(),
			planGuest:  `{"result":{"data":{"currentGuest":{"id":"private-guest","status":"GOING","count":1}}}}`,
			applyGuest: `{"result":{"data":{"currentGuest":{"id":"private-guest","status":"GOING"}}}}`,
		},
	}
	argv := []string{
		"rsvp", "set", "event-example",
		"--status", "going",
		"--display-name", "Example",
		"--party-size", "1",
		"--timezone", "America/Los_Angeles",
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			files := &memoryFilesystem{files: map[string][]byte{
				rsvpCredentialsPath: []byte(rsvpCredentials),
			}}
			dependencies := rsvpTestDependencies(files)
			dependencies.MutationRandom = strings.NewReader(strings.Repeat("s", 32))
			call := 0
			dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				call++
				switch call {
				case 1:
					return jsonResponse(
						http.StatusOK,
						rsvpEventResponse(t, testCase.planEvent),
					), nil
				case 2:
					return jsonResponse(http.StatusOK, testCase.planGuest), nil
				case 3:
					return jsonResponse(
						http.StatusOK,
						rsvpEventResponse(t, testCase.applyEvent),
					), nil
				case 4:
					return jsonResponse(http.StatusOK, testCase.applyGuest), nil
				default:
					t.Fatalf("stale plan dispatched request %d to %s", call, request.URL)
					return nil, nil
				}
			}}
			plan := app.Execute(
				context.Background(),
				app.Request{Argv: argv},
				dependencies,
			)
			if plan.ExitCode != 0 {
				t.Fatalf("plan = %#v, want success", plan)
			}

			applied := app.Execute(context.Background(), app.Request{
				Argv: append(
					append([]string{}, argv...),
					"--apply",
					"--plan",
					rsvpPlanToken(t, plan),
				),
			}, dependencies)

			if applied.ExitCode != 7 ||
				!strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) ||
				call != 4 {
				t.Fatalf("applied = %#v, calls = %d, want stale before dispatch", applied, call)
			}
			if strings.Contains(applied.Stdout+applied.Stderr, "private-guest") ||
				strings.Contains(applied.Stdout+applied.Stderr, "private-new-guest") {
				t.Fatal("stale-plan failure exposed a private guest ID")
			}
		})
	}
}

func TestExecuteRSVPSetTreatsRemovedEventAsStaleBeforeDispatch(t *testing.T) {
	files := &memoryFilesystem{files: map[string][]byte{
		rsvpCredentialsPath: []byte(rsvpCredentials),
	}}
	dependencies := rsvpTestDependencies(files)
	dependencies.MutationRandom = strings.NewReader(strings.Repeat("d", 32))
	call := 0
	dependencies.HTTP = scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		call++
		switch call {
		case 1:
			return jsonResponse(http.StatusOK, compatibleRSVPEventResponse), nil
		case 2:
			return jsonResponse(
				http.StatusOK,
				`{"result":{"data":{"currentGuest":null}}}`,
			), nil
		case 3:
			return jsonResponse(
				http.StatusNotFound,
				`{"error":{"message":"private detail","status":"NOT_FOUND"}}`,
			), nil
		default:
			t.Fatalf("removed event dispatched request %d", call)
			return nil, nil
		}
	}}
	argv := []string{
		"rsvp", "set", "event-example",
		"--status", "interested",
	}
	plan := app.Execute(
		context.Background(),
		app.Request{Argv: argv},
		dependencies,
	)
	applied := app.Execute(context.Background(), app.Request{
		Argv: append(
			append([]string{}, argv...),
			"--apply",
			"--plan",
			rsvpPlanToken(t, plan),
		),
	}, dependencies)

	if applied.ExitCode != 7 ||
		!strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) ||
		call != 3 {
		t.Fatalf("applied = %#v, calls = %d, want stale before dispatch", applied, call)
	}
	if strings.Contains(applied.Stdout+applied.Stderr, "private detail") {
		t.Fatal("stale-plan failure exposed remote detail")
	}
}

func TestExecuteRSVPSetBindsPlanToInputAccountAndFiveMinuteLifetime(t *testing.T) {
	argv := []string{
		"rsvp", "set", "event-example",
		"--status", "going",
		"--display-name", "Example",
		"--party-size", "1",
		"--timezone", "America/Los_Angeles",
	}
	cases := []struct {
		name   string
		before func(*memoryFilesystem)
		change func(*memoryFilesystem, *time.Time) []string
	}{
		{
			name: "normalized input",
			change: func(_ *memoryFilesystem, _ *time.Time) []string {
				changed := append([]string{}, argv...)
				changed[6] = "Different Example"
				return changed
			},
		},
		{
			name: "authenticated account",
			change: func(files *memoryFilesystem, _ *time.Time) []string {
				files.files[rsvpCredentialsPath] = []byte(
					`{"accessToken":"private-access-token","userId":"different-private-account","expiresAt":"2026-08-12T02:00:00Z"}`,
				)
				return argv
			},
		},
		{
			name: "token account overrides stale stored identity",
			before: func(files *memoryFilesystem) {
				files.files[rsvpCredentialsPath] = []byte(
					`{"accessToken":"header.eyJzdWIiOiJwcml2YXRlLWFjY291bnQtYSJ9.signature","userId":"stale-private-account","expiresAt":"2026-08-12T02:00:00Z"}`,
				)
			},
			change: func(files *memoryFilesystem, _ *time.Time) []string {
				files.files[rsvpCredentialsPath] = []byte(
					`{"accessToken":"header.eyJzdWIiOiJwcml2YXRlLWFjY291bnQtYiJ9.signature","userId":"stale-private-account","expiresAt":"2026-08-12T02:00:00Z"}`,
				)
				return argv
			},
		},
		{
			name: "five minute expiry",
			change: func(_ *memoryFilesystem, now *time.Time) []string {
				*now = now.Add(5 * time.Minute)
				return argv
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			files := &memoryFilesystem{files: map[string][]byte{
				rsvpCredentialsPath: []byte(rsvpCredentials),
			}}
			if testCase.before != nil {
				testCase.before(files)
			}
			now := time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
			dependencies := rsvpTestDependencies(files)
			dependencies.Now = func() time.Time { return now }
			dependencies.MutationRandom = strings.NewReader(strings.Repeat("b", 32))
			call := 0
			dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				call++
				switch call {
				case 1:
					return jsonResponse(http.StatusOK, compatibleRSVPEventResponse), nil
				case 2:
					return jsonResponse(
						http.StatusOK,
						`{"result":{"data":{"currentGuest":null}}}`,
					), nil
				default:
					t.Fatalf("invalid binding caused remote request %d to %s", call, request.URL)
					return nil, nil
				}
			}}
			plan := app.Execute(
				context.Background(),
				app.Request{Argv: argv},
				dependencies,
			)
			if plan.ExitCode != 0 {
				t.Fatalf("plan = %#v, want success", plan)
			}
			applyInput := testCase.change(files, &now)

			applied := app.Execute(context.Background(), app.Request{
				Argv: append(
					append([]string{}, applyInput...),
					"--apply",
					"--plan",
					rsvpPlanToken(t, plan),
				),
			}, dependencies)

			if applied.ExitCode != 7 ||
				!strings.Contains(applied.Stdout, `"type":"safety.plan_stale"`) ||
				call != 2 {
				t.Fatalf("applied = %#v, calls = %d, want stale before reads", applied, call)
			}
			for _, privateValue := range []string{
				"private-account",
				"different-private-account",
			} {
				if strings.Contains(applied.Stdout+applied.Stderr, privateValue) {
					t.Fatalf("stale plan exposed private account value %q", privateValue)
				}
			}
		})
	}
}

func TestExecuteRSVPSetConsumesPlanBeforeOneUncertainAttempt(t *testing.T) {
	cases := []struct {
		name        string
		response    string
		remoteError error
		failureType string
		code        string
	}{
		{
			name:        "ambiguous transport",
			remoteError: errors.New("private connection lost"),
			failureType: "remote.unavailable",
			code:        "RSVP_SUBMISSION_UNCERTAIN",
		},
		{
			name:        "interest completion predicate",
			response:    `{"result":{"data":{"success":true,"interested":false}}}`,
			failureType: "contract.protocol_changed",
			code:        "RSVP_PROTOCOL_CHANGED",
		},
		{
			name:        "JavaScript-falsy underflowed success",
			response:    `{"result":{"data":{"success":1e-999,"interested":true}}}`,
			failureType: "contract.protocol_changed",
			code:        "RSVP_PROTOCOL_CHANGED",
		},
	}
	argv := []string{
		"rsvp", "set", "event-example",
		"--status", "interested",
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			files := &memoryFilesystem{files: map[string][]byte{
				rsvpCredentialsPath: []byte(rsvpCredentials),
			}}
			dependencies := rsvpTestDependencies(files)
			dependencies.MutationRandom = strings.NewReader(strings.Repeat("x", 32))
			call := 0
			mutationAttempts := 0
			dependencies.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				call++
				switch call {
				case 1, 3:
					return jsonResponse(http.StatusOK, compatibleRSVPEventResponse), nil
				case 2, 4:
					return jsonResponse(
						http.StatusOK,
						`{"result":{"data":{"currentGuest":null}}}`,
					), nil
				case 5:
					mutationAttempts++
					if files.atomicWrites != 2 {
						t.Fatalf(
							"atomic writes before dispatch = %d, want plan consumed",
							files.atomicWrites,
						)
					}
					if testCase.remoteError != nil {
						return nil, testCase.remoteError
					}
					return jsonResponse(http.StatusOK, testCase.response), nil
				default:
					t.Fatalf("automatic retry or post-read %d: %s", call, request.URL)
					return nil, nil
				}
			}}
			plan := app.Execute(
				context.Background(),
				app.Request{Argv: argv},
				dependencies,
			)
			token := rsvpPlanToken(t, plan)
			applyArgv := append(
				append([]string{}, argv...),
				"--apply",
				"--plan",
				token,
			)

			applied := app.Execute(
				context.Background(),
				app.Request{Argv: applyArgv},
				dependencies,
			)

			if applied.ExitCode == 0 ||
				!strings.Contains(applied.Stdout, `"type":"`+testCase.failureType+`"`) ||
				!strings.Contains(applied.Stdout, `"code":"`+testCase.code+`"`) ||
				mutationAttempts != 1 ||
				call != 5 {
				t.Fatalf(
					"applied = %#v, attempts = %d, calls = %d",
					applied,
					mutationAttempts,
					call,
				)
			}
			reused := app.Execute(
				context.Background(),
				app.Request{Argv: applyArgv},
				dependencies,
			)
			if reused.ExitCode != 7 ||
				!strings.Contains(reused.Stdout, `"type":"safety.plan_stale"`) ||
				call != 5 ||
				mutationAttempts != 1 {
				t.Fatalf("reused = %#v, want consumed token without retry", reused)
			}
			if strings.Contains(applied.Stdout+applied.Stderr, "private connection lost") {
				t.Fatal("uncertain result exposed private transport details")
			}
		})
	}
}

func TestExecuteSchemaPublishesCompleteRSVPSurfaceAndSafety(t *testing.T) {
	type schemaEnvelope struct {
		Data struct {
			Command     string `json:"command"`
			Positionals []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"positionals"`
			Flags []struct {
				Name string `json:"name"`
			} `json:"flags"`
			InputSchema struct {
				OneOf []struct {
					Required   []string `json:"required"`
					Properties map[string]struct {
						Enum      []string `json:"enum"`
						MaxLength *int     `json:"maxLength"`
					} `json:"properties"`
				} `json:"oneOf"`
			} `json:"inputSchema"`
			SuccessSchema struct {
				OneOf []json.RawMessage `json:"oneOf"`
			} `json:"successSchema"`
			FailureTypes []string `json:"failureTypes"`
			Safety       struct {
				Kind                 string `json:"kind"`
				PlanRequired         bool   `json:"planRequired"`
				ConfirmationRequired bool   `json:"confirmationRequired"`
			} `json:"safety"`
		} `json:"data"`
	}
	getResult := app.Execute(context.Background(), app.Request{
		Argv: []string{"schema", "rsvp.get"},
	}, app.Dependencies{})
	if getResult.ExitCode != 0 ||
		!strings.Contains(getResult.Stdout, `"command":"rsvp.get"`) ||
		!strings.Contains(getResult.Stdout, `"enum":["ready-to-send"`) ||
		strings.Contains(getResult.Stdout, "guestId") ||
		strings.Contains(getResult.Stdout, "userId") {
		t.Fatalf("get schema = %#v, want private-ID-free reviewed RSVP read", getResult)
	}

	setResult := app.Execute(context.Background(), app.Request{
		Argv: []string{"schema", "rsvp.set"},
	}, app.Dependencies{})
	var set schemaEnvelope
	if setResult.ExitCode != 0 ||
		json.Unmarshal([]byte(setResult.Stdout), &set) != nil {
		t.Fatalf("set schema = %#v, want decodable schema", setResult)
	}
	if set.Data.Command != "rsvp.set" ||
		len(set.Data.Positionals) != 1 ||
		set.Data.Positionals[0].Name != "event-id" ||
		!set.Data.Positionals[0].Required {
		t.Fatalf("set positionals = %#v, want required event ID", set.Data.Positionals)
	}
	var flags []string
	for _, flag := range set.Data.Flags {
		flags = append(flags, flag.Name)
	}
	if !reflect.DeepEqual(flags, []string{
		"--input",
		"--status",
		"--display-name",
		"--party-size",
		"--plus-one",
		"--message",
		"--timezone",
		"--questionnaire-response",
		"--apply",
		"--plan",
	}) {
		t.Fatalf("set flags = %v, want complete RSVP invocation", flags)
	}
	if len(set.Data.InputSchema.OneOf) != 3 ||
		!reflect.DeepEqual(
			set.Data.InputSchema.OneOf[0].Properties["status"].Enum,
			[]string{"going"},
		) ||
		!reflect.DeepEqual(
			set.Data.InputSchema.OneOf[1].Properties["status"].Enum,
			[]string{"not-going"},
		) ||
		!reflect.DeepEqual(
			set.Data.InputSchema.OneOf[2].Properties["status"].Enum,
			[]string{"interested"},
		) ||
		set.Data.InputSchema.OneOf[0].Properties["displayName"].MaxLength == nil ||
		*set.Data.InputSchema.OneOf[0].Properties["displayName"].MaxLength != 50 {
		t.Fatalf("set input schema = %#v, want exact three-intent input", set.Data.InputSchema)
	}
	if len(set.Data.SuccessSchema.OneOf) != 2 ||
		set.Data.Safety.Kind != "standard-mutation" ||
		!set.Data.Safety.PlanRequired ||
		set.Data.Safety.ConfirmationRequired {
		t.Fatalf("set success/safety = %#v, want plan-or-submitted standard mutation", set.Data)
	}
	for _, failureType := range []string{
		"input.invalid",
		"auth.required",
		"state.conflict",
		"safety.plan_stale",
		"remote.unavailable",
		"contract.protocol_changed",
	} {
		if !slices.Contains(set.Data.FailureTypes, failureType) {
			t.Fatalf("failure types = %v, missing %s", set.Data.FailureTypes, failureType)
		}
	}
}

const (
	rsvpCredentialsPath         = "/config/partiful/credentials.json"
	rsvpMutationPath            = "/config/partiful/mutation-plans.json"
	rsvpCredentials             = `{"accessToken":"private-access-token","userId":"private-account","expiresAt":"2026-08-12T02:00:00Z"}`
	compatibleRSVPEventResponse = `{"result":{"data":{"event":{
		"rsvpsEnabled":true,
		"atCapacity":false,
		"plusOneNamesRequired":false,
		"questionnaireVersions":null
	}}}}`
)

func compatibleRSVPEvent() map[string]any {
	return map[string]any{
		"rsvpsEnabled":          true,
		"atCapacity":            false,
		"plusOneNamesRequired":  false,
		"questionnaireVersions": nil,
	}
}

func rsvpEventWith(name string, value any) map[string]any {
	event := compatibleRSVPEvent()
	event[name] = value
	return event
}

func rsvpEventResponse(t *testing.T, event map[string]any) string {
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

func rsvpTestDependencies(files *memoryFilesystem) app.Dependencies {
	return app.Dependencies{
		Files:           files,
		CredentialsPath: rsvpCredentialsPath,
		MutationPath:    rsvpMutationPath,
		Now: func() time.Time {
			return time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC)
		},
		AuthRandom: strings.NewReader(strings.Repeat("0123456789abcdef", 8)),
	}
}

func assertRSVPRequest(t *testing.T, request *http.Request, operation, body string) {
	t.Helper()
	if request.Method != http.MethodPost ||
		request.URL.String() != "https://api.partiful.com/"+operation {
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

func rsvpPlanToken(t *testing.T, result app.Result) string {
	t.Helper()
	var envelope struct {
		Data struct {
			PlanToken string `json:"planToken"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(result.Stdout), &envelope) != nil ||
		envelope.Data.PlanToken == "" {
		t.Fatalf("result has no plan token: %#v", result)
	}
	return envelope.Data.PlanToken
}
