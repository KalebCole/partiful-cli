package app

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

type eventLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type normalizedEventCreateInput struct {
	Title       string
	Start       time.Time
	StartText   string
	End         *time.Time
	EndText     *string
	Timezone    string
	Description *string
	Location    *string
	Visibility  string
	GuestLimit  *int
	Links       []eventLink
	PosterID    string
}

type normalizedEventUpdateInput struct {
	Title          *string
	HasTitle       bool
	Description    *string
	HasDescription bool
	Start          *time.Time
	StartText      *string
	HasStart       bool
	End            *time.Time
	EndText        *string
	HasEnd         bool
	Timezone       *string
	HasTimezone    bool
	GuestLimit     *int
	HasGuestLimit  bool
	Links          []eventLink
	HasLinks       bool
	PosterID       *string
	HasPosterID    bool
}

type normalizedEventCancelInput struct {
	Message      string `json:"message"`
	NotifyGuests bool   `json:"notifyGuests"`
}

type eventCreateOptions struct {
	Input     normalizedEventCreateInput
	Apply     bool
	PlanToken string
}

type eventUpdateOptions struct {
	EventID   string
	Input     normalizedEventUpdateInput
	Apply     bool
	PlanToken string
}

type eventCancelOptions struct {
	EventID      string
	Input        normalizedEventCancelInput
	Apply        bool
	ConfirmToken string
}

type eventCreatePublicInput struct {
	Title       string      `json:"title"`
	Start       string      `json:"start"`
	End         *string     `json:"end,omitempty"`
	Timezone    string      `json:"timezone"`
	Description *string     `json:"description,omitempty"`
	Location    *string     `json:"location,omitempty"`
	Visibility  string      `json:"visibility"`
	GuestLimit  *int        `json:"guestLimit,omitempty"`
	Links       []eventLink `json:"links,omitempty"`
	PosterID    string      `json:"posterId"`
}

func (input normalizedEventCreateInput) public() eventCreatePublicInput {
	return eventCreatePublicInput{
		Title:       input.Title,
		Start:       input.StartText,
		End:         input.EndText,
		Timezone:    input.Timezone,
		Description: input.Description,
		Location:    input.Location,
		Visibility:  input.Visibility,
		GuestLimit:  input.GuestLimit,
		Links:       append([]eventLink(nil), input.Links...),
		PosterID:    input.PosterID,
	}
}

func (input normalizedEventCreateInput) document() json.RawMessage {
	document, _ := json.Marshal(input.public())
	return document
}

func (input normalizedEventUpdateInput) fields() []string {
	fields := make([]string, 0, 8)
	if input.HasDescription {
		fields = append(fields, "description")
	}
	if input.HasEnd {
		fields = append(fields, "end")
	}
	if input.HasGuestLimit {
		fields = append(fields, "guestLimit")
	}
	if input.HasLinks {
		fields = append(fields, "links")
	}
	if input.HasPosterID {
		fields = append(fields, "posterId")
	}
	if input.HasStart {
		fields = append(fields, "start")
	}
	if input.HasTimezone {
		fields = append(fields, "timezone")
	}
	if input.HasTitle {
		fields = append(fields, "title")
	}
	return fields
}

func (input normalizedEventUpdateInput) public() map[string]any {
	document := make(map[string]any)
	if input.HasTitle {
		document["title"] = *input.Title
	}
	if input.HasDescription {
		document["description"] = input.Description
	}
	if input.HasStart {
		document["start"] = *input.StartText
	}
	if input.HasEnd {
		document["end"] = input.EndText
	}
	if input.HasTimezone {
		document["timezone"] = *input.Timezone
	}
	if input.HasGuestLimit {
		document["guestLimit"] = input.GuestLimit
	}
	if input.HasLinks {
		if input.Links == nil {
			document["links"] = nil
		} else {
			document["links"] = append([]eventLink(nil), input.Links...)
		}
	}
	if input.HasPosterID {
		document["posterId"] = input.PosterID
	}
	return document
}

