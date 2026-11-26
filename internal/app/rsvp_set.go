package app

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

type rsvpPreview struct {
	Operation     string                     `json:"operation"`
	Mode          string                     `json:"mode"`
	Input         any                        `json:"input"`
	Request       rsvpPreviewAddGuestRequest `json:"request"`
	Preconditions rsvpPublicPreconditions    `json:"preconditions"`
}

type rsvpPublicPreconditions struct {
	CurrentGuest    string `json:"currentGuest"`
	EventSafeguards string `json:"eventSafeguards"`
}

type rsvpPreviewAddGuestRequest struct {
	EventID string                `json:"eventId"`
	RSVP    rsvpPreviewGuestDraft `json:"rsvp"`
}

type rsvpPreviewGuestDraft struct {
	Name                  string                        `json:"name,omitempty"`
	Count                 int                           `json:"count"`
	PlusOnes              []remote.NamedPlusOne         `json:"plusOnes"`
	MessageProvided       bool                          `json:"messageProvided"`
	Status                string                        `json:"status"`
	GuestID               *string                       `json:"guestId,omitempty"`
	Timezone              string                        `json:"timezone"`
	QuestionnaireResponse *remote.QuestionnaireResponse `json:"questionnaireResponse,omitempty"`
	ShouldFollowOrgs      bool                          `json:"shouldFollowOrgs"`
	TicketPaid            bool                          `json:"ticketPaid,omitempty"`
}

type rsvpSubmitted struct {
	EventID   string `json:"eventId"`
	Intent    string `json:"intent"`
	Submitted bool   `json:"submitted"`
}

type rsvpPrivateCurrentGuest struct {
	Marker  string
	GuestID string
	Name    *string
	Status  string
	Count   *int
}

type rsvpPreparedRequest struct {
	Mode         string
	Params       remote.AddGuestParams
	CurrentGuest string
}

type rsvpCompatibilityError uint8

const (
	rsvpProtocolError rsvpCompatibilityError = iota + 1
	rsvpStateError
	rsvpQuestionnaireInputError
	rsvpMaybeNotAllowed
	rsvpMaybeInviteRequired
)

type rsvpRejection struct {
	kind     rsvpCompatibilityError
	message  string
	property string
}

func executeRSVPSet(
	ctx context.Context,
	request Request,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	execution mutationExecution,
	pretty bool,
) Result {
	options, inputError := parseRSVPSetOptions(
		request,
		definition,
		argv,
		dependencies,
	)
	if inputError != nil {
		return failure(definition.path, 2, *inputError, pretty)
	}
	session, sessionFailure := acquireProtectedMutationSession(
		ctx,
		definition.path,
		dependencies,
		execution,
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
	event, err := client.GetEventInfo(
		ctx,
		session.AccessToken,
		deviceID,
		options.EventID,
	)
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
	isMaybe := options.Input.Intent == "interested"
	if isMaybe && event.Safeguards.DisableMaybe.Value {
		return rsvpCompatibilityFailure(definition.path, rsvpRejection{kind: rsvpMaybeNotAllowed}, pretty)
	}
	currentGuest, err := client.GetCurrentGuest(
		ctx,
		session.AccessToken,
		deviceID,
		options.EventID,
	)
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return rsvpUnavailableFailure(definition.path, pretty)
		}
		return rsvpProtocolChangedFailure(definition.path, pretty)
	}
	privateCurrentGuest, rejection := normalizeRSVPCurrentGuest(currentGuest, isMaybe)
	if rejection.kind != 0 {
		return rsvpCompatibilityFailure(definition.path, rejection, pretty)
	}
	prepared, rejection := prepareRSVPRequest(
		options.EventID,
		options.Input,
		event,
		privateCurrentGuest,
	)
	if rejection.kind != 0 {
		return rsvpCompatibilityFailure(definition.path, rejection, pretty)
	}
	if execution.DryRun {
		return success(definition.path, rsvpPreview{
			Operation: "addGuest",
			Mode:      prepared.Mode,
			Input:     options.Input.public(),
			Request:   publicRSVPPreviewRequest(prepared.Params),
			Preconditions: rsvpPublicPreconditions{
				CurrentGuest:    prepared.CurrentGuest,
				EventSafeguards: "bound",
			},
		}, pretty)
	}
	if isMaybe {
		if session.UserID == "" {
			return internalFailure(definition.path, pretty)
		}
		clock := time.Now
		if dependencies.Now != nil {
			clock = dependencies.Now
		}
		err = client.AddGuestMaybe(ctx, session.AccessToken, deviceID, session.UserID,
			clock().UnixMilli(), prepared.Params)
	} else {
		err = client.AddGuest(ctx, session.AccessToken, deviceID, prepared.Params)
	}
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return rsvpSubmitUnavailableFailure(definition.path, pretty)
		}
		return rsvpProtocolChangedFailure(definition.path, pretty)
	}
	return success(definition.path, rsvpSubmitted{
		EventID:   options.EventID,
		Intent:    options.Input.Intent,
		Submitted: true,
	}, pretty)
}

