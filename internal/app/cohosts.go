package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/KalebCole/partiful-cli/internal/mutation"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

type cohostPlan struct {
	Operation        string            `json:"operation"`
	EventID          string            `json:"eventId"`
	Contact          *cohostPlanTarget `json:"contact,omitempty"`
	Request          any               `json:"request"`
	Effects          []string          `json:"effects"`
	Preconditions    map[string]string `json:"preconditions"`
	ExpiresInSeconds int               `json:"expiresInSeconds"`
	PlanToken        string            `json:"planToken"`
}

type cohostPlanTarget struct {
	DisplayName string `json:"displayName"`
}

type cohostPlanRequest struct {
	EventID string `json:"eventId"`
	Contact string `json:"contact,omitempty"`
}

type cohostContactResult struct {
	EventID string           `json:"eventId"`
	Cohost  cohostResultUser `json:"cohost"`
}

type cohostResultUser struct {
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
}

type cohostLinkResult struct {
	EventID string           `json:"eventId"`
	Link    cohostResultLink `json:"link"`
}

type cohostResultLink struct {
	URL   *string `json:"url"`
	State string  `json:"state"`
}

type cohostPrivateContactPreconditions struct {
	OwnerIDs eventFieldSnapshot   `json:"ownerIds"`
	Contact  cohostPrivateContact `json:"contact"`
	Target   cohostTargetState    `json:"target"`
}

type cohostPrivateContact struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
}

type cohostPrivateLinkPreconditions struct {
	OwnerIDs eventFieldSnapshot `json:"ownerIds"`
	Link     cohostLinkState    `json:"link"`
}

type cohostTargetState struct {
	Marker string `json:"marker"`
	Status string `json:"status,omitempty"`
}

type cohostLinkState struct {
	Marker string `json:"marker"`
	Path   string `json:"path,omitempty"`
}

type resolvedCohostContact struct {
	UserID           string
	DisplayName      string
	SharedEventCount int
}

func executeCohostInvite(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	return executeCohostContactAction(
		ctx,
		definition,
		argv,
		dependencies,
		pretty,
		"createCohostRequest",
		"invited",
		[]string{"Invites the contact to co-host the event."},
		func(state cohostTargetState) bool {
			return state.Marker == "absent" || state.Marker == "present" && state.Status == "DECLINED"
		},
		func(client remote.Client, ctx context.Context, accessToken, deviceID, userID string, params remote.CohostTargetParams) error {
			return client.CreateCohostRequest(ctx, accessToken, deviceID, userID, params)
		},
		"The selected contact cannot be invited with the reviewed cohost lifecycle.",
	)
}

func executeCohostRevokeInvite(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	return executeCohostContactAction(
		ctx,
		definition,
		argv,
		dependencies,
		pretty,
		"deleteCohostRequest",
		"revoked",
		[]string{"Revokes the contact's co-host invitation."},
		func(state cohostTargetState) bool {
			return state.Marker == "present" && (state.Status == "PENDING" || state.Status == "DECLINED")
		},
		func(client remote.Client, ctx context.Context, accessToken, deviceID, userID string, params remote.CohostTargetParams) error {
			return client.DeleteCohostRequest(ctx, accessToken, deviceID, userID, params)
		},
		"The selected contact does not have a revocable co-host invitation.",
	)
}

func executeCohostRemove(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	return executeCohostContactAction(
		ctx,
		definition,
		argv,
		dependencies,
		pretty,
		"removeCohost",
		"removed",
		[]string{"Removes the contact as a co-host."},
		func(state cohostTargetState) bool {
			return state.Marker == "present" && state.Status == "ACCEPTED"
		},
		func(client remote.Client, ctx context.Context, accessToken, deviceID, userID string, params remote.CohostTargetParams) error {
			return client.RemoveCohost(ctx, accessToken, deviceID, userID, params)
		},
		"The selected contact is not an accepted co-host.",
	)
}