func (input normalizedEventUpdateInput) document() json.RawMessage {
	document, _ := json.Marshal(input.public())
	return document
}

func (input normalizedEventCancelInput) document() json.RawMessage {
	document, _ := json.Marshal(input)
	return document
}

func parseEventCreateOptions(
	request Request,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
) (eventCreateOptions, *errorBody) {
	scalars, links, inputError := parseEventMutationFlags(definition, argv, len(definition.invocation), true)
	if inputError != nil {
		return eventCreateOptions{}, inputError
	}
	options := eventCreateOptions{}
	_, options.Apply = scalars["--apply"]
	options.PlanToken = scalars["--plan"]
	if options.Apply && options.PlanToken == "" {
		return eventCreateOptions{}, eventWriteInputFailure("PLAN_REQUIRED", "--apply requires --plan.")
	}
	if !options.Apply && options.PlanToken != "" {
		return eventCreateOptions{}, eventWriteInputFailure("APPLY_REQUIRED", "--plan requires --apply.")
	}
	document, readError := eventInputDocument(request, dependencies, scalars, links)
	if readError != nil {
		return eventCreateOptions{}, readError
	}
	input, inputError := normalizeEventCreateInput(document)
	if inputError != nil {
		return eventCreateOptions{}, inputError
	}
	options.Input = input
	return options, nil
}

func parseEventUpdateOptions(
	request Request,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
) (eventUpdateOptions, *errorBody) {
	eventID, inputError := parseEventID(definition, argv[:len(definition.invocation)+1])
	if inputError != nil {
		return eventUpdateOptions{}, inputError
	}
	scalars, links, parseError := parseEventMutationFlags(definition, argv, len(definition.invocation)+1, true)
	if parseError != nil {
		return eventUpdateOptions{}, parseError
	}
	options := eventUpdateOptions{EventID: eventID}
	_, options.Apply = scalars["--apply"]
	options.PlanToken = scalars["--plan"]
	if options.Apply && options.PlanToken == "" {
		return eventUpdateOptions{}, eventWriteInputFailure("PLAN_REQUIRED", "--apply requires --plan.")
	}
	if !options.Apply && options.PlanToken != "" {
		return eventUpdateOptions{}, eventWriteInputFailure("APPLY_REQUIRED", "--plan requires --apply.")
	}
	document, readError := eventInputDocument(request, dependencies, scalars, links)
	if readError != nil {
		return eventUpdateOptions{}, readError
	}
	input, inputError := normalizeEventUpdateInput(document)
	if inputError != nil {
		return eventUpdateOptions{}, inputError
	}
	if len(input.fields()) == 0 {
		return eventUpdateOptions{}, eventWriteInputFailure("UPDATE_FIELD_REQUIRED", "At least one writable field is required.")
	}
	options.Input = input
	return options, nil
}

func parseEventCancelOptions(
	request Request,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
) (eventCancelOptions, *errorBody) {
	eventID, inputError := parseEventID(definition, argv[:len(definition.invocation)+1])
	if inputError != nil {
		return eventCancelOptions{}, inputError
	}
	scalars, _, parseError := parseEventMutationFlags(definition, argv, len(definition.invocation)+1, false)
	if parseError != nil {
		return eventCancelOptions{}, parseError
	}
	options := eventCancelOptions{EventID: eventID}
	_, options.Apply = scalars["--apply"]
	options.ConfirmToken = scalars["--confirm"]
	if options.Apply && options.ConfirmToken == "" {
		return eventCancelOptions{}, confirmationRequiredErrorBody()
	}
	if !options.Apply && options.ConfirmToken != "" {
		return eventCancelOptions{}, eventWriteInputFailure("APPLY_REQUIRED", "--confirm requires --apply.")
	}
	document, readError := eventInputDocument(request, dependencies, scalars, nil)
	if readError != nil {
		return eventCancelOptions{}, readError
	}
	input, normalizeError := normalizeEventCancelInput(document)
	if normalizeError != nil {
		return eventCancelOptions{}, normalizeError
	}
	options.Input = input
	return options, nil
}

