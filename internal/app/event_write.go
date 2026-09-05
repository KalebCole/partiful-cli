package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

type eventCreatePreview struct {
	Operation     string                 `json:"operation"`
	Input         eventCreatePublicInput `json:"input"`
	Request       any                    `json:"request"`
	Preconditions map[string]string      `json:"preconditions"`
}

type eventUpdatePreview struct {
	Operation     string            `json:"operation"`
	EventID       string            `json:"eventId"`
	Fields        []string          `json:"fields"`
	Input         map[string]any    `json:"input"`
	Request       any               `json:"request"`
	Preconditions map[string]string `json:"preconditions"`
}

type eventCancelPreview struct {
	Operation     string                    `json:"operation"`
	EventID       string                    `json:"eventId"`
	Input         eventCancelPreviewInput   `json:"input"`
	Request       eventCancelPreviewRequest `json:"request"`
	Effects       []string                  `json:"effects"`
	Preconditions map[string]string         `json:"preconditions"`
}

type eventCancelPreviewInput struct {
	MessageProvided bool `json:"messageProvided"`
	NotifyGuests    bool `json:"notifyGuests"`
}

type eventCancelPreviewRequest struct {
	EventID                     string `json:"eventId"`
	CancellationMessageProvided bool   `json:"cancellationMessageProvided"`
	ShouldSkipNotifyGuests      bool   `json:"shouldSkipNotifyGuests"`
}

type eventCreateSubmitted struct {
	Submitted bool `json:"submitted"`
}

type eventUpdateSubmitted struct {
	EventID   string   `json:"eventId"`
	Fields    []string `json:"fields"`
	Submitted bool     `json:"submitted"`
}

type eventCancelSubmitted struct {
	EventID      string `json:"eventId"`
	NotifyGuests bool   `json:"notifyGuests"`
	Submitted    bool   `json:"submitted"`
}

type eventFieldSnapshot struct {
	State string          `json:"state"`
	Value json.RawMessage `json:"value,omitempty"`
}

func executeEventCreate(
	ctx context.Context,
	definition commandDefinition,
	options eventCreateOptions,
	dependencies Dependencies,
	execution mutationExecution,
	pretty bool,
) Result {
	client := remote.Client{HTTP: dependencies.HTTP}
	if execution.DryRun {
		privateRequest, prepareFailure := prepareEventCreate(
			ctx,
			definition.path,
			client,
			options.Input,
			pretty,
		)
		if prepareFailure != nil {
			return *prepareFailure
		}
		return success(definition.path, eventCreatePreview{
			Operation:     "createEvent",
			Input:         options.Input.public(),
			Request:       privateRequest,
			Preconditions: map[string]string{"poster": "bound"},
		}, pretty)
	}
	session, sessionFailure := acquireProtectedMutationSession(ctx, definition.path, dependencies, execution, pretty)
	if sessionFailure != nil {
		return *sessionFailure
	}
	if session.UserID == "" {
		return internalFailure(definition.path, pretty)
	}
	privateRequest, prepareFailure := prepareEventCreate(
		ctx,
		definition.path,
		client,
		options.Input,
		pretty,
	)
	if prepareFailure != nil {
		return *prepareFailure
	}
	if _, err := client.CreateEvent(ctx, session.AccessToken, session.UserID, privateRequest); err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return eventSubmissionUnavailableFailure(definition.path, "Create submission could not be confirmed. Inspect remote state before another attempt.", pretty)
		}
		return eventWriteProtocolChangedFailure(definition.path, "CREATE_EVENT_PROTOCOL_CHANGED", "The event create response no longer matches the reviewed remote contract.", "partiful: event create protocol changed\n", pretty)
	}
	return success(definition.path, eventCreateSubmitted{Submitted: true}, pretty)
}

func prepareEventCreate(
	ctx context.Context,
	command string,
	client remote.Client,
	input normalizedEventCreateInput,
	pretty bool,
) (remote.CreateEventParams, *Result) {
	posterImage, err := resolvePoster(ctx, client, input.PosterID)
	if err != nil {
		result := mapPosterError(command, err, pretty)
		return remote.CreateEventParams{}, &result
	}
	return remote.CreateEventParams{
		Event:     buildCreateEventDraft(input, posterImage),
		CohostIDs: []string{},
	}, nil
}

