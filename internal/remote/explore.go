package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	maximumDiscoverItems         = 100
	maximumDiscoverResponseBytes = 8 << 20
)

type DynamicArea struct {
	CountryCode string
	State       string
	City        string
}

type DiscoverLocation struct {
	DisplayAddressLines []string
	GoogleMapsURL       *string
}

type DiscoverEvent struct {
	ID                   string
	Title                *string
	Start                *string
	End                  *string
	Timezone             *string
	Location             *DiscoverLocation
	GoingGuestCount      *int
	MaybeGuestCount      *int
	InterestedGuestCount *int
	Tags                 []string
}

type DiscoverPage struct {
	Events     []DiscoverEvent
	NextCursor *string
}

type DiscoverSection struct {
	ID     string
	Events []DiscoverEvent
}

type DiscoverMutualGuest struct {
	Name   *string
	Status string
}

type DiscoverDecorators struct {
	ByEventID map[string][]DiscoverMutualGuest
}

type discoverFeedRequest struct {
	Data struct {
		Params struct {
			Area                          discoverArea `json:"area"`
			TagID                         string       `json:"tagId"`
			AllowedFeedPresentationStyles []string     `json:"allowedFeedPresentationStyles"`
		} `json:"params"`
		Paging struct {
			MaxResults int     `json:"maxResults"`
			Cursor     *string `json:"cursor,omitempty"`
		} `json:"paging"`
	} `json:"data"`
}

type discoverSectionsRequest struct {
	Data struct {
		Params struct {
			Area                             discoverArea `json:"area"`
			TagID                            string       `json:"tagId"`
			AllowedSectionPresentationStyles []string     `json:"allowedSectionPresentationStyles"`
			Locale                           string       `json:"locale"`
		} `json:"params"`
		Paging struct {
			MaxResults int    `json:"maxResults"`
			Cursor     string `json:"cursor"`
		} `json:"paging"`
		UserID string `json:"userId"`
	} `json:"data"`
}

type discoverDecoratorsRequest struct {
	Data struct {
		Params struct {
			EventIDs []string `json:"eventIds"`
		} `json:"params"`
		UserID string `json:"userId"`
	} `json:"data"`
}

type discoverArea struct {
	Type        string `json:"type"`
	CountryCode string `json:"countryCode"`
	State       string `json:"state"`
	City        string `json:"city"`
}

func (client Client) GetDiscoverFeedV2(ctx context.Context, accessToken string, area DynamicArea, maxResults int, cursor *string) (DiscoverPage, error) {
	if maxResults < 1 || maxResults > maximumDiscoverItems || !validDiscoverArea(area) || cursor != nil && strings.TrimSpace(*cursor) == "" {
		return DiscoverPage{}, fmt.Errorf("%w: discover feed parameters", ErrProtocolChanged)
	}
	var payload discoverFeedRequest
	payload.Data.Params.Area = discoverArea{Type: "DYNAMIC", CountryCode: area.CountryCode, State: area.State, City: area.City}
	payload.Data.Params.TagID = "DISCOVER_HOME"
	payload.Data.Params.AllowedFeedPresentationStyles = []string{"rows"}
	payload.Data.Paging.MaxResults, payload.Data.Paging.Cursor = maxResults, cursor
	body, err := client.callDiscover(ctx, accessToken, "getDiscoverFeedV2", payload)
	if err != nil {
		return DiscoverPage{}, err
	}
	root, err := decodeEventObject(body)
	if err != nil || !discoverClosedObject(root, "result") {
		return DiscoverPage{}, discoverProtocol("feed response")
	}
	result, err := eventObjectField(root, "result")
	if err != nil || !discoverClosedObject(result, "data", "paging") {
		return DiscoverPage{}, discoverProtocol("feed result")
	}
	data, err := eventObjectField(result, "data")
	if err != nil || !discoverClosedObject(data, "items", "presentationStyle") {
		return DiscoverPage{}, discoverProtocol("feed data")
	}
	paging, err := eventObjectField(result, "paging")
	if err != nil || !discoverClosedObject(paging, "nextCursor", "pageResultCount") {
		return DiscoverPage{}, discoverProtocol("feed paging")
	}
	if _, _, err := eventStringField(data, "presentationStyle", false); err != nil {
		return DiscoverPage{}, discoverProtocol("feed presentation style")
	}
	if _, err := discoverOptionalCount(paging, "pageResultCount"); err != nil {
		return DiscoverPage{}, discoverProtocol("feed page result count")
	}
	items, err := discoverItems(data, "items")
	if err != nil {
		return DiscoverPage{}, discoverProtocol("feed items")
	}
	next, err := discoverCursor(paging)
	if err != nil {
		return DiscoverPage{}, discoverProtocol("feed cursor")
	}
	return DiscoverPage{Events: items, NextCursor: next}, nil
}

