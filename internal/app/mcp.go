package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
)

// MCPDefinition is the public, API-derived operation contract used by the MCP adapter.
type MCPDefinition struct {
	Name         string
	Command      string
	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	ReadOnly     bool
	Destructive  bool
}

type MCPExecutionOptions struct {
	MaxItems int
}

// MCPDefinitions exposes only the curated product operations. CLI-only and
// credential-management commands deliberately remain absent.
func MCPDefinitions() []MCPDefinition {
	definitions := make([]MCPDefinition, 0, 18)
	for _, definition := range commandCatalog {
		name, eligible := mcpToolName(definition)
		if !eligible {
			continue
		}
		inputJSON, _ := json.Marshal(mcpInputSchema(definition))
		outputJSON, _ := json.Marshal(mcpOutputSchema(definition))
		definitions = append(definitions, MCPDefinition{
			Name:         name,
			Command:      definition.path,
			Description:  "Partiful " + strings.ReplaceAll(definition.path, ".", " ") + ".",
			InputSchema:  inputJSON,
			OutputSchema: outputJSON,
			ReadOnly:     definition.safety.Kind == "read-only",
			Destructive:  definition.safety.Destructive,
		})
	}
	slices.SortFunc(definitions, func(left, right MCPDefinition) int {
		return strings.Compare(left.Name, right.Name)
	})
	return definitions
}

func mcpToolName(definition commandDefinition) (string, bool) {
	switch definition.kind {
	case postersListCommand,
		postersSearchCommand,
		contactsListCommand,
		guestsListCommand,
		guestsInviteCommand,
		eventsListCommand,
		eventsGetCommand,
		eventsCreateCommand,
		eventsUpdateCommand,
		eventsCancelCommand,
		blastsSendCommand,
		rsvpGetCommand,
		rsvpSetCommand,
		cohostsInviteCommand,
		cohostsRevokeInviteCommand,
		cohostsRemoveCommand,
		cohostsLinkCreateCommand,
		cohostsLinkRevokeCommand:
		return strings.NewReplacer(".", "_", "-", "_").Replace(definition.path), true
	default:
		return "", false
	}
}

func mcpInputSchema(definition commandDefinition) map[string]any {
	var schema map[string]any
	encoded, _ := json.Marshal(definition.inputSchema)
	_ = json.Unmarshal(encoded, &schema)
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
		schema["properties"] = properties
	}
	if definition.kind == blastsSendCommand {
		delete(properties, "messageFile")
		properties["message"] = map[string]any{
			"type":        "string",
			"minLength":   1,
			"maxLength":   blastMessageMaximumRunes,
			"description": "Private blast text; it is never echoed in previews or errors.",
		}
		required, _ := schema["required"].([]any)
		for index, field := range required {
			if field == "messageFile" {
				required[index] = "message"
			}
		}
		schema["required"] = required
	}
	if _, ok := properties["all"]; ok {
		properties["all"] = map[string]any{
			"type":        "boolean",
			"const":       true,
			"description": "Fetch all pages up to maxItems.",
		}
	}
	for _, positional := range definition.positionals {
		name := mcpFieldName(positional.Name)
		addMCPInputProperty(
			schema,
			name,
			map[string]any{"type": "string", "minLength": 1},
			positional.Required,
		)
	}
	if definition.safety.Kind != "read-only" {
		addMCPInputProperty(schema, "dryRun", map[string]any{
			"type":        "boolean",
			"description": "Preview without target mutation or credential persistence.",
		}, false)
	}
	return schema
}

func addMCPInputProperty(
	schema map[string]any,
	name string,
	property map[string]any,
	required bool,
) {
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
		schema["properties"] = properties
	}
	properties[name] = property
	if required {
		requiredFields, _ := schema["required"].([]any)
		if !slices.Contains(requiredFields, any(name)) {
			schema["required"] = append(requiredFields, name)
		}
	}
	variants, _ := schema["oneOf"].([]any)
	for _, candidate := range variants {
		variant, ok := candidate.(map[string]any)
		if !ok {
			continue
		}
		addMCPInputProperty(variant, name, property, required)
	}
}

