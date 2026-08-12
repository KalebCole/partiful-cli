package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"
	"unicode/utf8"
)

const (
	firestoreDocumentsHost     = "https://firestore.googleapis.com"
	maximumEventWriteBodyBytes = 1 << 20
)

var firestoreIntegerPattern = regexp.MustCompile(`^-?[0-9]+$`)

type PartifulPoster struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	BlurHash    *string  `json:"blurHash,omitempty"`
	ContentType string   `json:"contentType"`
	Height      *int     `json:"height"`
	Width       *int     `json:"width"`
	Tags        []string `json:"tags"`
	Categories  []string `json:"categories"`
}

type PartifulPosterImage struct {
	Source      string         `json:"source"`
	Poster      PartifulPoster `json:"poster"`
	URL         string         `json:"url"`
	BlurHash    *string        `json:"blurHash"`
	ContentType string         `json:"contentType"`
	Name        string         `json:"name"`
	Height      *int           `json:"height"`
	Width       *int           `json:"width"`
}

type EventDisplaySettings struct {
	Theme     string `json:"theme"`
	Effect    string `json:"effect"`
	TitleFont string `json:"titleFont"`
}

type EventLocationInfo struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type EventCustomField struct {
	Icon  string `json:"icon"`
	Value string `json:"value"`
	URL   string `json:"url"`
}

type CreateEventDraft struct {
	Title                      string               `json:"title"`
	StartDate                  string               `json:"startDate"`
	EndDate                    *string              `json:"endDate,omitempty"`
	Timezone                   string               `json:"timezone"`
	GuestStatusCounts          map[string]int       `json:"guestStatusCounts"`
	DisplaySettings            EventDisplaySettings `json:"displaySettings"`
	Status                     string               `json:"status"`
	RSVPButtonGlyphType        string               `json:"rsvpButtonGlyphType"`
	Image                      PartifulPosterImage  `json:"image"`
	ShowHostList               bool                 `json:"showHostList"`
	ShowGuestCount             bool                 `json:"showGuestCount"`
	ShowGuestList              bool                 `json:"showGuestList"`
	ShowActivityTimestamps     bool                 `json:"showActivityTimestamps"`
	DisplayInviteButton        bool                 `json:"displayInviteButton"`
	Visibility                 string               `json:"visibility"`
	AllowGuestPhotoUpload      bool                 `json:"allowGuestPhotoUpload"`
	EnableGuestReminders       bool                 `json:"enableGuestReminders"`
	RSVPsEnabled               bool                 `json:"rsvpsEnabled"`
	AllowGuestsToInviteMutuals bool                 `json:"allowGuestsToInviteMutuals"`
	Description                *string              `json:"description,omitempty"`
	LocationInfo               *EventLocationInfo   `json:"locationInfo,omitempty"`
	IsPublic                   *bool                `json:"isPublic,omitempty"`
	MaxCapacity                *int                 `json:"maxCapacity,omitempty"`
	EnableWaitlist             *bool                `json:"enableWaitlist,omitempty"`
	CustomFields               []EventCustomField   `json:"customFields,omitempty"`
}

type CreateEventParams struct {
	Event     CreateEventDraft `json:"event"`
	CohostIDs []string         `json:"cohostIds"`
}

type CancelEventParams struct {
	EventID                string `json:"eventId"`
	CancellationMessage    string `json:"cancellationMessage"`
	ShouldSkipNotifyGuests bool   `json:"shouldSkipNotifyGuests"`
}

type callableMutationRequest[T any] struct {
	Data callableMutationRequestData[T] `json:"data"`
}

type callableMutationRequestData[T any] struct {
	Params T   `json:"params"`
	UserID any `json:"userId"`
}

type FirestoreWriteDocument struct {
	Fields map[string]any `json:"fields"`
}

type FirestoreStringValue struct {
	StringValue string `json:"stringValue"`
}

type FirestoreIntegerValue struct {
	IntegerValue string `json:"integerValue"`
}

type FirestoreBooleanValue struct {
	BooleanValue bool `json:"booleanValue"`
}

type FirestoreReferenceValue struct {
	ReferenceValue string `json:"referenceValue"`
}

type FirestoreArray struct {
	Values []any `json:"values"`
}

type FirestoreArrayValue struct {
	ArrayValue FirestoreArray `json:"arrayValue"`
}

type FirestoreMap struct {
	Fields map[string]any `json:"fields"`
}