func executeEventUpdate(
	ctx context.Context,
	definition commandDefinition,
	options eventUpdateOptions,
	dependencies Dependencies,
	execution mutationExecution,
	pretty bool,
) Result {
	session, sessionFailure := acquireProtectedMutationSession(ctx, definition.path, dependencies, execution, pretty)
	if sessionFailure != nil {
		return *sessionFailure
	}
	if session.UserID == "" {
		return internalFailure(definition.path, pretty)
	}
	clock := time.Now
	if dependencies.Now != nil {
		clock = dependencies.Now
	}
	deviceID, err := randomDeviceID(dependencies.AuthRandom)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	client := remote.Client{HTTP: dependencies.HTTP}
	event, err := client.GetEventInfo(ctx, session.AccessToken, deviceID, options.EventID)
	if err != nil {
		switch {
		case errors.Is(err, remote.ErrEventNotFound):
			return eventNotFoundFailure(definition.path, pretty)
		case errors.Is(err, remote.ErrUnavailable):
			return eventRemoteUnavailableFailure(definition.path, "The event update could not read current event data.", "partiful: event update unavailable\n", pretty)
		default:
			return eventWriteProtocolChangedFailure(definition.path, "EVENT_UPDATE_PROTOCOL_CHANGED", "The event update no longer matches the reviewed remote contract.", "partiful: event update protocol changed\n", pretty)
		}
	}
	if !event.OwnerIDsPresent || !slices.Contains(event.OwnerIDs, session.UserID) {
		return hostPermissionFailure(definition.path, pretty)
	}
	mergedRangeError := validateMergedUpdateRange(definition.path, event, options.Input, pretty)
	if mergedRangeError != nil {
		return *mergedRangeError
	}
	conditionFailure := validateEventUpdatePreconditions(
		definition.path,
		event,
		options.Input,
		clock(),
		pretty,
	)
	if conditionFailure != nil {
		return *conditionFailure
	}
	privateRequest, publicRequest, fieldPaths, prepareErr := buildUpdateRequest(
		ctx,
		definition.path,
		client,
		options.Input,
		session.UserID,
		pretty,
	)
	if prepareErr != nil {
		return *prepareErr
	}
	if execution.DryRun {
		return success(definition.path, eventUpdatePreview{
			Operation:     "firestorePatchEvent",
			EventID:       options.EventID,
			Fields:        options.Input.fields(),
			Input:         options.Input.public(),
			Request:       publicRequest,
			Preconditions: map[string]string{"ownership": "bound", "status": "bound", "targetFields": "bound"},
		}, pretty)
	}
	if err := client.FirestorePatchEvent(ctx, session.AccessToken, options.EventID, fieldPaths, privateRequest); err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return eventSubmissionUnavailableFailure(definition.path, "Update submission could not be confirmed. Inspect remote state before another attempt.", pretty)
		}
		return eventWriteProtocolChangedFailure(definition.path, "EVENT_UPDATE_PROTOCOL_CHANGED", "The event update response no longer matches the reviewed remote contract.", "partiful: event update protocol changed\n", pretty)
	}
	return success(definition.path, eventUpdateSubmitted{EventID: options.EventID, Fields: options.Input.fields(), Submitted: true}, pretty)
}

