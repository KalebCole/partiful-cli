package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

const (
	blastAudienceAllGuests      = "all-guests"
	blastOperation              = "createTextBlast"
	blastMessageMaximumRunes    = 480
	blastInviteLimitForAudience = 100
	blastOldEventGraceDays      = 67
	blastNoEndGraceHours        = 6
)

type blastMessageSummary struct {
	SHA256 string `json:"sha256"`
	Length int    `json:"length"`
}

type blastSendPublicInput struct {
	Audience        string              `json:"audience"`
	ShowOnEventPage bool                `json:"showOnEventPage"`
	Message         blastMessageSummary `json:"message"`
}

type blastSendPublicRequest struct {
	EventID string                        `json:"eventId"`
	Message blastSendPublicRequestMessage `json:"message"`
}

type blastSendPublicRequestMessage struct {
	TextSHA256      string   `json:"textSha256"`
	TextLength      int      `json:"textLength"`
	To              []string `json:"to"`
	ShowOnEventPage bool     `json:"showOnEventPage"`
}

type blastSendPreview struct {
	Operation     string                 `json:"operation"`
	EventID       string                 `json:"eventId"`
	Input         blastSendPublicInput   `json:"input"`
	Request       blastSendPublicRequest `json:"request"`
	Effects       []string               `json:"effects"`
	Preconditions map[string]string      `json:"preconditions"`
}

type blastSendSubmitted struct {
	EventID         string `json:"eventId"`
	Submitted       bool   `json:"submitted"`
	Audience        string `json:"audience"`
	ShowOnEventPage bool   `json:"showOnEventPage"`
	RecipientStatus string `json:"recipientStatus"`
}

type blastSendOptions struct {
	EventID         string
	Audience        string
	MessageFile     string
	ShowOnEventPage bool
}

type blastGuestSnapshot struct {
	Total          int            `json:"total"`
	CheckedInCount int            `json:"checkedInCount"`
	StatusCounts   map[string]int `json:"statusCounts"`
}

type blastPreparedMessage struct {
	Text   string
	Digest blastMessageSummary
}