func (client Client) GetDiscoverSectionsV2(ctx context.Context, accessToken, userID string, area DynamicArea) ([]DiscoverSection, error) {
	if !validDiscoverArea(area) || strings.TrimSpace(userID) == "" {
		return nil, discoverProtocol("sections parameters")
	}
	var payload discoverSectionsRequest
	payload.Data.Params.Area = discoverArea{Type: "DYNAMIC", CountryCode: area.CountryCode, State: area.State, City: area.City}
	payload.Data.Params.TagID, payload.Data.Params.Locale = "DISCOVER_HOME", "en"
	payload.Data.Params.AllowedSectionPresentationStyles = []string{"carousel-small", "carousel-medium", "carousel-large"}
	payload.Data.Paging.MaxResults, payload.Data.Paging.Cursor, payload.Data.UserID = 10, "1", userID
	body, err := client.callDiscover(ctx, accessToken, "getDiscoverSectionsV2", payload)
	if err != nil {
		return nil, err
	}
	root, err := decodeEventObject(body)
	if err != nil || !discoverClosedObject(root, "result") {
		return nil, discoverProtocol("sections response")
	}
	result, err := eventObjectField(root, "result")
	if err != nil || !discoverClosedObject(result, "data") {
		return nil, discoverProtocol("sections result")
	}
	data, err := eventObjectField(result, "data")
	if err != nil || !discoverClosedObject(data, "sections") {
		return nil, discoverProtocol("sections data")
	}
	raw, ok := data["sections"]
	if !ok || !isEventJSONKind(raw, '[') {
		return nil, discoverProtocol("sections value")
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil || len(values) > maximumDiscoverItems {
		return nil, discoverProtocol("sections value")
	}
	sections := make([]DiscoverSection, 0, len(values))
	for _, rawSection := range values {
		section, err := decodeDiscoverSection(rawSection)
		if err != nil {
			return nil, discoverProtocol("section")
		}
		sections = append(sections, section)
	}
	return sections, nil
}

func (client Client) GetDiscoverEventItemDecorators(ctx context.Context, accessToken, userID string, eventIDs []string) (DiscoverDecorators, error) {
	if strings.TrimSpace(userID) == "" || len(eventIDs) == 0 || len(eventIDs) > maximumDiscoverItems {
		return DiscoverDecorators{}, discoverProtocol("decorator parameters")
	}
	for _, id := range eventIDs {
		if strings.TrimSpace(id) == "" {
			return DiscoverDecorators{}, discoverProtocol("decorator parameters")
		}
	}
	var payload discoverDecoratorsRequest
	payload.Data.Params.EventIDs, payload.Data.UserID = eventIDs, userID
	body, err := client.callDiscover(ctx, accessToken, "getDiscoverEventItemDecorators", payload)
	if err != nil {
		return DiscoverDecorators{}, err
	}
	root, err := decodeEventObject(body)
	if err != nil || !discoverClosedObject(root, "result") {
		return DiscoverDecorators{}, discoverProtocol("decorator response")
	}
	result, err := eventObjectField(root, "result")
	if err != nil || !discoverClosedObject(result, "data") {
		return DiscoverDecorators{}, discoverProtocol("decorator result")
	}
	data, err := eventObjectField(result, "data")
	if err != nil || !discoverClosedObject(data, "decoratorsByEventId") {
		return DiscoverDecorators{}, discoverProtocol("decorator data")
	}
	raw, ok := data["decoratorsByEventId"]
	if !ok {
		return DiscoverDecorators{}, discoverProtocol("decorator value")
	}
	objects, err := decodeEventObject(raw)
	if err != nil {
		return DiscoverDecorators{}, discoverProtocol("decorator value")
	}
	decorators := DiscoverDecorators{ByEventID: make(map[string][]DiscoverMutualGuest, len(objects))}
	for id, rawDecorator := range objects {
		guests, err := decodeDiscoverDecorator(rawDecorator)
		if err != nil {
			return DiscoverDecorators{}, discoverProtocol("decorator")
		}
		decorators.ByEventID[id] = guests
	}
	return decorators, nil
}

func (client Client) callDiscover(ctx context.Context, accessToken, operation string, payload any) ([]byte, error) {
	if client.HTTP == nil {
		return nil, fmt.Errorf("%w: discover transport", ErrUnavailable)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: discover request", ErrUnavailable)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, partifulCallableHost+"/"+operation, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("%w: discover request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: discover request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return nil, discoverProtocol("response")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !eventJSONContentType(response.Header.Get("Content-Type")) {
		return nil, discoverProtocol("response")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumDiscoverResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: discover response read", ErrUnavailable)
	}
	if len(body) > maximumDiscoverResponseBytes || !utf8.Valid(body) {
		return nil, discoverProtocol("response")
	}
	return body, nil
}

func discoverItems(object map[string]json.RawMessage, field string) ([]DiscoverEvent, error) {
	raw, ok := object[field]
	if !ok || !isEventJSONKind(raw, '[') {
		return nil, errors.New("items")
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil || len(values) > maximumDiscoverItems {
		return nil, errors.New("items")
	}
	events := make([]DiscoverEvent, 0, len(values))
	for _, value := range values {
		event, err := decodeDiscoverItem(value)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func decodeDiscoverSection(raw json.RawMessage) (DiscoverSection, error) {
	object, err := decodeEventObject(raw)
	if err != nil {
		return DiscoverSection{}, err
	}
	id, present, err := eventStringField(object, "id", false)
	if err != nil || !present || !validDiscoverSectionID(*id) {
		return DiscoverSection{}, errors.New("section id")
	}
	events, err := discoverItems(object, "items")
	if err != nil {
		return DiscoverSection{}, err
	}
	return DiscoverSection{ID: *id, Events: events}, nil
}

func decodeDiscoverItem(raw json.RawMessage) (DiscoverEvent, error) {
	item, err := decodeEventObject(raw)
	if err != nil {
		return DiscoverEvent{}, err
	}
	rawEvent, ok := item["event"]
	if !ok {
		return DiscoverEvent{}, errors.New("event")
	}
	event, err := decodeEventObject(rawEvent)
	if err != nil {
		return DiscoverEvent{}, err
	}
	id, present, err := eventStringField(event, "id", false)
	if err != nil || !present || strings.TrimSpace(*id) == "" {
		return DiscoverEvent{}, errors.New("id")
	}
	title, _, err := eventStringField(event, "title", false)
	if err != nil {
		return DiscoverEvent{}, err
	}
	start, _, err := eventStringField(event, "startDate", false)
	if err != nil {
		return DiscoverEvent{}, err
	}
	end, _, err := eventStringField(event, "endDate", true)
	if err != nil {
		return DiscoverEvent{}, err
	}
	timezone, _, err := eventStringField(event, "timezone", false)
	if err != nil {
		return DiscoverEvent{}, err
	}
	location, err := decodeDiscoverLocation(event)
	if err != nil {
		return DiscoverEvent{}, err
	}
	going, err := discoverOptionalCount(event, "goingGuestCount")
	if err != nil {
		return DiscoverEvent{}, err
	}
	maybe, err := discoverOptionalCount(event, "maybeGuestCount")
	if err != nil {
		return DiscoverEvent{}, err
	}
	interested, err := discoverOptionalCount(event, "interestedGuestCount")
	if err != nil {
		return DiscoverEvent{}, err
	}
	var tags []string
	if rawTags, ok := item["tags"]; ok {
		if !isEventJSONKind(rawTags, '[') || json.Unmarshal(rawTags, &tags) != nil || tags == nil {
			return DiscoverEvent{}, errors.New("tags")
		}
	}
	return DiscoverEvent{ID: *id, Title: title, Start: start, End: end, Timezone: timezone, Location: location, GoingGuestCount: going, MaybeGuestCount: maybe, InterestedGuestCount: interested, Tags: tags}, nil
}

func decodeDiscoverLocation(event map[string]json.RawMessage) (*DiscoverLocation, error) {
	raw, ok := event["locationInfo"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	object, err := decodeEventObject(raw)
	if err != nil {
		return nil, err
	}
	location := &DiscoverLocation{}
	if lines, ok := object["displayAddressLines"]; ok {
		if !isEventJSONKind(lines, '[') || json.Unmarshal(lines, &location.DisplayAddressLines) != nil || location.DisplayAddressLines == nil {
			return nil, errors.New("address")
		}
	}
	if maps, ok := object["mapsInfo"]; ok && !bytes.Equal(bytes.TrimSpace(maps), []byte("null")) {
		mapObject, err := decodeEventObject(maps)
		if err != nil {
			return nil, err
		}
		url, _, err := eventStringField(mapObject, "googleMapsUrl", false)
		if err != nil {
			return nil, err
		}
		location.GoogleMapsURL = url
	}
	return location, nil
}

func discoverOptionalCount(object map[string]json.RawMessage, field string) (*int, error) {
	raw, ok := object[field]
	if !ok {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, errors.New("count")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("count")
	}
	number, ok := value.(json.Number)
	if !ok {
		return nil, errors.New("count")
	}
	parsed, err := number.Int64()
	if err != nil || parsed < 0 || parsed > int64(math.MaxInt) {
		return nil, errors.New("count")
	}
	integer := int(parsed)
	return &integer, nil
}

func discoverCursor(paging map[string]json.RawMessage) (*string, error) {
	raw, ok := paging["nextCursor"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var cursor string
	if decodeSingleDiscoverJSON(raw, &cursor) != nil || strings.TrimSpace(cursor) == "" {
		return nil, errors.New("cursor")
	}
	return &cursor, nil
}

func decodeDiscoverDecorator(raw json.RawMessage) ([]DiscoverMutualGuest, error) {
	object, err := decodeEventObject(raw)
	if err != nil {
		return nil, err
	}
	rawGuests, ok := object["mutualGuests"]
	if !ok || !isEventJSONKind(rawGuests, '[') {
		return nil, errors.New("guests")
	}
	var values []json.RawMessage
	if json.Unmarshal(rawGuests, &values) != nil || values == nil || len(values) > maximumDiscoverItems {
		return nil, errors.New("guests")
	}
	guests := make([]DiscoverMutualGuest, 0, len(values))
	for _, rawGuest := range values {
		guest, err := decodeEventObject(rawGuest)
		if err != nil {
			return nil, err
		}
		status, present, err := eventStringField(guest, "status", false)
		if err != nil || !present || !validGuestStatus(*status) {
			return nil, errors.New("status")
		}
		name, _, err := eventStringField(guest, "name", false)
		if err != nil {
			return nil, err
		}
		if rawUser, ok := guest["user"]; ok {
			user, err := decodeEventObject(rawUser)
			if err != nil {
				return nil, err
			}
			userName, _, err := eventStringField(user, "name", false)
			if err != nil {
				return nil, err
			}
			if name == nil {
				name = userName
			}
		}
		guests = append(guests, DiscoverMutualGuest{Name: name, Status: *status})
	}
	return guests, nil
}

func decodeSingleDiscoverJSON(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

func discoverClosedObject(object map[string]json.RawMessage, fields ...string) bool {
	for key := range object {
		allowed := false
		for _, field := range fields {
			if key == field {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return true
}

func validDiscoverArea(area DynamicArea) bool {
	return strings.TrimSpace(area.CountryCode) != "" && strings.TrimSpace(area.State) != "" && strings.TrimSpace(area.City) != ""
}
func validDiscoverSectionID(value string) bool {
	switch value {
	case "area-dynamic-trending-events", "area-mutual-events", "area-open-invite", "area-followed-events":
		return true
	}
	return false
}
func discoverProtocol(scope string) error {
	return fmt.Errorf("%w: discover %s", ErrProtocolChanged, scope)
}