func executeEventCancel(
	ctx context.Context,
	definition commandDefinition,
	options eventCancelOptions,
	dependencies Dependencies,
	execution mutationExecution,
	pretty bool,
) Result {
	session, sessionFailure := acquireProtectedMutationSession(ctx, definition.path, dependencies, execution, pretty)
	if sessionFailure != nil {
		return *sessionFailure
	}
	if session.UserID == "" {
		return internalFailure(definition.path, pretty)
	}
	clock := time.Now
	if dependencies.Now != nil {
		clock = dependencies.Now
	}
	deviceID, err := randomDeviceID(dependencies.AuthRandom)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	client := remote.Client{HTTP: dependencies.HTTP}
	event, err := client.GetEventInfo(ctx, session.AccessToken, deviceID, options.EventID)
	if err != nil {
		switch {
		case errors.Is(err, remote.ErrEventNotFound):
			return eventNotFoundFailure(definition.path, pretty)
		case errors.Is(err, remote.ErrUnavailable):
			return eventRemoteUnavailableFailure(definition.path, "The event cancellation could not read current event data.", "partiful: event cancel unavailable\n", pretty)
		default:
			return eventWriteProtocolChangedFailure(definition.path, "EVENT_CANCEL_PROTOCOL_CHANGED", "The event cancel flow no longer matches the reviewed remote contract.", "partiful: event cancel protocol changed\n", pretty)
		}
	}
	conditionFailure := validateEventCancelPreconditions(
		definition.path,
		event,
		session.UserID,
		clock(),
		pretty,
	)
	if conditionFailure != nil {
		return *conditionFailure
	}
	privateRequest := remote.CancelEventParams{EventID: options.EventID, CancellationMessage: options.Input.Message, ShouldSkipNotifyGuests: !options.Input.NotifyGuests}
	effects := []string{"Cancels the event"}
	if options.Input.NotifyGuests {
		effects = append(effects, "Sends a cancellation notification to guests")
	}
	if execution.DryRun {
		return success(definition.path, eventCancelPreview{
			Operation: "cancelEvent",
			EventID:   options.EventID,
			Input: eventCancelPreviewInput{
				MessageProvided: options.Input.Message != "",
				NotifyGuests:    options.Input.NotifyGuests,
			},
			Request: eventCancelPreviewRequest{
				EventID:                     options.EventID,
				CancellationMessageProvided: options.Input.Message != "",
				ShouldSkipNotifyGuests:      !options.Input.NotifyGuests,
			},
			Effects:       effects,
			Preconditions: map[string]string{"ownership": "bound", "status": "bound", "start": "bound", "guestCount": "bound"},
		}, pretty)
	}
	if confirmationFailure := execution.confirmDestructive(
		definition,
		event.Title,
		pretty,
	); confirmationFailure != nil {
		return *confirmationFailure
	}
	if err := client.CancelEvent(ctx, session.AccessToken, session.UserID, privateRequest); err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return eventSubmissionUnavailableFailure(definition.path, "Cancel submission could not be confirmed. Inspect remote state before another attempt.", pretty)
		}
		return eventWriteProtocolChangedFailure(definition.path, "EVENT_CANCEL_PROTOCOL_CHANGED", "The event cancel response no longer matches the reviewed remote contract.", "partiful: event cancel protocol changed\n", pretty)
	}
	return success(definition.path, eventCancelSubmitted{EventID: options.EventID, NotifyGuests: options.Input.NotifyGuests, Submitted: true}, pretty)
}

func buildCreateEventDraft(input normalizedEventCreateInput, poster remote.PartifulPosterImage) remote.CreateEventDraft {
	draft := remote.CreateEventDraft{
		Title:                      input.Title,
		StartDate:                  input.Start.UTC().Format(time.RFC3339),
		Timezone:                   input.Timezone,
		GuestStatusCounts:          createEventGuestStatusCounts(),
		DisplaySettings:            defaultCreateDisplaySettings(),
		Status:                     "UNSAVED",
		RSVPButtonGlyphType:        "emojis",
		Image:                      poster,
		ShowHostList:               true,
		ShowGuestCount:             true,
		ShowGuestList:              true,
		ShowActivityTimestamps:     true,
		DisplayInviteButton:        true,
		Visibility:                 "public",
		AllowGuestPhotoUpload:      true,
		EnableGuestReminders:       true,
		RSVPsEnabled:               true,
		AllowGuestsToInviteMutuals: true,
	}
	if input.End != nil {
		value := input.End.UTC().Format(time.RFC3339)
		draft.EndDate = &value
	}
	if input.Description != nil {
		draft.Description = input.Description
	}
	if input.Location != nil {
		draft.LocationInfo = &remote.EventLocationInfo{Type: "freeform", Value: *input.Location}
	}
	if input.Visibility == "public" {
		value := true
		draft.IsPublic = &value
	}
	if input.GuestLimit != nil {
		value := false
		draft.MaxCapacity = input.GuestLimit
		draft.EnableWaitlist = &value
	}
	if len(input.Links) != 0 {
		customFields := make([]remote.EventCustomField, 0, len(input.Links))
		for _, link := range input.Links {
			customFields = append(customFields, remote.EventCustomField{Icon: "link", Value: link.Label, URL: link.URL})
		}
		draft.CustomFields = customFields
	}
	return draft
}

