package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

type guestData struct {
	Items []guest `json:"items"`
}

type guest struct {
	DisplayName string `json:"displayName"`
	RSVPStatus  string `json:"rsvpStatus"`
	PartySize   int    `json:"partySize"`
	Cohost      bool   `json:"cohost"`
}

type guestInviteOptions struct {
	EventID      string
	ContactQuery string
}

type guestInvitePublicInput struct {
	Contact string `json:"contact"`
}

type guestInvitePreview struct {
	Operation     string                 `json:"operation"`
	EventID       string                 `json:"eventId"`
	Input         guestInvitePublicInput `json:"input"`
	Request       any                    `json:"request"`
	Effects       []string               `json:"effects"`
	Preconditions map[string]string      `json:"preconditions"`
}

type guestInviteSubmitted struct {
	EventID   string `json:"eventId"`
	Submitted bool   `json:"submitted"`
}

type guestInviteRequestPreview struct {
	EventID               string           `json:"eventId"`
	UserIDsToInvite       []string         `json:"userIdsToInvite"`
	InvitationMessage     string           `json:"invitationMessage"`
	OtherMutualsCount     int              `json:"otherMutualsCount"`
	PhoneContactsToInvite []map[string]any `json:"phoneContactsToInvite"`
	EmailsToInvite        []map[string]any `json:"emailsToInvite"`
}

func executeGuestsList(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	pretty bool,
) Result {
	eventID, options, inputError := parseGuestListOptions(definition, argv)
	if inputError != nil {
		return failure(definition.path, 2, *inputError, pretty)
	}
	filterHash := normalizedFilterHash(definition.path, eventID)
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
	if session.UserID == "" {
		return internalFailure(definition.path, pretty)
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
			return guestsUnavailableFailure(definition.path, "The guest list is unavailable.", "partiful: guest list unavailable\n", true, pretty)
		default:
			return guestsProtocolChangedFailure(definition.path, "GUESTS_PROTOCOL_CHANGED", "The guests no longer match the reviewed remote contract.", pretty)
		}
	}
	if !event.OwnerIDsPresent {
		return guestsProtocolChangedFailure(definition.path, "GUESTS_PROTOCOL_CHANGED", "The guests no longer match the reviewed remote contract.", pretty)
	}
	if !slices.Contains(event.OwnerIDs, session.UserID) {
		return hostPermissionFailure(definition.path, pretty)
	}
	catalog, err := client.GetGuests(ctx, session.AccessToken, deviceID, eventID)
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return guestsUnavailableFailure(definition.path, "The guest list is unavailable.", "partiful: guest list unavailable\n", true, pretty)
		}
		return guestsProtocolChangedFailure(definition.path, "GUESTS_PROTOCOL_CHANGED", "The guests no longer match the reviewed remote contract.", pretty)
	}
	items, err := projectGuestList(catalog.Guests, event.OwnerIDs, session.UserID)
	if err != nil {
		return guestsProtocolChangedFailure(definition.path, "GUESTS_PROTOCOL_CHANGED", "The guests no longer match the reviewed remote contract.", pretty)
	}
	offset := 0
	if options.cursorProvided {
		var cursorFailure *cursorValidationFailure
		offset, cursorFailure = cursorSnapshotOffset(
			decodedCursor,
			catalog.PayloadSHA256,
			len(items),
			"The guest list changed after this cursor was issued.",
		)
		if cursorFailure != nil {
			return failure(definition.path, cursorFailure.exitCode, cursorFailure.body, pretty)
		}
	}
	end := min(offset+options.limit, len(items))
	pageItems := make([]guest, end-offset)
	copy(pageItems, items[offset:end])
	var cursor *string
	hasMore := end < len(items)
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
	return collectionSuccess(definition.path, guestData{Items: pageItems}, pageMeta{
		Limit:      options.limit,
		NextCursor: cursor,
		HasMore:    hasMore,
	}, pretty)
}