func mcpOutputSchema(definition commandDefinition) map[string]any {
	var data any
	encoded, _ := json.Marshal(definition.successSchema)
	_ = json.Unmarshal(encoded, &data)
	return map[string]any{
		"type":     "object",
		"required": []string{"ok", "data", "meta"},
		"properties": map[string]any{
			"ok":   map[string]any{"const": true},
			"data": data,
			"meta": map[string]any{
				"type":     "object",
				"required": []string{"command", "cliVersion", "productContractRevision", "remoteContractRevision", "warnings"},
				"properties": map[string]any{
					"command":                 map[string]any{"const": definition.path},
					"cliVersion":              map[string]any{"type": "string"},
					"productContractRevision": map[string]any{"type": "string"},
					"remoteContractRevision":  map[string]any{"type": "string"},
					"warnings": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"page": map[string]any{
						"type":     "object",
						"required": []string{"limit", "nextCursor", "hasMore"},
						"properties": map[string]any{
							"limit":      map[string]any{"type": "integer", "minimum": 1},
							"nextCursor": map[string]any{"type": []string{"string", "null"}},
							"hasMore":    map[string]any{"type": "boolean"},
							"truncated":  map[string]any{"type": "boolean"},
							"truncationReason": map[string]any{
								"type": []string{"string", "null"},
							},
						},
						"additionalProperties": false,
					},
				},
				"additionalProperties": false,
			},
		},
		"additionalProperties": false,
	}
}

func mcpFieldName(name string) string {
	if name == "event-id" {
		return "eventId"
	}
	return strings.ReplaceAll(name, "-", "")
}

// ExecuteMCP decodes an MCP object into the same typed invocation used after
// CLI parsing. It never constructs argv and never invokes Execute.
func ExecuteMCP(
	ctx context.Context,
	tool string,
	arguments map[string]any,
	dependencies Dependencies,
	options ...MCPExecutionOptions,
) Result {
	definition, found := mcpCommandDefinition(tool)
	if !found {
		return failure("unknown", 2, errorBody{
			Type:      "usage.invalid",
			Code:      "MCP_TOOL_NOT_FOUND",
			Message:   "Unknown MCP tool.",
			Retryable: false,
			Details:   map[string]any{},
		}, false)
	}
	document, inputError := mcpArgumentDocument(arguments)
	if inputError != nil {
		return failure(definition.path, 2, *inputError, false)
	}
	executionOptions := MCPExecutionOptions{}
	if len(options) > 0 {
		executionOptions = options[0]
	}
	invocation, inputError := parseMCPProductInvocation(definition, document, executionOptions)
	if inputError != nil {
		return failure(definition.path, exitCodeForType(inputError.Type), *inputError, false)
	}
	return invokeProductOperation(ctx, invocation, dependencies, false)
}

func mcpCommandDefinition(tool string) (commandDefinition, bool) {
	for _, candidate := range commandCatalog {
		name, eligible := mcpToolName(candidate)
		if eligible && name == tool {
			return candidate, true
		}
	}
	return commandDefinition{}, false
}

func mcpArgumentDocument(arguments map[string]any) (map[string]json.RawMessage, *errorBody) {
	if arguments == nil {
		return map[string]json.RawMessage{}, nil
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, mcpArgumentsInvalid("Tool arguments must be a JSON object.")
	}
	var document map[string]json.RawMessage
	if json.Unmarshal(encoded, &document) != nil || document == nil {
		return nil, mcpArgumentsInvalid("Tool arguments must be a JSON object.")
	}
	return document, nil
}