func resolvePoster(
	ctx context.Context,
	client remote.Client,
	posterID string,
) (remote.PartifulPosterImage, error) {
	catalog, err := client.GetPosterCatalog(ctx)
	if err != nil {
		return remote.PartifulPosterImage{}, err
	}
	matches := make([]remote.Poster, 0, 1)
	for _, poster := range catalog.Posters {
		if poster.ID == posterID {
			matches = append(matches, poster)
		}
	}
	if len(matches) == 0 {
		return remote.PartifulPosterImage{}, errPosterNotFound
	}
	if len(matches) != 1 {
		return remote.PartifulPosterImage{}, errPosterDuplicate
	}
	return remote.NewPartifulPosterImage(matches[0]), nil
}

var (
	errPosterNotFound  = errors.New("poster not found")
	errPosterDuplicate = errors.New("poster duplicate")
)

func mapPosterError(command string, err error, pretty bool) Result {
	switch {
	case errors.Is(err, errPosterNotFound):
		result := failure(command, 5, errorBody{Type: "resource.not_found", Code: "POSTER_NOT_FOUND", Message: "The poster was not found.", Retryable: false, Details: map[string]any{}}, pretty)
		result.Stderr = "partiful: poster not found\n"
		return result
	case errors.Is(err, errPosterDuplicate):
		return eventWriteProtocolChangedFailure(command, "POSTER_CATALOG_PROTOCOL_CHANGED", "The poster catalog no longer matches the reviewed remote contract.", "partiful: poster catalog protocol changed\n", pretty)
	case errors.Is(err, remote.ErrUnavailable):
		return eventRemoteUnavailableFailure(command, "The poster catalog is unavailable.", "partiful: poster catalog unavailable\n", pretty)
	default:
		return eventWriteProtocolChangedFailure(command, "POSTER_CATALOG_PROTOCOL_CHANGED", "The poster catalog no longer matches the reviewed remote contract.", "partiful: poster catalog protocol changed\n", pretty)
	}
}

func validateEventUpdatePreconditions(
	command string,
	event remote.Event,
	input normalizedEventUpdateInput,
	now time.Time,
	pretty bool,
) *Result {
	if input.HasStart || input.HasEnd || input.HasTimezone {
		if event.HasGuests.State == remote.FieldAbsent {
			return resultPointer(eventTimeProtocolChangedFailure(command, pretty))
		}
		if event.Safeguards.Ticketing.State == remote.FieldValue {
			return resultPointer(eventPreconditionFailure(command, pretty))
		}
		if event.HasGuests.State != remote.FieldValue {
			return resultPointer(eventTimeProtocolChangedFailure(command, pretty))
		}
		if event.HasGuests.Value {
			start, failure := requiredCurrentEventTime(command, event, "startDate", pretty)
			if failure != nil {
				return resultPointer(*failure)
			}
			end, hasEnd, failure := optionalCurrentEventTime(command, event, "endDate", pretty)
			if failure != nil {
				return resultPointer(*failure)
			}
			boundary := start.Add(8 * time.Hour)
			if hasEnd {
				boundary = end.Add(2 * time.Hour)
			}
			if now.After(boundary) {
				return resultPointer(eventPreconditionFailure(command, pretty))
			}
		}
	}
	return nil
}

