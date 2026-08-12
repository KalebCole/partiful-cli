package app

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/KalebCole/partiful-cli/internal/mutation"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

type eventCreatePlan struct {
	Operation        string                 `json:"operation"`
	Input            eventCreatePublicInput `json:"input"`
	Request          any                    `json:"request"`
	Preconditions    map[string]string      `json:"preconditions"`
	ExpiresInSeconds int                    `json:"expiresInSeconds"`
	PlanToken        string                 `json:"planToken"`
}

type eventUpdatePlan struct {
	Operation        string            `json:"operation"`
	EventID          string            `json:"eventId"`
	Fields           []string          `json:"fields"`
	Input            map[string]any    `json:"input"`
	Request          any               `json:"request"`
	Preconditions    map[string]string `json:"preconditions"`
	ExpiresInSeconds int               `json:"expiresInSeconds"`
	PlanToken        string            `json:"planToken"`
}

type eventCancelPlan struct {
	Operation        string                     `json:"operation"`
	EventID          string                     `json:"eventId"`
	Input            normalizedEventCancelInput `json:"input"`
	Request          remote.CancelEventParams   `json:"request"`
	Effects          []string                   `json:"effects"`
	Preconditions    map[string]string          `json:"preconditions"`
	ExpiresInSeconds int                        `json:"expiresInSeconds"`
	PlanToken        string                     `json:"planToken"`
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

type eventPosterBinding struct {
	CatalogDigest string                `json:"catalogDigest"`
	Poster        remote.PartifulPoster `json:"poster"`
}

type eventFieldSnapshot struct {
	State string          `json:"state"`
	Value json.RawMessage `json:"value,omitempty"`
}

type eventUpdatePrivatePreconditions struct {
	OwnerIDs  eventFieldSnapshot            `json:"ownerIds"`
	Status    eventFieldSnapshot            `json:"status"`
	Target    map[string]eventFieldSnapshot `json:"target"`
	DateGuard map[string]eventFieldSnapshot `json:"dateGuard,omitempty"`
}

type eventCancelPrivatePreconditions struct {
	OwnerIDs   eventFieldSnapshot `json:"ownerIds"`
	Status     eventFieldSnapshot `json:"status"`
	StartDate  eventFieldSnapshot `json:"startDate"`
	GuestCount eventFieldSnapshot `json:"guestCount"`
}

func executeEventCreate(
	ctx context.Context,
	request Request,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	options, inputError := parseEventCreateOptions(request, definition, argv, dependencies)
	if inputError != nil {
		return failure(definition.path, exitCodeForType(inputError.Type), *inputError, pretty)
	}
	session, sessionFailure := acquireProtectedSession(ctx, definition.path, dependencies, pretty)
	if sessionFailure != nil {
		return *sessionFailure
	}
	if session.AccountFingerprint == "" {
		return internalFailure(definition.path, pretty)
	}
	clock := time.Now
	if dependencies.Now != nil {
		clock = dependencies.Now
	}
	authority := mutation.Authority{Files: dependencies.Files, Path: eventMutationPath(dependencies), Now: clock, Random: dependencies.MutationRandom}
	inputDocument := options.Input.document()
	var inspected mutation.Record
	if options.Apply {
		var err error
		inspected, err = authority.Inspect(options.PlanToken, definition.path, "createEvent", session.AccountFingerprint, inputDocument)
		if err != nil {
			if errors.Is(err, mutation.ErrStale) {
				return eventPlanStaleFailure(definition.path, pretty)
			}
			return internalFailure(definition.path, pretty)
		}
	}
	client := remote.Client{HTTP: dependencies.HTTP}
	posterImage, posterBinding, err := resolvePosterBinding(ctx, client, options.Input.PosterID)
	if err != nil {
		return mapPosterError(definition.path, err, pretty)
	}
	privateRequest := remote.CreateEventParams{Event: buildCreateEventDraft(options.Input, posterImage), CohostIDs: []string{}}
	requestDocument, _ := json.Marshal(privateRequest)
	preconditionDocument, _ := json.Marshal(posterBinding)
	binding := mutation.Binding{Command: definition.path, Operation: "createEvent", AccountFingerprint: session.AccountFingerprint, Input: inputDocument, Request: requestDocument, Preconditions: preconditionDocument}
	if options.Apply {
		if !bytes.Equal(inspected.Binding.Request, requestDocument) || !bytes.Equal(inspected.Binding.Preconditions, preconditionDocument) {
			return eventPlanStaleFailure(definition.path, pretty)
		}
		if err := authority.Consume(options.PlanToken, binding); err != nil {
			if errors.Is(err, mutation.ErrStale) {
				return eventPlanStaleFailure(definition.path, pretty)
			}
			return internalFailure(definition.path, pretty)
		}
		_, err := client.CreateEvent(ctx, session.AccessToken, session.UserID, privateRequest)
		if err != nil {
			if errors.Is(err, remote.ErrUnavailable) {
				return eventSubmissionUnavailableFailure(definition.path, "Create submission could not be confirmed. Create a new plan before another attempt.", pretty)
			}
			return eventWriteProtocolChangedFailure(definition.path, "CREATE_EVENT_PROTOCOL_CHANGED", "The event create response no longer matches the reviewed remote contract.", "partiful: event create protocol changed\n", pretty)
		}
		return success(definition.path, eventCreateSubmitted{Submitted: true}, pretty)
	}
	token, err := authority.Create(binding)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	return success(definition.path, eventCreatePlan{
		Operation:        "createEvent",
		Input:            options.Input.public(),
		Request:          privateRequest,
		Preconditions:    map[string]string{"poster": "bound"},
		ExpiresInSeconds: 300,
		PlanToken:        token,
	}, pretty)
}

func executeEventUpdate(
	ctx context.Context,
	request Request,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	options, inputError := parseEventUpdateOptions(request, definition, argv, dependencies)
	if inputError != nil {
		return failure(definition.path, exitCodeForType(inputError.Type), *inputError, pretty)
	}
	session, sessionFailure := acquireProtectedSession(ctx, definition.path, dependencies, pretty)
	if sessionFailure != nil {
		return *sessionFailure
	}
	if session.AccountFingerprint == "" || session.UserID == "" {
		return internalFailure(definition.path, pretty)
	}
	clock := time.Now
	if dependencies.Now != nil {
		clock = dependencies.Now
	}
	authority := mutation.Authority{Files: dependencies.Files, Path: eventMutationPath(dependencies), Now: clock, Random: dependencies.MutationRandom}
	inputDocument := options.Input.document()
	var inspected mutation.Record
	if options.Apply {
		var err error
		inspected, err = authority.Inspect(options.PlanToken, definition.path, "firestorePatchEvent", session.AccountFingerprint, inputDocument)
		if err != nil {
			if errors.Is(err, mutation.ErrStale) {
				return eventPlanStaleFailure(definition.path, pretty)
			}
			return internalFailure(definition.path, pretty)
		}
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
			if options.Apply {
				return eventPlanStaleFailure(definition.path, pretty)
			}
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
	preconditions, conditionFailure := buildUpdatePreconditions(
		definition.path,
		event,
		session.UserID,
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
		event,
		options.Input,
		session.UserID,
		pretty,
	)
	if prepareErr != nil {
		return *prepareErr
	}
	requestDocument, _ := json.Marshal(privateRequest)
	preconditionDocument, _ := json.Marshal(preconditions)
	binding := mutation.Binding{Command: definition.path, Operation: "firestorePatchEvent", AccountFingerprint: session.AccountFingerprint, Input: inputDocument, Request: requestDocument, Preconditions: preconditionDocument}
	if options.Apply {
		if !bytes.Equal(inspected.Binding.Request, requestDocument) || !bytes.Equal(inspected.Binding.Preconditions, preconditionDocument) {
			return eventPlanStaleFailure(definition.path, pretty)
		}
		if err := authority.Consume(options.PlanToken, binding); err != nil {
			if errors.Is(err, mutation.ErrStale) {
				return eventPlanStaleFailure(definition.path, pretty)
			}
			return internalFailure(definition.path, pretty)
		}
		if err := client.FirestorePatchEvent(ctx, session.AccessToken, options.EventID, fieldPaths, privateRequest); err != nil {
			if errors.Is(err, remote.ErrUnavailable) {
				return eventSubmissionUnavailableFailure(definition.path, "Update submission could not be confirmed. Create a new plan before another attempt.", pretty)
			}
			return eventWriteProtocolChangedFailure(definition.path, "EVENT_UPDATE_PROTOCOL_CHANGED", "The event update response no longer matches the reviewed remote contract.", "partiful: event update protocol changed\n", pretty)
		}
		return success(definition.path, eventUpdateSubmitted{EventID: options.EventID, Fields: options.Input.fields(), Submitted: true}, pretty)
	}
	token, err := authority.Create(binding)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	return success(definition.path, eventUpdatePlan{
		Operation:        "firestorePatchEvent",
		EventID:          options.EventID,
		Fields:           options.Input.fields(),
		Input:            options.Input.public(),
		Request:          publicRequest,
		Preconditions:    map[string]string{"ownership": "bound", "status": "bound", "targetFields": "bound"},
		ExpiresInSeconds: 300,
		PlanToken:        token,
	}, pretty)
}

func executeEventCancel(
	ctx context.Context,
	request Request,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	options, inputError := parseEventCancelOptions(request, definition, argv, dependencies)
	if inputError != nil {
		return failure(definition.path, exitCodeForType(inputError.Type), *inputError, pretty)
	}
	session, sessionFailure := acquireProtectedSession(ctx, definition.path, dependencies, pretty)
	if sessionFailure != nil {
		return *sessionFailure
	}
	if session.AccountFingerprint == "" || session.UserID == "" {
		return internalFailure(definition.path, pretty)
	}
	clock := time.Now
	if dependencies.Now != nil {
		clock = dependencies.Now
	}
	authority := mutation.Authority{Files: dependencies.Files, Path: eventMutationPath(dependencies), Now: clock, Random: dependencies.MutationRandom}
	inputDocument := options.Input.document()
	var inspected mutation.Record
	if options.Apply {
		var err error
		inspected, err = authority.Inspect(options.ConfirmToken, definition.path, "cancelEvent", session.AccountFingerprint, inputDocument)
		if err != nil {
			if errors.Is(err, mutation.ErrStale) {
				return eventPlanStaleFailure(definition.path, pretty)
			}
			return internalFailure(definition.path, pretty)
		}
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
			if options.Apply {
				return eventPlanStaleFailure(definition.path, pretty)
			}
			return eventNotFoundFailure(definition.path, pretty)
		case errors.Is(err, remote.ErrUnavailable):
			return eventRemoteUnavailableFailure(definition.path, "The event cancellation could not read current event data.", "partiful: event cancel unavailable\n", pretty)
		default:
			return eventWriteProtocolChangedFailure(definition.path, "EVENT_CANCEL_PROTOCOL_CHANGED", "The event cancel flow no longer matches the reviewed remote contract.", "partiful: event cancel protocol changed\n", pretty)
		}
	}
	preconditions, conditionFailure := buildCancelPreconditions(
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
	requestDocument, _ := json.Marshal(privateRequest)
	preconditionDocument, _ := json.Marshal(preconditions)
	binding := mutation.Binding{Command: definition.path, Operation: "cancelEvent", AccountFingerprint: session.AccountFingerprint, Input: inputDocument, Request: requestDocument, Preconditions: preconditionDocument}
	if options.Apply {
		if !bytes.Equal(inspected.Binding.Request, requestDocument) || !bytes.Equal(inspected.Binding.Preconditions, preconditionDocument) {
			return eventPlanStaleFailure(definition.path, pretty)
		}
		if err := authority.Consume(options.ConfirmToken, binding); err != nil {
			if errors.Is(err, mutation.ErrStale) {
				return eventPlanStaleFailure(definition.path, pretty)
			}
			return internalFailure(definition.path, pretty)
		}
		if err := client.CancelEvent(ctx, session.AccessToken, session.UserID, privateRequest); err != nil {
			if errors.Is(err, remote.ErrUnavailable) {
				return eventSubmissionUnavailableFailure(definition.path, "Cancel submission could not be confirmed. Create a new plan before another attempt.", pretty)
			}
			return eventWriteProtocolChangedFailure(definition.path, "EVENT_CANCEL_PROTOCOL_CHANGED", "The event cancel response no longer matches the reviewed remote contract.", "partiful: event cancel protocol changed\n", pretty)
		}
		return success(definition.path, eventCancelSubmitted{EventID: options.EventID, NotifyGuests: options.Input.NotifyGuests, Submitted: true}, pretty)
	}
	token, err := authority.Create(binding)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	effects := []string{"Cancels the event"}
	if options.Input.NotifyGuests {
		effects = append(effects, "Sends a cancellation notification to guests")
	}
	return success(definition.path, eventCancelPlan{
		Operation:        "cancelEvent",
		EventID:          options.EventID,
		Input:            options.Input,
		Request:          privateRequest,
		Effects:          effects,
		Preconditions:    map[string]string{"ownership": "bound", "status": "bound", "start": "bound", "guestCount": "bound"},
		ExpiresInSeconds: 300,
		PlanToken:        token,
	}, pretty)
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

func resolvePosterBinding(ctx context.Context, client remote.Client, posterID string) (remote.PartifulPosterImage, eventPosterBinding, error) {
	catalog, err := client.GetPosterCatalog(ctx)
	if err != nil {
		return remote.PartifulPosterImage{}, eventPosterBinding{}, err
	}
	matches := make([]remote.Poster, 0, 1)
	for _, poster := range catalog.Posters {
		if poster.ID == posterID {
			matches = append(matches, poster)
		}
	}
	if len(matches) == 0 {
		return remote.PartifulPosterImage{}, eventPosterBinding{}, errPosterNotFound
	}
	if len(matches) != 1 {
		return remote.PartifulPosterImage{}, eventPosterBinding{}, errPosterDuplicate
	}
	posterImage := remote.NewPartifulPosterImage(matches[0])
	return posterImage, eventPosterBinding{CatalogDigest: hex.EncodeToString(catalog.PayloadSHA256[:]), Poster: posterImage.Poster}, nil
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

func buildUpdatePreconditions(
	command string,
	event remote.Event,
	currentUserID string,
	input normalizedEventUpdateInput,
	now time.Time,
	pretty bool,
) (eventUpdatePrivatePreconditions, *Result) {
	preconditions := eventUpdatePrivatePreconditions{
		OwnerIDs: rawEventField(event, "ownerIds"),
		Status:   rawEventField(event, "status"),
		Target:   make(map[string]eventFieldSnapshot),
	}
	if input.HasTitle {
		preconditions.Target["title"] = rawEventField(event, "title")
	}
	if input.HasDescription {
		preconditions.Target["description"] = rawEventField(event, "description")
	}
	if input.HasStart {
		preconditions.Target["startDate"] = rawEventField(event, "startDate")
	}
	if input.HasEnd {
		preconditions.Target["endDate"] = rawEventField(event, "endDate")
	}
	if input.HasTimezone {
		preconditions.Target["timezone"] = rawEventField(event, "timezone")
	}
	if input.HasGuestLimit {
		preconditions.Target["enableWaitlist"] = rawEventField(event, "enableWaitlist")
		preconditions.Target["maxCapacity"] = rawEventField(event, "maxCapacity")
	}
	if input.HasLinks {
		preconditions.Target["customFields"] = rawEventField(event, "customFields")
	}
	if input.HasPosterID {
		preconditions.Target["image"] = rawEventField(event, "image")
	}
	if input.HasStart || input.HasEnd || input.HasTimezone {
		if event.HasGuests.State == remote.FieldAbsent {
			return eventUpdatePrivatePreconditions{}, resultPointer(eventTimeProtocolChangedFailure(command, pretty))
		}
		if event.Safeguards.Ticketing.State == remote.FieldValue {
			return eventUpdatePrivatePreconditions{}, resultPointer(eventPreconditionFailure(command, pretty))
		}
		preconditions.DateGuard = map[string]eventFieldSnapshot{
			"startDate": rawEventField(event, "startDate"),
			"endDate":   rawEventField(event, "endDate"),
			"hasGuests": rawEventField(event, "hasGuests"),
			"ticketing": rawEventField(event, "ticketing"),
		}
		if event.HasGuests.State != remote.FieldValue {
			return eventUpdatePrivatePreconditions{}, resultPointer(eventTimeProtocolChangedFailure(command, pretty))
		}
		if event.HasGuests.Value {
			start, failure := requiredCurrentEventTime(command, event, "startDate", pretty)
			if failure != nil {
				return eventUpdatePrivatePreconditions{}, resultPointer(*failure)
			}
			end, hasEnd, failure := optionalCurrentEventTime(command, event, "endDate", pretty)
			if failure != nil {
				return eventUpdatePrivatePreconditions{}, resultPointer(*failure)
			}
			boundary := start.Add(8 * time.Hour)
			if hasEnd {
				boundary = end.Add(2 * time.Hour)
			}
			if now.After(boundary) {
				return eventUpdatePrivatePreconditions{}, resultPointer(eventPreconditionFailure(command, pretty))
			}
		}
	}
	_ = currentUserID
	return preconditions, nil
}

func buildUpdateRequest(
	ctx context.Context,
	command string,
	client remote.Client,
	event remote.Event,
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
			image, _, err := resolvePosterBinding(ctx, client, *input.PosterID)
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
	_ = event
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

func buildCancelPreconditions(
	command string,
	event remote.Event,
	currentUserID string,
	now time.Time,
	pretty bool,
) (eventCancelPrivatePreconditions, *Result) {
	preconditions := eventCancelPrivatePreconditions{
		OwnerIDs:   rawEventField(event, "ownerIds"),
		Status:     rawEventField(event, "status"),
		StartDate:  rawEventField(event, "startDate"),
		GuestCount: rawEventField(event, "guestCount"),
	}
	if preconditions.OwnerIDs.State != "value" {
		result := eventTimeProtocolChangedFailure(command, pretty)
		return eventCancelPrivatePreconditions{}, &result
	}
	if !event.OwnerIDsPresent || !slices.Contains(event.OwnerIDs, currentUserID) {
		result := hostPermissionFailure(command, pretty)
		return eventCancelPrivatePreconditions{}, &result
	}
	if preconditions.Status.State != "value" || event.Status == nil {
		result := eventTimeProtocolChangedFailure(command, pretty)
		return eventCancelPrivatePreconditions{}, &result
	}
	if *event.Status != "PUBLISHED" {
		result := eventPreconditionFailure(command, pretty)
		return eventCancelPrivatePreconditions{}, &result
	}
	if preconditions.GuestCount.State != "value" || event.GuestCount.State != remote.FieldValue {
		result := eventTimeProtocolChangedFailure(command, pretty)
		return eventCancelPrivatePreconditions{}, &result
	}
	if event.GuestCount.Value <= 0 {
		result := eventPreconditionFailure(command, pretty)
		return eventCancelPrivatePreconditions{}, &result
	}
	start, failure := requiredCurrentEventTime(command, event, "startDate", pretty)
	if failure != nil {
		return eventCancelPrivatePreconditions{}, resultPointer(*failure)
	}
	if !start.After(now) {
		result := eventPreconditionFailure(command, pretty)
		return eventCancelPrivatePreconditions{}, &result
	}
	return preconditions, nil
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

func eventPlanStaleFailure(command string, pretty bool) Result {
	result := failure(command, 7, errorBody{Type: "safety.plan_stale", Code: "PLAN_STALE", Message: "The mutation plan is expired, used, or no longer matches.", Retryable: false, Details: map[string]any{}}, pretty)
	result.Stderr = "partiful: mutation plan stale\n"
	return result
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
	case "safety.confirmation_required", "safety.plan_stale":
		return 7
	default:
		return 2
	}
}

func eventMutationPath(dependencies Dependencies) string {
	if dependencies.MutationPath != "" {
		return dependencies.MutationPath
	}
	return "/config/partiful/mutation-plans.json"
}

func resultPointer(result Result) *Result {
	return &result
}