type FirestoreMapValue struct {
	MapValue FirestoreMap `json:"mapValue"`
}

func NewPartifulPosterImage(poster Poster) PartifulPosterImage {
	catalogRecord := PartifulPoster{
		ID:          poster.ID,
		Name:        poster.Name,
		URL:         poster.URL,
		BlurHash:    poster.BlurHash,
		ContentType: poster.ContentType,
		Height:      poster.Height,
		Width:       poster.Width,
		Tags:        append([]string(nil), poster.Tags...),
		Categories:  append([]string(nil), poster.Categories...),
	}
	return PartifulPosterImage{
		Source:      "partiful_posters",
		Poster:      catalogRecord,
		URL:         poster.URL,
		BlurHash:    poster.BlurHash,
		ContentType: poster.ContentType,
		Name:        poster.Name,
		Height:      poster.Height,
		Width:       poster.Width,
	}
}

func (client Client) CreateEvent(
	ctx context.Context,
	accessToken string,
	userID string,
	params CreateEventParams,
) (string, error) {
	completion, err := callEventMutation(client, ctx, accessToken, "createEvent", userID, params)
	if err != nil {
		return "", err
	}
	var eventID string
	if json.Unmarshal(completion, &eventID) != nil || eventID == "" {
		return "", fmt.Errorf("%w: create event completion", ErrProtocolChanged)
	}
	return eventID, nil
}

func (client Client) CancelEvent(
	ctx context.Context,
	accessToken string,
	userID string,
	params CancelEventParams,
) error {
	_, err := callEventMutation(client, ctx, accessToken, "cancelEvent", userID, params)
	return err
}

func callEventMutation[T any](
	client Client,
	ctx context.Context,
	accessToken string,
	operation string,
	userID string,
	params T,
) (json.RawMessage, error) {
	if client.HTTP == nil {
		return nil, fmt.Errorf("%w: event write transport", ErrUnavailable)
	}
	var encodedUserID any
	if userID != "" {
		encodedUserID = userID
	}
	payload, _ := json.Marshal(callableMutationRequest[T]{
		Data: callableMutationRequestData[T]{
			Params: params,
			UserID: encodedUserID,
		},
	})
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		partifulCallableHost+"/"+operation,
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: event write request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: event write request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("%w: event write response", ErrProtocolChanged)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: event write status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("%w: event write content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumEventWriteBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: event write response read", ErrUnavailable)
	}
	if len(body) > maximumEventWriteBodyBytes || !utf8.Valid(body) {
		return nil, fmt.Errorf("%w: event write response body", ErrProtocolChanged)
	}
	root, err := decodeEventObject(body)
	if err != nil {
		return nil, fmt.Errorf("%w: event write response body", ErrProtocolChanged)
	}
	if data, ok := root["data"]; ok {
		return bytes.Clone(data), nil
	}
	if result, ok := root["result"]; ok {
		return bytes.Clone(result), nil
	}
	return nil, fmt.Errorf("%w: event write completion", ErrProtocolChanged)
}

