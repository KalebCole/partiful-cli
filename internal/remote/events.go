package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"
)

const (
	maximumEventListItems = 1000
	maximumEventListBytes = 8 << 20
	maximumEventInfoBytes = 8 << 20
)

var ErrEventListBoundExceeded = errors.New("event list bound exceeded")
var ErrEventNotFound = errors.New("event not found")

type Event struct {
	ID              string
	Title           *string
	Start           *string
	End             *string
	Timezone        *string
	Status          *string
	OwnerIDs        []string
	OwnerIDsPresent bool
	GuestPresent    bool
	GuestStatus     *string
	GuestCount      IntegerField
	HasGuests       BooleanField
	RawFields       map[string]json.RawMessage
	Safeguards      EventSafeguards
}

type FieldState string

const (
	FieldAbsent FieldState = "absent"
	FieldNull   FieldState = "null"
	FieldValue  FieldState = "value"
)

type BooleanField struct {
	State FieldState `json:"state"`
	Value bool       `json:"value,omitempty"`
}

type IntegerField struct {
	State FieldState `json:"state"`
	Value int        `json:"value,omitempty"`
}

type StringField struct {
	State FieldState `json:"state"`
	Value string     `json:"value,omitempty"`
}

type PresenceField struct {
	State FieldState `json:"state"`
}

type ArrayField struct {
	State  FieldState `json:"state"`
	Length int        `json:"length,omitempty"`
}

type EventSafeguards struct {
	RSVPsEnabled          BooleanField  `json:"rsvpsEnabled"`
	AtCapacity            BooleanField  `json:"atCapacity"`
	PlusOneNamesRequired  BooleanField  `json:"plusOneNamesRequired"`
	GuestAction           StringField   `json:"guestAction"`
	Ticketing             PresenceField `json:"ticketing"`
	Password              PresenceField `json:"password"`
	PasswordProtected     PresenceField `json:"passwordProtected"`
	QuestionnaireEnabled  BooleanField  `json:"questionnaireEnabled"`
	QuestionnaireVersions ArrayField    `json:"questionnaireVersions"`
	MaxCountPerGuest      IntegerField  `json:"maxCountPerGuest"`
	MaxCapacity           IntegerField  `json:"maxCapacity"`
	RemainingCapacity     IntegerField  `json:"remainingCapacity"`
	EnableWaitlist        BooleanField  `json:"enableWaitlist"`
}

type EventCatalog struct {
	Events        []Event
	PayloadSHA256 [sha256.Size]byte
}

type eventListRequest struct {
	Data eventListRequestData `json:"data"`
}

type eventListRequestData struct {
	Params            struct{} `json:"params"`
	AmplitudeDeviceID string   `json:"amplitudeDeviceId"`
}

type eventInfoRequest struct {
	Data eventInfoRequestData `json:"data"`
}

type eventInfoRequestData struct {
	Params            eventInfoParams `json:"params"`
	AmplitudeDeviceID string          `json:"amplitudeDeviceId"`
}

type eventInfoParams struct {
	EventID string `json:"eventId"`
}

func (client Client) GetUpcomingEvents(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
) (EventCatalog, error) {
	return client.getEventCatalog(
		ctx,
		accessToken,
		amplitudeDeviceID,
		"getMyUpcomingEventsForHomePage",
		"upcomingEvents",
	)
}

func (client Client) GetPastEvents(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
) (EventCatalog, error) {
	return client.getEventCatalog(
		ctx,
		accessToken,
		amplitudeDeviceID,
		"getMyPastEventsForHomePage",
		"pastEvents",
	)
}

