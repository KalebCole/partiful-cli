// PROTOTYPE: authenticated, read-only Partiful Explore data-model probe.
// This is intentionally outside the production command catalog.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/KalebCole/partiful-cli/internal/auth"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

const apiBase = "https://api.partiful.com"

type mutualPreview struct {
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
}

type eventOutput struct {
	EventID                string          `json:"eventId"`
	URL                    string          `json:"url"`
	Title                  string          `json:"title"`
	Start                  string          `json:"start"`
	End                    *string         `json:"end"`
	Timezone               string          `json:"timezone"`
	Location               []string        `json:"location"`
	MapsURL                *string         `json:"mapsUrl"`
	GoingCount             *int            `json:"goingCount"`
	MaybeCount             *int            `json:"maybeCount"`
	InterestedCount        *int            `json:"interestedCount"`
	Tags                   []string        `json:"tags"`
	Mutuals                []mutualPreview `json:"mutuals"`
	MutualsPreviewComplete *bool           `json:"mutualsPreviewComplete"`
}

type output struct {
	Prototype         bool           `json:"prototype"`
	ReadOnly          bool           `json:"readOnly"`
	Area              map[string]any `json:"area"`
	Tag               string         `json:"tag"`
	Section           string         `json:"section"`
	DateFilter        string         `json:"dateFilter"`
	PagesRead         int            `json:"pagesRead"`
	HasMore           bool           `json:"hasMore"`
	FetchedEventCount int            `json:"fetchedEventCount"`
	FilterComplete    bool           `json:"filterComplete"`
	Events            []eventOutput  `json:"events"`
	Verdict           []string       `json:"verdict"`
}

