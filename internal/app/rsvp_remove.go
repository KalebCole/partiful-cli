package app

import (
	"context"
	"errors"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

type rsvpRemovePreview struct {
	Operation     string                   `json:"operation"`
	EventID       string                   `json:"eventId"`
	Request       remote.RemoveGuestParams `json:"request"`
	Effects       []string                 `json:"effects"`
	Preconditions map[string]string        `json:"preconditions"`
}

type rsvpRemoveSubmitted struct {
	EventID   string `json:"eventId"`
	Submitted bool   `json:"submitted"`
}

func executeRSVPRemove(ctx context.Context, definition commandDefinition, argv []string, dependencies Dependencies, execution mutationExecution, pretty bool) Result {
	eventID, inputError := parseEventID(definition, argv)
	if inputError != nil {
		return failure(definition.path, 2, *inputError, pretty)
	}
	session, sessionFailure := acquireProtectedMutationSession(ctx, definition.path, dependencies, execution, pretty)
	if sessionFailure != nil {
		return *sessionFailure
	}
	deviceID, err := randomDeviceID(dependencies.AuthRandom)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	client := remote.Client{HTTP: dependencies.HTTP}
	event, err := client.GetEventInfo(ctx, session.AccessToken, deviceID, eventID)
	if err != nil {
		switch {
		case errors.Is(err, remote.ErrEventNotFound):
			return eventNotFoundFailure(definition.path, pretty)
		case errors.Is(err, remote.ErrUnavailable):
			return rsvpUnavailableFailure(definition.path, pretty)
		default:
			return rsvpProtocolChangedFailure(definition.path, pretty)
		}
	}
	current, err := client.GetCurrentGuest(ctx, session.AccessToken, deviceID, eventID)
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return rsvpUnavailableFailure(definition.path, pretty)
		}
		return rsvpProtocolChangedFailure(definition.path, pretty)
	}
	guest, rejection := normalizeRSVPCurrentGuest(current)
	if rejection.kind != 0 {
		return rsvpCompatibilityFailure(definition.path, rejection, pretty)
	}
	if guest.Marker != "present" {
		return failure(definition.path, 6, errorBody{
			Type: "state.conflict", Code: "RSVP_GUEST_REQUIRED",
			Message:   "You are not a guest on this event; there is no guest record to remove.",
			Retryable: false, Details: map[string]any{},
		}, pretty)
	}
	if execution.DryRun {
		return success(definition.path, rsvpRemovePreview{
			Operation: "removeGuest", EventID: eventID,
			Request:       remote.RemoveGuestParams{EventID: eventID, GuestID: "<redacted>"},
			Effects:       []string{"Removes you from the event entirely; this event will no longer appear on your homepage."},
			Preconditions: map[string]string{"currentGuest": "present"},
		}, pretty)
	}
	if confirmationFailure := requireDestructiveConfirmation(definition, event.Title, execution, dependencies, pretty); confirmationFailure != nil {
		return *confirmationFailure
	}
	if err := client.RemoveGuest(ctx, session.AccessToken, deviceID, remote.RemoveGuestParams{EventID: eventID, GuestID: guest.GuestID}); err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return rsvpSubmitUnavailableFailure(definition.path, pretty)
		}
		return rsvpProtocolChangedFailure(definition.path, pretty)
	}
	return success(definition.path, rsvpRemoveSubmitted{EventID: eventID, Submitted: true}, pretty)
}

func rsvpRemoveSuccessSchema() jsonSchema {
	request := objectSchema([]string{"eventId", "guestId"}, map[string]jsonSchema{
		"eventId": {Type: "string"}, "guestId": {Type: "string", Enum: []string{"<redacted>"}},
	})
	effect := jsonSchema{Type: "string"}
	preview := objectSchema([]string{"operation", "eventId", "request", "effects", "preconditions"}, map[string]jsonSchema{
		"operation": {Type: "string", Enum: []string{"removeGuest"}},
		"eventId":   {Type: "string"}, "request": request,
		"effects":       {Type: "array", Items: &effect},
		"preconditions": objectSchema([]string{"currentGuest"}, map[string]jsonSchema{"currentGuest": {Type: "string", Enum: []string{"present"}}}),
	})
	submitted := objectSchema([]string{"eventId", "submitted"}, map[string]jsonSchema{
		"eventId": {Type: "string"}, "submitted": {Type: "boolean", Const: true},
	})
	return jsonSchema{Type: "object", OneOf: []jsonSchema{preview, submitted}}
}
