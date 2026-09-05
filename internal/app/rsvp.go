package app

import (
	"context"
	"errors"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

type rsvpRead struct {
	EventID string  `json:"eventId"`
	Status  *string `json:"status"`
}

func executeRSVPGet(
	ctx context.Context,
	definition commandDefinition,
	eventID string,
	dependencies Dependencies,
	pretty bool,
) Result {
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
	guest, err := (remote.Client{HTTP: dependencies.HTTP}).GetCurrentGuest(
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
	return success(definition.path, rsvpRead{EventID: eventID, Status: status}, pretty)
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