func buildUpdateRequest(
	ctx context.Context,
	command string,
	client remote.Client,
	input normalizedEventUpdateInput,
	currentUserID string,
	pretty bool,
) (remote.FirestoreWriteDocument, map[string]any, []string, *Result) {
	fields := make(map[string]any)
	publicFields := make(map[string]any)
	mask := make([]string, 0, 9)
	if input.HasTitle {
		fields["title"] = remote.FirestoreStringValue{StringValue: *input.Title}
		publicFields["title"] = remote.FirestoreStringValue{StringValue: *input.Title}
		mask = append(mask, "title")
	}
	if input.HasDescription {
		mask = append(mask, "description")
		if input.Description != nil {
			fields["description"] = remote.FirestoreStringValue{StringValue: *input.Description}
			publicFields["description"] = remote.FirestoreStringValue{StringValue: *input.Description}
		}
	}
	if input.HasStart {
		value := input.Start.UTC().Format(time.RFC3339)
		fields["startDate"] = remote.FirestoreStringValue{StringValue: value}
		publicFields["startDate"] = remote.FirestoreStringValue{StringValue: value}
		mask = append(mask, "startDate")
	}
	if input.HasEnd {
		mask = append(mask, "endDate")
		if input.End != nil {
			value := input.End.UTC().Format(time.RFC3339)
			fields["endDate"] = remote.FirestoreStringValue{StringValue: value}
			publicFields["endDate"] = remote.FirestoreStringValue{StringValue: value}
		}
	}
	if input.HasTimezone {
		fields["timezone"] = remote.FirestoreStringValue{StringValue: *input.Timezone}
		publicFields["timezone"] = remote.FirestoreStringValue{StringValue: *input.Timezone}
		mask = append(mask, "timezone")
	}
	if input.HasGuestLimit {
		mask = append(mask, "enableWaitlist", "maxCapacity")
		if input.GuestLimit != nil {
			fields["enableWaitlist"] = remote.FirestoreBooleanValue{BooleanValue: false}
			fields["maxCapacity"] = remote.FirestoreIntegerValue{IntegerValue: fmt.Sprintf("%d", *input.GuestLimit)}
			publicFields["enableWaitlist"] = remote.FirestoreBooleanValue{BooleanValue: false}
			publicFields["maxCapacity"] = remote.FirestoreIntegerValue{IntegerValue: fmt.Sprintf("%d", *input.GuestLimit)}
		}
	}
	if input.HasLinks {
		mask = append(mask, "customFields")
		if input.Links != nil {
			fields["customFields"] = remote.FirestoreArrayValue{ArrayValue: remote.FirestoreArray{Values: firestoreLinks(input.Links)}}
			publicFields["customFields"] = fields["customFields"]
		}
	}
	if input.HasPosterID {
		mask = append(mask, "image")
		if input.PosterID != nil {
			image, err := resolvePoster(ctx, client, *input.PosterID)
			if err != nil {
				result := mapPosterError(command, err, pretty)
				return remote.FirestoreWriteDocument{}, nil, nil, &result
			}
			encoded := firestoreImageValue(image)
			fields["image"] = encoded
			publicFields["image"] = encoded
		}
	}
	fields["updatedBy"] = remote.FirestoreReferenceValue{ReferenceValue: "projects/getpartiful/databases/(default)/documents/users/" + currentUserID}
	publicFields["updatedBy"] = remote.FirestoreReferenceValue{ReferenceValue: "<redacted>"}
	mask = append(mask, "updatedBy")
	sort.Strings(mask)
	return remote.FirestoreWriteDocument{Fields: fields}, map[string]any{"fields": publicFields}, mask, nil
}

func firestoreLinks(links []eventLink) []any {
	values := make([]any, 0, len(links))
	for _, link := range links {
		values = append(values, remote.FirestoreMapValue{MapValue: remote.FirestoreMap{Fields: map[string]any{
			"icon":  remote.FirestoreStringValue{StringValue: "link"},
			"url":   remote.FirestoreStringValue{StringValue: link.URL},
			"value": remote.FirestoreStringValue{StringValue: link.Label},
		}}})
	}
	return values
}