func main() {
	city := flag.String("city", "Seattle", "Dynamic Explore city")
	state := flag.String("state", "Washington", "Dynamic Explore state")
	countryCode := flag.String("country", "US", "Dynamic Explore country code")
	tag := flag.String("tag", "DISCOVER_HOME", "Explore tag ID")
	section := flag.String("section", "", "trending, friends, open-invite, or followed")
	dateFilter := flag.String("date-filter", "anytime", "anytime, today, tomorrow, this-week, or this-weekend")
	maxResults := flag.Int("max-results", 20, "Requested page size")
	pages := flag.Int("pages", 1, "Number of feed pages to read")
	flag.Parse()
	if *pages < 1 || *pages > 5 || *maxResults < 1 || *maxResults > 100 {
		fatal("pages must be 1-5 and max-results must be 1-100")
	}
	validDateFilters := map[string]bool{"anytime": true, "today": true, "tomorrow": true, "this-week": true, "this-weekend": true}
	if !validDateFilters[*dateFilter] {
		fatal("date-filter must be anytime, today, tomorrow, this-week, or this-weekend")
	}
	sectionIDs := map[string]string{
		"trending":    "area-dynamic-trending-events",
		"friends":     "area-mutual-events",
		"open-invite": "area-open-invite",
		"followed":    "area-followed-events",
	}
	if *section != "" {
		if _, ok := sectionIDs[*section]; !ok {
			fatal("section must be trending, friends, open-invite, or followed")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	credentialsPath, err := auth.DefaultCredentialsPath()
	if err != nil {
		fatal("credential path unavailable")
	}
	httpClient := remote.NewHTTPClient(nil)
	session, err := auth.AcquireSession(ctx, auth.OSFileSystem{}, credentialsPath, time.Now, remote.AuthClient{HTTP: httpClient})
	if err != nil {
		fatal("authenticated session unavailable: " + err.Error())
	}

	area := map[string]any{
		"type":        "DYNAMIC",
		"countryCode": *countryCode,
		"state":       *state,
		"city":        *city,
	}

	events := make([]eventOutput, 0)
	var cursor any
	pagesRead := 0
	if *section != "" {
		document := post(ctx, httpClient, session.AccessToken, "/getDiscoverSectionsV2", map[string]any{"data": map[string]any{
			"params": map[string]any{
				"area": area, "tagId": *tag, "locale": "en",
				"allowedSectionPresentationStyles": []string{"carousel-small", "carousel-medium", "carousel-large"},
			},
			"paging": map[string]any{"maxResults": 10, "cursor": "1"},
			"userId": session.UserID,
		}})
		data := object(object(document, "result"), "data")
		for _, sectionValue := range array(data, "sections") {
			remoteSection, _ := sectionValue.(map[string]any)
			if remoteSection["id"] == sectionIDs[*section] {
				events = appendDiscoverItems(events, array(remoteSection, "items"))
			}
		}
		pagesRead = 1
	} else {
		for page := 0; page < *pages; page++ {
			paging := map[string]any{"maxResults": *maxResults}
			if cursor != nil {
				paging["cursor"] = cursor
			}
			document := post(ctx, httpClient, session.AccessToken, "/getDiscoverFeedV2", map[string]any{"data": map[string]any{
				"params": map[string]any{
					"area": area, "tagId": *tag, "allowedFeedPresentationStyles": []string{"rows"},
				},
				"paging": paging,
			}})
			result := object(document, "result")
			events = appendDiscoverItems(events, array(object(result, "data"), "items"))
			pagesRead++
			cursor = object(result, "paging")["nextCursor"]
			if cursor == nil || cursor == "" {
				break
			}
		}
	}

	eventIDs := make([]string, 0, len(events))
	for _, event := range events {
		if event.EventID != "" {
			eventIDs = append(eventIDs, event.EventID)
		}
	}
	if len(eventIDs) > 0 {
		decoratorDocument := post(ctx, httpClient, session.AccessToken, "/getDiscoverEventItemDecorators", map[string]any{"data": map[string]any{
			"params": map[string]any{"eventIds": eventIDs},
			"userId": session.UserID,
		}})
		decorators := object(object(object(decoratorDocument, "result"), "data"), "decoratorsByEventId")
		for index := range events {
			decorator, ok := decorators[events[index].EventID].(map[string]any)
			if !ok {
				continue
			}
			for _, guestValue := range array(decorator, "mutualGuests") {
				guest, ok := guestValue.(map[string]any)
				if !ok {
					continue
				}
				name, _ := guest["name"].(string)
				if name == "" {
					if user, ok := guest["user"].(map[string]any); ok {
						name, _ = user["name"].(string)
					}
				}
				status, _ := guest["status"].(string)
				if name != "" {
					events[index].Mutuals = append(events[index].Mutuals, mutualPreview{DisplayName: name, Status: strings.ToLower(status)})
				}
			}
		}
	}

	fetchedEventCount := len(events)
	events = filterEventsByDateFacet(events, *dateFilter, time.Now())
	result := output{
		Prototype:         true,
		ReadOnly:          true,
		Area:              area,
		Tag:               *tag,
		Section:           *section,
		DateFilter:        *dateFilter,
		PagesRead:         pagesRead,
		HasMore:           cursor != nil && cursor != "",
		FetchedEventCount: fetchedEventCount,
		FilterComplete:    *dateFilter == "anytime" || cursor == nil || cursor == "",
		Events:            events,
		Verdict: []string{
			"Seattle uses the getDiscoverFeedV2 callable with a DYNAMIC area object.",
			"The feed supports cursor pagination and tagId filtering.",
			"Date facets are client-side overlap filters over event-local calendar dates.",
			"Seattle social discovery modes are generated sections selected from getDiscoverSectionsV2.",
			"Authenticated getDiscoverEventItemDecorators returns mutual guest names and RSVP statuses.",
			"Decorator output is a preview; completeness is not established.",
		},
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fatal("encode output")
	}
}

func appendDiscoverItems(events []eventOutput, items []any) []eventOutput {
	for _, itemValue := range items {
		item, ok := itemValue.(map[string]any)
		if !ok {
			continue
		}
		event, ok := item["event"].(map[string]any)
		if !ok {
			continue
		}
		events = append(events, normalizeEvent(event, stringArray(item["tags"])))
	}
	return events
}

func filterEventsByDateFacet(events []eventOutput, facet string, now time.Time) []eventOutput {
	if facet == "anytime" {
		return events
	}
	filtered := make([]eventOutput, 0)
	for _, event := range events {
		start, err := time.Parse(time.RFC3339, event.Start)
		if err != nil || event.Start == "TBD" {
			continue
		}
		location, err := time.LoadLocation(event.Timezone)
		if err != nil {
			continue
		}
		localNow := now.In(location)
		from, to := dateFacetRange(facet, localNow)
		localStart := start.In(location)
		localEnd := localStart
		if event.End != nil {
			if parsedEnd, err := time.Parse(time.RFC3339, *event.End); err == nil {
				localEnd = parsedEnd.In(location)
			}
		}
		startKey := localStart.Format("2006-01-02")
		endKey := localEnd.Format("2006-01-02")
		if endKey >= from.Format("2006-01-02") && startKey <= to.Format("2006-01-02") {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func dateFacetRange(facet string, now time.Time) (time.Time, time.Time) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch facet {
	case "today":
		return today, today
	case "tomorrow":
		tomorrow := today.AddDate(0, 0, 1)
		return tomorrow, tomorrow
	case "this-week", "this-weekend":
		daysSinceMonday := (int(today.Weekday()) + 6) % 7
		monday := today.AddDate(0, 0, -daysSinceMonday)
		from := monday
		if facet == "this-weekend" {
			from = monday.AddDate(0, 0, 5)
		}
		if today.After(from) {
			from = today
		}
		return from, monday.AddDate(0, 0, 6)
	default:
		return today, today
	}
}

func post(ctx context.Context, client *http.Client, accessToken, endpoint string, body map[string]any) map[string]any {
	payload, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+endpoint, bytes.NewReader(payload))
	if err != nil {
		fatal("request construction failed")
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://partiful.com")
	request.Header.Set("Referer", "https://partiful.com/")
	response, err := client.Do(request)
	if err != nil {
		fatal(endpoint + " request failed")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		fatal(endpoint + " response read failed")
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		fatal(endpoint + " returned non-JSON")
	}
	if response.StatusCode != http.StatusOK {
		fatal(endpoint + " rejected the read-only prototype: " + safeRemoteError(document))
	}
	return document
}

func normalizeEvent(event map[string]any, tags []string) eventOutput {
	id, _ := event["id"].(string)
	title, _ := event["title"].(string)
	start, _ := event["startDate"].(string)
	timezone, _ := event["timezone"].(string)
	result := eventOutput{
		EventID:                id,
		URL:                    "https://partiful.com/e/" + id,
		Title:                  title,
		Start:                  start,
		Timezone:               timezone,
		Location:               []string{},
		GoingCount:             optionalInt(event["goingGuestCount"]),
		MaybeCount:             optionalInt(event["maybeGuestCount"]),
		InterestedCount:        optionalInt(event["interestedGuestCount"]),
		Tags:                   tags,
		Mutuals:                []mutualPreview{},
		MutualsPreviewComplete: nil,
	}
	if end, ok := event["endDate"].(string); ok {
		result.End = &end
	}
	if location, ok := event["locationInfo"].(map[string]any); ok {
		for _, line := range array(location, "displayAddressLines") {
			if value, ok := line.(string); ok {
				result.Location = append(result.Location, value)
			}
		}
		if mapsInfo, ok := location["mapsInfo"].(map[string]any); ok {
			if value, ok := mapsInfo["googleMapsUrl"].(string); ok {
				result.MapsURL = &value
			}
		}
	}
	return result
}

func stringArray(value any) []string {
	result := make([]string, 0)
	values, _ := value.([]any)
	for _, item := range values {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func object(parent map[string]any, key string) map[string]any {
	value, ok := parent[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return value
}

func array(parent map[string]any, key string) []any {
	value, _ := parent[key].([]any)
	return value
}

func optionalInt(value any) *int {
	number, ok := value.(float64)
	if !ok {
		return nil
	}
	converted := int(number)
	return &converted
}

func safeRemoteError(document map[string]any) string {
	remoteError := object(document, "error")
	status, _ := remoteError["status"].(string)
	message, _ := remoteError["message"].(string)
	if status == "" && message == "" {
		return "remote request rejected"
	}
	return status + ": " + message
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "prototype:", message)
	os.Exit(1)
}
