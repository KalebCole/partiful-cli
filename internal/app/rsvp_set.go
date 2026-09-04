package app

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

type rsvpPreview struct {
	Operation     string                  `json:"operation"`
	Mode          string                  `json:"mode"`
	Input         any                     `json:"input"`
	Request       any                     `json:"request"`
	Preconditions rsvpPublicPreconditions `json:"preconditions"`
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
	Name                  string                        `json:"name"`
	Count                 int                           `json:"count"`
	PlusOnes              []remote.NamedPlusOne         `json:"plusOnes"`
	MessageProvided       bool                          `json:"messageProvided"`
	Status                string                        `json:"status"`
	GuestID               *string                       `json:"guestId,omitempty"`
	Timezone              string                        `json:"timezone"`
	QuestionnaireResponse *remote.QuestionnaireResponse `json:"questionnaireResponse,omitempty"`
	ShouldFollowOrgs      bool                          `json:"shouldFollowOrgs"`
}

type rsvpSubmitted struct {
	EventID   string `json:"eventId"`
	Intent    string `json:"intent"`
	Submitted bool   `json:"submitted"`
}

type rsvpPrivateCurrentGuest struct {
	Marker  string `json:"marker"`
	GuestID string `json:"guestId,omitempty"`
	Status  string `json:"status,omitempty"`
	Count   *int   `json:"count,omitempty"`
}

type rsvpPreparedRequest struct {
	Operation    string
	Mode         string
	Private      any
	Public       any
	CurrentGuest string
}

type rsvpCompatibilityError uint8