func executeGuestsInvite(
	ctx context.Context,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	execution mutationExecution,
	pretty bool,
) Result {
	options, inputError := parseGuestInviteOptions(definition, argv)
	if inputError != nil {
		return failure(definition.path, exitCodeForType(inputError.Type), *inputError, pretty)
	}
	session, sessionFailure := acquireProtectedMutationSession(ctx, definition.path, dependencies, execution, pretty)
	if sessionFailure != nil {
		return *sessionFailure
	}
	if session.UserID == "" {
		return internalFailure(definition.path, pretty)
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
			return guestsUnavailableFailure(definition.path, "The guest invite could not read current event data.", "partiful: guest invite unavailable\n", true, pretty)
		default:
			return guestsProtocolChangedFailure(definition.path, "GUESTS_INVITE_PROTOCOL_CHANGED", "The guest invite no longer matches the reviewed remote contract.", pretty)
		}
	}
	if !event.OwnerIDsPresent {
		return guestsProtocolChangedFailure(definition.path, "GUESTS_INVITE_PROTOCOL_CHANGED", "The guest invite no longer matches the reviewed remote contract.", pretty)
	}
	if !slices.Contains(event.OwnerIDs, session.UserID) {
		return hostPermissionFailure(definition.path, pretty)
	}
	contacts, err := client.GetContacts(ctx, session.AccessToken, deviceID)
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return guestsUnavailableFailure(definition.path, "The guest invite could not read contacts.", "partiful: guest invite unavailable\n", true, pretty)
		}
		if errors.Is(err, remote.ErrUnauthenticated) {
			return authenticationExpiredFailure(
				definition.path,
				"REMOTE_SESSION_UNAUTHENTICATED",
				"Stored authentication is no longer accepted. Log in again.",
				pretty,
			)
		}
		return guestsProtocolChangedFailure(definition.path, "GUESTS_INVITE_PROTOCOL_CHANGED", "The guest invite no longer matches the reviewed remote contract.", pretty)
	}
	contact, failureBody := resolveInviteContact(contacts.Contacts, options.ContactQuery)
	if failureBody != nil {
		return failure(definition.path, exitCodeForType(failureBody.Type), *failureBody, pretty)
	}
	privateRequest := remote.InviteGuestsAsHostParams{
		EventID:               options.EventID,
		UserIDsToInvite:       []string{contact.ID},
		InvitationMessage:     "",
		OtherMutualsCount:     0,
		PhoneContactsToInvite: []map[string]any{},
		EmailsToInvite:        []map[string]any{},
	}
	if execution.DryRun {
		return success(definition.path, guestInvitePreview{
			Operation: "addInvitedGuestsAsHost",
			EventID:   options.EventID,
			Input: guestInvitePublicInput{
				Contact: contact.Name,
			},
			Request: guestInvitePublicRequest(privateRequest),
			Effects: []string{
				"Submits one host invite request for the selected contact",
			},
			Preconditions: map[string]string{
				"ownership": "bound",
				"contact":   "bound",
			},
		}, pretty)
	}
	if err := client.InviteGuestsAsHost(
		ctx,
		session.AccessToken,
		session.UserID,
		deviceID,
		privateRequest,
	); err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return guestsUnavailableFailure(definition.path, "Guest invite submission could not be confirmed. Inspect remote state before another attempt.", "partiful: guest invite submission uncertain\n", false, pretty)
		}
		return guestsProtocolChangedFailure(definition.path, "GUESTS_INVITE_PROTOCOL_CHANGED", "The guest invite response no longer matches the reviewed remote contract.", pretty)
	}
	return success(definition.path, guestInviteSubmitted{
		EventID:   options.EventID,
		Submitted: true,
	}, pretty)
}

func projectGuestList(
	guests []remote.Guest,
	ownerIDs []string,
	currentUserID string,
) ([]guest, error) {
	projected := make([]guest, 0, len(guests))
	for _, remoteGuest := range guests {
		if remoteGuest.AnchorGuestID != nil {
			continue
		}
		status, err := projectEventRSVP(&remoteGuest.Status)
		if err != nil || status == nil {
			return nil, errors.New("guest status has no product mapping")
		}
		projected = append(projected, guest{
			DisplayName: remoteGuest.Name,
			RSVPStatus:  *status,
			PartySize:   remoteGuest.Count,
			Cohost: remoteGuest.UserID != nil &&
				*remoteGuest.UserID != currentUserID &&
				slices.Contains(ownerIDs, *remoteGuest.UserID),
		})
	}
	return projected, nil
}