func parseEventMutationFlags(
	definition commandDefinition,
	argv []string,
	start int,
	allowLinks bool,
) (map[string]string, []string, *errorBody) {
	allowed := make(map[string]flagDefinition, len(definition.flags))
	for _, flag := range definition.flags {
		allowed[flag.Name] = flag
	}
	scalars := make(map[string]string)
	var links []string
	for index := start; index < len(argv); index++ {
		name := argv[index]
		flag, ok := allowed[name]
		if !ok {
			return nil, nil, eventWriteInputFailure("FLAG_UNKNOWN", "The command contains an unknown flag.")
		}
		if name == "--link" && allowLinks {
			if index+1 >= len(argv) {
				return nil, nil, eventWriteInputFailureWithDetail("FLAG_VALUE_REQUIRED", "A flag value is required.", "flag", name)
			}
			index++
			links = append(links, argv[index])
			continue
		}
		if _, repeated := scalars[name]; repeated {
			return nil, nil, eventWriteInputFailureWithDetail("FLAG_REPEATED", "A scalar flag cannot be repeated.", "flag", name)
		}
		if !flag.TakesValue {
			scalars[name] = "true"
			continue
		}
		if index+1 >= len(argv) {
			return nil, nil, eventWriteInputFailureWithDetail("FLAG_VALUE_REQUIRED", "A flag value is required.", "flag", name)
		}
		index++
		scalars[name] = argv[index]
	}
	return scalars, links, nil
}

func eventInputDocument(
	request Request,
	dependencies Dependencies,
	scalars map[string]string,
	links []string,
) (map[string]json.RawMessage, *errorBody) {
	inputPath, hasInput := scalars["--input"]
	hasFieldFlags := len(links) != 0
	for name := range scalars {
		switch name {
		case "--apply", "--plan", "--confirm", "--input":
		default:
			hasFieldFlags = true
		}
	}
	if hasInput && hasFieldFlags {
		return nil, eventWriteInputFailure("INPUT_SOURCE_CONFLICT", "--input cannot be combined with field flags.")
	}
	if hasInput {
		raw, ok := readRSVPInput(request.Stdin, dependencies.Files, inputPath)
		var document map[string]json.RawMessage
		if !ok || decodeRSVPObject(raw, &document) != nil {
			return nil, eventWriteInputFailure("INPUT_DOCUMENT_INVALID", "Input must be one valid JSON object.")
		}
		return document, nil
	}
	return eventFlagsDocument(scalars, links), nil
}

func eventFlagsDocument(values map[string]string, links []string) map[string]json.RawMessage {
	document := make(map[string]json.RawMessage)
	copyString := func(flag, field string) {
		if value, ok := values[flag]; ok {
			raw, _ := json.Marshal(value)
			document[field] = raw
		}
	}
	copyString("--title", "title")
	copyString("--start", "start")
	copyString("--end", "end")
	copyString("--timezone", "timezone")
	copyString("--description", "description")
	copyString("--location", "location")
	copyString("--visibility", "visibility")
	copyString("--poster-id", "posterId")
	copyString("--message", "message")
	if value, ok := values["--notify-guests"]; ok {
		document["notifyGuests"] = json.RawMessage(strings.ToLower(value))
	}
	if value, ok := values["--guest-limit"]; ok {
		document["guestLimit"] = json.RawMessage(value)
	}
	if links != nil {
		mapped := make([]eventLink, 0, len(links))
		for _, entry := range links {
			parts := strings.SplitN(entry, "=", 2)
			if len(parts) == 2 {
				mapped = append(mapped, eventLink{Label: parts[0], URL: parts[1]})
			}
		}
		raw, _ := json.Marshal(mapped)
		document["links"] = raw
	}
	return document
}