func parseMCPProductInvocation(
	definition commandDefinition,
	document map[string]json.RawMessage,
	options MCPExecutionOptions,
) (productInvocation, *errorBody) {
	invocation := productInvocation{
		definition: definition,
	}
	if definition.safety.Kind != "read-only" {
		if raw, ok := document["dryRun"]; ok {
			var dryRun bool
			if json.Unmarshal(raw, &dryRun) != nil {
				return productInvocation{}, mcpArgumentsInvalid("dryRun must be a boolean.")
			}
			invocation.execution.DryRun = dryRun
			delete(document, "dryRun")
		}
	}

	switch definition.kind {
	case postersListCommand, postersSearchCommand, contactsListCommand, eventsListCommand:
		collection, inputError := parseMCPCollectionOptions(definition, document, options.MaxItems)
		invocation.collection = collection
		return invocation, inputError
	case guestsListCommand:
		eventID, inputError := takeMCPEventID(document)
		if inputError != nil {
			return productInvocation{}, inputError
		}
		collection, inputError := parseMCPCollectionOptions(definition, document, options.MaxItems)
		invocation.guestList = guestListOperationInput{EventID: eventID, Collection: collection}
		return invocation, inputError
	case guestsInviteCommand:
		eventID, inputError := takeMCPEventID(document)
		if inputError != nil {
			return productInvocation{}, inputError
		}
		contact, inputError := takeMCPRequiredString(document, "contact", "CONTACT_REQUIRED", "Contact is required.")
		if inputError != nil {
			return productInvocation{}, inputError
		}
		if inputError := rejectMCPFields(document); inputError != nil {
			return productInvocation{}, inputError
		}
		invocation.guestInvite = guestInviteOptions{EventID: eventID, ContactQuery: contact}
		return invocation, nil
	case eventsGetCommand, rsvpGetCommand:
		eventID, inputError := takeMCPEventID(document)
		if inputError != nil {
			return productInvocation{}, inputError
		}
		if inputError := rejectMCPFields(document); inputError != nil {
			return productInvocation{}, inputError
		}
		invocation.eventID = eventID
		return invocation, nil
	case eventsCreateCommand:
		input, inputError := normalizeEventCreateInput(document)
		invocation.eventCreate = eventCreateOptions{Input: input}
		return invocation, inputError
	case eventsUpdateCommand:
		eventID, inputError := takeMCPEventID(document)
		if inputError != nil {
			return productInvocation{}, inputError
		}
		input, inputError := normalizeEventUpdateInput(document)
		if inputError == nil && len(input.fields()) == 0 {
			inputError = eventWriteInputFailure("UPDATE_FIELD_REQUIRED", "At least one writable field is required.")
		}
		invocation.eventUpdate = eventUpdateOptions{EventID: eventID, Input: input}
		return invocation, inputError
	case eventsCancelCommand:
		eventID, inputError := takeMCPEventID(document)
		if inputError != nil {
			return productInvocation{}, inputError
		}
		input, inputError := normalizeEventCancelInput(document)
		invocation.eventCancel = eventCancelOptions{EventID: eventID, Input: input}
		return invocation, inputError
	case blastsSendCommand:
		input, inputError := parseMCPBlastSend(document)
		invocation.blastSend = input
		return invocation, inputError
	case rsvpSetCommand:
		eventID, inputError := takeMCPEventID(document)
		if inputError != nil {
			return productInvocation{}, inputError
		}
		input, inputError := normalizeRSVPInput(document)
		invocation.rsvpSet = rsvpSetOptions{EventID: eventID, Input: input}
		return invocation, inputError
	case cohostsInviteCommand, cohostsRevokeInviteCommand, cohostsRemoveCommand:
		eventID, inputError := takeMCPEventID(document)
		if inputError != nil {
			return productInvocation{}, inputError
		}
		contact, inputError := takeMCPRequiredString(document, "contact", "CONTACT_REQUIRED", "Contact is required.")
		if inputError != nil {
			return productInvocation{}, inputError
		}
		if inputError := rejectMCPFields(document); inputError != nil {
			return productInvocation{}, inputError
		}
		invocation.cohostAction = cohostActionOptions{
			EventID: eventID,
			Input:   cohostContactInput{Contact: contact},
		}
		return invocation, nil
	case cohostsLinkCreateCommand, cohostsLinkRevokeCommand:
		eventID, inputError := takeMCPEventID(document)
		if inputError != nil {
			return productInvocation{}, inputError
		}
		if inputError := rejectMCPFields(document); inputError != nil {
			return productInvocation{}, inputError
		}
		invocation.cohostLink = cohostLinkOptions{EventID: eventID}
		return invocation, nil
	default:
		return productInvocation{}, mcpArgumentsInvalid("The tool is not an executable product operation.")
	}
}