func parseGuestInviteOptions(
	definition commandDefinition,
	argv []string,
) (guestInviteOptions, *errorBody) {
	if len(argv) < len(definition.invocation)+1 {
		return guestInviteOptions{}, &errorBody{
			Type:      "input.invalid",
			Code:      "EVENT_ID_REQUIRED",
			Message:   "Event ID is required.",
			Retryable: false,
			Details:   map[string]any{},
		}
	}
	eventID := strings.TrimSpace(argv[len(definition.invocation)])
	if eventID == "" {
		return guestInviteOptions{}, &errorBody{
			Type:      "input.invalid",
			Code:      "EVENT_ID_REQUIRED",
			Message:   "Event ID is required.",
			Retryable: false,
			Details:   map[string]any{},
		}
	}
	values := make(map[string]string)
	allowed := make(map[string]flagDefinition, len(definition.flags))
	for _, flag := range definition.flags {
		allowed[flag.Name] = flag
	}
	for index := len(definition.invocation) + 1; index < len(argv); index++ {
		name := argv[index]
		flag, ok := allowed[name]
		if !ok {
			return guestInviteOptions{}, &errorBody{
				Type:      "input.invalid",
				Code:      "FLAG_UNKNOWN",
				Message:   "The command contains an unknown flag.",
				Retryable: false,
				Details:   map[string]any{},
			}
		}
		if _, repeated := values[name]; repeated {
			return guestInviteOptions{}, &errorBody{
				Type:      "input.invalid",
				Code:      "FLAG_REPEATED",
				Message:   "A scalar flag cannot be repeated.",
				Retryable: false,
				Details:   map[string]any{"flag": name},
			}
		}
		if !flag.TakesValue {
			values[name] = "true"
			continue
		}
		if index+1 >= len(argv) {
			return guestInviteOptions{}, &errorBody{
				Type:      "input.invalid",
				Code:      "FLAG_VALUE_REQUIRED",
				Message:   "A flag value is required.",
				Retryable: false,
				Details:   map[string]any{"flag": name},
			}
		}
		index++
		values[name] = argv[index]
	}
	options := guestInviteOptions{EventID: eventID}
	contactQuery := strings.TrimSpace(values["--contact"])
	if contactQuery == "" {
		return guestInviteOptions{}, &errorBody{
			Type:      "input.invalid",
			Code:      "CONTACT_REQUIRED",
			Message:   "--contact is required.",
			Retryable: false,
			Details:   map[string]any{},
		}
	}
	options.ContactQuery = contactQuery
	return options, nil
}

func parseGuestListOptions(
	definition commandDefinition,
	argv []string,
) (string, collectionOptions, *errorBody) {
	if len(argv) < len(definition.invocation)+1 {
		_, inputError := parseEventID(definition, argv)
		return "", collectionOptions{}, inputError
	}
	eventID, inputError := parseEventID(
		definition,
		argv[:len(definition.invocation)+1],
	)
	if inputError != nil {
		return "", collectionOptions{}, inputError
	}
	if len(argv) == len(definition.invocation)+1 {
		return eventID, collectionOptions{limit: defaultCollectionLimit}, nil
	}
	collectionDefinition := definition
	collectionDefinition.invocation = append(
		append([]string{}, definition.invocation...),
		eventID,
	)
	options, parseError := parseCollectionOptions(collectionDefinition, argv)
	if parseError != nil {
		return "", collectionOptions{}, parseError
	}
	return eventID, options, nil
}

func resolveInviteContact(
	contacts []remote.Contact,
	query string,
) (remote.Contact, *errorBody) {
	filtered := filterContacts(contacts, "")
	normalized := strings.ToLower(strings.TrimSpace(query))
	exact := make([]remote.Contact, 0, 1)
	for _, contact := range filtered {
		if strings.ToLower(strings.TrimSpace(contact.Name)) == normalized {
			exact = append(exact, contact)
		}
	}
	candidates := exact
	if len(candidates) == 0 {
		for _, contact := range filtered {
			if containsFold(contact.Name, normalized) {
				candidates = append(candidates, contact)
			}
		}
	}
	switch len(candidates) {
	case 0:
		return remote.Contact{}, &errorBody{
			Type:      "resource.not_found",
			Code:      "CONTACT_NOT_FOUND",
			Message:   "The contact was not found.",
			Retryable: false,
			Details:   map[string]any{},
		}
	case 1:
		return candidates[0], nil
	default:
		publicCandidates := make([]map[string]any, 0, len(candidates))
		for _, candidate := range candidates {
			publicCandidates = append(publicCandidates, map[string]any{
				"displayName":      candidate.Name,
				"sharedEventCount": candidate.SharedEventCount,
			})
		}
		return remote.Contact{}, &errorBody{
			Type:      "match.ambiguous",
			Code:      "CONTACT_AMBIGUOUS",
			Message:   "More than one contact matches --contact.",
			Retryable: false,
			Details: map[string]any{
				"candidates": publicCandidates,
			},
		}
	}
}