func executeBlastSend(
	ctx context.Context,
	request Request,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	execution mutationExecution,
	pretty bool,
) Result {
	options, inputError := parseBlastSendOptions(definition, argv)
	if inputError != nil {
		return failure(definition.path, exitCodeForType(inputError.Type), *inputError, pretty)
	}
	message, inputError := readBlastMessage(request, dependencies, options.MessageFile)
	if inputError != nil {
		return failure(definition.path, exitCodeForType(inputError.Type), *inputError, pretty)
	}
	publicInput := blastSendPublicInput{
		Audience:        options.Audience,
		ShowOnEventPage: options.ShowOnEventPage,
		Message:         message.Digest,
	}

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
			return blastRemoteUnavailableFailure(definition.path, "The text blast could not read current event data.", "partiful: text blast unavailable\n", pretty)
		default:
			return blastProtocolChangedFailure(definition.path, "TEXT_BLAST_PROTOCOL_CHANGED", "The text blast flow no longer matches the reviewed remote contract.", "partiful: text blast protocol changed\n", pretty)
		}
	}
	if !event.OwnerIDsPresent || !slices.Contains(event.OwnerIDs, session.UserID) {
		return hostPermissionFailure(definition.path, pretty)
	}
	guests, err := client.ListEventGuests(ctx, session.AccessToken, options.EventID)
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return blastRemoteUnavailableFailure(definition.path, "The text blast could not read current guest data.", "partiful: text blast unavailable\n", pretty)
		}
		return blastProtocolChangedFailure(definition.path, "TEXT_BLAST_PROTOCOL_CHANGED", "The text blast flow no longer matches the reviewed remote contract.", "partiful: text blast protocol changed\n", pretty)
	}
	textBlastCount, err := client.CountTextBlasts(ctx, session.AccessToken, options.EventID)
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return blastRemoteUnavailableFailure(definition.path, "The text blast could not read current blast data.", "partiful: text blast unavailable\n", pretty)
		}
		return blastProtocolChangedFailure(definition.path, "TEXT_BLAST_PROTOCOL_CHANGED", "The text blast flow no longer matches the reviewed remote contract.", "partiful: text blast protocol changed\n", pretty)
	}
	toGroups, conditionFailure := validateBlastPreconditions(
		definition.path,
		event,
		guests,
		textBlastCount,
		clock(),
		pretty,
	)
	if conditionFailure != nil {
		return *conditionFailure
	}
	publicRequest := blastSendPublicRequest{
		EventID: options.EventID,
		Message: blastSendPublicRequestMessage{
			TextSHA256:      message.Digest.SHA256,
			TextLength:      message.Digest.Length,
			To:              append([]string(nil), toGroups...),
			ShowOnEventPage: options.ShowOnEventPage,
		},
	}
	effects := []string{"Contacts event guests with a text blast."}
	if options.ShowOnEventPage {
		effects = append(effects, "Shows the blast in the event activity feed.")
	}
	if execution.DryRun {
		return success(definition.path, blastSendPreview{
			Operation: blastOperation,
			EventID:   options.EventID,
			Input:     publicInput,
			Request:   publicRequest,
			Effects:   effects,
			Preconditions: map[string]string{
				"ownership":      "bound",
				"eventTiming":    "bound",
				"audience":       "bound",
				"guestSnapshot":  "bound",
				"existingBlasts": "bound",
			},
		}, pretty)
	}
	if err := client.CreateTextBlast(ctx, session.AccessToken, deviceID, session.UserID, remote.CreateTextBlastParams{
		EventID: options.EventID,
		Message: remote.TextBlastMessage{
			Text:            message.Text,
			To:              append([]string(nil), toGroups...),
			ShowOnEventPage: options.ShowOnEventPage,
		},
	}); err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return blastSubmissionUnavailableFailure(definition.path, pretty)
		}
		return blastProtocolChangedFailure(definition.path, "TEXT_BLAST_PROTOCOL_CHANGED", "The text blast response no longer matches the reviewed remote contract.", "partiful: text blast protocol changed\n", pretty)
	}
	return success(definition.path, blastSendSubmitted{
		EventID:         options.EventID,
		Submitted:       true,
		Audience:        options.Audience,
		ShowOnEventPage: options.ShowOnEventPage,
		RecipientStatus: "not-reported",
	}, pretty)
}

func parseBlastSendOptions(definition commandDefinition, argv []string) (blastSendOptions, *errorBody) {
	eventID, inputError := parseEventID(definition, argv[:len(definition.invocation)+1])
	if inputError != nil {
		return blastSendOptions{}, inputError
	}
	allowed := make(map[string]flagDefinition, len(definition.flags))
	for _, flag := range definition.flags {
		allowed[flag.Name] = flag
	}
	options := blastSendOptions{EventID: eventID}
	scalars := map[string]string{}
	for index := len(definition.invocation) + 1; index < len(argv); index++ {
		name := argv[index]
		flag, ok := allowed[name]
		if !ok {
			return blastSendOptions{}, eventWriteInputFailure("FLAG_UNKNOWN", "The command contains an unknown flag.")
		}
		if _, repeated := scalars[name]; repeated {
			return blastSendOptions{}, eventWriteInputFailureWithDetail("FLAG_REPEATED", "A scalar flag cannot be repeated.", "flag", name)
		}
		if !flag.TakesValue {
			scalars[name] = "true"
			continue
		}
		if index+1 >= len(argv) {
			return blastSendOptions{}, eventWriteInputFailureWithDetail("FLAG_VALUE_REQUIRED", "A flag value is required.", "flag", name)
		}
		index++
		scalars[name] = argv[index]
	}
	options.Audience = scalars["--audience"]
	if options.Audience == "" {
		return blastSendOptions{}, eventWriteInputFailure("AUDIENCE_REQUIRED", "--audience is required.")
	}
	if options.Audience != blastAudienceAllGuests {
		return blastSendOptions{}, eventWriteInputFailure("AUDIENCE_INVALID", "Only all-guests is supported.")
	}
	options.MessageFile = scalars["--message-file"]
	if options.MessageFile == "" {
		return blastSendOptions{}, eventWriteInputFailure("MESSAGE_FILE_REQUIRED", "--message-file is required.")
	}
	_, options.ShowOnEventPage = scalars["--show-on-event-page"]
	return options, nil
}