func normalizeEventCreateInput(document map[string]json.RawMessage) (normalizedEventCreateInput, *errorBody) {
	allowed := map[string]bool{
		"title":       true,
		"start":       true,
		"end":         true,
		"timezone":    true,
		"description": true,
		"location":    true,
		"visibility":  true,
		"guestLimit":  true,
		"links":       true,
		"posterId":    true,
	}
	for name := range document {
		if !allowed[name] {
			return normalizedEventCreateInput{}, eventWriteInputFailure("INPUT_FIELD_UNKNOWN", "Event input contains an unknown field.")
		}
	}
	title, ok := requiredEventString(document, "title", true)
	if !ok {
		return normalizedEventCreateInput{}, eventWriteInputFailure("TITLE_REQUIRED", "Title is required.")
	}
	start, startText, ok := requiredEventTime(document, "start")
	if !ok {
		return normalizedEventCreateInput{}, eventWriteInputFailure("START_INVALID", "Start must be an RFC 3339 timestamp.")
	}
	timezone, ok := requiredTimezone(document, "timezone")
	if !ok {
		return normalizedEventCreateInput{}, eventWriteInputFailure("TIMEZONE_INVALID", "Timezone must be a valid IANA timezone.")
	}
	end, endText, endStatus := optionalEventTime(document, "end")
	if endStatus == -1 {
		return normalizedEventCreateInput{}, eventWriteInputFailure("END_INVALID", "End must be an RFC 3339 timestamp or null.")
	}
	description, ok := optionalTrimmedString(document, "description", true)
	if !ok {
		return normalizedEventCreateInput{}, eventWriteInputFailure("DESCRIPTION_INVALID", "Description must be a string or null.")
	}
	location, ok := optionalTrimmedString(document, "location", true)
	if !ok {
		return normalizedEventCreateInput{}, eventWriteInputFailure("LOCATION_INVALID", "Location must be a string or null.")
	}
	visibility, ok := optionalVisibility(document)
	if !ok {
		return normalizedEventCreateInput{}, eventWriteInputFailure("VISIBILITY_INVALID", "Visibility must be private or public.")
	}
	guestLimit, guestLimitStatus := optionalPositiveInteger(document, "guestLimit", true)
	if guestLimitStatus == -1 {
		return normalizedEventCreateInput{}, eventWriteInputFailure("GUEST_LIMIT_INVALID", "Guest limit must be a positive integer or null.")
	}
	links, linksStatus := optionalLinks(document, true)
	if linksStatus == -1 {
		return normalizedEventCreateInput{}, eventWriteInputFailure("LINKS_INVALID", "Links must be an array of {label,url} objects or null.")
	}
	posterID, ok := optionalPosterID(document, true)
	if !ok {
		return normalizedEventCreateInput{}, eventWriteInputFailure("POSTER_ID_INVALID", "Poster ID must be a string or null.")
	}
	if end != nil && end.Before(start) {
		return normalizedEventCreateInput{}, eventWriteInputFailure("EVENT_RANGE_INVALID", "End must not be before start.")
	}
	return normalizedEventCreateInput{
		Title:       title,
		Start:       start,
		StartText:   startText,
		End:         end,
		EndText:     endText,
		Timezone:    timezone,
		Description: description,
		Location:    location,
		Visibility:  visibility,
		GuestLimit:  guestLimit,
		Links:       links,
		PosterID:    posterID,
	}, nil
}

