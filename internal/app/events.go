package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

type eventData struct {
	Items []eventSummary `json:"items"`
}

type eventSummary struct {
	EventID  string  `json:"eventId"`
	Title    *string `json:"title"`
	Start    *string `json:"start"`
	End      *string `json:"end"`
	Timezone *string `json:"timezone"`
	State    *string `json:"state"`
	UserRole *string `json:"userRole"`
	MyRSVP   *string `json:"myRsvp"`
}

type eventDetail struct {
	EventID     string  `json:"eventId"`
	Title       *string `json:"title"`
	Start       *string `json:"start"`
	End         *string `json:"end"`
	Timezone    *string `json:"timezone"`
	State       *string `json:"state"`
	UserRole    any     `json:"userRole"`
	MyRSVP      any     `json:"myRsvp"`
	Description any     `json:"description"`
	Location    any     `json:"location"`
	Address     any     `json:"address"`
	Visibility  any     `json:"visibility"`
	GuestLimit  any     `json:"guestLimit"`
	Poster      any     `json:"poster"`
	Links       any     `json:"links"`
}

func executeEventsList(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	options, inputError := parseCollectionOptions(definition, argv)
	if inputError != nil {
		return failure(definition.path, 2, *inputError, pretty)
	}
	filterHash := normalizedFilterHash(definition.path, options.when)
	var decodedCursor cursorPayload
	var cursorKey []byte
	var err error
	if options.cursorProvided {
		cursorKey, err = loadCursorKey(dependencies)
		if err != nil {
			return internalFailure(definition.path, pretty)
		}
		var cursorFailure *cursorValidationFailure
		decodedCursor, cursorFailure = decodeCursor(options.cursor, filterHash, cursorKey)
		if cursorFailure != nil {
			return failure(definition.path, cursorFailure.exitCode, cursorFailure.body, pretty)
		}
	}
	session, sessionFailure := acquireProtectedSession(
		ctx,
		definition.path,
		dependencies,
		pretty,
	)
	if sessionFailure != nil {
		return *sessionFailure
	}
	deviceID, err := randomDeviceID(dependencies.AuthRandom)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	client := remote.Client{HTTP: dependencies.HTTP}
	var catalog remote.EventCatalog
	switch options.when {
	case "upcoming":
		catalog, err = client.GetUpcomingEvents(
			ctx,
			session.AccessToken,
			deviceID,
		)
	case "past":
		catalog, err = client.GetPastEvents(
			ctx,
			session.AccessToken,
			deviceID,
		)
	}
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return eventListUnavailableFailure(definition.path, pretty)
		}
		if errors.Is(err, remote.ErrEventListBoundExceeded) {
			return eventListBoundFailure(definition.path, pretty)
		}
		return eventListProtocolChangedFailure(definition.path, pretty)
	}
	offset := 0
	if options.cursorProvided {
		var cursorFailure *cursorValidationFailure
		offset, cursorFailure = cursorSnapshotOffset(
			decodedCursor,
			catalog.PayloadSHA256,
			len(catalog.Events),
			"The event list changed after this cursor was issued.",
		)
		if cursorFailure != nil {
			return failure(definition.path, cursorFailure.exitCode, cursorFailure.body, pretty)
		}
	}
	projected := make([]eventSummary, 0, len(catalog.Events))
	for _, event := range catalog.Events {
		item, err := projectEventSummary(event, session.UserID)
		if err != nil {
			return eventListProtocolChangedFailure(definition.path, pretty)
		}
		projected = append(projected, item)
	}
	end := min(offset+options.limit, len(catalog.Events))
	items := make([]eventSummary, end-offset)
	copy(items, projected[offset:end])
	var cursor *string
	hasMore := end < len(catalog.Events)
	if hasMore {
		if cursorKey == nil {
			cursorKey, err = loadCursorKey(dependencies)
			if err != nil {
				return internalFailure(definition.path, pretty)
			}
		}
		value, err := nextCursor(
			catalog.PayloadSHA256,
			filterHash,
			end,
			cursorKey,
			dependencies.CursorRandom,
		)
		if err != nil {
			return internalFailure(definition.path, pretty)
		}
		cursor = &value
	}
	return collectionSuccess(definition.path, eventData{Items: items}, pageMeta{
		Limit:      options.limit,
		NextCursor: cursor,
		HasMore:    hasMore,
	}, pretty)
}

func projectEventSummary(event remote.Event, currentUserID string) (eventSummary, error) {
	state, err := projectEventState(event.Status)
	if err != nil {
		return eventSummary{}, err
	}
	var role *string
	if event.OwnerIDsPresent && currentUserID != "" {
		value := "none"
		if slices.Contains(event.OwnerIDs, currentUserID) {
			value = "host"
		} else if event.GuestPresent {
			value = "attendee"
		}
		role = &value
	}
	rsvp, err := projectEventRSVP(event.GuestStatus)
	if err != nil {
		return eventSummary{}, err
	}
	return eventSummary{
		EventID:  event.ID,
		Title:    event.Title,
		Start:    event.Start,
		End:      event.End,
		Timezone: event.Timezone,
		State:    state,
		UserRole: role,
		MyRSVP:   rsvp,
	}, nil
}