func executeCohostContactAction(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
	operation string,
	successStatus string,
	effects []string,
	allowed func(cohostTargetState) bool,
	dispatch func(remote.Client, context.Context, string, string, string, remote.CohostTargetParams) error,
	preconditionMessage string,
) Result {
	options, inputError := parseCohostActionOptions(definition, argv)
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
		inspected, err = authority.Inspect(options.ConfirmToken, definition.path, operation, session.AccountFingerprint, inputDocument)
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
			return eventRemoteUnavailableFailure(definition.path, "The cohost action could not read current event data.", "partiful: cohost action unavailable\n", pretty)
		default:
			return cohostProtocolChangedFailure(definition.path, operation, pretty)
		}
	}
	if !event.OwnerIDsPresent {
		return cohostProtocolChangedFailure(definition.path, operation, pretty)
	}
	if !containsString(event.OwnerIDs, session.UserID) {
		if options.Apply {
			return eventPlanStaleFailure(definition.path, pretty)
		}
		return hostPermissionFailure(definition.path, pretty)
	}
	contacts, err := client.GetContacts(ctx, session.AccessToken, deviceID)
	if err != nil {
		switch {
		case errors.Is(err, remote.ErrUnavailable):
			return eventRemoteUnavailableFailure(definition.path, "The cohost action could not read current contact data.", "partiful: cohost action unavailable\n", pretty)
		case errors.Is(err, remote.ErrUnauthenticated):
			return authenticationExpiredFailure(
				definition.path,
				"REMOTE_SESSION_UNAUTHENTICATED",
				"Stored authentication is no longer accepted. Log in again.",
				pretty,
			)
		default:
			return contactsProtocolChangedFailure(definition.path, pretty)
		}
	}
	requests, err := client.GetCohostRequests(ctx, session.AccessToken, options.EventID)
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return eventRemoteUnavailableFailure(definition.path, "The cohost action could not read current cohost state.", "partiful: cohost action unavailable\n", pretty)
		}
		return cohostProtocolChangedFailure(definition.path, operation, pretty)
	}

	var contact resolvedCohostContact
	if options.Apply {
		expected, decodeErr := decodeCohostTargetRequest(inspected.Binding.Request)
		if decodeErr != nil {
			return internalFailure(definition.path, pretty)
		}
		found, ok := findCohostContactByID(expected.TargetUserID, contacts.Contacts)
		if !ok {
			return eventPlanStaleFailure(definition.path, pretty)
		}
		contact = found
	} else {
		resolved, resolveFailure := resolveCohostContact(options.Input.Contact, contacts.Contacts)
		if resolveFailure != nil {
			return failure(definition.path, resolveFailure.exitCode, resolveFailure.body, pretty)
		}
		contact = resolved
	}

	targetState, stateErr := currentCohostTargetState(requests, contact.UserID)
	if stateErr != nil {
		return cohostProtocolChangedFailure(definition.path, operation, pretty)
	}
	preconditions := cohostPrivateContactPreconditions{
		OwnerIDs: rawEventField(event, "ownerIds"),
		Contact: cohostPrivateContact{
			UserID:      contact.UserID,
			DisplayName: contact.DisplayName,
		},
		Target: targetState,
	}
	requestDocument, _ := json.Marshal(remote.CohostTargetParams{
		EventID:      options.EventID,
		TargetUserID: contact.UserID,
	})
	preconditionDocument, _ := json.Marshal(preconditions)
	if options.Apply && !bytes.Equal(inspected.Binding.Preconditions, preconditionDocument) {
		return eventPlanStaleFailure(definition.path, pretty)
	}
	if !allowed(targetState) {
		return cohostStateFailure(definition.path, preconditionMessage, pretty)
	}
	binding := mutation.Binding{
		Command:            definition.path,
		Operation:          operation,
		AccountFingerprint: session.AccountFingerprint,
		Input:              inputDocument,
		Request:            requestDocument,
		Preconditions:      preconditionDocument,
	}
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
		if err := dispatch(client, ctx, session.AccessToken, deviceID, session.UserID, remote.CohostTargetParams{
			EventID:      options.EventID,
			TargetUserID: contact.UserID,
		}); err != nil {
			if errors.Is(err, remote.ErrUnavailable) {
				return eventSubmissionUnavailableFailure(definition.path, "Cohost submission could not be confirmed. Create a new plan before another attempt.", pretty)
			}
			return cohostProtocolChangedFailure(definition.path, operation, pretty)
		}
		return success(definition.path, cohostContactResult{
			EventID: options.EventID,
			Cohost: cohostResultUser{
				DisplayName: contact.DisplayName,
				Status:      successStatus,
			},
		}, pretty)
	}
	token, err := authority.Create(binding)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	return success(definition.path, cohostPlan{
		Operation: operation,
		EventID:   options.EventID,
		Contact:   &cohostPlanTarget{DisplayName: contact.DisplayName},
		Request: cohostPlanRequest{
			EventID: options.EventID,
			Contact: contact.DisplayName,
		},
		Effects:          effects,
		Preconditions:    map[string]string{"ownership": "bound", "contact": "bound", "cohostState": "bound"},
		ExpiresInSeconds: 300,
		PlanToken:        token,
	}, pretty)
}