func parseMCPCollectionOptions(
	definition commandDefinition,
	document map[string]json.RawMessage,
	maxItems int,
) (collectionOptions, *errorBody) {
	options := collectionOptions{limit: defaultCollectionLimit}
	limitProvided := false
	if raw, ok := document["limit"]; ok {
		value, valid := decodeMCPInteger(raw)
		if !valid || value < 1 || value > maximumCollectionLimit {
			return collectionOptions{}, &errorBody{
				Type:      "input.invalid",
				Code:      "LIMIT_INVALID",
				Message:   "Limit must be an integer from 1 to 100.",
				Retryable: false,
				Details:   map[string]any{},
			}
		}
		options.limit = value
		limitProvided = true
		delete(document, "limit")
	}
	if raw, ok := document["cursor"]; ok {
		if json.Unmarshal(raw, &options.cursor) != nil || options.cursor == "" {
			failure := invalidCursorFailure()
			return collectionOptions{}, &failure.body
		}
		options.cursorProvided = true
		delete(document, "cursor")
	}
	if raw, ok := document["all"]; ok {
		if json.Unmarshal(raw, &options.all) != nil {
			return collectionOptions{}, mcpArgumentsInvalid("all must be a boolean.")
		}
		if !options.all {
			return collectionOptions{}, mcpArgumentsInvalid("all must be true when provided.")
		}
		if limitProvided {
			return collectionOptions{}, mcpArgumentsInvalid("limit cannot be combined with all.")
		}
		delete(document, "all")
	}
	if raw, ok := document["maxItems"]; ok {
		value, valid := decodeMCPInteger(raw)
		if !valid || value < 1 || value > 1000 {
			return collectionOptions{}, &errorBody{
				Type:      "input.invalid",
				Code:      "MAX_ITEMS_INVALID",
				Message:   "Max items must be an integer from 1 to 1000.",
				Retryable: false,
				Details:   map[string]any{},
			}
		}
		options.max = value
		delete(document, "maxItems")
	}
	if definition.kind == postersSearchCommand || definition.kind == contactsListCommand {
		if raw, ok := document["query"]; ok {
			var query string
			if json.Unmarshal(raw, &query) != nil {
				return collectionOptions{}, mcpArgumentsInvalid("query must be a string.")
			}
			options.query = strings.ToLower(strings.TrimSpace(query))
			delete(document, "query")
		}
		if definition.kind == postersSearchCommand && options.query == "" {
			return collectionOptions{}, &errorBody{
				Type:      "input.invalid",
				Code:      "QUERY_REQUIRED",
				Message:   "Search query must not be empty.",
				Retryable: false,
				Details:   map[string]any{},
			}
		}
	}
	if definition.kind == eventsListCommand {
		raw, ok := document["when"]
		if ok {
			_ = json.Unmarshal(raw, &options.when)
			options.when = strings.ToLower(strings.TrimSpace(options.when))
			delete(document, "when")
		}
		if !ok || options.when != "upcoming" && options.when != "past" {
			return collectionOptions{}, &errorBody{
				Type:      "input.invalid",
				Code:      "WHEN_INVALID",
				Message:   "when must be upcoming or past.",
				Retryable: false,
				Details:   map[string]any{},
			}
		}
	}
	if inputError := rejectMCPFields(document); inputError != nil {
		return collectionOptions{}, inputError
	}
	if options.all && options.max == 0 {
		return collectionOptions{}, &errorBody{
			Type:      "input.invalid",
			Code:      "MAX_ITEMS_REQUIRED",
			Message:   "all requires maxItems.",
			Retryable: false,
			Details:   map[string]any{},
		}
	}
	if !options.all && options.max != 0 {
		return collectionOptions{}, &errorBody{
			Type:      "input.invalid",
			Code:      "ALL_REQUIRED",
			Message:   "maxItems requires all.",
			Retryable: false,
			Details:   map[string]any{},
		}
	}
	if options.all {
		options.limit = options.max
	}
	if maxItems > 0 && options.limit > maxItems {
		options.limit = maxItems
		options.serverLimited = true
	}
	return options, nil
}