func normalizeEventUpdateInput(document map[string]json.RawMessage) (normalizedEventUpdateInput, *errorBody) {
	allowed := map[string]bool{
		"title":       true,
		"description": true,
		"start":       true,
		"end":         true,
		"timezone":    true,
		"guestLimit":  true,
		"links":       true,
		"posterId":    true,
	}
	for name := range document {
		if !allowed[name] {
			return normalizedEventUpdateInput{}, eventWriteInputFailure("INPUT_FIELD_UNKNOWN", "Event input contains an unknown field.")
		}
	}
	var input normalizedEventUpdateInput
	if value, ok := document["title"]; ok {
		decoded, valid := decodeRequiredString(value, true)
		if !valid {
			return normalizedEventUpdateInput{}, eventWriteInputFailure("TITLE_INVALID", "Title must be a non-empty string.")
		}
		input.Title = &decoded
		input.HasTitle = true
	}
	if value, ok := document["description"]; ok {
		decoded, valid, present := decodeNullableString(value, true)
		if !valid {
			return normalizedEventUpdateInput{}, eventWriteInputFailure("DESCRIPTION_INVALID", "Description must be a string or null.")
		}
		input.Description = decoded
		input.HasDescription = present
	}
	if value, ok := document["start"]; ok {
		decoded, text, valid := decodeRequiredTime(value)
		if !valid {
			return normalizedEventUpdateInput{}, eventWriteInputFailure("START_INVALID", "Start must be an RFC 3339 timestamp.")
		}
		input.Start = &decoded
		input.StartText = &text
		input.HasStart = true
	}
	if value, ok := document["end"]; ok {
		decoded, text, status := decodeNullableTime(value)
		if status == -1 {
			return normalizedEventUpdateInput{}, eventWriteInputFailure("END_INVALID", "End must be an RFC 3339 timestamp or null.")
		}
		input.End = decoded
		input.EndText = text
		input.HasEnd = true
	}
	if value, ok := document["timezone"]; ok {
		decoded, valid := decodeTimezone(value)
		if !valid {
			return normalizedEventUpdateInput{}, eventWriteInputFailure("TIMEZONE_INVALID", "Timezone must be a valid IANA timezone.")
		}
		input.Timezone = &decoded
		input.HasTimezone = true
	}
	if value, ok := document["guestLimit"]; ok {
		decoded, status := decodeNullablePositiveInteger(value)
		if status == -1 {
			return normalizedEventUpdateInput{}, eventWriteInputFailure("GUEST_LIMIT_INVALID", "Guest limit must be a positive integer or null.")
		}
		input.GuestLimit = decoded
		input.HasGuestLimit = true
	}
	if value, ok := document["links"]; ok {
		decoded, status := decodeNullableLinks(value)
		if status == -1 {
			return normalizedEventUpdateInput{}, eventWriteInputFailure("LINKS_INVALID", "Links must be an array of {label,url} objects or null.")
		}
		input.Links = decoded
		input.HasLinks = true
	}
	if value, ok := document["posterId"]; ok {
		decoded, valid, present := decodeNullableString(value, true)
		if !valid {
			return normalizedEventUpdateInput{}, eventWriteInputFailure("POSTER_ID_INVALID", "Poster ID must be a string or null.")
		}
		input.PosterID = decoded
		input.HasPosterID = present
	}
	return input, nil
}

func normalizeEventCancelInput(document map[string]json.RawMessage) (normalizedEventCancelInput, *errorBody) {
	allowed := map[string]bool{"message": true, "notifyGuests": true}
	for name := range document {
		if !allowed[name] {
			return normalizedEventCancelInput{}, eventWriteInputFailure("INPUT_FIELD_UNKNOWN", "Event input contains an unknown field.")
		}
	}
	message := ""
	if value, ok := document["message"]; ok {
		var decoded string
		if json.Unmarshal(value, &decoded) != nil {
			return normalizedEventCancelInput{}, eventWriteInputFailure("MESSAGE_INVALID", "Message must be a string.")
		}
		message = decoded
	}
	notifyGuests := true
	if value, ok := document["notifyGuests"]; ok {
		var decoded bool
		if json.Unmarshal(value, &decoded) != nil {
			return normalizedEventCancelInput{}, eventWriteInputFailure("NOTIFY_GUESTS_INVALID", "notifyGuests must be true or false.")
		}
		notifyGuests = decoded
	}
	return normalizedEventCancelInput{Message: message, NotifyGuests: notifyGuests}, nil
}

func requiredEventString(document map[string]json.RawMessage, field string, trim bool) (string, bool) {
	raw, ok := document[field]
	if !ok {
		return "", false
	}
	value, valid := decodeRequiredString(raw, trim)
	return value, valid
}