func executeCohostLinkCreate(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	return executeCohostLinkAction(
		ctx,
		definition,
		argv,
		dependencies,
		pretty,
		"generateEventCohostLink",
		[]string{"Creates a co-host invite link."},
		func(state cohostLinkState) bool { return state.Marker == "absent" },
		func(client remote.Client, ctx context.Context, accessToken, deviceID, userID string, params remote.CohostLinkParams) (*string, error) {
			path, err := client.GenerateEventCohostLink(ctx, accessToken, deviceID, userID, params)
			if err != nil {
				return nil, err
			}
			url, ok := cohostPathToURL(path)
			if !ok {
				return nil, remote.ErrProtocolChanged
			}
			return &url, nil
		},
		"The co-host invite link already exists.",
		"active",
		true,
	)
}

func executeCohostLinkRevoke(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	return executeCohostLinkAction(
		ctx,
		definition,
		argv,
		dependencies,
		pretty,
		"revokeEventCohostLink",
		[]string{"Revokes the current co-host invite link."},
		func(state cohostLinkState) bool { return state.Marker == "present" },
		func(client remote.Client, ctx context.Context, accessToken, deviceID, userID string, params remote.CohostLinkParams) (*string, error) {
			if err := client.RevokeEventCohostLink(ctx, accessToken, deviceID, userID, params); err != nil {
				return nil, err
			}
			return nil, nil
		},
		"The co-host invite link is already revoked.",
		"revoked",
		false,
	)
}

func executeCohostLinkAction(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
	operation string,
	effects []string,
	allowed func(cohostLinkState) bool,
	dispatch func(remote.Client, context.Context, string, string, string, remote.CohostLinkParams) (*string, error),
	preconditionMessage string,
	successState string,
	emitURL bool,
) Result {
	options, inputError := parseCohostLinkOptions(definition, argv)
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
	inputDocument := json.RawMessage(`{}`)
	var inspected mutation.Record
	if options.Apply {
		var err error
		inspected, err = authority.Inspect(options.ConfirmToken, definition.path, operation, session.AccountFingerprint, inputDocument)
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
			return eventRemoteUnavailableFailure(definition.path, "The cohost link action could not read current event data.", "partiful: cohost action unavailable\n", pretty)
		default:
			return cohostProtocolChangedFailure(definition.path, operation, pretty)
		}
	}
	if !event.OwnerIDsPresent {
		return cohostProtocolChangedFailure(definition.path, operation, pretty)
	}
	if !containsString(event.OwnerIDs, session.UserID) {
		if options.Apply {
			return eventPlanStaleFailure(definition.path, pretty)
		}
		return hostPermissionFailure(definition.path, pretty)
	}
	link, err := client.GetCohostLink(ctx, session.AccessToken, options.EventID)
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return eventRemoteUnavailableFailure(definition.path, "The cohost link action could not read current link state.", "partiful: cohost action unavailable\n", pretty)
		}
		return cohostProtocolChangedFailure(definition.path, operation, pretty)
	}
	linkState := snapshotCohostLinkState(link)
	preconditions := cohostPrivateLinkPreconditions{
		OwnerIDs: rawEventField(event, "ownerIds"),
		Link:     linkState,
	}
	requestDocument, _ := json.Marshal(remote.CohostLinkParams{EventID: options.EventID})
	preconditionDocument, _ := json.Marshal(preconditions)
	if options.Apply && !bytes.Equal(inspected.Binding.Preconditions, preconditionDocument) {
		return eventPlanStaleFailure(definition.path, pretty)
	}
	if !allowed(linkState) {
		return cohostStateFailure(definition.path, preconditionMessage, pretty)
	}
	binding := mutation.Binding{
		Command:            definition.path,
		Operation:          operation,
		AccountFingerprint: session.AccountFingerprint,
		Input:              inputDocument,
		Request:            requestDocument,
		Preconditions:      preconditionDocument,
	}
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
		linkURL, err := dispatch(client, ctx, session.AccessToken, deviceID, session.UserID, remote.CohostLinkParams{EventID: options.EventID})
		if err != nil {
			if errors.Is(err, remote.ErrUnavailable) {
				return eventSubmissionUnavailableFailure(definition.path, "Cohost submission could not be confirmed. Create a new plan before another attempt.", pretty)
			}
			return cohostProtocolChangedFailure(definition.path, operation, pretty)
		}
		if !emitURL {
			linkURL = nil
		}
		return success(definition.path, cohostLinkResult{
			EventID: options.EventID,
			Link: cohostResultLink{
				URL:   linkURL,
				State: successState,
			},
		}, pretty)
	}
	token, err := authority.Create(binding)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	return success(definition.path, cohostPlan{
		Operation:        operation,
		EventID:          options.EventID,
		Request:          cohostPlanRequest{EventID: options.EventID},
		Effects:          effects,
		Preconditions:    map[string]string{"ownership": "bound", "link": "bound"},
		ExpiresInSeconds: 300,
		PlanToken:        token,
	}, pretty)
}

