package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// MCPDefinitions exposes only implemented product operations. CLI-only and
// credential-management commands deliberately remain absent.
func MCPDefinitions() []MCPDefinition {
	definitions := make([]MCPDefinition, 0, len(commandCatalog))
	for _, definition := range commandCatalog {
		if !mcpEligible(definition) {
			continue
		}
		input := mcpInputSchema(definition)
		inputJSON, _ := json.Marshal(input)
		outputJSON, _ := json.Marshal(mcpOutputSchema(definition))
		definitions = append(definitions, MCPDefinition{
			Name:         strings.ReplaceAll(definition.path, ".", "_"),
			Command:      definition.path,
			Description:  "Partiful " + strings.ReplaceAll(definition.path, ".", " ") + ".",
			InputSchema:  inputJSON,
			OutputSchema: outputJSON,
			ReadOnly:     definition.safety.Kind == "read-only",
			Destructive:  definition.safety.Destructive,
		})
	}
	slices.SortFunc(definitions, func(left, right MCPDefinition) int { return strings.Compare(left.Name, right.Name) })
	return definitions
}

func mcpEligible(definition commandDefinition) bool {
	switch definition.kind {
	case authLoginCommand, authStatusCommand, authLogoutCommand, schemaCommand, doctorCommand, versionCommand:
		return false
	default:
		return true
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
		properties["message"] = map[string]any{"type": "string", "minLength": 1, "description": "Private blast text; it is never echoed in previews or errors."}
		required, _ := schema["required"].([]any)
		for index, field := range required {
			if field == "messageFile" {
				required[index] = "message"
			}
		}
		schema["required"] = required
	}
	for _, positional := range definition.positionals {
		properties[mcpFieldName(positional.Name)] = map[string]any{"type": "string", "minLength": 1}
		if positional.Required {
			required, _ := schema["required"].([]any)
			alreadyRequired := false
			for _, field := range required {
				if field == mcpFieldName(positional.Name) {
					alreadyRequired = true
					break
				}
			}
			if !alreadyRequired {
				schema["required"] = append(required, mcpFieldName(positional.Name))
			}
		}
	}
	if definition.safety.Kind != "read-only" {
		properties["dryRun"] = map[string]any{"type": "boolean", "description": "Preview without target mutation or credential persistence."}
	}
	return schema
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
			"meta": map[string]any{"type": "object"},
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

// ExecuteMCP invokes the same application dispatcher used by the CLI. The MCP
// adapter maps a typed operation object onto the command contract without
// shelling out or accepting arbitrary argv.
func ExecuteMCP(ctx context.Context, tool string, arguments map[string]any, dependencies Dependencies) Result {
	var definition commandDefinition
	found := false
	for _, candidate := range commandCatalog {
		if mcpEligible(candidate) && strings.ReplaceAll(candidate.path, ".", "_") == tool {
			definition, found = candidate, true
			break
		}
	}
	if !found {
		return failure("unknown", 2, errorBody{Type: "usage.invalid", Code: "MCP_TOOL_NOT_FOUND", Message: "Unknown MCP tool.", Retryable: false, Details: map[string]any{}}, false)
	}
	argv, stdin, err := mcpArguments(definition, arguments)
	if err != nil {
		return failure(definition.path, 2, errorBody{Type: "input.invalid", Code: "MCP_ARGUMENTS_INVALID", Message: err.Error(), Retryable: false, Details: map[string]any{}}, false)
	}
	// MCP invocation is explicit and must never invoke a terminal confirmation.
	if definition.safety.Destructive {
		argv = append(argv, "--force")
	}
	return Execute(ctx, Request{Argv: argv, Stdin: stdin}, dependencies)
}

func mcpArguments(definition commandDefinition, arguments map[string]any) ([]string, io.Reader, error) {
	argv := append([]string{}, definition.invocation...)
	for _, positional := range definition.positionals {
		name := mcpFieldName(positional.Name)
		value, ok := arguments[name]
		if !ok {
			if positional.Required {
				return nil, nil, fmt.Errorf("%s is required", name)
			}
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, nil, fmt.Errorf("%s must be a non-empty string", name)
		}
		argv = append(argv, text)
	}
	stdin := io.Reader(nil)
	for key, value := range arguments {
		if key == "eventId" {
			continue
		}
		if key == "dryRun" {
			if enabled, ok := value.(bool); !ok {
				return nil, nil, fmt.Errorf("dryRun must be a boolean")
			} else if enabled {
				argv = append(argv, "--dry-run")
			}
			continue
		}
		flag, known := mcpFlag(definition, key)
		if !known {
			return nil, nil, fmt.Errorf("unknown field %q", key)
		}
		if key == "message" && definition.kind == blastsSendCommand {
			text, ok := value.(string)
			if !ok {
				return nil, nil, fmt.Errorf("message must be a string")
			}
			argv = append(argv, "--message-file", "-")
			stdin = strings.NewReader(text)
			continue
		}
		if boolean, ok := value.(bool); ok {
			if key == "showOnEventPage" {
				if boolean {
					argv = append(argv, flag)
				}
				continue
			}
			argv = append(argv, flag, fmt.Sprintf("%t", boolean))
			continue
		}
		if list, ok := value.([]any); ok {
			for _, item := range list {
				text, ok := item.(string)
				if !ok {
					return nil, nil, fmt.Errorf("%s must contain strings", key)
				}
				argv = append(argv, flag, text)
			}
			continue
		}
		if object, ok := value.(map[string]any); ok {
			encoded, _ := json.Marshal(object)
			argv = append(argv, flag, string(encoded))
			continue
		}
		text, ok := value.(string)
		if !ok {
			return nil, nil, fmt.Errorf("%s has an invalid type", key)
		}
		argv = append(argv, flag, text)
	}
	return argv, stdin, nil
}

func mcpFlag(definition commandDefinition, key string) (string, bool) {
	overrides := map[string]string{"maxItems": "--max-items", "showOnEventPage": "--show-on-event-page", "plusOnes": "--plus-one", "questionnaireResponse": "--questionnaire-response", "notifyGuests": "--notify-guests"}
	if value, ok := overrides[key]; ok {
		return value, true
	}
	for _, flag := range definition.flags {
		name := strings.TrimPrefix(flag.Name, "--")
		if strings.ReplaceAll(name, "-", "") == strings.ToLower(key) || name == key {
			return flag.Name, true
		}
	}
	return "", false
}