func publicRSVPPreviewRequest(params remote.AddGuestParams) rsvpPreviewAddGuestRequest {
	name := params.RSVP.Name
	if params.RSVP.Status == "MAYBE" {
		// The name came from the private guest read, rather than user input.
		name = ""
	}
	var guestID *string
	if params.RSVP.GuestID != nil {
		redacted := "<redacted>"
		guestID = &redacted
	}
	return rsvpPreviewAddGuestRequest{
		EventID: params.EventID,
		RSVP: rsvpPreviewGuestDraft{
			Name:                  name,
			Count:                 params.RSVP.Count,
			PlusOnes:              params.RSVP.PlusOnes,
			MessageProvided:       params.RSVP.Message != nil,
			Status:                params.RSVP.Status,
			GuestID:               guestID,
			Timezone:              params.RSVP.Timezone,
			QuestionnaireResponse: params.RSVP.QuestionnaireResponse,
			ShouldFollowOrgs:      params.RSVP.ShouldFollowOrgs,
			TicketPaid:            params.RSVP.TicketPaid,
		},
	}
}

func prepareRSVPRequest(
	eventID string,
	input normalizedRSVPInput,
	event remote.Event,
	currentGuest rsvpPrivateCurrentGuest,
) (rsvpPreparedRequest, rsvpRejection) {
	if rejection := validateRSVPEvent(event.Safeguards, input, currentGuest); rejection.kind != 0 {
		return rsvpPreparedRequest{}, rejection
	}
	mode := "create"
	var guestID *string
	if currentGuest.Marker == "present" {
		mode = "update"
		guestID = &currentGuest.GuestID
	}
	if input.Intent == "interested" {
		if guestID == nil {
			return rsvpPreparedRequest{}, rsvpRejection{kind: rsvpMaybeInviteRequired}
		}
		if currentGuest.Name == nil || strings.TrimSpace(*currentGuest.Name) == "" {
			return rsvpPreparedRequest{}, rsvpRejection{kind: rsvpProtocolError}
		}
		timezone := "UTC"
		if event.Timezone != nil && *event.Timezone != "" && validRSVPTimezone(*event.Timezone) {
			timezone = *event.Timezone
		}
		return rsvpPreparedRequest{
			Mode:         mode,
			CurrentGuest: currentGuest.Marker,
			Params: remote.AddGuestParams{
				EventID: eventID,
				RSVP: remote.RSVPDraft{
					Name:     *currentGuest.Name,
					Count:    1,
					PlusOnes: []remote.NamedPlusOne{},
					Status:   "MAYBE",
					GuestID:  guestID,
					Timezone: timezone,
				},
			},
		}, rsvpRejection{}
	}
	product := input.AddGuest
	plusOnes := make([]remote.NamedPlusOne, len(product.PlusOnes))
	for index, name := range product.PlusOnes {
		plusOnes[index] = remote.NamedPlusOne{Name: name}
	}
	var questionnaire *remote.QuestionnaireResponse
	if product.QuestionnaireResponse != nil {
		questionnaire = &remote.QuestionnaireResponse{
			QuestionnaireVersion: product.QuestionnaireResponse.QuestionnaireVersion,
			Answers:              cloneRSVPAnswers(product.QuestionnaireResponse.Answers),
		}
	}
	status := "GOING"
	if input.Intent == "not-going" {
		status = "DECLINED"
	}
	params := remote.AddGuestParams{
		EventID: eventID,
		RSVP: remote.RSVPDraft{
			Name:                  product.DisplayName,
			Count:                 product.PartySize,
			PlusOnes:              plusOnes,
			Message:               product.Message,
			Status:                status,
			GuestID:               guestID,
			Timezone:              product.Timezone,
			QuestionnaireResponse: questionnaire,
			ShouldFollowOrgs:      false,
			TicketPaid:            honorSystemSuggestion(event.Safeguards.Ticketing) && status == "GOING",
		},
	}
	return rsvpPreparedRequest{
		Mode:         mode,
		Params:       params,
		CurrentGuest: currentGuest.Marker,
	}, rsvpRejection{}
}

