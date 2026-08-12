package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const maximumRSVPInputBytes = 64 << 10

type questionnaireInput struct {
	QuestionnaireVersion int               `json:"questionnaireVersion"`
	Answers              map[string]string `json:"answers"`
}

type addGuestProductInput struct {
	Status                string              `json:"status"`
	DisplayName           string              `json:"displayName"`
	PartySize             int                 `json:"partySize"`
	PlusOnes              []string            `json:"plusOnes"`
	Message               *string             `json:"message"`
	Timezone              string              `json:"timezone"`
	QuestionnaireResponse *questionnaireInput `json:"questionnaireResponse"`
}

type interestProductInput struct {
	Status string `json:"status"`
}

type normalizedRSVPInput struct {
	Intent   string
	AddGuest *addGuestProductInput
}

type rsvpSetOptions struct {
	EventID   string
	Input     normalizedRSVPInput
	Apply     bool
	PlanToken string
}

func (input normalizedRSVPInput) public() any {
	if input.AddGuest != nil {
		return *input.AddGuest
	}
	return interestProductInput{Status: input.Intent}
}

func (input normalizedRSVPInput) document() json.RawMessage {
	document, _ := json.Marshal(input.public())
	return document
}

func parseRSVPSetOptions(
	request Request,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
) (rsvpSetOptions, *errorBody) {
	if len(argv) < len(definition.invocation)+1 ||
		strings.TrimSpace(argv[len(definition.invocation)]) == "" {
		return rsvpSetOptions{}, rsvpInputFailure(
			"EVENT_ID_REQUIRED",
			"Event ID is required.",
		)
	}
	options := rsvpSetOptions{EventID: argv[len(definition.invocation)]}
	scalars := make(map[string]string)
	var plusOnes []string
	for index := len(definition.invocation) + 1; index < len(argv); index++ {
		name := argv[index]
		switch name {
		case "--apply":
			if _, repeated := scalars[name]; repeated {
				return rsvpSetOptions{}, repeatedRSVPFlag(name)
			}
			scalars[name] = "true"
		case "--input",
			"--status",
			"--display-name",
			"--party-size",
			"--plus-one",
			"--message",
			"--timezone",
			"--questionnaire-response",
			"--plan":
			if name != "--plus-one" {
				if _, repeated := scalars[name]; repeated {
					return rsvpSetOptions{}, repeatedRSVPFlag(name)
				}
			}
			if index+1 >= len(argv) {
				return rsvpSetOptions{}, rsvpInputFailureWithDetail(
					"FLAG_VALUE_REQUIRED",
					"A flag value is required.",
					"flag",
					name,
				)
			}
			index++
			if name == "--plus-one" {
				plusOnes = append(plusOnes, argv[index])
			} else {
				scalars[name] = argv[index]
			}
		default:
			return rsvpSetOptions{}, rsvpInputFailure(
				"FLAG_UNKNOWN",
				"The command contains an unknown flag.",
			)
		}
	}
	_, options.Apply = scalars["--apply"]
	options.PlanToken = scalars["--plan"]
	if options.Apply && options.PlanToken == "" {
		return rsvpSetOptions{}, rsvpInputFailure(
			"PLAN_REQUIRED",
			"--apply requires --plan.",
		)
	}
	if !options.Apply && options.PlanToken != "" {
		return rsvpSetOptions{}, rsvpInputFailure(
			"APPLY_REQUIRED",
			"--plan requires --apply.",
		)
	}

	var document map[string]json.RawMessage
	inputPath, hasInput := scalars["--input"]
	hasFieldFlags := len(plusOnes) != 0
	for _, name := range []string{
		"--status",
		"--display-name",
		"--party-size",
		"--message",
		"--timezone",
		"--questionnaire-response",
	} {
		_, hasField := scalars[name]
		hasFieldFlags = hasFieldFlags || hasField
	}
	if hasInput && hasFieldFlags {
		return rsvpSetOptions{}, rsvpInputFailure(
			"INPUT_SOURCE_CONFLICT",
			"--input cannot be combined with RSVP field flags.",
		)
	}
	if hasInput {
		raw, ok := readRSVPInput(request.Stdin, dependencies.Files, inputPath)
		if !ok || decodeRSVPObject(raw, &document) != nil {
			return rsvpSetOptions{}, rsvpInputFailure(
				"INPUT_DOCUMENT_INVALID",
				"RSVP input must be one valid JSON object.",
			)
		}
	} else {
		document = rsvpFlagsDocument(scalars, plusOnes)
	}
	input, inputError := normalizeRSVPInput(document)
	if inputError != nil {
		return rsvpSetOptions{}, inputError
	}
	options.Input = input
	return options, nil
}