func readBlastMessage(
	request Request,
	dependencies Dependencies,
	messageFile string,
) (blastPreparedMessage, *errorBody) {
	var (
		document []byte
		err      error
	)
	switch messageFile {
	case "-":
		if request.Stdin == nil {
			document = []byte{}
		} else {
			document, err = io.ReadAll(request.Stdin)
		}
	default:
		if dependencies.Files == nil {
			return blastPreparedMessage{}, eventWriteInputFailure("MESSAGE_FILE_UNREADABLE", "The message file could not be read.")
		}
		document, err = dependencies.Files.ReadFile(messageFile)
	}
	if err != nil {
		return blastPreparedMessage{}, eventWriteInputFailure("MESSAGE_FILE_UNREADABLE", "The message file could not be read.")
	}
	if !utf8.Valid(document) {
		return blastPreparedMessage{}, eventWriteInputFailure("MESSAGE_INVALID_UTF8", "The message must be valid UTF-8 text.")
	}
	text := string(document)
	length := utf8.RuneCountInString(text)
	if length == 0 {
		return blastPreparedMessage{}, eventWriteInputFailure("MESSAGE_EMPTY", "The message must not be empty.")
	}
	if length > blastMessageMaximumRunes {
		return blastPreparedMessage{}, eventWriteInputFailure("MESSAGE_TOO_LONG", "The message exceeds the reviewed maximum length.")
	}
	digest := sha256.Sum256(document)
	return blastPreparedMessage{
		Text: text,
		Digest: blastMessageSummary{
			SHA256: hex.EncodeToString(digest[:]),
			Length: length,
		},
	}, nil
}

func validateBlastPreconditions(
	command string,
	event remote.Event,
	guests []remote.EventGuest,
	textBlastCount int,
	now time.Time,
	pretty bool,
) ([]string, *Result) {
	expired, err := blastEventExpired(event, now)
	if err != nil {
		result := blastProtocolChangedFailure(command, "TEXT_BLAST_PROTOCOL_CHANGED", "The text blast flow no longer matches the reviewed remote contract.", "partiful: text blast protocol changed\n", pretty)
		return nil, &result
	}
	if expired {
		result := eventPreconditionFailure(command, pretty)
		return nil, &result
	}
	snapshot := buildBlastGuestSnapshot(guests)
	toGroups, err := deriveBlastAudience(event, snapshot)
	if err != nil {
		result := blastProtocolChangedFailure(command, "TEXT_BLAST_PROTOCOL_CHANGED", "The text blast flow no longer matches the reviewed remote contract.", "partiful: text blast protocol changed\n", pretty)
		return nil, &result
	}
	if len(toGroups) == 0 || snapshot.Total == 0 || textBlastCount >= 10 {
		result := eventPreconditionFailure(command, pretty)
		return nil, &result
	}
	return toGroups, nil
}

func buildBlastGuestSnapshot(guests []remote.EventGuest) blastGuestSnapshot {
	counts := make(map[string]int)
	checkedInCount := 0
	for _, guest := range guests {
		counts[guest.Status]++
		if guest.CheckedIn {
			checkedInCount++
		}
	}
	return blastGuestSnapshot{Total: len(guests), CheckedInCount: checkedInCount, StatusCounts: counts}
}

