package remote

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/auth"
)

type specServer struct {
	URL string `json:"url"`
}
type specParameter struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
}
type specOperation struct {
	ID         string                `json:"operationId"`
	Servers    []specServer          `json:"servers"`
	Tags       []string              `json:"tags"`
	Parameters []specParameter       `json:"parameters"`
	Security   []map[string][]string `json:"security"`
	Evidence   struct {
		Status string `json:"status"`
		Date   string `json:"date"`
		Source string `json:"source"`
	} `json:"x-evidence"`
}
type remoteSpec struct {
	Servers    []specServer                        `json:"servers"`
	Paths      map[string]map[string]specOperation `json:"paths"`
	Components struct {
		SecuritySchemes map[string]struct {
			Type string `json:"type"`
			In   string `json:"in"`
			Name string `json:"name"`
		} `json:"securitySchemes"`
	} `json:"components"`
}

func readRemoteSpec(t *testing.T) (remoteSpec, map[string]any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "spec", "partiful.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec remoteSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return spec, document
}

func TestOpenAPIOperationsHaveEvidenceAndResolvableReferences(t *testing.T) {
	spec, document := readRemoteSpec(t)
	seen := map[string]bool{}
	for path, methods := range spec.Paths {
		for method, operation := range methods {
			if operation.ID == "" || seen[operation.ID] {
				t.Fatalf("missing or duplicate operationId at %s %s", method, path)
			}
			seen[operation.ID] = true
			evidence := operation.Evidence
			if evidence.Status != "observed" && evidence.Status != "explicit-unknown" {
				t.Fatalf("%s: invalid evidence status", operation.ID)
			}
			if _, err := time.Parse("2006-01-02", evidence.Date); err != nil {
				t.Fatalf("%s: invalid evidence date", operation.ID)
			}
			if strings.TrimSpace(evidence.Source) == "" {
				t.Fatalf("%s: missing evidence source", operation.ID)
			}
			// Evidence citations are repository-relative so a clean checkout suffices.
			for _, citation := range regexp.MustCompile(`(?:docs|spec)/[a-zA-Z0-9_./-]+\.(?:md|json)`).FindAllString(evidence.Source, -1) {
				if _, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(citation))); err != nil {
					t.Fatalf("%s: unresolved citation %s", operation.ID, citation)
				}
			}
			servers := operation.Servers
			if len(servers) == 0 {
				servers = spec.Servers
			}
			if len(servers) == 0 {
				t.Fatalf("%s: missing server", operation.ID)
			}
			if servers[0].URL == "https://firestore.googleapis.com" && !containsString(operation.Tags, "firestore") {
				t.Fatalf("%s: missing firestore tag", operation.ID)
			}
			for _, placeholder := range regexp.MustCompile(`\{([^}]+)\}`).FindAllStringSubmatch(path, -1) {
				found := false
				for _, parameter := range operation.Parameters {
					if parameter.In == "path" && parameter.Name == placeholder[1] && parameter.Required {
						found = true
					}
				}
				if !found {
					t.Fatalf("%s: undeclared path parameter %s", operation.ID, placeholder[1])
				}
			}
		}
	}
	var walk func(any)
	walk = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			if ref, ok := node["$ref"].(string); ok {
				if !strings.HasPrefix(ref, "#/") {
					t.Fatalf("unsupported reference %s", ref)
				}
				var target any = document
				for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
					part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
					object, ok := target.(map[string]any)
					if !ok {
						t.Fatalf("unresolved reference %s", ref)
					}
					target, ok = object[part]
					if !ok {
						t.Fatalf("unresolved reference %s", ref)
					}
				}
			}
			for _, child := range node {
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(document)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestRemoteCallsMatchOpenAPI(t *testing.T) {
	spec, _ := readRemoteSpec(t)
	ctx := context.Background()
	// Every request is intercepted before dispatch. The returned error stops reads,
	// pagination, and writes without contacting a server or using live credentials.
	var client Client
	var authClient AuthClient
	calls := map[string]func(){
		"getMyUpcomingEventsForHomePage": func() { _, _ = client.GetUpcomingEvents(ctx, "test-token", "test-device") },
		"getMyPastEventsForHomePage":     func() { _, _ = client.GetPastEvents(ctx, "test-token", "test-device") },
		"getEventInfo":                   func() { _, _ = client.GetEventInfo(ctx, "test-token", "test-device", "event-example") },
		"getContacts":                    func() { _, _ = client.GetContacts(ctx, "test-token", "test-device") },
		"getGuests":                      func() { _, _ = client.GetGuests(ctx, "test-token", "test-device", "event-example") },
		"getCurrentGuest":                func() { _, _ = client.GetCurrentGuest(ctx, "test-token", "test-device", "event-example") },
		"createEvent":                    func() { _, _ = client.CreateEvent(ctx, "test-token", "test-user", CreateEventParams{}) },
		"cancelEvent":                    func() { _ = client.CancelEvent(ctx, "test-token", "test-user", CancelEventParams{}) },
		"firestorePatchEvent": func() {
			_ = client.FirestorePatchEvent(ctx, "test-token", "event-example", []string{"title"}, FirestoreWriteDocument{})
		},
		"addGuest":          func() { _ = client.AddGuest(ctx, "test-token", "test-device", AddGuestParams{}) },
		"markEventInterest": func() { _ = client.MarkEventInterest(ctx, "test-token", "test-device", MarkEventInterestParams{}) },
		"addInvitedGuestsAsHost": func() {
			_ = client.InviteGuestsAsHost(ctx, "test-token", "test-user", "test-device", InviteGuestsAsHostParams{})
		},
		"createTextBlast": func() {
			_ = client.CreateTextBlast(ctx, "test-token", "test-device", "test-user", CreateTextBlastParams{})
		},
		"createCohostRequest": func() {
			_ = client.CreateCohostRequest(ctx, "test-token", "test-device", "test-user", CohostTargetParams{})
		},
		"deleteCohostRequest": func() {
			_ = client.DeleteCohostRequest(ctx, "test-token", "test-device", "test-user", CohostTargetParams{})
		},
		"removeCohost": func() { _ = client.RemoveCohost(ctx, "test-token", "test-device", "test-user", CohostTargetParams{}) },
		"generateEventCohostLink": func() {
			_, _ = client.GenerateEventCohostLink(ctx, "test-token", "test-device", "test-user", CohostLinkParams{})
		},
		"revokeEventCohostLink": func() {
			_ = client.RevokeEventCohostLink(ctx, "test-token", "test-device", "test-user", CohostLinkParams{})
		},
		"firestoreListCohostRequests": func() { _, _ = client.GetCohostRequests(ctx, "test-token", "event-example") },
		"firestoreGetCohostLink":      func() { _, _ = client.GetCohostLink(ctx, "test-token", "event-example") },
		"firestoreListEventGuests":    func() { _, _ = client.ListEventGuests(ctx, "test-token", "event-example") },
		"firestoreListHostMessages":   func() { _, _ = client.CountTextBlasts(ctx, "test-token", "event-example") },
		"getPosterCatalog":            func() { _, _ = client.GetPosterCatalog(ctx) },
		"sendAuthCodeTrusted":         func() { _ = authClient.SendAuthCode(ctx, auth.SendAuthCodeRequest{}) },
		"getLoginToken":               func() { _, _ = authClient.GetLoginToken(ctx, auth.GetLoginTokenRequest{}) },
		"signInWithCustomToken":       func() { _, _ = authClient.SignInWithCustomToken(ctx, "test-custom-token") },
		"refreshToken":                func() { _, _ = authClient.RefreshToken(ctx, "test-refresh-token") },
	}
	for id, call := range calls {
		t.Run(id, func(t *testing.T) {
			count := 0
			transport := guestHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
				count++
				matches := []string{}
				for path, methods := range spec.Paths {
					operation, ok := methods[strings.ToLower(request.Method)]
					if !ok {
						continue
					}
					pattern := "^" + regexp.MustCompile(`\\\{[^}]+\\\}`).ReplaceAllString(regexp.QuoteMeta(path), `[^/]+`) + "$"
					if !regexp.MustCompile(pattern).MatchString(request.URL.Path) {
						continue
					}
					servers := operation.Servers
					if len(servers) == 0 {
						servers = spec.Servers
					}
					if len(servers) == 0 || servers[0].URL != request.URL.Scheme+"://"+request.URL.Host {
						continue
					}
					matches = append(matches, operation.ID)
					queryNames := map[string]bool{}
					for _, parameter := range operation.Parameters {
						var present bool
						switch parameter.In {
						case "query":
							queryNames[parameter.Name] = true
							present = request.URL.Query().Has(parameter.Name)
						case "header":
							present = request.Header.Get(parameter.Name) != ""
						case "path":
							present = true
						}
						if parameter.Required && !present {
							t.Errorf("%s: missing required %s parameter %s", id, parameter.In, parameter.Name)
						}
					}
					for _, security := range operation.Security {
						for name := range security {
							scheme := spec.Components.SecuritySchemes[name]
							if scheme.Type == "apiKey" && scheme.In == "query" {
								queryNames[scheme.Name] = true
								if !request.URL.Query().Has(scheme.Name) {
									t.Errorf("%s: missing API key", id)
								}
							}
							if scheme.Type == "http" && request.Header.Get("Authorization") == "" {
								t.Errorf("%s: missing bearer header", id)
							}
						}
					}
					for name := range request.URL.Query() {
						if !queryNames[name] {
							t.Errorf("%s: query parameter absent from spec: %s", id, name)
						}
					}
				}
				if !reflect.DeepEqual(matches, []string{id}) {
					t.Errorf("%s: outbound request matches operations %v", id, matches)
				}
				return nil, errors.New("contract test: stop before remote dispatch")
			})
			client = Client{HTTP: transport}
			authClient = AuthClient{HTTP: transport}
			call()
			if count != 1 {
				t.Fatalf("%s: captured %d requests, want one", id, count)
			}
		})
	}
}
