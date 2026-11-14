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

func TestRSVPRemoveResolvesOnlyCurrentGuestAndRedactsPreview(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		deps := rsvpTestDependencies(&memoryFilesystem{files: map[string][]byte{rsvpCredentialsPath: []byte(rsvpCredentials)}})
		calls := 0
		deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			calls++
			if request.Header.Get("Authorization") != "Bearer private-access-token" || request.Header.Get("Content-Type") != "application/json" {
				t.Fatal("missing auth or JSON header")
			}
			switch calls {
			case 1:
				assertRSVPRequest(t, request, "getEventInfo", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
				return jsonResponse(200, compatibleRSVPEventResponse), nil
			case 2:
				assertRSVPRequest(t, request, "getCurrentGuest", `{"data":{"params":{"eventId":"event-example"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
				return jsonResponse(200, `{"result":{"data":{"currentGuest":{"id":"private-guest-id","status":"DECLINED","count":1}}}}`), nil
			case 3:
				if dryRun {
					t.Fatal("dry-run dispatched mutation")
				}
				assertRSVPRequest(t, request, "removeGuest", `{"data":{"params":{"eventId":"event-example","guestId":"private-guest-id"},"amplitudeDeviceId":"MDEyMzQ1Njc4OWFiY2RlZg"}}`)
				return jsonResponse(200, `{"result":{"data":{}}}`), nil
			default:
				t.Fatal("unexpected retry or post-read")
				return nil, nil
			}
		}}
		argv := []string{"rsvp", "remove", "event-example", "--force", "--no-input"}
		if dryRun {
			argv = append(argv, "--dry-run")
		}
		result := executeChecked(t, context.Background(), app.Request{Argv: argv}, deps)
		if result.ExitCode != 0 || strings.Contains(result.Stdout+result.Stderr, "private-guest-id") {
			t.Fatalf("result: %#v", result)
		}
		if dryRun {
			var preview struct {
				Data struct {
					Operation string `json:"operation"`
					Request   struct {
						GuestID string `json:"guestId"`
					} `json:"request"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(result.Stdout), &preview) != nil || calls != 2 || preview.Data.Request.GuestID != "<redacted>" || preview.Data.Operation != "removeGuest" {
				t.Fatalf("preview: %#v, calls %d", result, calls)
			}
		} else if calls != 3 || !strings.Contains(result.Stdout, `"data":{"eventId":"event-example","submitted":true}`) {
			t.Fatalf("submission: %#v", result)
		}
	}
}

func TestRSVPRemoveCurrentGuestFailuresPreventMutation(t *testing.T) {
	cases := []struct {
		name, body, code string
		exit             int
		unavailable      bool
	}{
		{name: "not a guest", body: `{"result":{"data":{"currentGuest":null}}}`, code: "RSVP_GUEST_REQUIRED", exit: 6},
		{name: "missing identity", body: `{"result":{"data":{"currentGuest":{"status":"GOING","count":1}}}}`, code: "RSVP_PROTOCOL_CHANGED", exit: 9},
		{name: "missing count", body: `{"result":{"data":{"currentGuest":{"id":"private-guest-id","status":"GOING"}}}}`, code: "RSVP_PROTOCOL_CHANGED", exit: 9},
		{name: "unavailable", code: "RSVP_UNAVAILABLE", exit: 8, unavailable: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := rsvpTestDependencies(&memoryFilesystem{files: map[string][]byte{rsvpCredentialsPath: []byte(rsvpCredentials)}})
			calls := 0
			deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return jsonResponse(200, compatibleRSVPEventResponse), nil
				}
				if calls != 2 || request.URL.Path != "/getCurrentGuest" {
					t.Fatal("unexpected mutation")
				}
				if tc.unavailable {
					return nil, errors.New("private-access-token")
				}
				return jsonResponse(200, tc.body), nil
			}}
			result := executeChecked(t, context.Background(), app.Request{Argv: []string{"rsvp", "remove", "event-example", "--force"}}, deps)
			if result.ExitCode != tc.exit || !strings.Contains(result.Stdout, `"code":"`+tc.code+`"`) {
				t.Fatalf("result: %#v", result)
			}
		})
	}
}

func TestRSVPRemovalCompletionFailsClosed(t *testing.T) {
	for _, body := range []string{`{"result":{"data":null}}`, `{"result":{"data":[]}}`, `{"result":{"data":true}}`, `{"result":{"data":{"unexpected":"private-guest-id"}}}`, `{"result":{}}`} {
		deps := rsvpTestDependencies(&memoryFilesystem{files: map[string][]byte{rsvpCredentialsPath: []byte(rsvpCredentials)}})
		writes := 0
		deps.HTTP = scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			switch request.URL.Path {
			case "/getEventInfo":
				return jsonResponse(200, compatibleRSVPEventResponse), nil
			case "/getCurrentGuest":
				return jsonResponse(200, `{"result":{"data":{"currentGuest":{"id":"private-guest-id","status":"GOING","count":1}}}}`), nil
			case "/removeGuest":
				writes++
				return jsonResponse(200, body), nil
			default:
				t.Fatal("unexpected request")
				return nil, nil
			}
		}}
		result := executeChecked(t, context.Background(), app.Request{Argv: []string{"rsvp", "remove", "event-example", "--force"}}, deps)
		if result.ExitCode != 9 || writes != 1 {
			t.Fatalf("result: %#v, writes: %d", result, writes)
		}
	}
}

func TestRSVPHelpDistinguishesDecliningFromLeaving(t *testing.T) {
	for _, tc := range []struct{ command, text string }{{"set", "Can't Go; still listed as a guest"}, {"remove", "no longer appear on your homepage"}} {
		result := executeChecked(t, context.Background(), app.Request{Argv: []string{"rsvp", tc.command, "--help"}}, app.Dependencies{})
		if result.ExitCode != 0 || !strings.Contains(result.Stdout, tc.text) {
			t.Fatalf("help: %#v", result)
		}
	}
}