func resolveCohostContact(query string, contacts []remote.Contact) (resolvedCohostContact, *cursorValidationFailure) {
	lowered := strings.ToLower(strings.TrimSpace(query))
	if lowered == "" {
		return resolvedCohostContact{}, &cursorValidationFailure{
			exitCode: 2,
			body: errorBody{
				Type:      "input.invalid",
				Code:      "CONTACT_REQUIRED",
				Message:   "--contact is required.",
				Retryable: false,
				Details:   map[string]any{},
			},
		}
	}
	unique := uniqueContacts(contacts)
	exact := make([]remote.Contact, 0, 1)
	for _, contact := range unique {
		if strings.ToLower(strings.TrimSpace(contact.Name)) == lowered {
			exact = append(exact, contact)
		}
	}
	candidates := exact
	if len(candidates) == 0 {
		for _, contact := range unique {
			if containsFold(contact.Name, lowered) {
				candidates = append(candidates, contact)
			}
		}
	}
	if len(candidates) == 0 {
		return resolvedCohostContact{}, &cursorValidationFailure{
			exitCode: 5,
			body: errorBody{
				Type:      "resource.not_found",
				Code:      "CONTACT_NOT_FOUND",
				Message:   "The contact was not found.",
				Retryable: false,
				Details:   map[string]any{},
			},
		}
	}
	if len(candidates) > 1 {
		safe := make([]map[string]any, 0, len(candidates))
		for _, candidate := range candidates {
			safe = append(safe, map[string]any{
				"displayName":      candidate.Name,
				"sharedEventCount": candidate.SharedEventCount,
			})
		}
		return resolvedCohostContact{}, &cursorValidationFailure{
			exitCode: 2,
			body: errorBody{
				Type:      "match.ambiguous",
				Code:      "CONTACT_AMBIGUOUS",
				Message:   "More than one contact matches that name.",
				Retryable: false,
				Details:   map[string]any{"candidates": safe},
			},
		}
	}
	return resolvedCohostContact{
		UserID:           candidates[0].ID,
		DisplayName:      candidates[0].Name,
		SharedEventCount: candidates[0].SharedEventCount,
	}, nil
}

func findCohostContactByID(userID string, contacts []remote.Contact) (resolvedCohostContact, bool) {
	for _, contact := range uniqueContacts(contacts) {
		if contact.ID == userID {
			return resolvedCohostContact{
				UserID:           contact.ID,
				DisplayName:      contact.Name,
				SharedEventCount: contact.SharedEventCount,
			}, true
		}
	}
	return resolvedCohostContact{}, false
}

func uniqueContacts(contacts []remote.Contact) []remote.Contact {
	seen := make(map[string]struct{}, len(contacts))
	unique := make([]remote.Contact, 0, len(contacts))
	for _, contact := range contacts {
		if contact.ID == "" || contact.Name == "" {
			continue
		}
		if _, ok := seen[contact.ID]; ok {
			continue
		}
		seen[contact.ID] = struct{}{}
		unique = append(unique, contact)
	}
	return unique
}

func currentCohostTargetState(requests []remote.CohostRequest, userID string) (cohostTargetState, error) {
	state := cohostTargetState{Marker: "absent"}
	for _, request := range requests {
		if request.TargetUserID != userID {
			continue
		}
		if state.Marker != "absent" {
			return cohostTargetState{}, errors.New("duplicate cohost request")
		}
		state = cohostTargetState{Marker: "present", Status: request.Status}
	}
	return state, nil
}

func snapshotCohostLinkState(link remote.CohostLink) cohostLinkState {
	if !link.Present {
		return cohostLinkState{Marker: "absent"}
	}
	return cohostLinkState{Marker: "present", Path: link.Path}
}

func decodeCohostTargetRequest(raw json.RawMessage) (remote.CohostTargetParams, error) {
	var request remote.CohostTargetParams
	return request, json.Unmarshal(raw, &request)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func cohostStateFailure(command, message string, pretty bool) Result {
	result := failure(command, 6, errorBody{
		Type:      "state.conflict",
		Code:      "COHOST_PRECONDITION_FAILED",
		Message:   message,
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: cohost precondition failed\n"
	return result
}

func cohostProtocolChangedFailure(command, operation string, pretty bool) Result {
	result := failure(command, 9, errorBody{
		Type:      "contract.protocol_changed",
		Code:      "COHOST_PROTOCOL_CHANGED",
		Message:   "The cohost action no longer matches the reviewed remote contract.",
		Retryable: false,
		Details:   map[string]any{"operation": operation},
	}, pretty)
	result.Stderr = "partiful: cohost protocol changed\n"
	return result
}

func cohostPathToURL(path string) (string, bool) {
	if !strings.HasPrefix(path, "/e/") || !strings.Contains(path, "accept-cohost=") {
		return "", false
	}
	return "https://partiful.com" + path, true
}