func (client Client) GetEventInfo(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	eventID string,
) (Event, error) {
	if client.HTTP == nil {
		return Event{}, fmt.Errorf("%w: event transport", ErrUnavailable)
	}
	payload, _ := json.Marshal(eventInfoRequest{Data: eventInfoRequestData{
		Params:            eventInfoParams{EventID: eventID},
		AmplitudeDeviceID: amplitudeDeviceID,
	}})
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		partifulCallableHost+"/getEventInfo",
		bytes.NewReader(payload),
	)
	if err != nil {
		return Event{}, fmt.Errorf("%w: event request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return Event{}, fmt.Errorf("%w: event request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return Event{}, fmt.Errorf("%w: event response", ErrProtocolChanged)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK &&
		response.StatusCode != http.StatusNotFound {
		return Event{}, fmt.Errorf("%w: event status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return Event{}, fmt.Errorf("%w: event content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumEventInfoBytes+1))
	if err != nil {
		return Event{}, fmt.Errorf("%w: event response read", ErrUnavailable)
	}
	if len(body) > maximumEventInfoBytes || !utf8.Valid(body) {
		return Event{}, fmt.Errorf("%w: event response body", ErrProtocolChanged)
	}
	if response.StatusCode == http.StatusNotFound {
		if !validEventNotFound(body) {
			return Event{}, fmt.Errorf("%w: event not-found body", ErrProtocolChanged)
		}
		return Event{}, ErrEventNotFound
	}
	root, err := decodeEventObject(body)
	if err != nil {
		return Event{}, fmt.Errorf("%w: event response body", ErrProtocolChanged)
	}
	result, err := eventObjectField(root, "result")
	if err != nil {
		return Event{}, fmt.Errorf("%w: event result", ErrProtocolChanged)
	}
	data, err := eventObjectField(result, "data")
	if err != nil {
		return Event{}, fmt.Errorf("%w: event data", ErrProtocolChanged)
	}
	rawEvent, ok := data["event"]
	if !ok {
		return Event{}, fmt.Errorf("%w: event value", ErrProtocolChanged)
	}
	event, err := decodeEventInfo(rawEvent)
	if err != nil {
		return Event{}, fmt.Errorf("%w: event value", ErrProtocolChanged)
	}
	return event, nil
}

func (client Client) getEventCatalog(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	operation string,
	collectionField string,
) (EventCatalog, error) {
	if client.HTTP == nil {
		return EventCatalog{}, fmt.Errorf("%w: event transport", ErrUnavailable)
	}
	payload, _ := json.Marshal(eventListRequest{Data: eventListRequestData{
		AmplitudeDeviceID: amplitudeDeviceID,
	}})
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		partifulCallableHost+"/"+operation,
		bytes.NewReader(payload),
	)
	if err != nil {
		return EventCatalog{}, fmt.Errorf("%w: event request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return EventCatalog{}, fmt.Errorf("%w: event request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return EventCatalog{}, fmt.Errorf("%w: event response", ErrProtocolChanged)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return EventCatalog{}, fmt.Errorf("%w: event status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return EventCatalog{}, fmt.Errorf("%w: event content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumEventListBytes+1))
	if err != nil {
		return EventCatalog{}, fmt.Errorf("%w: event response read", ErrUnavailable)
	}
	if len(body) > maximumEventListBytes {
		return EventCatalog{}, fmt.Errorf(
			"%w: %w",
			ErrProtocolChanged,
			ErrEventListBoundExceeded,
		)
	}
	if !utf8.Valid(body) {
		return EventCatalog{}, fmt.Errorf("%w: event response body", ErrProtocolChanged)
	}
	root, err := decodeEventObject(body)
	if err != nil {
		return EventCatalog{}, fmt.Errorf("%w: event response body", ErrProtocolChanged)
	}
	result, err := eventObjectField(root, "result")
	if err != nil {
		return EventCatalog{}, fmt.Errorf("%w: event result", ErrProtocolChanged)
	}
	data, err := eventObjectField(result, "data")
	if err != nil {
		return EventCatalog{}, fmt.Errorf("%w: event data", ErrProtocolChanged)
	}
	rawEvents, ok := data[collectionField]
	if !ok || !isEventJSONKind(rawEvents, '[') {
		return EventCatalog{}, fmt.Errorf("%w: event collection", ErrProtocolChanged)
	}
	var rawItems []json.RawMessage
	if json.Unmarshal(rawEvents, &rawItems) != nil {
		return EventCatalog{}, fmt.Errorf("%w: event collection", ErrProtocolChanged)
	}
	if len(rawItems) > maximumEventListItems {
		return EventCatalog{}, fmt.Errorf(
			"%w: %w",
			ErrProtocolChanged,
			ErrEventListBoundExceeded,
		)
	}
	events := make([]Event, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, err := decodeHomePageEvent(rawItem)
		if err != nil {
			return EventCatalog{}, fmt.Errorf("%w: event item", ErrProtocolChanged)
		}
		events = append(events, item)
	}
	return EventCatalog{
		Events:        events,
		PayloadSHA256: sha256.Sum256(body),
	}, nil
}

func decodeHomePageEvent(raw json.RawMessage) (Event, error) {
	object, err := decodeEventObject(raw)
	if err != nil {
		return Event{}, err
	}
	id, present, err := eventStringField(object, "id", false)
	if err != nil || !present {
		return Event{}, errors.New("event id is invalid")
	}
	title, _, err := eventStringField(object, "title", false)
	if err != nil {
		return Event{}, err
	}
	start, _, err := eventStringField(object, "startDate", false)
	if err != nil {
		return Event{}, err
	}
	end, _, err := eventStringField(object, "endDate", true)
	if err != nil {
		return Event{}, err
	}
	timezone, _, err := eventStringField(object, "timezone", false)
	if err != nil {
		return Event{}, err
	}
	status, statusPresent, err := eventStringField(object, "status", false)
	if err != nil || statusPresent && !validEventStatus(*status) {
		return Event{}, errors.New("event status is invalid")
	}
	if err := validateEventStringOrNull(object, "location"); err != nil {
		return Event{}, err
	}
	if err := validateEventObjectOrNull(object, "image", true); err != nil {
		return Event{}, err
	}
	if err := validateEventObjectOrNull(object, "displaySettings", false); err != nil {
		return Event{}, err
	}

	var ownerIDs []string
	ownerIDsPresent := false
	if rawOwners, ok := object["ownerIds"]; ok {
		ownerIDsPresent = true
		if !isEventJSONKind(rawOwners, '[') ||
			json.Unmarshal(rawOwners, &ownerIDs) != nil ||
			ownerIDs == nil {
			return Event{}, errors.New("event owners are invalid")
		}
	}

	guestPresent := false
	var guestStatus *string
	if rawGuest, ok := object["guest"]; ok {
		guestPresent = true
		guest, err := decodeEventObject(rawGuest)
		if err != nil {
			return Event{}, err
		}
		status, present, err := eventStringField(guest, "status", false)
		if err != nil || present && !validGuestStatus(*status) {
			return Event{}, errors.New("event guest status is invalid")
		}
		guestStatus = status
	}

	return Event{
		ID:              *id,
		Title:           title,
		Start:           start,
		End:             end,
		Timezone:        timezone,
		Status:          status,
		OwnerIDs:        ownerIDs,
		OwnerIDsPresent: ownerIDsPresent,
		GuestPresent:    guestPresent,
		GuestStatus:     guestStatus,
	}, nil
}

func decodeEventInfo(raw json.RawMessage) (Event, error) {
	object, err := decodeEventObject(raw)
	if err != nil {
		return Event{}, err
	}
	if _, _, err := eventStringField(object, "id", false); err != nil {
		return Event{}, err
	}
	title, _, err := eventStringField(object, "title", false)
	if err != nil {
		return Event{}, err
	}
	start, _, err := eventStringField(object, "startDate", false)
	if err != nil {
		return Event{}, err
	}
	end, _, err := eventStringField(object, "endDate", true)
	if err != nil {
		return Event{}, err
	}
	timezone, _, err := eventStringField(object, "timezone", false)
	if err != nil {
		return Event{}, err
	}
	status, statusPresent, err := eventStringField(object, "status", false)
	if err != nil || statusPresent && !validEventStatus(*status) {
		return Event{}, errors.New("event status is invalid")
	}
	if err := validateEventObjectOrNull(object, "image", true); err != nil {
		return Event{}, err
	}
	if err := validateEventObjectOrNull(object, "displaySettings", false); err != nil {
		return Event{}, err
	}
	safeguards, err := decodeEventSafeguards(object)
	if err != nil {
		return Event{}, err
	}
	var ownerIDs []string
	ownerIDsPresent := false
	if rawOwners, ok := object["ownerIds"]; ok {
		ownerIDsPresent = true
		if !isEventJSONKind(rawOwners, '[') ||
			json.Unmarshal(rawOwners, &ownerIDs) != nil ||
			ownerIDs == nil {
			return Event{}, errors.New("event owners are invalid")
		}
	}
	guestCount, err := eventIntegerField(object, "guestCount", true)
	if err != nil {
		return Event{}, err
	}
	hasGuests, err := eventBooleanField(object, "hasGuests", false)
	if err != nil {
		return Event{}, err
	}
	rawFields := make(map[string]json.RawMessage, len(object))
	for name, raw := range object {
		rawFields[name] = bytes.Clone(raw)
	}
	return Event{
		Title:           title,
		Start:           start,
		End:             end,
		Timezone:        timezone,
		Status:          status,
		OwnerIDs:        ownerIDs,
		OwnerIDsPresent: ownerIDsPresent,
		GuestCount:      guestCount,
		HasGuests:       hasGuests,
		RawFields:       rawFields,
		Safeguards:      safeguards,
	}, nil
}

func decodeEventSafeguards(
	object map[string]json.RawMessage,
) (EventSafeguards, error) {
	rsvpsEnabled, err := eventBooleanField(object, "rsvpsEnabled", false)
	if err != nil {
		return EventSafeguards{}, err
	}
	atCapacity, err := eventBooleanField(object, "atCapacity", false)
	if err != nil {
		return EventSafeguards{}, err
	}
	plusOneNamesRequired, err := eventBooleanField(
		object,
		"plusOneNamesRequired",
		false,
	)
	if err != nil {
		return EventSafeguards{}, err
	}
	guestAction, err := eventStringSafeguard(object, "guestAction")
	if err != nil ||
		guestAction.State == FieldValue &&
			guestAction.Value != "APPLY" &&
			guestAction.Value != "RSVP" {
		return EventSafeguards{}, errors.New("guest action is invalid")
	}
	ticketing, err := eventObjectPresence(object, "ticketing")
	if err != nil {
		return EventSafeguards{}, err
	}
	questionnaireEnabled, err := eventBooleanField(
		object,
		"questionnaireEnabled",
		false,
	)
	if err != nil {
		return EventSafeguards{}, err
	}
	questionnaireVersions, err := eventArrayField(object, "questionnaireVersions")
	if err != nil {
		return EventSafeguards{}, err
	}
	maxCountPerGuest, err := eventIntegerField(
		object,
		"maxCountPerGuest",
		false,
	)
	if err != nil {
		return EventSafeguards{}, err
	}
	maxCapacity, err := eventIntegerField(object, "maxCapacity", true)
	if err != nil {
		return EventSafeguards{}, err
	}
	remainingCapacity, err := eventIntegerField(
		object,
		"remainingCapacity",
		false,
	)
	if err != nil {
		return EventSafeguards{}, err
	}
	enableWaitlist, err := eventBooleanField(object, "enableWaitlist", true)
	if err != nil {
		return EventSafeguards{}, err
	}
	password := eventUntypedPresence(object, "password")
	passwordProtected := eventUntypedPresence(object, "passwordProtected")
	return EventSafeguards{
		RSVPsEnabled:          rsvpsEnabled,
		AtCapacity:            atCapacity,
		PlusOneNamesRequired:  plusOneNamesRequired,
		GuestAction:           guestAction,
		Ticketing:             ticketing,
		Password:              password,
		PasswordProtected:     passwordProtected,
		QuestionnaireEnabled:  questionnaireEnabled,
		QuestionnaireVersions: questionnaireVersions,
		MaxCountPerGuest:      maxCountPerGuest,
		MaxCapacity:           maxCapacity,
		RemainingCapacity:     remainingCapacity,
		EnableWaitlist:        enableWaitlist,
	}, nil
}

func eventBooleanField(
	object map[string]json.RawMessage,
	name string,
	nullAllowed bool,
) (BooleanField, error) {
	raw, ok := object[name]
	if !ok {
		return BooleanField{State: FieldAbsent}, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if nullAllowed {
			return BooleanField{State: FieldNull}, nil
		}
		return BooleanField{}, errors.New("boolean is null")
	}
	var value bool
	if json.Unmarshal(raw, &value) != nil {
		return BooleanField{}, errors.New("boolean is invalid")
	}
	return BooleanField{State: FieldValue, Value: value}, nil
}

func eventStringSafeguard(
	object map[string]json.RawMessage,
	name string,
) (StringField, error) {
	value, present, err := eventStringField(object, name, false)
	if err != nil {
		return StringField{}, err
	}
	if !present {
		return StringField{State: FieldAbsent}, nil
	}
	return StringField{State: FieldValue, Value: *value}, nil
}

func eventObjectPresence(
	object map[string]json.RawMessage,
	name string,
) (PresenceField, error) {
	raw, ok := object[name]
	if !ok {
		return PresenceField{State: FieldAbsent}, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return PresenceField{State: FieldNull}, nil
	}
	if _, err := decodeEventObject(raw); err != nil {
		return PresenceField{}, err
	}
	return PresenceField{State: FieldValue}, nil
}

func eventUntypedPresence(
	object map[string]json.RawMessage,
	name string,
) PresenceField {
	raw, ok := object[name]
	if !ok {
		return PresenceField{State: FieldAbsent}
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return PresenceField{State: FieldNull}
	}
	return PresenceField{State: FieldValue}
}

func eventArrayField(
	object map[string]json.RawMessage,
	name string,
) (ArrayField, error) {
	raw, ok := object[name]
	if !ok {
		return ArrayField{State: FieldAbsent}, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ArrayField{State: FieldNull}, nil
	}
	if !isEventJSONKind(raw, '[') {
		return ArrayField{}, errors.New("array is invalid")
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return ArrayField{}, errors.New("array is invalid")
	}
	return ArrayField{State: FieldValue, Length: len(values)}, nil
}

func eventIntegerField(
	object map[string]json.RawMessage,
	name string,
	nullAllowed bool,
) (IntegerField, error) {
	raw, ok := object[name]
	if !ok {
		return IntegerField{State: FieldAbsent}, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if nullAllowed {
			return IntegerField{State: FieldNull}, nil
		}
		return IntegerField{}, errors.New("integer is null")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return IntegerField{}, errors.New("integer is invalid")
	}
	number, ok := value.(json.Number)
	if !ok {
		return IntegerField{}, errors.New("integer is invalid")
	}
	parsed, err := number.Int64()
	if err != nil || int64(int(parsed)) != parsed {
		return IntegerField{}, errors.New("integer is invalid")
	}
	return IntegerField{State: FieldValue, Value: int(parsed)}, nil
}

func validEventNotFound(body []byte) bool {
	root, err := decodeEventObject(body)
	if err != nil {
		return false
	}
	failure, err := eventObjectField(root, "error")
	if err != nil {
		return false
	}
	_, messagePresent, messageErr := eventStringField(failure, "message", false)
	status, statusPresent, statusErr := eventStringField(failure, "status", false)
	return messageErr == nil &&
		messagePresent &&
		statusErr == nil &&
		statusPresent &&
		*status == "NOT_FOUND"
}

func decodeEventObject(document []byte) (map[string]json.RawMessage, error) {
	if !isEventJSONKind(document, '{') {
		return nil, errors.New("JSON value is not an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	var object map[string]json.RawMessage
	if decoder.Decode(&object) != nil || object == nil {
		return nil, errors.New("JSON object is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON object has trailing data")
	}
	return object, nil
}

func eventObjectField(
	object map[string]json.RawMessage,
	name string,
) (map[string]json.RawMessage, error) {
	raw, ok := object[name]
	if !ok {
		return nil, errors.New("required object is absent")
	}
	return decodeEventObject(raw)
}

func eventStringField(
	object map[string]json.RawMessage,
	name string,
	nullAllowed bool,
) (*string, bool, error) {
	raw, ok := object[name]
	if !ok {
		return nil, false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if nullAllowed {
			return nil, true, nil
		}
		return nil, true, errors.New("string is null")
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return nil, true, errors.New("string is invalid")
	}
	return &value, true, nil
}

func validateEventStringOrNull(object map[string]json.RawMessage, name string) error {
	_, _, err := eventStringField(object, name, true)
	return err
}

func validateEventObjectOrNull(
	object map[string]json.RawMessage,
	name string,
	nullAllowed bool,
) error {
	raw, ok := object[name]
	if !ok {
		return nil
	}
	if nullAllowed && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	_, err := decodeEventObject(raw)
	return err
}

func isEventJSONKind(raw []byte, kind byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == kind
}

func eventJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}

func validEventStatus(value string) bool {
	return value == "UNSAVED" || value == "PUBLISHED" || value == "CANCELED"
}

func validGuestStatus(value string) bool {
	switch value {
	case "READY_TO_SEND",
		"SENDING",
		"SEND_ERROR",
		"DELIVERY_ERROR",
		"SENT",
		"INTERESTED",
		"WAITLIST",
		"MAYBE",
		"DECLINED",
		"GOING",
		"PENDING_APPROVAL",
		"APPROVED",
		"WITHDRAWN",
		"WAITLISTED_FOR_APPROVAL",
		"REJECTED",
		"RESPONDED_TO_FIND_A_TIME":
		return true
	default:
		return false
	}
}
