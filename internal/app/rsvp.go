package app

import (
	"context"
	"errors"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

type rsvpRead struct {
	EventID      string  `json:"eventId"`
	Status       *string `json:"status"`
	MaybeAllowed *bool   `json:"maybeAllowed,omitempty"`
}

func executeRSVPGet(
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
	client := remote.Client{HTTP: dependencies.HTTP}
	guest, err := client.GetCurrentGuest(
		ctx,
		session.AccessToken,
		deviceID,
		eventID,
	)
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return rsvpUnavailableFailure(definition.path, pretty)
		}
		return rsvpProtocolChangedFailure(definition.path, pretty)
	}
	var status *string
	if guest.Present {
		status, err = projectEventRSVP(&guest.Status)
		if err != nil {
			return rsvpProtocolChangedFailure(definition.path, pretty)
		}
	}
	result := rsvpRead{EventID: eventID, Status: status}
	// The guest read establishes the RSVP. Event details supply an optional setting.
	if event, err := client.GetEventInfo(ctx, session.AccessToken, deviceID, eventID); err == nil {
		result.MaybeAllowed = projectMaybeAllowed(event.Safeguards)
	}
	return success(definition.path, result, pretty)
}

func rsvpUnavailableFailure(command string, pretty bool) Result {
	result := failure(command, 8, errorBody{
		Type:      "remote.unavailable",
		Code:      "RSVP_UNAVAILABLE",
		Message:   "The RSVP is unavailable.",
		Retryable: true,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: RSVP unavailable\n"
	return result
}

func rsvpProtocolChangedFailure(command string, pretty bool) Result {
	result := failure(command, 9, errorBody{
		Type:      "contract.protocol_changed",
		Code:      "RSVP_PROTOCOL_CHANGED",
		Message:   "The RSVP no longer matches the reviewed remote contract.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: RSVP protocol changed\n"
	return result
}