func firestoreImageValue(image remote.PartifulPosterImage) remote.FirestoreMapValue {
	posterFields := map[string]any{
		"categories":  remote.FirestoreArrayValue{ArrayValue: remote.FirestoreArray{Values: firestoreStringArray(image.Poster.Categories)}},
		"contentType": remote.FirestoreStringValue{StringValue: image.Poster.ContentType},
		"height":      firestoreNullableInteger(image.Poster.Height),
		"id":          remote.FirestoreStringValue{StringValue: image.Poster.ID},
		"name":        remote.FirestoreStringValue{StringValue: image.Poster.Name},
		"tags":        remote.FirestoreArrayValue{ArrayValue: remote.FirestoreArray{Values: firestoreStringArray(image.Poster.Tags)}},
		"url":         remote.FirestoreStringValue{StringValue: image.Poster.URL},
		"width":       firestoreNullableInteger(image.Poster.Width),
	}
	if image.Poster.BlurHash != nil {
		posterFields["blurHash"] = remote.FirestoreStringValue{StringValue: *image.Poster.BlurHash}
	}
	imageFields := map[string]any{
		"blurHash":    firestoreNullableString(image.BlurHash),
		"contentType": remote.FirestoreStringValue{StringValue: image.ContentType},
		"height":      firestoreNullableInteger(image.Height),
		"name":        remote.FirestoreStringValue{StringValue: image.Name},
		"poster":      remote.FirestoreMapValue{MapValue: remote.FirestoreMap{Fields: posterFields}},
		"source":      remote.FirestoreStringValue{StringValue: image.Source},
		"url":         remote.FirestoreStringValue{StringValue: image.URL},
		"width":       firestoreNullableInteger(image.Width),
	}
	return remote.FirestoreMapValue{MapValue: remote.FirestoreMap{Fields: imageFields}}
}

func firestoreNullableString(value *string) any {
	if value == nil {
		return map[string]any{"nullValue": nil}
	}
	return remote.FirestoreStringValue{StringValue: *value}
}

func firestoreNullableInteger(value *int) any {
	if value == nil {
		return map[string]any{"nullValue": nil}
	}
	return remote.FirestoreIntegerValue{IntegerValue: fmt.Sprintf("%d", *value)}
}

func firestoreStringArray(values []string) []any {
	encoded := make([]any, 0, len(values))
	for _, value := range values {
		encoded = append(encoded, remote.FirestoreStringValue{StringValue: value})
	}
	return encoded
}

func validateEventCancelPreconditions(
	command string,
	event remote.Event,
	currentUserID string,
	now time.Time,
	pretty bool,
) *Result {
	if rawEventField(event, "ownerIds").State != "value" {
		result := eventTimeProtocolChangedFailure(command, pretty)
		return &result
	}
	if !event.OwnerIDsPresent || !slices.Contains(event.OwnerIDs, currentUserID) {
		result := hostPermissionFailure(command, pretty)
		return &result
	}
	if rawEventField(event, "status").State != "value" || event.Status == nil {
		result := eventTimeProtocolChangedFailure(command, pretty)
		return &result
	}
	if *event.Status != "PUBLISHED" {
		result := eventPreconditionFailure(command, pretty)
		return &result
	}
	if rawEventField(event, "guestCount").State != "value" || event.GuestCount.State != remote.FieldValue {
		result := eventTimeProtocolChangedFailure(command, pretty)
		return &result
	}
	if event.GuestCount.Value <= 0 {
		result := eventPreconditionFailure(command, pretty)
		return &result
	}
	start, failure := requiredCurrentEventTime(command, event, "startDate", pretty)
	if failure != nil {
		return resultPointer(*failure)
	}
	if !start.After(now) {
		result := eventPreconditionFailure(command, pretty)
		return &result
	}
	return nil
}

func validateMergedUpdateRange(command string, event remote.Event, input normalizedEventUpdateInput, pretty bool) *Result {
	if !input.HasStart && !input.HasEnd {
		return nil
	}
	var start *time.Time
	if input.HasStart {
		start = input.Start
	} else {
		parsed, failure := requiredCurrentEventTime(command, event, "startDate", pretty)
		if failure != nil {
			return failure
		}
		start = &parsed
	}
	var end *time.Time
	if input.HasEnd {
		end = input.End
	} else if parsed, hasValue, failure := optionalCurrentEventTime(command, event, "endDate", pretty); failure == nil && hasValue {
		end = &parsed
	} else if failure != nil {
		return failure
	}
	if start != nil && end != nil && end.Before(*start) {
		result := failure(command, 2, *eventWriteInputFailure("EVENT_RANGE_INVALID", "End must not be before start."), pretty)
		return &result
	}
	return nil
}