func normalizeRSVPCurrentGuest(
	current remote.CurrentGuest,
	isMaybe bool,
) (rsvpPrivateCurrentGuest, rsvpRejection) {
	if !current.Present {
		return rsvpPrivateCurrentGuest{Marker: "absent"}, rsvpRejection{}
	}
	snapshot := rsvpPrivateCurrentGuest{
		Marker:  "present",
		GuestID: current.ID,
		Name:    current.Name,
		Status:  current.Status,
	}
	if strings.TrimSpace(current.ID) == "" || current.Status == "" {
		return snapshot, rsvpRejection{kind: rsvpProtocolError}
	}
	if isMaybe {
		// Maybe always sends count 1 and never uses the prior attendance count.
		return snapshot, rsvpRejection{}
	}
	if current.Count == nil ||
		*current.Count < 0 ||
		math.Trunc(*current.Count) != *current.Count ||
		*current.Count > float64(maxInt()) {
		return snapshot, rsvpRejection{kind: rsvpProtocolError}
	}
	count := int(*current.Count)
	snapshot.Count = &count
	return snapshot, rsvpRejection{}
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func validateRSVPEvent(
	event remote.EventSafeguards,
	input normalizedRSVPInput,
	current rsvpPrivateCurrentGuest,
) rsvpRejection {
	if event.RSVPsEnabled.State != remote.FieldValue ||
		event.AtCapacity.State != remote.FieldValue ||
		event.PlusOneNamesRequired.State != remote.FieldValue ||
		event.QuestionnaireVersions.State == remote.FieldAbsent ||
		event.MaxCountPerGuest.State == remote.FieldNull ||
		event.RemainingCapacity.State == remote.FieldNull {
		return rsvpRejection{kind: rsvpProtocolError}
	}
	if event.MaxCountPerGuest.State == remote.FieldValue &&
		event.MaxCountPerGuest.Value < 1 {
		return rsvpRejection{kind: rsvpProtocolError}
	}
	if event.GuestAction.State != remote.FieldAbsent &&
		(event.GuestAction.State != remote.FieldValue ||
			event.GuestAction.Value != "RSVP" &&
				event.GuestAction.Value != "APPLY") {
		return rsvpRejection{kind: rsvpProtocolError}
	}
	if event.Password.State == remote.FieldNull ||
		event.PasswordProtected.State == remote.FieldNull ||
		event.QuestionnaireEnabled.State == remote.FieldNull {
		return rsvpRejection{kind: rsvpProtocolError}
	}
	if !event.RSVPsEnabled.Value {
		return rsvpStateRejection("rsvpsEnabled", "The event has rsvpsEnabled turned off.")
	}
	if event.GuestAction.State == remote.FieldValue &&
		event.GuestAction.Value == "APPLY" {
		return rsvpStateRejection("guestAction", "The event guestAction does not support a direct RSVP.")
	}
	if rejection := rsvpTicketingRejection(event.Ticketing, input.Intent); rejection.kind != 0 {
		return rejection
	}
	if event.Password.State == remote.FieldValue {
		return rsvpStateRejection("password", "The event is password protected.")
	}
	if event.PasswordProtected.State == remote.FieldValue {
		return rsvpStateRejection("passwordProtected", "The event is password protected.")
	}
	if input.Intent == "interested" {
		return rsvpRejection{}
	}
	product := input.AddGuest
	if event.MaxCountPerGuest.State == remote.FieldValue &&
		product.PartySize > event.MaxCountPerGuest.Value {
		return rsvpStateRejection("maxCountPerGuest", "The party is larger than maxCountPerGuest.")
	}
	if event.MaxCapacity.State == remote.FieldValue &&
		event.RemainingCapacity.State != remote.FieldValue {
		return rsvpStateRejection("remainingCapacity", "The event has maxCapacity without remainingCapacity.")
	}
	if input.Intent == "going" {
		if event.AtCapacity.Value {
			return rsvpStateRejection("atCapacity", "The event is at capacity.")
		}
		currentCapacityCount := 0
		if current.Marker == "present" &&
			(current.Status == "GOING" || current.Status == "APPROVED") {
			currentCapacityCount = *current.Count
		}
		additionalCount := max(0, product.PartySize-currentCapacityCount)
		if event.RemainingCapacity.State == remote.FieldValue &&
			additionalCount > event.RemainingCapacity.Value {
			return rsvpStateRejection("remainingCapacity", "The event does not have enough remainingCapacity.")
		}
		if event.QuestionnaireEnabled.State == remote.FieldValue &&
			event.QuestionnaireEnabled.Value {
			if event.QuestionnaireVersions.State != remote.FieldValue ||
				event.QuestionnaireVersions.Length == 0 {
				return rsvpStateRejection("questionnaireVersions", "The event questionnaireVersions are unavailable.")
			}
			if product.QuestionnaireResponse == nil ||
				product.QuestionnaireResponse.QuestionnaireVersion !=
					event.QuestionnaireVersions.Length-1 {
				return rsvpRejection{kind: rsvpQuestionnaireInputError}
			}
		} else if product.QuestionnaireResponse != nil {
			return rsvpRejection{kind: rsvpQuestionnaireInputError}
		}
	}
	return rsvpRejection{}
}

func honorSystemSuggestion(ticketing remote.TicketingField) bool {
	return ticketing.State == remote.FieldValue &&
		ticketing.Class == remote.TicketingOptionalChipIn
}

func rsvpTicketingRejection(
	ticketing remote.TicketingField,
	intent string,
) rsvpRejection {
	switch ticketing.Class {
	case "", remote.TicketingOptionalChipIn:
		return rsvpRejection{}
	case remote.TicketingRequiredChipIn:
		if intent == "going" {
			return rsvpStateRejection("ticketing", "The event requires a chip-in payment.")
		}
		return rsvpRejection{}
	default:
		return rsvpStateRejection("ticketing", "The event uses ticketing.")
	}
}

func rsvpStateRejection(property, message string) rsvpRejection {
	return rsvpRejection{
		kind:     rsvpStateError,
		message:  message,
		property: property,
	}
}

func cloneRSVPAnswers(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func rsvpCompatibilityFailure(
	command string,
	rejection rsvpRejection,
	pretty bool,
) Result {
	switch rejection.kind {
	case rsvpMaybeNotAllowed:
		return rsvpMaybeConflict(command, "MAYBE_NOT_ALLOWED", "The host has disabled Maybe for this event.", pretty)
	case rsvpMaybeInviteRequired:
		return rsvpMaybeConflict(command, "MAYBE_INVITE_REQUIRED", "Maybe requires an existing guest record for this event.", pretty)
	case rsvpStateError:
		details := map[string]any{}
		if rejection.property != "" {
			details["property"] = rejection.property
		}
		result := failure(command, 6, errorBody{
			Type:      "state.conflict",
			Code:      "RSVP_EVENT_UNSUPPORTED",
			Message:   rejection.message,
			Retryable: false,
			Details:   details,
		}, pretty)
		result.Stderr = "partiful: " + rejection.message + "\n"
		return result
	case rsvpQuestionnaireInputError:
		return failure(command, 2, errorBody{
			Type:      "input.invalid",
			Code:      "QUESTIONNAIRE_RESPONSE_INVALID",
			Message:   "Questionnaire response does not match the event.",
			Retryable: false,
			Details:   map[string]any{},
		}, pretty)
	default:
		return rsvpProtocolChangedFailure(command, pretty)
	}
}

func rsvpMaybeConflict(command, code, message string, pretty bool) Result {
	result := failure(command, 6, errorBody{
		Type:      "state.conflict",
		Code:      code,
		Message:   message,
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: " + message + "\n"
	return result
}

func rsvpSubmitUnavailableFailure(command string, pretty bool) Result {
	result := failure(command, 8, errorBody{
		Type:      "remote.unavailable",
		Code:      "RSVP_SUBMISSION_UNCERTAIN",
		Message:   "RSVP submission could not be confirmed. Inspect remote state before another attempt.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: RSVP submission uncertain\n"
	return result
}
