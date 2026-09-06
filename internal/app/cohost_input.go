package app

import (
	"encoding/json"
	"strings"
)

type cohostContactInput struct {
	Contact string `json:"contact"`
}

type cohostActionOptions struct {
	EventID string
	Input   cohostContactInput
}

type cohostLinkOptions struct {
	EventID string
}

func (input cohostContactInput) document() json.RawMessage {
	document, _ := json.Marshal(input)
	return document
}

func parseCohostActionOptions(
	definition commandDefinition,
	argv []string,
) (cohostActionOptions, *errorBody) {
	eventID, inputError := parseEventID(definition, argv[:len(definition.invocation)+1])
	if inputError != nil {
		return cohostActionOptions{}, inputError
	}
	scalars, parseError := parseCohostFlags(definition, argv, len(definition.invocation)+1)
	if parseError != nil {
		return cohostActionOptions{}, parseError
	}
	options := cohostActionOptions{EventID: eventID}
	contact := strings.TrimSpace(scalars["--contact"])
	if contact == "" {
		return cohostActionOptions{}, eventWriteInputFailure("CONTACT_REQUIRED", "--contact is required.")
	}
	options.Input = cohostContactInput{Contact: contact}
	return options, nil
}

func parseCohostLinkOptions(
	definition commandDefinition,
	argv []string,
) (cohostLinkOptions, *errorBody) {
	eventID, inputError := parseEventID(definition, argv[:len(definition.invocation)+1])
	if inputError != nil {
		return cohostLinkOptions{}, inputError
	}
	_, parseError := parseCohostFlags(definition, argv, len(definition.invocation)+1)
	if parseError != nil {
		return cohostLinkOptions{}, parseError
	}
	return cohostLinkOptions{EventID: eventID}, nil
}

func parseCohostFlags(
	definition commandDefinition,
	argv []string,
	start int,
) (map[string]string, *errorBody) {
	allowed := make(map[string]flagDefinition, len(definition.flags))
	for _, flag := range definition.flags {
		allowed[flag.Name] = flag
	}
	scalars := make(map[string]string)
	for index := start; index < len(argv); index++ {
		name := argv[index]
		flag, ok := allowed[name]
		if !ok {
			return nil, eventWriteInputFailure("FLAG_UNKNOWN", "The command contains an unknown flag.")
		}
		if _, repeated := scalars[name]; repeated {
			return nil, eventWriteInputFailureWithDetail("FLAG_REPEATED", "A scalar flag cannot be repeated.", "flag", name)
		}
		if !flag.TakesValue {
			scalars[name] = "true"
			continue
		}
		if index+1 >= len(argv) {
			return nil, eventWriteInputFailureWithDetail("FLAG_VALUE_REQUIRED", "A flag value is required.", "flag", name)
		}
		index++
		scalars[name] = argv[index]
	}
	return scalars, nil
}

func cohostContactInputSchema() jsonSchema {
	one := 1
	return objectSchema(
		[]string{"contact"},
		map[string]jsonSchema{
			"contact": {Type: "string", MinLength: &one},
		},
	)
}

func cohostInviteSuccessSchema() jsonSchema {
	return cohostContactActionSuccessSchema("createCohostRequest", "invited")
}

func cohostRevokeInviteSuccessSchema() jsonSchema {
	return cohostContactActionSuccessSchema("deleteCohostRequest", "revoked")
}

func cohostRemoveSuccessSchema() jsonSchema {
	return cohostContactActionSuccessSchema("removeCohost", "removed")
}

func cohostContactActionSuccessSchema(operation, status string) jsonSchema {
	one := 1
	preview := objectSchema(
		[]string{"operation", "eventId", "contact", "request", "effects", "preconditions"},
		map[string]jsonSchema{
			"operation": {Type: "string", Enum: []string{operation}},
			"eventId":   {Type: "string", MinLength: &one},
			"contact": objectSchema(
				[]string{"displayName"},
				map[string]jsonSchema{"displayName": {Type: "string", MinLength: &one}},
			),
			"request":       {Type: "object"},
			"effects":       {Type: "array", Items: pointerSchema(jsonSchema{Type: "string"})},
			"preconditions": {Type: "object"},
		},
	)
	success := objectSchema(
		[]string{"eventId", "cohost"},
		map[string]jsonSchema{
			"eventId": {Type: "string", MinLength: &one},
			"cohost": objectSchema(
				[]string{"displayName", "status"},
				map[string]jsonSchema{
					"displayName": {Type: "string", MinLength: &one},
					"status":      {Type: "string", Enum: []string{status}},
				},
			),
		},
	)
	return jsonSchema{Type: "object", OneOf: []jsonSchema{preview, success}}
}

func cohostLinkCreateSuccessSchema() jsonSchema {
	return cohostLinkActionSuccessSchema("generateEventCohostLink", "active", true)
}

func cohostLinkRevokeSuccessSchema() jsonSchema {
	return cohostLinkActionSuccessSchema("revokeEventCohostLink", "revoked", false)
}

func cohostLinkActionSuccessSchema(operation, state string, includeURL bool) jsonSchema {
	one := 1
	preview := objectSchema(
		[]string{"operation", "eventId", "request", "effects", "preconditions"},
		map[string]jsonSchema{
			"operation":     {Type: "string", Enum: []string{operation}},
			"eventId":       {Type: "string", MinLength: &one},
			"request":       {Type: "object"},
			"effects":       {Type: "array", Items: pointerSchema(jsonSchema{Type: "string"})},
			"preconditions": {Type: "object"},
		},
	)
	linkURLType := []string{"null"}
	if includeURL {
		linkURLType = []string{"string"}
	}
	success := objectSchema(
		[]string{"eventId", "link"},
		map[string]jsonSchema{
			"eventId": {Type: "string", MinLength: &one},
			"link": objectSchema(
				[]string{"url", "state"},
				map[string]jsonSchema{
					"url":   {Type: linkURLType, Format: "uri"},
					"state": {Type: "string", Enum: []string{state}},
				},
			),
		},
	)
	return jsonSchema{Type: "object", OneOf: []jsonSchema{preview, success}}
}