func parseMCPBlastSend(document map[string]json.RawMessage) (blastSendOperationInput, *errorBody) {
	eventID, inputError := takeMCPEventID(document)
	if inputError != nil {
		return blastSendOperationInput{}, inputError
	}
	audience, inputError := takeMCPRequiredString(document, "audience", "AUDIENCE_REQUIRED", "Audience is required.")
	if inputError != nil {
		return blastSendOperationInput{}, inputError
	}
	if audience != blastAudienceAllGuests {
		return blastSendOperationInput{}, eventWriteInputFailure("AUDIENCE_INVALID", "Only all-guests is supported.")
	}
	message, inputError := takeMCPRequiredString(document, "message", "MESSAGE_EMPTY", "The message must not be empty.")
	if inputError != nil {
		return blastSendOperationInput{}, inputError
	}
	showOnEventPage := false
	if raw, ok := document["showOnEventPage"]; ok {
		if json.Unmarshal(raw, &showOnEventPage) != nil {
			return blastSendOperationInput{}, mcpArgumentsInvalid("showOnEventPage must be a boolean.")
		}
		delete(document, "showOnEventPage")
	}
	if inputError := rejectMCPFields(document); inputError != nil {
		return blastSendOperationInput{}, inputError
	}
	prepared, inputError := prepareBlastMessage([]byte(message))
	return blastSendOperationInput{
		EventID:         eventID,
		Audience:        audience,
		ShowOnEventPage: showOnEventPage,
		Message:         prepared,
	}, inputError
}

func takeMCPEventID(document map[string]json.RawMessage) (string, *errorBody) {
	return takeMCPRequiredString(document, "eventId", "EVENT_ID_REQUIRED", "Event ID is required.")
}

func takeMCPRequiredString(
	document map[string]json.RawMessage,
	field string,
	code string,
	message string,
) (string, *errorBody) {
	raw, ok := document[field]
	if !ok {
		return "", eventWriteInputFailure(code, message)
	}
	delete(document, field)
	var value string
	if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return "", eventWriteInputFailure(code, message)
	}
	return strings.TrimSpace(value), nil
}

func rejectMCPFields(document map[string]json.RawMessage) *errorBody {
	if len(document) == 0 {
		return nil
	}
	return eventWriteInputFailure("INPUT_FIELD_UNKNOWN", "Tool input contains an unknown field.")
}

func decodeMCPInteger(raw json.RawMessage) (int, bool) {
	document := map[string]json.RawMessage{"value": raw}
	return requiredRSVPInteger(document, "value")
}

func mcpArgumentsInvalid(message string) *errorBody {
	return &errorBody{
		Type:      "input.invalid",
		Code:      "MCP_ARGUMENTS_INVALID",
		Message:   message,
		Retryable: false,
		Details:   map[string]any{},
	}
}