func (client Client) FirestorePatchEvent(
	ctx context.Context,
	accessToken string,
	eventID string,
	updateMask []string,
	document FirestoreWriteDocument,
) error {
	if client.HTTP == nil {
		return fmt.Errorf("%w: firestore transport", ErrUnavailable)
	}
	payload, _ := json.Marshal(document)
	query := url.Values{}
	query.Set("currentDocument.exists", "true")
	for _, fieldPath := range updateMask {
		query.Add("updateMask.fieldPaths", fieldPath)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPatch,
		firestoreDocumentsHost+"/v1/projects/getpartiful/databases/(default)/documents/events/"+url.PathEscape(eventID)+"?"+query.Encode(),
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("%w: firestore request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("%w: firestore request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return fmt.Errorf("%w: firestore response", ErrProtocolChanged)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: firestore status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return fmt.Errorf("%w: firestore content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumEventWriteBodyBytes+1))
	if err != nil {
		return fmt.Errorf("%w: firestore response read", ErrUnavailable)
	}
	if len(body) > maximumEventWriteBodyBytes || !utf8.Valid(body) {
		return fmt.Errorf("%w: firestore response body", ErrProtocolChanged)
	}
	if err := validateFirestoreDocument(body); err != nil {
		return fmt.Errorf("%w: firestore document", ErrProtocolChanged)
	}
	return nil
}

func validateFirestoreDocument(body []byte) error {
	root, err := decodeEventObject(body)
	if err != nil {
		return err
	}
	allowed := map[string]bool{
		"name":       true,
		"fields":     true,
		"createTime": true,
		"updateTime": true,
	}
	for key := range root {
		if !allowed[key] {
			return fmt.Errorf("unexpected document key")
		}
	}
	if raw, ok := root["name"]; ok {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return fmt.Errorf("invalid name")
		}
	}
	for _, key := range []string{"createTime", "updateTime"} {
		if raw, ok := root[key]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return fmt.Errorf("invalid timestamp")
			}
			if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
				return fmt.Errorf("invalid timestamp")
			}
		}
	}
	if raw, ok := root["fields"]; ok {
		fields, err := decodeEventObject(raw)
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := validateFirestoreValue(fields[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateFirestoreValue(raw json.RawMessage) error {
	object, err := decodeEventObject(raw)
	if err != nil {
		return err
	}
	if len(object) != 1 {
		return fmt.Errorf("firestore value must have one variant")
	}
	for key, value := range object {
		switch key {
		case "nullValue":
			if !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return fmt.Errorf("invalid null value")
			}
			return nil
		case "booleanValue":
			var decoded bool
			if json.Unmarshal(value, &decoded) != nil {
				return fmt.Errorf("invalid boolean value")
			}
			return nil
		case "integerValue":
			var decoded string
			if json.Unmarshal(value, &decoded) != nil || !firestoreIntegerPattern.MatchString(decoded) {
				return fmt.Errorf("invalid integer value")
			}
			return nil
		case "doubleValue":
			var numeric float64
			if json.Unmarshal(value, &numeric) == nil {
				return nil
			}
			var special string
			if json.Unmarshal(value, &special) != nil ||
				(special != "NaN" && special != "Infinity" && special != "-Infinity") {
				return fmt.Errorf("invalid double value")
			}
			return nil
		case "timestampValue":
			var decoded string
			if json.Unmarshal(value, &decoded) != nil {
				return fmt.Errorf("invalid timestamp value")
			}
			if _, err := time.Parse(time.RFC3339Nano, decoded); err != nil {
				return fmt.Errorf("invalid timestamp value")
			}
			return nil
		case "stringValue", "bytesValue", "referenceValue":
			var decoded string
			if json.Unmarshal(value, &decoded) != nil {
				return fmt.Errorf("invalid string-like value")
			}
			return nil
		case "geoPointValue":
			geoPoint, err := decodeEventObject(value)
			if err != nil || len(geoPoint) != 2 {
				return fmt.Errorf("invalid geopoint")
			}
			for _, field := range []string{"latitude", "longitude"} {
				rawNumber, ok := geoPoint[field]
				if !ok {
					return fmt.Errorf("invalid geopoint")
				}
				var decoded float64
				if json.Unmarshal(rawNumber, &decoded) != nil {
					return fmt.Errorf("invalid geopoint")
				}
			}
			return nil
		case "arrayValue":
			arrayObject, err := decodeEventObject(value)
			if err != nil {
				return err
			}
			for nestedKey := range arrayObject {
				if nestedKey != "values" {
					return fmt.Errorf("invalid array value")
				}
			}
			rawValues, ok := arrayObject["values"]
			if !ok {
				return nil
			}
			if !isEventJSONKind(rawValues, '[') {
				return fmt.Errorf("invalid array value")
			}
			var values []json.RawMessage
			if json.Unmarshal(rawValues, &values) != nil || values == nil {
				return fmt.Errorf("invalid array value")
			}
			for _, entry := range values {
				if err := validateFirestoreValue(entry); err != nil {
					return err
				}
			}
			return nil
		case "mapValue":
			mapObject, err := decodeEventObject(value)
			if err != nil {
				return err
			}
			for nestedKey := range mapObject {
				if nestedKey != "fields" {
					return fmt.Errorf("invalid map value")
				}
			}
			rawFields, ok := mapObject["fields"]
			if !ok {
				return nil
			}
			fields, err := decodeEventObject(rawFields)
			if err != nil {
				return err
			}
			for _, nested := range fields {
				if err := validateFirestoreValue(nested); err != nil {
					return err
				}
			}
			return nil
		default:
			return fmt.Errorf("unknown firestore value variant")
		}
	}
	return fmt.Errorf("missing firestore value variant")
}