func requiredCurrentEventTime(
	command string,
	event remote.Event,
	field string,
	pretty bool,
) (time.Time, *Result) {
	snapshot := rawEventField(event, field)
	if snapshot.State != "value" {
		result := eventTimeProtocolChangedFailure(command, pretty)
		return time.Time{}, &result
	}
	var value string
	if json.Unmarshal(snapshot.Value, &value) != nil {
		result := eventTimeProtocolChangedFailure(command, pretty)
		return time.Time{}, &result
	}
	decoded, err := time.Parse(time.RFC3339, value)
	if err != nil {
		result := eventTimeProtocolChangedFailure(command, pretty)
		return time.Time{}, &result
	}
	return decoded, nil
}

func optionalCurrentEventTime(
	command string,
	event remote.Event,
	field string,
	pretty bool,
) (time.Time, bool, *Result) {
	snapshot := rawEventField(event, field)
	switch snapshot.State {
	case "absent", "null":
		return time.Time{}, false, nil
	case "value":
		var value string
		if json.Unmarshal(snapshot.Value, &value) != nil {
			result := eventTimeProtocolChangedFailure(command, pretty)
			return time.Time{}, false, &result
		}
		decoded, err := time.Parse(time.RFC3339, value)
		if err != nil {
			result := eventTimeProtocolChangedFailure(command, pretty)
			return time.Time{}, false, &result
		}
		return decoded, true, nil
	default:
		result := eventTimeProtocolChangedFailure(command, pretty)
		return time.Time{}, false, &result
	}
}

func eventTimeProtocolChangedFailure(command string, pretty bool) Result {
	if command == "events.cancel" {
		return eventWriteProtocolChangedFailure(
			command,
			"EVENT_CANCEL_PROTOCOL_CHANGED",
			"The event cancel flow no longer matches the reviewed remote contract.",
			"partiful: event cancel protocol changed\n",
			pretty,
		)
	}
	return eventWriteProtocolChangedFailure(
		command,
		"EVENT_UPDATE_PROTOCOL_CHANGED",
		"The event update no longer matches the reviewed remote contract.",
		"partiful: event update protocol changed\n",
		pretty,
	)
}

func rawEventField(event remote.Event, name string) eventFieldSnapshot {
	raw, ok := event.RawFields[name]
	if !ok {
		return eventFieldSnapshot{State: "absent"}
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return eventFieldSnapshot{State: "null"}
	}
	return eventFieldSnapshot{State: "value", Value: bytes.Clone(raw)}
}

func hostPermissionFailure(command string, pretty bool) Result {
	result := failure(command, 4, errorBody{Type: "permission.denied", Code: "HOST_PERMISSION_REQUIRED", Message: "This command requires host access to the event.", Retryable: false, Details: map[string]any{"requiredRole": "host"}}, pretty)
	result.Stderr = "partiful: host access required\n"
	return result
}

func eventPreconditionFailure(command string, pretty bool) Result {
	result := failure(command, 6, errorBody{Type: "state.conflict", Code: "EVENT_PRECONDITION_FAILED", Message: "The event no longer satisfies this command's reviewed preconditions.", Retryable: false, Details: map[string]any{}}, pretty)
	result.Stderr = "partiful: event precondition failed\n"
	return result
}

func eventRemoteUnavailableFailure(command, message, stderr string, pretty bool) Result {
	result := failure(command, 8, errorBody{Type: "remote.unavailable", Code: "EVENT_REMOTE_UNAVAILABLE", Message: message, Retryable: true, Details: map[string]any{}}, pretty)
	result.Stderr = stderr
	return result
}

func eventWriteProtocolChangedFailure(command, code, message, stderr string, pretty bool) Result {
	result := failure(command, 9, errorBody{Type: "contract.protocol_changed", Code: code, Message: message, Retryable: false, Details: map[string]any{}}, pretty)
	result.Stderr = stderr
	return result
}

func eventSubmissionUnavailableFailure(command, message string, pretty bool) Result {
	result := failure(command, 8, errorBody{Type: "remote.unavailable", Code: "EVENT_SUBMISSION_UNCERTAIN", Message: message, Retryable: false, Details: map[string]any{}}, pretty)
	result.Stderr = "partiful: event submission uncertain\n"
	return result
}

func exitCodeForType(failureType string) int {
	switch failureType {
	case "safety.confirmation_required":
		return 7
	default:
		return 2
	}
}

func resultPointer(result Result) *Result {
	return &result
}
