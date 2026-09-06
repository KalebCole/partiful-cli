package app_test

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func TestExecuteExploreListValidatesAreaBeforeAuthentication(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{Argv: []string{"explore", "list", "--city", "Seattle", "--state", "WA"}}, app.Dependencies{HTTP: scriptedHTTP{do: func(*http.Request) (*http.Response, error) { t.Fatal("network called"); return nil, nil }}})
	if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"code":"AREA_REQUIRED"`) {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteExploreListProjectsReviewedPublicFields(t *testing.T) {
	credentials := `{"accessToken":"private-token","userId":"private-user","expiresAt":"2026-09-06T00:00:00Z"}`
	calls := 0
	result := app.Execute(context.Background(), app.Request{Argv: []string{"explore", "list", "--city", "Seattle", "--state", "WA", "--country", "US"}}, app.Dependencies{
		Files: fakeFilesystem{readFile: func(string) ([]byte, error) { return []byte(credentials), nil }}, CredentialsPath: "/credentials",
		Now: func() time.Time { return time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC) },
		HTTP: scriptedHTTP{do: func(request *http.Request) (*http.Response, error) {
			calls++
			body, _ := io.ReadAll(request.Body)
			switch request.URL.Path {
			case "/getDiscoverFeedV2":
				if !strings.Contains(string(body), `"countryCode":"US"`) {
					t.Fatalf("area request = %s", body)
				}
				return jsonResponse(200, `{"result":{"data":{"items":[{"event":{"id":"event-public","title":"Example","startDate":"2026-09-05T19:00:00-07:00","endDate":null,"timezone":"America/Los_Angeles","locationInfo":{"displayAddressLines":["Seattle"],"mapsInfo":{"googleMapsUrl":"https://maps.example/"}},"goingGuestCount":2}}]},"paging":{"nextCursor":null}}}`), nil
			case "/getDiscoverEventItemDecorators":
				return jsonResponse(200, `{"result":{"data":{"decoratorsByEventId":{"event-public":{"mutualGuests":[{"name":"Friend","status":"GOING"}]}}}}}`), nil
			default:
				t.Fatalf("unexpected request %s", request.URL)
				return nil, nil
			}
		}},
	})
	var output struct {
		Data struct {
			Items []struct {
				EventID, URL  string
				Location []string `json:"location"`
				MapsURL *string `json:"mapsUrl"`
				GoingCount *int `json:"goingCount"`
				Mutuals       []struct {
					DisplayName *string `json:"displayName"`
					Status      *string `json:"status"`
				} `json:"mutuals"`
				MutualsPreviewComplete any `json:"mutualsPreviewComplete"`
			} `json:"items"`
		} `json:"data"`
		Meta struct {
			Page struct {
				FilterComplete *bool `json:"filterComplete"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &output); err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || calls != 2 || len(output.Data.Items) != 1 || output.Data.Items[0].EventID != "event-public" || output.Data.Items[0].URL != "https://partiful.com/e/event-public" || len(output.Data.Items[0].Location) != 1 || output.Data.Items[0].Location[0] != "Seattle" || output.Data.Items[0].MapsURL == nil || *output.Data.Items[0].MapsURL != "https://maps.example/" || output.Data.Items[0].GoingCount == nil || *output.Data.Items[0].GoingCount != 2 || output.Data.Items[0].Mutuals[0].Status == nil || *output.Data.Items[0].Mutuals[0].Status != "going" || output.Data.Items[0].MutualsPreviewComplete != nil || output.Meta.Page.FilterComplete == nil || !*output.Meta.Page.FilterComplete {
		t.Fatalf("result=%#v output=%#v", result, output)
	}
	for _, private := range []string{"private-token", "private-user"} {
		if strings.Contains(result.Stdout+result.Stderr, private) {
			t.Fatalf("private output %q", private)
		}
	}
}

func TestExecuteExploreListRequiresAuthentication(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{Argv: []string{"explore", "list", "--city", "Seattle", "--state", "WA", "--country", "US"}}, app.Dependencies{Files: fakeFilesystem{readFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist }}, CredentialsPath: "/credentials"})
	if result.ExitCode != 3 || !strings.Contains(result.Stdout, `"type":"auth.required"`) {
		t.Fatalf("result=%#v", result)
	}
}