func readRSVPInput(
	stdin io.Reader,
	files interface {
		ReadFile(string) ([]byte, error)
	},
	path string,
) ([]byte, bool) {
	if path == "-" {
		if stdin == nil {
			return nil, false
		}
		raw, err := io.ReadAll(io.LimitReader(stdin, maximumRSVPInputBytes+1))
		return raw, err == nil && len(raw) <= maximumRSVPInputBytes && utf8.Valid(raw)
	}
	if path == "" || files == nil {
		return nil, false
	}
	raw, err := files.ReadFile(path)
	return raw, err == nil && len(raw) <= maximumRSVPInputBytes && utf8.Valid(raw)
}

func decodeRSVPObject(raw []byte, destination *map[string]json.RawMessage) error {
	if !utf8.Valid(raw) {
		return errors.New("input is not UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(destination) != nil || *destination == nil {
		return errors.New("input is not an object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("input has trailing JSON")
	}
	return nil
}

func rsvpFlagsDocument(
	values map[string]string,
	plusOnes []string,
) map[string]json.RawMessage {
	document := make(map[string]json.RawMessage)
	copyString := func(flag, field string) {
		if value, ok := values[flag]; ok {
			raw, _ := json.Marshal(value)
			document[field] = raw
		}
	}
	copyString("--status", "status")
	copyString("--display-name", "displayName")
	copyString("--timezone", "timezone")
	copyString("--message", "message")
	if value, ok := values["--party-size"]; ok {
		document["partySize"] = json.RawMessage(value)
	}
	if plusOnes != nil {
		raw, _ := json.Marshal(plusOnes)
		document["plusOnes"] = raw
	}
	if value, ok := values["--questionnaire-response"]; ok {
		document["questionnaireResponse"] = json.RawMessage(value)
	}
	return document
}

func normalizeRSVPInput(
	document map[string]json.RawMessage,
) (normalizedRSVPInput, *errorBody) {
	allowed := map[string]bool{
		"status":                true,
		"displayName":           true,
		"partySize":             true,
		"plusOnes":              true,
		"message":               true,
		"timezone":              true,
		"questionnaireResponse": true,
	}
	for name := range document {
		if !allowed[name] {
			return normalizedRSVPInput{}, rsvpInputFailure(
				"INPUT_FIELD_UNKNOWN",
				"RSVP input contains an unknown field.",
			)
		}
	}
	status, ok := requiredRSVPString(document, "status")
	if !ok {
		return normalizedRSVPInput{}, rsvpInputFailure(
			"RSVP_STATUS_INVALID",
			"RSVP status must be going, not-going, or interested.",
		)
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "going" && status != "not-going" && status != "interested" {
		return normalizedRSVPInput{}, rsvpInputFailure(
			"RSVP_STATUS_INVALID",
			"RSVP status must be going, not-going, or interested.",
		)
	}
	if status == "interested" {
		if len(document) != 1 {
			return normalizedRSVPInput{}, rsvpInputFailure(
				"INTEREST_INPUT_INVALID",
				"Interested accepts only the status field.",
			)
		}
		return normalizedRSVPInput{Intent: status}, nil
	}

	displayName, ok := requiredRSVPString(document, "displayName")
	if !ok || !utf8.ValidString(displayName) {
		return normalizedRSVPInput{}, rsvpInputFailure(
			"DISPLAY_NAME_INVALID",
			"Display name must contain 1 to 50 characters.",
		)
	}
	displayName = strings.TrimSpace(displayName)
	if countCharacters(displayName) < 1 || countCharacters(displayName) > 50 {
		return normalizedRSVPInput{}, rsvpInputFailure(
			"DISPLAY_NAME_INVALID",
			"Display name must contain 1 to 50 characters.",
		)
	}
	partySize, ok := requiredRSVPInteger(document, "partySize")
	if !ok || partySize < 1 {
		return normalizedRSVPInput{}, rsvpInputFailure(
			"PARTY_SIZE_INVALID",
			"Party size must be a positive integer.",
		)
	}
	var plusOnes []string
	if raw, present := document["plusOnes"]; present {
		if json.Unmarshal(raw, &plusOnes) != nil || plusOnes == nil {
			return normalizedRSVPInput{}, rsvpInputFailure(
				"PLUS_ONES_INVALID",
				"Plus ones must be an array of nonempty names.",
			)
		}
	} else {
		plusOnes = []string{}
	}
	for index, name := range plusOnes {
		if !utf8.ValidString(name) {
			return normalizedRSVPInput{}, rsvpInputFailure(
				"PLUS_ONES_INVALID",
				"Plus ones must be an array of nonempty names.",
			)
		}
		plusOnes[index] = strings.TrimSpace(name)
		if plusOnes[index] == "" {
			return normalizedRSVPInput{}, rsvpInputFailure(
				"PLUS_ONES_INVALID",
				"Plus ones must be an array of nonempty names.",
			)
		}
	}
	if partySize != 1+len(plusOnes) {
		return normalizedRSVPInput{}, rsvpInputFailure(
			"PARTY_SIZE_MISMATCH",
			"Party size must equal one plus the number of named plus ones.",
		)
	}
	timezone, ok := requiredRSVPString(document, "timezone")
	if !ok || timezone == "" || !validRSVPTimezone(timezone) {
		return normalizedRSVPInput{}, rsvpInputFailure(
			"TIMEZONE_INVALID",
			"Timezone must be a valid IANA timezone.",
		)
	}
	message, valid := optionalRSVPMessage(document["message"])
	if !valid {
		return normalizedRSVPInput{}, rsvpInputFailure(
			"MESSAGE_INVALID",
			"Message must contain at most 400 characters.",
		)
	}
	questionnaire, valid := optionalQuestionnaire(document["questionnaireResponse"])
	if !valid || status == "not-going" && questionnaire != nil {
		return normalizedRSVPInput{}, rsvpInputFailure(
			"QUESTIONNAIRE_RESPONSE_INVALID",
			"Questionnaire response is invalid for this RSVP.",
		)
	}
	return normalizedRSVPInput{
		Intent: status,
		AddGuest: &addGuestProductInput{
			Status:                status,
			DisplayName:           displayName,
			PartySize:             partySize,
			PlusOnes:              plusOnes,
			Message:               message,
			Timezone:              timezone,
			QuestionnaireResponse: questionnaire,
		},
	}, nil
}

func requiredRSVPString(
	document map[string]json.RawMessage,
	name string,
) (string, bool) {
	raw, ok := document[name]
	if !ok {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func requiredRSVPInteger(
	document map[string]json.RawMessage,
	name string,
) (int, bool) {
	raw, ok := document[name]
	if !ok {
		return 0, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if decoder.Decode(&decoded) != nil {
		return 0, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return 0, false
	}
	number, ok := decoded.(json.Number)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseInt(number.String(), 10, 0)
	return int(value), err == nil
}

func optionalRSVPMessage(raw json.RawMessage) (*string, bool) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || !utf8.ValidString(value) {
		return nil, false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, true
	}
	if countCharacters(value) > 400 {
		return nil, false
	}
	return &value, true
}

func optionalQuestionnaire(raw json.RawMessage) (*questionnaireInput, bool) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response questionnaireInput
	if decoder.Decode(&response) != nil ||
		response.QuestionnaireVersion < 0 ||
		response.Answers == nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, false
	}
	for questionID, answer := range response.Answers {
		if questionID == "" || !utf8.ValidString(questionID) || !utf8.ValidString(answer) {
			return nil, false
		}
	}
	return &response, true
}

func validRSVPTimezone(value string) bool {
	if strings.TrimSpace(value) != value || value == "Local" {
		return false
	}
	_, err := time.LoadLocation(value)
	return err == nil
}

func countCharacters(value string) int {
	return utf8.RuneCountInString(value)
}

func repeatedRSVPFlag(name string) *errorBody {
	return rsvpInputFailureWithDetail(
		"FLAG_REPEATED",
		"A scalar flag cannot be repeated.",
		"flag",
		name,
	)
}

func rsvpInputFailure(code, message string) *errorBody {
	return &errorBody{
		Type:      "input.invalid",
		Code:      code,
		Message:   message,
		Retryable: false,
		Details:   map[string]any{},
	}
}

func rsvpInputFailureWithDetail(
	code string,
	message string,
	name string,
	value string,
) *errorBody {
	failure := rsvpInputFailure(code, message)
	failure.Details[name] = value
	return failure
}

func rsvpSetInputSchema() jsonSchema {
	zero := 0
	one := 1
	fifty := 50
	fourHundred := 400
	stringItem := jsonSchema{Type: "string", MinLength: &one, Pattern: `\S`}
	answers := jsonSchema{
		Type:                 "object",
		AdditionalProperties: jsonSchema{Type: "string"},
	}
	questionnaire := objectSchema(
		[]string{"questionnaireVersion", "answers"},
		map[string]jsonSchema{
			"questionnaireVersion": {Type: "integer", Minimum: &zero},
			"answers":              answers,
		},
	)
	commonProperties := func(status string) map[string]jsonSchema {
		return map[string]jsonSchema{
			"eventId": {Type: "string", MinLength: &one},
			"status":  {Type: "string", Enum: []string{status}},
			"apply":   {Type: "boolean"},
			"plan":    {Type: "string", MinLength: &one},
		}
	}
	addGuestVariant := func(status string) jsonSchema {
		properties := commonProperties(status)
		properties["displayName"] = jsonSchema{
			Type:      "string",
			MinLength: &one,
			MaxLength: &fifty,
			Pattern:   `\S`,
		}
		properties["partySize"] = jsonSchema{Type: "integer", Minimum: &one}
		properties["plusOnes"] = jsonSchema{Type: "array", Items: &stringItem}
		properties["message"] = jsonSchema{
			Type:      []string{"string", "null"},
			MaxLength: &fourHundred,
		}
		properties["timezone"] = jsonSchema{Type: "string", MinLength: &one}
		properties["questionnaireResponse"] = jsonSchema{Type: "null"}
		if status == "going" {
			properties["questionnaireResponse"] = jsonSchema{
				OneOf: []jsonSchema{questionnaire, {Type: "null"}},
			}
		}
		schema := objectSchema(
			[]string{"eventId", "status", "displayName", "partySize", "timezone"},
			properties,
		)
		schema.DependentRequired = map[string][]string{
			"apply": {"plan"},
			"plan":  {"apply"},
		}
		return schema
	}
	interest := objectSchema(
		[]string{"eventId", "status"},
		commonProperties("interested"),
	)
	interest.DependentRequired = map[string][]string{
		"apply": {"plan"},
		"plan":  {"apply"},
	}
	return jsonSchema{Type: "object", OneOf: []jsonSchema{
		addGuestVariant("going"),
		addGuestVariant("not-going"),
		interest,
	}}
}

func rsvpSetSuccessSchema() jsonSchema {
	zero := 0
	one := 1
	fifty := 50
	fourHundred := 400
	threeHundred := 300
	stringItem := jsonSchema{Type: "string", MinLength: &one, Pattern: `\S`}
	answers := jsonSchema{
		Type:                 "object",
		AdditionalProperties: jsonSchema{Type: "string"},
	}
	questionnaire := objectSchema(
		[]string{"questionnaireVersion", "answers"},
		map[string]jsonSchema{
			"questionnaireVersion": {Type: "integer", Minimum: &zero},
			"answers":              answers,
		},
	)
	productInput := func(status string) jsonSchema {
		if status == "interested" {
			return objectSchema(
				[]string{"status"},
				map[string]jsonSchema{
					"status": {Type: "string", Enum: []string{"interested"}},
				},
			)
		}
		properties := map[string]jsonSchema{
			"status":                {Type: "string", Enum: []string{status}},
			"displayName":           {Type: "string", MinLength: &one, MaxLength: &fifty, Pattern: `\S`},
			"partySize":             {Type: "integer", Minimum: &one},
			"plusOnes":              {Type: "array", Items: &stringItem},
			"message":               {Type: []string{"string", "null"}, MaxLength: &fourHundred},
			"timezone":              {Type: "string", MinLength: &one},
			"questionnaireResponse": {Type: "null"},
		}
		if status == "going" {
			properties["questionnaireResponse"] = jsonSchema{
				OneOf: []jsonSchema{questionnaire, {Type: "null"}},
			}
		}
		return objectSchema(
			[]string{
				"status",
				"displayName",
				"partySize",
				"plusOnes",
				"message",
				"timezone",
				"questionnaireResponse",
			},
			properties,
		)
	}
	submitted := objectSchema(
		[]string{"eventId", "intent", "submitted"},
		map[string]jsonSchema{
			"eventId":   {Type: "string", MinLength: &one},
			"intent":    {Type: "string", Enum: []string{"going", "not-going", "interested"}},
			"submitted": {Type: "boolean", Const: true},
		},
	)
	plusOne := objectSchema(
		[]string{"name"},
		map[string]jsonSchema{
			"name": {Type: "string", MinLength: &one, Pattern: `\S`},
		},
	)
	draft := objectSchema(
		[]string{"name", "count", "plusOnes", "status", "timezone", "shouldFollowOrgs"},
		map[string]jsonSchema{
			"name":                  {Type: "string", MinLength: &one, MaxLength: &fifty},
			"count":                 {Type: "integer", Minimum: &one},
			"plusOnes":              {Type: "array", Items: &plusOne},
			"message":               {Type: "string", MaxLength: &fourHundred},
			"status":                {Type: "string", Enum: []string{"GOING", "DECLINED"}},
			"guestId":               {Type: "string", Enum: []string{"<redacted>"}},
			"timezone":              {Type: "string", MinLength: &one},
			"questionnaireResponse": questionnaire,
			"shouldFollowOrgs":      {Type: "boolean", Const: false},
		},
	)
	addGuestRequest := objectSchema(
		[]string{"eventId", "rsvp"},
		map[string]jsonSchema{
			"eventId": {Type: "string", MinLength: &one},
			"rsvp":    draft,
		},
	)
	interestRequest := objectSchema(
		[]string{"eventId", "interested"},
		map[string]jsonSchema{
			"eventId":    {Type: "string", MinLength: &one},
			"interested": {Type: "boolean", Const: true},
		},
	)
	preconditions := objectSchema(
		[]string{"currentGuest", "eventSafeguards"},
		map[string]jsonSchema{
			"currentGuest":    {Type: "string", Enum: []string{"absent", "present"}},
			"eventSafeguards": {Type: "string", Enum: []string{"bound"}},
		},
	)
	plan := objectSchema(
		[]string{
			"operation",
			"mode",
			"input",
			"request",
			"preconditions",
			"expiresInSeconds",
			"planToken",
		},
		map[string]jsonSchema{
			"operation": {Type: "string", Enum: []string{"addGuest", "markEventInterest"}},
			"mode":      {Type: "string", Enum: []string{"create", "update"}},
			"input": {
				OneOf: []jsonSchema{
					productInput("going"),
					productInput("not-going"),
					productInput("interested"),
				},
			},
			"request": {
				OneOf: []jsonSchema{addGuestRequest, interestRequest},
			},
			"preconditions":    preconditions,
			"expiresInSeconds": {Type: "integer", Minimum: &threeHundred, Maximum: &threeHundred},
			"planToken":        {Type: "string", MinLength: &one},
		},
	)
	return jsonSchema{Type: "object", OneOf: []jsonSchema{plan, submitted}}
}

func standardMutationSafety() safetyDefinition {
	return safetyDefinition{
		Kind:                 "standard-mutation",
		PlanRequired:         true,
		ConfirmationRequired: false,
	}
}
