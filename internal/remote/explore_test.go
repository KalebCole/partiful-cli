package remote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type discoverHTTPClientFunc func(*http.Request) (*http.Response, error)

func (do discoverHTTPClientFunc) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

func discoverResponse(status int, contentType, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestGetDiscoverFeedV2RequestShape(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		cursor *string
		want   string
	}{
		{"without cursor", nil, `{"data":{"params":{"area":{"type":"DYNAMIC","countryCode":"US","state":"WA","city":"Seattle"},"tagId":"DISCOVER_HOME","allowedFeedPresentationStyles":["rows"]},"paging":{"maxResults":25}}}`},
		{"with cursor", stringPointer("next-page"), `{"data":{"params":{"area":{"type":"DYNAMIC","countryCode":"US","state":"WA","city":"Seattle"},"tagId":"DISCOVER_HOME","allowedFeedPresentationStyles":["rows"]},"paging":{"maxResults":25,"cursor":"next-page"}}}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client := Client{HTTP: discoverHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodPost || request.URL.String() != partifulCallableHost+"/getDiscoverFeedV2" {
					t.Fatalf("request = %s %s", request.Method, request.URL)
				}
				if request.Header.Get("Authorization") != "Bearer test-token" || request.Header.Get("Content-Type") != "application/json" {
					t.Fatal("callable headers are wrong")
				}
				body, _ := io.ReadAll(request.Body)
				if string(body) != testCase.want {
					t.Fatalf("body = %s, want %s", body, testCase.want)
				}
				return discoverResponse(http.StatusOK, "application/json", `{"result":{"data":{"items":[]},"paging":{"nextCursor":null}}}`), nil
			})}
			page, err := client.GetDiscoverFeedV2(context.Background(), "test-token", DynamicArea{CountryCode: "US", State: "WA", City: "Seattle"}, 25, testCase.cursor)
			if err != nil || len(page.Events) != 0 || page.NextCursor != nil {
				t.Fatalf("page = %#v, err = %v", page, err)
			}
		})
	}
}

func TestGetDiscoverSectionsV2RequestShape(t *testing.T) {
	client := Client{HTTP: discoverHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != partifulCallableHost+"/getDiscoverSectionsV2" {
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer test-token" || request.Header.Get("Content-Type") != "application/json" {
			t.Fatal("callable headers are wrong")
		}
		body, _ := io.ReadAll(request.Body)
		want := `{"data":{"params":{"area":{"type":"DYNAMIC","countryCode":"US","state":"WA","city":"Seattle"},"tagId":"DISCOVER_HOME","allowedSectionPresentationStyles":["carousel-small","carousel-medium","carousel-large"],"locale":"en"},"paging":{"maxResults":10,"cursor":"1"},"userId":"test-user"}}`
		if string(body) != want {
			t.Fatalf("body = %s, want %s", body, want)
		}
		return discoverResponse(http.StatusOK, "application/json", `{"result":{"data":{"sections":[]}}}`), nil
	})}
	sections, err := client.GetDiscoverSectionsV2(context.Background(), "test-token", "test-user", DynamicArea{CountryCode: "US", State: "WA", City: "Seattle"})
	if err != nil || len(sections) != 0 {
		t.Fatalf("sections = %#v, err = %v", sections, err)
	}
}

func TestGetDiscoverEventItemDecoratorsRequestShape(t *testing.T) {
	client := Client{HTTP: discoverHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != partifulCallableHost+"/getDiscoverEventItemDecorators" {
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer test-token" || request.Header.Get("Content-Type") != "application/json" {
			t.Fatal("callable headers are wrong")
		}
		body, _ := io.ReadAll(request.Body)
		want := `{"data":{"params":{"eventIds":["event-a","event-b"]},"userId":"test-user"}}`
		if string(body) != want {
			t.Fatalf("body = %s, want %s", body, want)
		}
		return discoverResponse(http.StatusOK, "application/json", `{"result":{"data":{"decoratorsByEventId":{}}}}`), nil
	})}
	decorators, err := client.GetDiscoverEventItemDecorators(context.Background(), "test-token", "test-user", []string{"event-a", "event-b"})
	if err != nil || len(decorators.ByEventID) != 0 {
		t.Fatalf("decorators = %#v, err = %v", decorators, err)
	}
}

func TestGetDiscoverDecodesReviewedNullableAndLocationVariants(t *testing.T) {
	feed := `{"result":{"data":{"items":[{"event":{"id":"event-a","title":"Example","startDate":"2026-09-05T12:00:00Z","endDate":null,"timezone":"America/Los_Angeles","locationInfo":{"displayAddressLines":["Seattle"],"mapsInfo":{"googleMapsUrl":"https://maps.example/"}},"goingGuestCount":2,"maybeGuestCount":0,"interestedGuestCount":1},"tags":["tag-a"]}]},"paging":{"nextCursor":"next-page"}}}`
	client := Client{HTTP: discoverHTTPClientFunc(func(*http.Request) (*http.Response, error) {
		return discoverResponse(http.StatusOK, "application/json; charset=utf-8", feed), nil
	})}
	page, err := client.GetDiscoverFeedV2(context.Background(), "test-token", DynamicArea{CountryCode: "US", State: "WA", City: "Seattle"}, 1, nil)
	if err != nil || len(page.Events) != 1 || page.Events[0].Title == nil || page.Events[0].Location == nil || page.NextCursor == nil {
		t.Fatalf("page = %#v, err = %v", page, err)
	}

	for _, location := range []string{`null`, `{"displayAddressLines":[]}`, `{"mapsInfo":null}`} {
		body := `{"result":{"data":{"items":[{"event":{"id":"event-a","locationInfo":` + location + `}}]},"paging":{}}}`
		client.HTTP = discoverHTTPClientFunc(func(*http.Request) (*http.Response, error) {
			return discoverResponse(http.StatusOK, "application/json", body), nil
		})
		if _, err := client.GetDiscoverFeedV2(context.Background(), "test-token", DynamicArea{CountryCode: "US", State: "WA", City: "Seattle"}, 1, nil); err != nil {
			t.Fatalf("location %s: %v", location, err)
		}
	}
}

func TestGetDiscoverRejectsMalformedOrUnavailableResponses(t *testing.T) {
	validFeed := `{"result":{"data":{"items":[]},"paging":{}}}`
	cases := []struct {
		name, body, contentType string
		status                  int
		nilBody, transport      bool
	}{
		{"malformed envelope", `{}`, "application/json", 200, false, false},
		{"malformed item", `{"result":{"data":{"items":[{"event":{"id":""}}]},"paging":{}}}`, "application/json", 200, false, false},
		{"malformed cursor", `{"result":{"data":{"items":[]},"paging":{"nextCursor":3}}}`, "application/json", 200, false, false},
		{"fractional count", `{"result":{"data":{"items":[{"event":{"id":"event-a","goingGuestCount":1.5}}]},"paging":{}}}`, "application/json", 200, false, false},
		{"negative count", `{"result":{"data":{"items":[{"event":{"id":"event-a","goingGuestCount":-1}}]},"paging":{}}}`, "application/json", 200, false, false},
		{"malformed location", `{"result":{"data":{"items":[{"event":{"id":"event-a","locationInfo":{"displayAddressLines":[3]}}]},"paging":{}}}`, "application/json", 200, false, false},
		{"non JSON", validFeed, "text/plain", 200, false, false},
		{"non 200", validFeed, "application/json", 500, false, false},
		{"nil body", "", "application/json", 200, true, false},
		{"transport", "", "", 0, false, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := Client{HTTP: discoverHTTPClientFunc(func(*http.Request) (*http.Response, error) {
				if testCase.transport {
					return nil, errors.New("test transport")
				}
				response := discoverResponse(testCase.status, testCase.contentType, testCase.body)
				if testCase.nilBody {
					response.Body = nil
				}
				return response, nil
			})}
			_, err := client.GetDiscoverFeedV2(context.Background(), "test-token", DynamicArea{CountryCode: "US", State: "WA", City: "Seattle"}, 1, nil)
			if testCase.transport {
				if !errors.Is(err, ErrUnavailable) {
					t.Fatalf("error = %v", err)
				}
			} else if !errors.Is(err, ErrProtocolChanged) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestGetDiscoverRejectsOversizeResponse(t *testing.T) {
	client := Client{HTTP: discoverHTTPClientFunc(func(*http.Request) (*http.Response, error) {
		return discoverResponse(http.StatusOK, "application/json", strings.Repeat(" ", maximumDiscoverResponseBytes+1)), nil
	})}
	_, err := client.GetDiscoverFeedV2(context.Background(), "test-token", DynamicArea{CountryCode: "US", State: "WA", City: "Seattle"}, 1, nil)
	if !errors.Is(err, ErrProtocolChanged) {
		t.Fatalf("error = %v, want protocol changed", err)
	}
}

func TestGetDiscoverSectionsAndDecoratorsBoundaries(t *testing.T) {
	area := DynamicArea{CountryCode: "US", State: "WA", City: "Seattle"}
	sectionBodies := []string{
		`{"result":{"data":{"sections":[]}}}`,
		`{"result":{"data":{"sections":[{"id":"area-dynamic-trending-events","items":[]}]}}}`,
	}
	for _, body := range sectionBodies {
		client := Client{HTTP: discoverHTTPClientFunc(func(*http.Request) (*http.Response, error) {
			return discoverResponse(200, "application/json", body), nil
		})}
		if _, err := client.GetDiscoverSectionsV2(context.Background(), "test-token", "test-user", area); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{
		`{"result":{"data":{"decoratorsByEventId":{}}}}`,
		`{"result":{"data":{"decoratorsByEventId":{"event-a":{"mutualGuests":[{"name":"Example","status":"GOING"}]}}}}}`,
	} {
		client := Client{HTTP: discoverHTTPClientFunc(func(*http.Request) (*http.Response, error) {
			return discoverResponse(200, "application/json", body), nil
		})}
		if _, err := client.GetDiscoverEventItemDecorators(context.Background(), "test-token", "test-user", []string{"event-a"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{
		`{"result":{"data":{"sections":[{"id":"unknown","items":[]}]}}}`,
		`{"result":{"data":{"sections":[{"id":"area-dynamic-trending-events","items":"wrong"}]}}}`,
		`{"result":{"data":{"decoratorsByEventId":{"event-a":{}}}}}`,
		`{"result":{"data":{"decoratorsByEventId":{"event-a":{"mutualGuests":[{"status":"UNKNOWN"}]}}}}}`,
	} {
		client := Client{HTTP: discoverHTTPClientFunc(func(*http.Request) (*http.Response, error) {
			return discoverResponse(200, "application/json", body), nil
		})}
		if strings.Contains(body, "sections") {
			_, err := client.GetDiscoverSectionsV2(context.Background(), "test-token", "test-user", area)
			if !errors.Is(err, ErrProtocolChanged) {
				t.Fatalf("error = %v", err)
			}
		} else {
			_, err := client.GetDiscoverEventItemDecorators(context.Background(), "test-token", "test-user", []string{"event-a"})
			if !errors.Is(err, ErrProtocolChanged) {
				t.Fatalf("error = %v", err)
			}
		}
	}
}

func TestGetDiscoverRejectsBounds(t *testing.T) {
	area := DynamicArea{CountryCode: "US", State: "WA", City: "Seattle"}
	client := Client{}
	if _, err := client.GetDiscoverFeedV2(context.Background(), "test-token", area, 101, nil); !errors.Is(err, ErrProtocolChanged) {
		t.Fatalf("feed bound error = %v", err)
	}
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "event-a"
	}
	if _, err := client.GetDiscoverEventItemDecorators(context.Background(), "test-token", "test-user", ids); !errors.Is(err, ErrProtocolChanged) {
		t.Fatalf("decorator bound error = %v", err)
	}
}

func stringPointer(value string) *string { return &value }