func optionalTrimmedString(document map[string]json.RawMessage, field string, nullAllowed bool) (*string, bool) {
	raw, ok := document[field]
	if !ok {
		return nil, true
	}
	decoded, valid, _ := decodeNullableString(raw, true)
	if !nullAllowed && decoded == nil {
		return nil, false
	}
	return decoded, valid
}

func requiredEventTime(document map[string]json.RawMessage, field string) (time.Time, string, bool) {
	raw, ok := document[field]
	if !ok {
		return time.Time{}, "", false
	}
	return decodeRequiredTime(raw)
}

func optionalEventTime(document map[string]json.RawMessage, field string) (*time.Time, *string, int) {
	raw, ok := document[field]
	if !ok {
		return nil, nil, 0
	}
	return decodeNullableTime(raw)
}

func requiredTimezone(document map[string]json.RawMessage, field string) (string, bool) {
	raw, ok := document[field]
	if !ok {
		return "", false
	}
	return decodeTimezone(raw)
}

func optionalVisibility(document map[string]json.RawMessage) (string, bool) {
	raw, ok := document["visibility"]
	if !ok {
		return "private", true
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value == "private" || value == "public"
}

func optionalPositiveInteger(document map[string]json.RawMessage, field string, nullAsMissing bool) (*int, int) {
	raw, ok := document[field]
	if !ok {
		return nil, 0
	}
	decoded, status := decodeNullablePositiveInteger(raw)
	if decoded == nil && status == 1 && nullAsMissing {
		return nil, 0
	}
	return decoded, status
}

func optionalLinks(document map[string]json.RawMessage, nullAsMissing bool) ([]eventLink, int) {
	raw, ok := document["links"]
	if !ok {
		return nil, 0
	}
	decoded, status := decodeNullableLinks(raw)
	if decoded == nil && status == 1 && nullAsMissing {
		return nil, 0
	}
	return decoded, status
}

func optionalPosterID(document map[string]json.RawMessage, nullAsDefault bool) (string, bool) {
	raw, ok := document["posterId"]
	if !ok {
		return "Let's Party", true
	}
	decoded, valid, _ := decodeNullableString(raw, true)
	if !valid {
		return "", false
	}
	if decoded == nil && nullAsDefault {
		return "Let's Party", true
	}
	if decoded == nil {
		return "", true
	}
	return *decoded, true
}

func decodeRequiredString(raw json.RawMessage, trim bool) (string, bool) {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	if trim {
		value = strings.TrimSpace(value)
	}
	return value, value != ""
}

func decodeNullableString(raw json.RawMessage, trim bool) (*string, bool, bool) {
	if string(raw) == "null" {
		return nil, true, true
	}
	value, valid := decodeRequiredString(raw, trim)
	if !valid {
		return nil, false, false
	}
	return &value, true, true
}

func decodeRequiredTime(raw json.RawMessage) (time.Time, string, bool) {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return time.Time{}, "", false
	}
	value = strings.TrimSpace(value)
	decoded, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, "", false
	}
	return decoded, value, true
}

func decodeNullableTime(raw json.RawMessage) (*time.Time, *string, int) {
	if string(raw) == "null" {
		return nil, nil, 1
	}
	decoded, text, ok := decodeRequiredTime(raw)
	if !ok {
		return nil, nil, -1
	}
	return &decoded, &text, 1
}

func decodeTimezone(raw json.RawMessage) (string, bool) {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if _, err := time.LoadLocation(value); err != nil {
		return "", false
	}
	return value, true
}

func decodeNullablePositiveInteger(raw json.RawMessage) (*int, int) {
	if string(raw) == "null" {
		return nil, 1
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, -1
	}
	number, ok := value.(json.Number)
	if !ok {
		return nil, -1
	}
	parsed, err := strconv.Atoi(number.String())
	if err != nil || parsed < 1 {
		return nil, -1
	}
	return &parsed, 1
}