func deriveBlastAudience(event remote.Event, guests blastGuestSnapshot) ([]string, error) {
	mode, err := blastAudienceMode(event)
	if err != nil {
		return nil, err
	}
	groups := make([]string, 0, 7)
	invitedCount := 0
	for status, count := range guests.StatusCounts {
		if remoteInvitedGuestStatus(status) {
			invitedCount += count
		}
	}
	if invitedCount > 0 && invitedCount <= blastInviteLimitForAudience {
		groups = append(groups, "invited")
	}
	if guests.CheckedInCount > 0 {
		groups = append(groups, "checkedIn")
	}
	switch mode {
	case "apply":
		for _, status := range []string{"APPROVED", "PENDING_APPROVAL", "WAITLISTED_FOR_APPROVAL", "WITHDRAWN", "REJECTED"} {
			if guests.StatusCounts[status] > 0 {
				groups = append(groups, status)
			}
		}
	case "find-a-time":
		if guests.StatusCounts["RESPONDED_TO_FIND_A_TIME"] > 0 {
			groups = append(groups, "RESPONDED_TO_FIND_A_TIME")
		}
	default:
		for _, status := range []string{"GOING", "MAYBE", "DECLINED"} {
			if guests.StatusCounts[status] > 0 {
				groups = append(groups, status)
			}
		}
		if event.Safeguards.EnableWaitlist.State == remote.FieldValue && event.Safeguards.EnableWaitlist.Value && guests.StatusCounts["WAITLIST"] > 0 {
			groups = append(groups, "WAITLIST")
		}
	}
	return groups, nil
}

func blastAudienceMode(event remote.Event) (string, error) {
	if event.Safeguards.GuestAction.State == remote.FieldValue && event.Safeguards.GuestAction.Value == "APPLY" {
		return "apply", nil
	}
	active, err := blastFindATimeActive(event)
	if err != nil {
		return "", err
	}
	if active {
		return "find-a-time", nil
	}
	return "rsvp", nil
}

func blastFindATimeActive(event remote.Event) (bool, error) {
	raw, ok := event.RawFields["findATime"]
	if !ok {
		return false, fmt.Errorf("findATime is missing")
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return false, nil
	}
	object, err := blastDecodeObject(trimmed)
	if err != nil {
		return false, err
	}
	enabled := false
	if rawEnabled, ok := object["enabled"]; ok {
		if json.Unmarshal(rawEnabled, &enabled) != nil {
			return false, fmt.Errorf("findATime.enabled is invalid")
		}
	}
	rawOptionMap, optionMapPresent := object["optionMap"]
	optionMapActive := optionMapPresent && !bytes.Equal(bytes.TrimSpace(rawOptionMap), []byte("null"))
	selectedRaw, selectedPresent := object["selectedOptionId"]
	selectedActive := selectedPresent && !bytes.Equal(bytes.TrimSpace(selectedRaw), []byte("null"))
	return enabled && optionMapActive && !selectedActive, nil
}

func blastDecodeObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var object map[string]json.RawMessage
	if decoder.Decode(&object) != nil || object == nil {
		return nil, fmt.Errorf("object is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("object is invalid")
	}
	return object, nil
}

func blastEventExpired(event remote.Event, now time.Time) (bool, error) {
	if event.Start == nil || *event.Start == "TBD" {
		return false, fmt.Errorf("startDate is invalid")
	}
	start, err := time.Parse(time.RFC3339, *event.Start)
	if err != nil {
		return false, err
	}
	expiryBase := start.Add(blastNoEndGraceHours * time.Hour)
	if event.End != nil && *event.End != "" {
		end, err := time.Parse(time.RFC3339, *event.End)
		if err != nil {
			return false, err
		}
		expiryBase = end
	}
	return expiryBase.AddDate(0, 0, blastOldEventGraceDays).Before(now), nil
}