func executeEventGet(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	eventID, inputError := parseEventID(definition, argv)
	if inputError != nil {
		return failure(definition.path, 2, *inputError, pretty)
	}
	session, sessionFailure := acquireProtectedSession(
		ctx,
		definition.path,
		dependencies,
		pretty,
	)
	if sessionFailure != nil {
		return *sessionFailure
	}
	deviceID, err := randomDeviceID(dependencies.AuthRandom)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	event, err := (remote.Client{HTTP: dependencies.HTTP}).GetEventInfo(
		ctx,
		session.AccessToken,
		deviceID,
		eventID,
	)
	if err != nil {
		switch {
		case errors.Is(err, remote.ErrEventNotFound):
			return eventNotFoundFailure(definition.path, pretty)
		case errors.Is(err, remote.ErrUnavailable):
			return eventUnavailableFailure(definition.path, pretty)
		default:
			return eventProtocolChangedFailure(definition.path, pretty)
		}
	}
	state, err := projectEventState(event.Status)
	if err != nil {
		return eventProtocolChangedFailure(definition.path, pretty)
	}
	return success(definition.path, eventDetail{
		EventID:  eventID,
		Title:    event.Title,
		Start:    event.Start,
		End:      event.End,
		Timezone: event.Timezone,
		State:    state,
	}, pretty)
}

func parseEventID(
	definition commandDefinition,
	argv []string,
) (string, *errorBody) {
	if len(argv) < len(definition.invocation)+1 ||
		strings.TrimSpace(argv[len(definition.invocation)]) == "" {
		return "", &errorBody{
			Type:      "input.invalid",
			Code:      "EVENT_ID_REQUIRED",
			Message:   "Event ID is required.",
			Retryable: false,
			Details:   map[string]any{},
		}
	}
	if len(argv) != len(definition.invocation)+1 {
		return "", &errorBody{
			Type:      "input.invalid",
			Code:      "POSITIONAL_INVALID",
			Message:   "The command has an unexpected positional argument.",
			Retryable: false,
			Details:   map[string]any{},
		}
	}
	return argv[len(definition.invocation)], nil
}

func projectEventState(status *string) (*string, error) {
	if status == nil {
		return nil, nil
	}
	var state string
	switch *status {
	case "PUBLISHED":
		state = "active"
	case "CANCELED":
		state = "cancelled"
	default:
		return nil, errors.New("event status has no product mapping")
	}
	return &state, nil
}

func projectEventRSVP(status *string) (*string, error) {
	if status == nil {
		return nil, nil
	}
	mappings := map[string]string{
		"READY_TO_SEND":            "ready-to-send",
		"SENDING":                  "sending",
		"SEND_ERROR":               "send-error",
		"DELIVERY_ERROR":           "delivery-error",
		"SENT":                     "sent",
		"INTERESTED":               "interested",
		"WAITLIST":                 "waitlist",
		"MAYBE":                    "maybe",
		"DECLINED":                 "declined",
		"GOING":                    "going",
		"PENDING_APPROVAL":         "pending-approval",
		"APPROVED":                 "approved",
		"WITHDRAWN":                "withdrawn",
		"WAITLISTED_FOR_APPROVAL":  "waitlisted-for-approval",
		"REJECTED":                 "rejected",
		"RESPONDED_TO_FIND_A_TIME": "responded-to-find-a-time",
	}
	value, ok := mappings[*status]
	if !ok {
		return nil, errors.New("guest status has no product mapping")
	}
	return &value, nil
}

func eventReadRsvpValues() []string {
	return []string{
		"ready-to-send",
		"sending",
		"send-error",
		"delivery-error",
		"sent",
		"interested",
		"waitlist",
		"maybe",
		"declined",
		"going",
		"pending-approval",
		"approved",
		"withdrawn",
		"waitlisted-for-approval",
		"rejected",
		"responded-to-find-a-time",
	}
}

func eventListUnavailableFailure(command string, pretty bool) Result {
	result := failure(command, 8, errorBody{
		Type:      "remote.unavailable",
		Code:      "EVENT_LIST_UNAVAILABLE",
		Message:   "Events are unavailable.",
		Retryable: true,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: events unavailable\n"
	return result
}

func eventListBoundFailure(command string, pretty bool) Result {
	result := failure(command, 9, errorBody{
		Type:      "contract.protocol_changed",
		Code:      "EVENT_LIST_BOUND_EXCEEDED",
		Message:   "The event list exceeded its reviewed local safety bound.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: event list protocol changed\n"
	return result
}

func eventListProtocolChangedFailure(command string, pretty bool) Result {
	result := failure(command, 9, errorBody{
		Type:      "contract.protocol_changed",
		Code:      "EVENT_LIST_PROTOCOL_CHANGED",
		Message:   "Events no longer match the reviewed remote contract.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: event list protocol changed\n"
	return result
}

func eventNotFoundFailure(command string, pretty bool) Result {
	result := failure(command, 5, errorBody{
		Type:      "resource.not_found",
		Code:      "EVENT_NOT_FOUND",
		Message:   "The event was not found.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: event not found\n"
	return result
}

func eventUnavailableFailure(command string, pretty bool) Result {
	result := failure(command, 8, errorBody{
		Type:      "remote.unavailable",
		Code:      "EVENT_UNAVAILABLE",
		Message:   "The event is unavailable.",
		Retryable: true,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: event unavailable\n"
	return result
}

func eventProtocolChangedFailure(command string, pretty bool) Result {
	result := failure(command, 9, errorBody{
		Type:      "contract.protocol_changed",
		Code:      "EVENT_PROTOCOL_CHANGED",
		Message:   "The event no longer matches the reviewed remote contract.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: event protocol changed\n"
	return result
}