const (
	rsvpProtocolError rsvpCompatibilityError = iota + 1
	rsvpStateError
	rsvpQuestionnaireInputError
)

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
	privateCurrentGuest, currentGuestError := normalizeRSVPCurrentGuest(currentGuest)
	if currentGuestError != 0 {
		return rsvpCompatibilityFailure(
			definition.path,
			currentGuestError,
			pretty,
		)
	}
	prepared, compatibilityError := prepareRSVPRequest(
		options.EventID,
		options.Input,
		event.Safeguards,
		privateCurrentGuest,
	)
	if compatibilityError != 0 {
		return rsvpCompatibilityFailure(
			definition.path,
			compatibilityError,
			pretty,
		)
	}
	if execution.DryRun {
		return success(definition.path, rsvpPreview{
			Operation: prepared.Operation,
			Mode:      prepared.Mode,
			Input:     options.Input.public(),
			Request:   publicRSVPPreviewRequest(prepared.Public),
			Preconditions: rsvpPublicPreconditions{
				CurrentGuest:    prepared.CurrentGuest,
				EventSafeguards: "bound",
			},
		}, pretty)
	}
	switch params := prepared.Private.(type) {
	case remote.AddGuestParams:
		err = client.AddGuest(
			ctx,
			session.AccessToken,
			deviceID,
			params,
		)
	case remote.MarkEventInterestParams:
		err = client.MarkEventInterest(
			ctx,
			session.AccessToken,
			deviceID,
			params,
		)
	default:
		return internalFailure(definition.path, pretty)
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

func publicRSVPPreviewRequest(request any) any {
	params, ok := request.(remote.AddGuestParams)
	if !ok {
		return request
	}
	return rsvpPreviewAddGuestRequest{
		EventID: params.EventID,
		RSVP: rsvpPreviewGuestDraft{
			Name:                  params.RSVP.Name,
			Count:                 params.RSVP.Count,
			PlusOnes:              params.RSVP.PlusOnes,
			MessageProvided:       params.RSVP.Message != nil,
			Status:                params.RSVP.Status,
			GuestID:               params.RSVP.GuestID,
			Timezone:              params.RSVP.Timezone,
			QuestionnaireResponse: params.RSVP.QuestionnaireResponse,
			ShouldFollowOrgs:      params.RSVP.ShouldFollowOrgs,
		},
	}
}

func prepareRSVPRequest(
	eventID string,
	input normalizedRSVPInput,
	event remote.EventSafeguards,
	currentGuest rsvpPrivateCurrentGuest,
) (rsvpPreparedRequest, rsvpCompatibilityError) {
	if errKind := validateRSVPEvent(event, input, currentGuest); errKind != 0 {
		return rsvpPreparedRequest{}, errKind
	}
	mode := "create"
	if currentGuest.Marker == "present" {
		mode = "update"
	}
	if input.Intent == "interested" {
		params := remote.MarkEventInterestParams{
			EventID:    eventID,
			Interested: true,
		}
		return rsvpPreparedRequest{
			Operation:    "markEventInterest",
			Mode:         mode,
			Private:      params,
			Public:       params,
			CurrentGuest: currentGuest.Marker,
		}, 0
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
	var guestID *string
	if currentGuest.Marker == "present" {
		guestID = &currentGuest.GuestID
	}
	privateParams := remote.AddGuestParams{
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
		},
	}
	publicParams := privateParams
	if publicParams.RSVP.GuestID != nil {
		redacted := "<redacted>"
		publicParams.RSVP.GuestID = &redacted
	}
	return rsvpPreparedRequest{
		Operation:    "addGuest",
		Mode:         mode,
		Private:      privateParams,
		Public:       publicParams,
		CurrentGuest: currentGuest.Marker,
	}, 0
}

func normalizeRSVPCurrentGuest(
	current remote.CurrentGuest,
) (rsvpPrivateCurrentGuest, rsvpCompatibilityError) {
	if !current.Present {
		return rsvpPrivateCurrentGuest{Marker: "absent"}, 0
	}
	snapshot := rsvpPrivateCurrentGuest{
		Marker:  "present",
		GuestID: current.ID,
		Status:  current.Status,
	}
	if strings.TrimSpace(current.ID) == "" ||
		current.Status == "" ||
		current.Count == nil ||
		*current.Count < 0 ||
		math.Trunc(*current.Count) != *current.Count ||
		*current.Count > float64(maxInt()) {
		return snapshot, rsvpProtocolError
	}
	count := int(*current.Count)
	snapshot.Count = &count
	return snapshot, 0
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func validateRSVPEvent(
	event remote.EventSafeguards,
	input normalizedRSVPInput,
	current rsvpPrivateCurrentGuest,
) rsvpCompatibilityError {
	if event.RSVPsEnabled.State != remote.FieldValue ||
		event.AtCapacity.State != remote.FieldValue ||
		event.PlusOneNamesRequired.State != remote.FieldValue ||
		event.QuestionnaireVersions.State == remote.FieldAbsent ||
		event.MaxCountPerGuest.State == remote.FieldNull ||
		event.RemainingCapacity.State == remote.FieldNull {
		return rsvpProtocolError
	}
	if event.MaxCountPerGuest.State == remote.FieldValue &&
		event.MaxCountPerGuest.Value < 1 {
		return rsvpProtocolError
	}
	if event.GuestAction.State != remote.FieldAbsent &&
		(event.GuestAction.State != remote.FieldValue ||
			event.GuestAction.Value != "RSVP" &&
				event.GuestAction.Value != "APPLY") {
		return rsvpProtocolError
	}
	if event.Password.State == remote.FieldNull ||
		event.PasswordProtected.State == remote.FieldNull ||
		event.QuestionnaireEnabled.State == remote.FieldNull {
		return rsvpProtocolError
	}
	if !event.RSVPsEnabled.Value ||
		event.GuestAction.State == remote.FieldValue &&
			event.GuestAction.Value == "APPLY" ||
		event.Ticketing.State == remote.FieldValue ||
		event.Password.State == remote.FieldValue ||
		event.PasswordProtected.State == remote.FieldValue {
		return rsvpStateError
	}
	if input.Intent == "interested" {
		return 0
	}
	product := input.AddGuest
	if event.MaxCountPerGuest.State == remote.FieldValue &&
		product.PartySize > event.MaxCountPerGuest.Value {
		return rsvpStateError
	}
	if event.MaxCapacity.State == remote.FieldValue &&
		event.RemainingCapacity.State != remote.FieldValue {
		return rsvpStateError
	}
	if input.Intent == "going" {
		if event.AtCapacity.Value {
			return rsvpStateError
		}
		currentCapacityCount := 0
		if current.Marker == "present" &&
			(current.Status == "GOING" || current.Status == "APPROVED") {
			currentCapacityCount = *current.Count
		}
		additionalCount := max(0, product.PartySize-currentCapacityCount)
		if event.RemainingCapacity.State == remote.FieldValue &&
			additionalCount > event.RemainingCapacity.Value {
			return rsvpStateError
		}
		if event.QuestionnaireEnabled.State == remote.FieldValue &&
			event.QuestionnaireEnabled.Value {
			if event.QuestionnaireVersions.State != remote.FieldValue ||
				event.QuestionnaireVersions.Length == 0 {
				return rsvpStateError
			}
			if product.QuestionnaireResponse == nil ||
				product.QuestionnaireResponse.QuestionnaireVersion !=
					event.QuestionnaireVersions.Length-1 {
				return rsvpQuestionnaireInputError
			}
		} else if product.QuestionnaireResponse != nil {
			return rsvpQuestionnaireInputError
		}
	}
	return 0
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
	kind rsvpCompatibilityError,
	pretty bool,
) Result {
	switch kind {
	case rsvpStateError:
		result := failure(command, 6, errorBody{
			Type:      "state.conflict",
			Code:      "RSVP_EVENT_UNSUPPORTED",
			Message:   "The event does not support this RSVP safely.",
			Retryable: false,
			Details:   map[string]any{},
		}, pretty)
		result.Stderr = "partiful: RSVP event unsupported\n"
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