func guestInvitePublicRequest(
	request remote.InviteGuestsAsHostParams,
) guestInviteRequestPreview {
	return guestInviteRequestPreview{
		EventID:               request.EventID,
		UserIDsToInvite:       []string{"<redacted>"},
		InvitationMessage:     request.InvitationMessage,
		OtherMutualsCount:     request.OtherMutualsCount,
		PhoneContactsToInvite: request.PhoneContactsToInvite,
		EmailsToInvite:        request.EmailsToInvite,
	}
}

func guestsCollectionSuccessSchema() jsonSchema {
	zero := 0
	one := 1
	item := objectSchema(
		[]string{"displayName", "rsvpStatus", "partySize", "cohost"},
		map[string]jsonSchema{
			"displayName": {Type: "string", MinLength: &one},
			"rsvpStatus":  {Type: "string", Enum: eventReadRsvpValues()},
			"partySize":   {Type: "integer", Minimum: &zero},
			"cohost":      {Type: "boolean"},
		},
	)
	items := jsonSchema{Type: "array", Items: &item}
	return objectSchema([]string{"items"}, map[string]jsonSchema{"items": items})
}

func guestListInputSchema() jsonSchema {
	one := 1
	schema := collectionInputSchema(false)
	schema.Required = append(schema.Required, "eventId")
	schema.Properties["eventId"] = jsonSchema{Type: "string", MinLength: &one}
	return schema
}

func guestInviteInputSchema() jsonSchema {
	one := 1
	return objectSchema(
		[]string{"eventId", "contact"},
		map[string]jsonSchema{
			"eventId": {Type: "string", MinLength: &one},
			"contact": {Type: "string", MinLength: &one, Pattern: `\S`},
		},
	)
}

func guestInviteSuccessSchema() jsonSchema {
	one := 1
	preview := objectSchema(
		[]string{
			"operation",
			"eventId",
			"input",
			"request",
			"effects",
			"preconditions",
		},
		map[string]jsonSchema{
			"operation":     {Type: "string", Enum: []string{"addInvitedGuestsAsHost"}},
			"eventId":       {Type: "string", MinLength: &one},
			"input":         objectSchema([]string{"contact"}, map[string]jsonSchema{"contact": {Type: "string", MinLength: &one, Pattern: `\S`}}),
			"request":       {Type: "object"},
			"effects":       {Type: "array", Items: pointerSchema(jsonSchema{Type: "string"})},
			"preconditions": objectSchema([]string{"ownership", "contact"}, map[string]jsonSchema{"ownership": {Type: "string", Enum: []string{"bound"}}, "contact": {Type: "string", Enum: []string{"bound"}}}),
		},
	)
	submitted := objectSchema(
		[]string{"eventId", "submitted"},
		map[string]jsonSchema{
			"eventId":   {Type: "string", MinLength: &one},
			"submitted": {Type: "boolean", Const: true},
		},
	)
	return jsonSchema{Type: "object", OneOf: []jsonSchema{preview, submitted}}
}

func guestsUnavailableFailure(
	command string,
	message string,
	stderr string,
	retryable bool,
	pretty bool,
) Result {
	result := failure(command, 8, errorBody{
		Type:      "remote.unavailable",
		Code:      "GUESTS_UNAVAILABLE",
		Message:   message,
		Retryable: retryable,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = stderr
	return result
}

func guestsProtocolChangedFailure(
	command string,
	code string,
	message string,
	pretty bool,
) Result {
	result := failure(command, 9, errorBody{
		Type:      "contract.protocol_changed",
		Code:      code,
		Message:   message,
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: guests protocol changed\n"
	return result
}