func decodeNullableLinks(raw json.RawMessage) ([]eventLink, int) {
	if string(raw) == "null" {
		return nil, 1
	}
	var entries []map[string]json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || entries == nil {
		return nil, -1
	}
	links := make([]eventLink, 0, len(entries))
	for _, entry := range entries {
		if len(entry) != 2 {
			return nil, -1
		}
		label, ok := requiredEventString(entry, "label", true)
		if !ok {
			return nil, -1
		}
		urlValue, ok := requiredEventString(entry, "url", true)
		if !ok || !validHTTPURL(urlValue) {
			return nil, -1
		}
		links = append(links, eventLink{Label: label, URL: urlValue})
	}
	return links, 1
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.IsAbs() && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func eventWriteInputFailure(code, message string) *errorBody {
	return &errorBody{Type: "input.invalid", Code: code, Message: message, Retryable: false, Details: map[string]any{}}
}

func eventWriteInputFailureWithDetail(code, message, key string, value any) *errorBody {
	return &errorBody{Type: "input.invalid", Code: code, Message: message, Retryable: false, Details: map[string]any{key: value}}
}

func confirmationRequiredErrorBody() *errorBody {
	return &errorBody{Type: "safety.confirmation_required", Code: "CONFIRMATION_REQUIRED", Message: "--apply requires --confirm.", Retryable: false, Details: map[string]any{}}
}

func eventLinkSchema() jsonSchema {
	one := 1
	return objectSchema(
		[]string{"label", "url"},
		map[string]jsonSchema{
			"label": {Type: "string", MinLength: &one},
			"url":   {Type: "string", Format: "uri"},
		},
	)
}

func eventCreateInputSchema() jsonSchema {
	one := 1
	return objectSchema(
		[]string{"title", "start", "timezone"},
		map[string]jsonSchema{
			"title":       {Type: "string", MinLength: &one},
			"start":       {Type: "string", Format: "date-time"},
			"end":         {Type: []string{"string", "null"}, Format: "date-time"},
			"timezone":    {Type: "string", MinLength: &one},
			"description": {Type: []string{"string", "null"}},
			"location":    {Type: []string{"string", "null"}},
			"visibility":  {Type: "string", Enum: []string{"private", "public"}},
			"guestLimit":  {Type: []string{"integer", "null"}},
			"links":       {Type: []string{"array", "null"}, Items: pointerSchema(eventLinkSchema())},
			"posterId":    {Type: []string{"string", "null"}},
		},
	)
}

func eventUpdateInputSchema() jsonSchema {
	return objectSchema(
		nil,
		map[string]jsonSchema{
			"title":       {Type: "string"},
			"description": {Type: []string{"string", "null"}},
			"start":       {Type: "string", Format: "date-time"},
			"end":         {Type: []string{"string", "null"}, Format: "date-time"},
			"timezone":    {Type: "string"},
			"guestLimit":  {Type: []string{"integer", "null"}},
			"links":       {Type: []string{"array", "null"}, Items: pointerSchema(eventLinkSchema())},
			"posterId":    {Type: []string{"string", "null"}},
		},
	)
}

func eventCancelInputSchema() jsonSchema {
	return objectSchema(
		nil,
		map[string]jsonSchema{
			"message":      {Type: "string"},
			"notifyGuests": {Type: "boolean"},
		},
	)
}

func eventCreateSuccessSchema() jsonSchema {
	one := 1
	threeHundred := 300
	plan := objectSchema(
		[]string{"operation", "input", "request", "preconditions", "expiresInSeconds", "planToken"},
		map[string]jsonSchema{
			"operation":        {Type: "string", Enum: []string{"createEvent"}},
			"input":            eventCreateInputSchema(),
			"request":          objectSchema([]string{"event", "cohostIds"}, map[string]jsonSchema{"event": {Type: "object"}, "cohostIds": {Type: "array", Items: pointerSchema(jsonSchema{Type: "string"})}}),
			"preconditions":    objectSchema([]string{"poster"}, map[string]jsonSchema{"poster": {Type: "string", Enum: []string{"bound"}}}),
			"expiresInSeconds": {Type: "integer", Minimum: &threeHundred, Maximum: &threeHundred},
			"planToken":        {Type: "string", MinLength: &one},
		},
	)
	submitted := objectSchema([]string{"submitted"}, map[string]jsonSchema{"submitted": {Type: "boolean", Const: true}})
	return jsonSchema{Type: "object", OneOf: []jsonSchema{plan, submitted}}
}

func eventUpdateSuccessSchema() jsonSchema {
	one := 1
	threeHundred := 300
	plan := objectSchema(
		[]string{"operation", "eventId", "fields", "input", "request", "preconditions", "expiresInSeconds", "planToken"},
		map[string]jsonSchema{
			"operation":        {Type: "string", Enum: []string{"firestorePatchEvent"}},
			"eventId":          {Type: "string", MinLength: &one},
			"fields":           {Type: "array", Items: pointerSchema(jsonSchema{Type: "string"})},
			"input":            eventUpdateInputSchema(),
			"request":          {Type: "object"},
			"preconditions":    {Type: "object"},
			"expiresInSeconds": {Type: "integer", Minimum: &threeHundred, Maximum: &threeHundred},
			"planToken":        {Type: "string", MinLength: &one},
		},
	)
	submitted := objectSchema(
		[]string{"eventId", "fields", "submitted"},
		map[string]jsonSchema{
			"eventId":   {Type: "string", MinLength: &one},
			"fields":    {Type: "array", Items: pointerSchema(jsonSchema{Type: "string"})},
			"submitted": {Type: "boolean", Const: true},
		},
	)
	return jsonSchema{Type: "object", OneOf: []jsonSchema{plan, submitted}}
}

func eventCancelSuccessSchema() jsonSchema {
	one := 1
	threeHundred := 300
	plan := objectSchema(
		[]string{"operation", "eventId", "input", "request", "effects", "preconditions", "expiresInSeconds", "planToken"},
		map[string]jsonSchema{
			"operation":        {Type: "string", Enum: []string{"cancelEvent"}},
			"eventId":          {Type: "string", MinLength: &one},
			"input":            eventCancelInputSchema(),
			"request":          {Type: "object"},
			"effects":          {Type: "array", Items: pointerSchema(jsonSchema{Type: "string"})},
			"preconditions":    {Type: "object"},
			"expiresInSeconds": {Type: "integer", Minimum: &threeHundred, Maximum: &threeHundred},
			"planToken":        {Type: "string", MinLength: &one},
		},
	)
	submitted := objectSchema(
		[]string{"eventId", "notifyGuests", "submitted"},
		map[string]jsonSchema{
			"eventId":      {Type: "string", MinLength: &one},
			"notifyGuests": {Type: "boolean"},
			"submitted":    {Type: "boolean", Const: true},
		},
	)
	return jsonSchema{Type: "object", OneOf: []jsonSchema{plan, submitted}}
}

func consequentialActionSafety() safetyDefinition {
	return safetyDefinition{Kind: "consequential-action", PlanRequired: true, ConfirmationRequired: true}
}

func pointerSchema(schema jsonSchema) *jsonSchema {
	return &schema
}

func createEventGuestStatusCounts() map[string]int {
	return map[string]int{
		"APPROVED":                 0,
		"DECLINED":                 0,
		"DELIVERY_ERROR":           0,
		"GOING":                    0,
		"INTERESTED":               0,
		"MAYBE":                    0,
		"PENDING_APPROVAL":         0,
		"READY_TO_SEND":            0,
		"REJECTED":                 0,
		"RESPONDED_TO_FIND_A_TIME": 0,
		"SENDING":                  0,
		"SEND_ERROR":               0,
		"SENT":                     0,
		"WAITLIST":                 0,
		"WAITLISTED_FOR_APPROVAL":  0,
		"WITHDRAWN":                0,
	}
}

func defaultCreateDisplaySettings() remote.EventDisplaySettings {
	return remote.EventDisplaySettings{Theme: "cloudflow", Effect: "fireflies", TitleFont: "display"}
}