func remoteInvitedGuestStatus(status string) bool {
	return status == "READY_TO_SEND" ||
		status == "SENDING" ||
		status == "SENT" ||
		status == "SEND_ERROR" ||
		status == "DELIVERY_ERROR"
}

func blastSendInputSchema() jsonSchema {
	one := 1
	return objectSchema(
		[]string{"audience", "messageFile"},
		map[string]jsonSchema{
			"audience":        {Type: "string", Enum: []string{blastAudienceAllGuests}},
			"messageFile":     {Type: "string", MinLength: &one},
			"showOnEventPage": {Type: "boolean"},
		},
	)
}

func blastSendSuccessSchema() jsonSchema {
	one := 1
	preview := objectSchema(
		[]string{"operation", "eventId", "input", "request", "effects", "preconditions"},
		map[string]jsonSchema{
			"operation": {Type: "string", Enum: []string{blastOperation}},
			"eventId":   {Type: "string", MinLength: &one},
			"input": objectSchema(
				[]string{"audience", "showOnEventPage", "message"},
				map[string]jsonSchema{
					"audience":        {Type: "string", Enum: []string{blastAudienceAllGuests}},
					"showOnEventPage": {Type: "boolean"},
					"message": objectSchema([]string{"sha256", "length"}, map[string]jsonSchema{
						"sha256": {Type: "string", MinLength: &one},
						"length": {Type: "integer"},
					}),
				},
			),
			"request": objectSchema(
				[]string{"eventId", "message"},
				map[string]jsonSchema{
					"eventId": {Type: "string", MinLength: &one},
					"message": objectSchema([]string{"textSha256", "textLength", "to", "showOnEventPage"}, map[string]jsonSchema{
						"textSha256":      {Type: "string", MinLength: &one},
						"textLength":      {Type: "integer"},
						"to":              {Type: "array", Items: pointerSchema(jsonSchema{Type: "string"})},
						"showOnEventPage": {Type: "boolean"},
					}),
				},
			),
			"effects":       {Type: "array", Items: pointerSchema(jsonSchema{Type: "string"})},
			"preconditions": {Type: "object"},
		},
	)
	submitted := objectSchema(
		[]string{"eventId", "submitted", "audience", "showOnEventPage", "recipientStatus"},
		map[string]jsonSchema{
			"eventId":         {Type: "string", MinLength: &one},
			"submitted":       {Type: "boolean", Const: true},
			"audience":        {Type: "string", Enum: []string{blastAudienceAllGuests}},
			"showOnEventPage": {Type: "boolean"},
			"recipientStatus": {Type: "string", Enum: []string{"not-reported"}},
		},
	)
	return jsonSchema{Type: "object", OneOf: []jsonSchema{preview, submitted}}
}

func blastRemoteUnavailableFailure(command, message, stderr string, pretty bool) Result {
	result := failure(command, 8, errorBody{Type: "remote.unavailable", Code: "TEXT_BLAST_UNAVAILABLE", Message: message, Retryable: true, Details: map[string]any{}}, pretty)
	result.Stderr = stderr
	return result
}

func blastProtocolChangedFailure(command, code, message, stderr string, pretty bool) Result {
	result := failure(command, 9, errorBody{Type: "contract.protocol_changed", Code: code, Message: message, Retryable: false, Details: map[string]any{}}, pretty)
	result.Stderr = stderr
	return result
}

func blastSubmissionUnavailableFailure(command string, pretty bool) Result {
	result := failure(command, 8, errorBody{Type: "remote.unavailable", Code: "TEXT_BLAST_SUBMISSION_UNCERTAIN", Message: "Text blast submission could not be confirmed. Inspect remote state before another attempt.", Retryable: false, Details: map[string]any{}}, pretty)
	result.Stderr = "partiful: text blast submission uncertain\n"
	return result
}
