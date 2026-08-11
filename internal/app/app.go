package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"time"

	"github.com/KalebCole/partiful-cli/internal/auth"
)

const (
	Version                 = "1.0.0"
	ProductContractRevision = "2026-08-10.1"
	RemoteContractRevision  = "2026-08-10.1"
)

type Request struct {
	Argv  []string
	Stdin io.Reader
}

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Dependencies struct {
	Files                auth.FileSystem
	CredentialsPath      string
	CredentialsPathError error
	Now                  func() time.Time
}

type commandKind uint8

const (
	versionCommand commandKind = iota
	schemaCommand
	authStatusCommand
	authLogoutCommand
	doctorCommand
)

type commandDefinition struct {
	path          string
	invocation    []string
	kind          commandKind
	positionals   []positionalDefinition
	flags         []flagDefinition
	inputSchema   jsonSchema
	successSchema jsonSchema
	failureTypes  []string
	safety        safetyDefinition
}

var commandCatalog = []commandDefinition{
	{
		path:          "version",
		invocation:    []string{"--version"},
		kind:          versionCommand,
		positionals:   []positionalDefinition{},
		flags:         []flagDefinition{},
		inputSchema:   emptyInputSchema(),
		successSchema: versionSuccessSchema(),
		failureTypes:  []string{},
		safety:        readOnlySafety(),
	},
	{
		path:       "schema",
		invocation: []string{"schema"},
		kind:       schemaCommand,
		positionals: []positionalDefinition{{
			Name:        "command.path",
			Required:    false,
			Description: "Completed public command path.",
		}},
		flags:         []flagDefinition{},
		inputSchema:   emptyInputSchema(),
		successSchema: schemaSuccessSchema(),
		failureTypes:  []string{"usage.invalid"},
		safety:        readOnlySafety(),
	},
	{
		path:          "auth.status",
		invocation:    []string{"auth", "status"},
		kind:          authStatusCommand,
		positionals:   []positionalDefinition{},
		flags:         []flagDefinition{},
		inputSchema:   emptyInputSchema(),
		successSchema: authStateSuccessSchema(),
		failureTypes:  []string{"internal.failure"},
		safety:        readOnlySafety(),
	},
	{
		path:          "auth.logout",
		invocation:    []string{"auth", "logout"},
		kind:          authLogoutCommand,
		positionals:   []positionalDefinition{},
		flags:         []flagDefinition{},
		inputSchema:   emptyInputSchema(),
		successSchema: authStateSuccessSchema(),
		failureTypes:  []string{"internal.failure"},
		safety: safetyDefinition{
			Kind:                 "local-mutation",
			PlanRequired:         false,
			ConfirmationRequired: false,
		},
	},
	{
		path:          "doctor",
		invocation:    []string{"doctor"},
		kind:          doctorCommand,
		positionals:   []positionalDefinition{},
		flags:         []flagDefinition{},
		inputSchema:   emptyInputSchema(),
		successSchema: doctorSuccessSchema(),
		failureTypes:  []string{},
		safety:        readOnlySafety(),
	},
}

type positionalDefinition struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type flagDefinition struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type jsonSchema struct {
	Type                 any                   `json:"type"`
	AdditionalProperties *bool                 `json:"additionalProperties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	Properties           map[string]jsonSchema `json:"properties,omitempty"`
	Enum                 []string              `json:"enum,omitempty"`
	Format               string                `json:"format,omitempty"`
	Items                *jsonSchema           `json:"items,omitempty"`
	OneOf                []jsonSchema          `json:"oneOf,omitempty"`
}

type safetyDefinition struct {
	Kind                 string `json:"kind"`
	PlanRequired         bool   `json:"planRequired"`
	ConfirmationRequired bool   `json:"confirmationRequired"`
}

func emptyInputSchema() jsonSchema {
	allowAdditional := false
	return jsonSchema{Type: "object", AdditionalProperties: &allowAdditional}
}

func objectSchema(required []string, properties map[string]jsonSchema) jsonSchema {
	allowAdditional := false
	return jsonSchema{
		Type:                 "object",
		AdditionalProperties: &allowAdditional,
		Required:             required,
		Properties:           properties,
	}
}

func authStateSuccessSchema() jsonSchema {
	return objectSchema(
		[]string{"authenticated", "tokenState", "expiresAt"},
		map[string]jsonSchema{
			"authenticated": {Type: "boolean"},
			"tokenState": {
				Type: "string",
				Enum: []string{"healthy", "expiring", "expired", "missing"},
			},
			"expiresAt": {Type: []string{"string", "null"}, Format: "date-time"},
		},
	)
}

func versionSuccessSchema() jsonSchema {
	return objectSchema(
		[]string{"version", "productContractRevision", "remoteContractRevision"},
		map[string]jsonSchema{
			"version":                 {Type: "string"},
			"productContractRevision": {Type: "string"},
			"remoteContractRevision":  {Type: "string"},
		},
	)
}

func schemaSuccessSchema() jsonSchema {
	stringItem := jsonSchema{Type: "string"}
	commandList := objectSchema(
		[]string{"commands"},
		map[string]jsonSchema{
			"commands": {Type: "array", Items: &stringItem},
		},
	)

	descriptor := objectSchema(
		[]string{"name", "required", "description"},
		map[string]jsonSchema{
			"name":        {Type: "string"},
			"required":    {Type: "boolean"},
			"description": {Type: "string"},
		},
	)
	stringList := jsonSchema{Type: "array", Items: &stringItem}
	descriptorList := jsonSchema{Type: "array", Items: &descriptor}
	safety := objectSchema(
		[]string{"kind", "planRequired", "confirmationRequired"},
		map[string]jsonSchema{
			"kind": {
				Type: "string",
				Enum: []string{"read-only", "local-mutation"},
			},
			"planRequired":         {Type: "boolean"},
			"confirmationRequired": {Type: "boolean"},
		},
	)
	commandDetail := objectSchema(
		[]string{
			"command",
			"positionals",
			"flags",
			"inputSchema",
			"successSchema",
			"failureTypes",
			"safety",
		},
		map[string]jsonSchema{
			"command":       {Type: "string"},
			"positionals":   descriptorList,
			"flags":         descriptorList,
			"inputSchema":   {Type: "object"},
			"successSchema": {Type: "object"},
			"failureTypes":  stringList,
			"safety":        safety,
		},
	)
	return jsonSchema{
		Type:  "object",
		OneOf: []jsonSchema{commandList, commandDetail},
	}
}

func doctorSuccessSchema() jsonSchema {
	check := objectSchema(
		[]string{"name", "status", "message", "remediation"},
		map[string]jsonSchema{
			"name":        {Type: "string"},
			"status":      {Type: "string", Enum: []string{"pass", "warn", "fail"}},
			"message":     {Type: "string"},
			"remediation": {Type: []string{"string", "null"}},
		},
	)
	checks := jsonSchema{Type: "array", Items: &check}
	return objectSchema(
		[]string{"healthy", "checks"},
		map[string]jsonSchema{
			"healthy": {Type: "boolean"},
			"checks":  checks,
		},
	)
}

func readOnlySafety() safetyDefinition {
	return safetyDefinition{
		Kind:                 "read-only",
		PlanRequired:         false,
		ConfirmationRequired: false,
	}
}

type successEnvelope struct {
	OK   bool        `json:"ok"`
	Data any         `json:"data"`
	Meta successMeta `json:"meta"`
}

type successMeta struct {
	Command                 string   `json:"command"`
	CLIVersion              string   `json:"cliVersion"`
	ProductContractRevision string   `json:"productContractRevision"`
	RemoteContractRevision  string   `json:"remoteContractRevision"`
	Warnings                []string `json:"warnings"`
}

type versionData struct {
	Version                 string `json:"version"`
	ProductContractRevision string `json:"productContractRevision"`
	RemoteContractRevision  string `json:"remoteContractRevision"`
}

func Execute(_ context.Context, request Request, dependencies Dependencies) Result {
	argv := make([]string, 0, len(request.Argv))
	pretty := slices.Contains(request.Argv, "--pretty")
	seenGlobalFlags := make(map[string]bool)
	for _, argument := range request.Argv {
		switch argument {
		case "--pretty":
			if seenGlobalFlags[argument] {
				return repeatedFlagFailure(commandName(request.Argv), argument, pretty)
			}
			seenGlobalFlags[argument] = true
			continue
		case "--non-interactive":
			if seenGlobalFlags[argument] {
				return repeatedFlagFailure(commandName(request.Argv), argument, pretty)
			}
			seenGlobalFlags[argument] = true
			continue
		}
		argv = append(argv, argument)
	}
	for _, definition := range commandCatalog {
		if definition.matches(argv) {
			switch definition.kind {
			case versionCommand:
				return success(definition.path, versionData{
					Version:                 Version,
					ProductContractRevision: ProductContractRevision,
					RemoteContractRevision:  RemoteContractRevision,
				}, pretty)
			case schemaCommand:
				if len(argv) == 1 {
					return success(definition.path, schemaCatalogData(), pretty)
				}
				if selected, ok := findDefinition(argv[1]); ok {
					return success(definition.path, projectSchema(selected), pretty)
				}
				return failure(definition.path, 2, errorBody{
					Type:      "usage.invalid",
					Code:      "COMMAND_SCHEMA_NOT_FOUND",
					Message:   "No completed command has that schema path.",
					Retryable: false,
					Details:   map[string]any{},
				}, pretty)
			case authStatusCommand:
				if dependencies.CredentialsPathError != nil {
					return configurationDirectoryFailure(definition.path, pretty)
				}
				now := time.Now()
				if dependencies.Now != nil {
					now = dependencies.Now()
				}
				state, err := auth.Status(
					dependencies.Files,
					dependencies.CredentialsPath,
					now,
				)
				if err != nil {
					if errors.Is(err, auth.ErrInvalid) {
						return credentialInvalidFailure(definition.path, pretty)
					}
					if errors.Is(err, auth.ErrUnavailable) {
						return credentialUnavailableFailure(definition.path, pretty)
					}
					return internalFailure(definition.path, pretty)
				}
				return success(definition.path, state, pretty)
			case authLogoutCommand:
				if dependencies.CredentialsPathError != nil {
					return configurationDirectoryFailure(definition.path, pretty)
				}
				state, err := auth.Logout(
					dependencies.Files,
					dependencies.CredentialsPath,
				)
				if err != nil {
					if errors.Is(err, auth.ErrUnavailable) {
						return credentialUnavailableFailure(definition.path, pretty)
					}
					return internalFailure(definition.path, pretty)
				}
				return success(definition.path, state, pretty)
			case doctorCommand:
				if dependencies.CredentialsPathError != nil {
					remediation := "Set a usable user configuration directory."
					return credentialsDoctorResult(
						definition.path,
						false,
						"fail",
						"Configuration directory is unavailable.",
						&remediation,
						pretty,
					)
				}
				now := time.Now()
				if dependencies.Now != nil {
					now = dependencies.Now()
				}
				state, err := auth.Status(
					dependencies.Files,
					dependencies.CredentialsPath,
					now,
				)
				if err == nil && state.TokenState == "missing" {
					remediation := "Establish authentication before using commands that require it."
					return credentialsDoctorResult(
						definition.path,
						false,
						"fail",
						"Authentication credentials are missing.",
						&remediation,
						pretty,
					)
				}
				if err == nil && state.TokenState == "expiring" {
					remediation := "Refresh authentication before the credentials expire."
					return credentialsDoctorResult(
						definition.path,
						true,
						"warn",
						"Authentication credentials expire soon.",
						&remediation,
						pretty,
					)
				}
				if err == nil && state.TokenState == "expired" {
					remediation := "Re-establish authentication."
					return credentialsDoctorResult(
						definition.path,
						false,
						"fail",
						"Authentication credentials have expired.",
						&remediation,
						pretty,
					)
				}
				if errors.Is(err, auth.ErrInvalid) {
					remediation := "Remove the invalid credentials and re-establish authentication."
					return credentialsDoctorResult(
						definition.path,
						false,
						"fail",
						"Authentication credentials are invalid.",
						&remediation,
						pretty,
					)
				}
				if errors.Is(err, auth.ErrUnavailable) {
					remediation := "Check local credential file permissions."
					return credentialsDoctorResult(
						definition.path,
						false,
						"fail",
						"Credential storage is unavailable.",
						&remediation,
						pretty,
					)
				}
				if err != nil || state.TokenState != "healthy" {
					return internalFailure(definition.path, pretty)
				}
				return credentialsDoctorResult(
					definition.path,
					true,
					"pass",
					"Authentication credentials are available.",
					nil,
					pretty,
				)
			}
		}
	}
	return failure(commandName(request.Argv), 2, errorBody{
		Type:      "usage.invalid",
		Code:      "COMMAND_NOT_FOUND",
		Message:   "Unknown command.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
}

func (definition commandDefinition) matches(argv []string) bool {
	if definition.kind == schemaCommand && len(argv) == 2 {
		return argv[0] == "schema"
	}
	return slices.Equal(argv, definition.invocation)
}

type schemaCatalog struct {
	Commands []string `json:"commands"`
}

func schemaCatalogData() schemaCatalog {
	paths := make([]string, 0, len(commandCatalog))
	for _, definition := range commandCatalog {
		paths = append(paths, definition.path)
	}
	slices.Sort(paths)
	return schemaCatalog{Commands: paths}
}

type commandSchema struct {
	Command       string                 `json:"command"`
	Positionals   []positionalDefinition `json:"positionals"`
	Flags         []flagDefinition       `json:"flags"`
	InputSchema   jsonSchema             `json:"inputSchema"`
	SuccessSchema jsonSchema             `json:"successSchema"`
	FailureTypes  []string               `json:"failureTypes"`
	Safety        safetyDefinition       `json:"safety"`
}

type doctorData struct {
	Healthy bool          `json:"healthy"`
	Checks  []doctorCheck `json:"checks"`
}

type doctorCheck struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Message     string  `json:"message"`
	Remediation *string `json:"remediation"`
}

func credentialsDoctorResult(
	command string,
	healthy bool,
	status string,
	message string,
	remediation *string,
	pretty bool,
) Result {
	return success(command, doctorData{
		Healthy: healthy,
		Checks: []doctorCheck{{
			Name:        "credentials",
			Status:      status,
			Message:     message,
			Remediation: remediation,
		}},
	}, pretty)
}

func findDefinition(path string) (commandDefinition, bool) {
	for _, definition := range commandCatalog {
		if definition.path == path {
			return definition, true
		}
	}
	return commandDefinition{}, false
}

func projectSchema(definition commandDefinition) commandSchema {
	return commandSchema{
		Command:       definition.path,
		Positionals:   definition.positionals,
		Flags:         definition.flags,
		InputSchema:   definition.inputSchema,
		SuccessSchema: definition.successSchema,
		FailureTypes:  definition.failureTypes,
		Safety:        definition.safety,
	}
}

func commandName(argv []string) string {
	filtered := make([]string, 0, len(argv))
	for _, argument := range argv {
		if argument != "--pretty" && argument != "--non-interactive" {
			filtered = append(filtered, argument)
		}
	}
	for _, definition := range commandCatalog {
		if len(filtered) >= len(definition.invocation) &&
			slices.Equal(filtered[:len(definition.invocation)], definition.invocation) {
			return definition.path
		}
	}
	return "unknown"
}

func repeatedFlagFailure(command, flag string, pretty bool) Result {
	return failure(command, 2, errorBody{
		Type:      "input.invalid",
		Code:      "FLAG_REPEATED",
		Message:   "A scalar flag cannot be repeated.",
		Retryable: false,
		Details:   map[string]any{"flag": flag},
	}, pretty)
}

func internalFailure(command string, pretty bool) Result {
	result := failure(command, 10, errorBody{
		Type:      "internal.failure",
		Code:      "LOCAL_OPERATION_FAILED",
		Message:   "The local operation could not be completed.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: local operation failed\n"
	return result
}

func credentialInvalidFailure(command string, pretty bool) Result {
	result := failure(command, 10, errorBody{
		Type:      "internal.failure",
		Code:      "CREDENTIALS_INVALID",
		Message:   "Local credentials are invalid.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: local operation failed\n"
	return result
}

func credentialUnavailableFailure(command string, pretty bool) Result {
	result := failure(command, 10, errorBody{
		Type:      "internal.failure",
		Code:      "CREDENTIAL_STORE_UNAVAILABLE",
		Message:   "Local credential storage is unavailable.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: local operation failed\n"
	return result
}

func configurationDirectoryFailure(command string, pretty bool) Result {
	result := failure(command, 10, errorBody{
		Type:      "internal.failure",
		Code:      "CONFIG_DIRECTORY_UNAVAILABLE",
		Message:   "Local configuration directory is unavailable.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: local operation failed\n"
	return result
}

func success(command string, data any, pretty bool) Result {
	document := encode(successEnvelope{
		OK:   true,
		Data: data,
		Meta: successMeta{
			Command:                 command,
			CLIVersion:              Version,
			ProductContractRevision: ProductContractRevision,
			RemoteContractRevision:  RemoteContractRevision,
			Warnings:                []string{},
		},
	}, pretty)
	return Result{Stdout: document}
}

type failureEnvelope struct {
	OK    bool        `json:"ok"`
	Error errorBody   `json:"error"`
	Meta  failureMeta `json:"meta"`
}

type errorBody struct {
	Type      string         `json:"type"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details"`
}

type failureMeta struct {
	Command                 string `json:"command"`
	CLIVersion              string `json:"cliVersion"`
	ProductContractRevision string `json:"productContractRevision"`
	RemoteContractRevision  string `json:"remoteContractRevision"`
}

func failure(command string, exitCode int, body errorBody, pretty bool) Result {
	document := encode(failureEnvelope{
		OK:    false,
		Error: body,
		Meta: failureMeta{
			Command:                 command,
			CLIVersion:              Version,
			ProductContractRevision: ProductContractRevision,
			RemoteContractRevision:  RemoteContractRevision,
		},
	}, pretty)
	return Result{Stdout: document, ExitCode: exitCode}
}

func encode(value any, pretty bool) string {
	var document []byte
	if pretty {
		document, _ = json.MarshalIndent(value, "", "  ")
	} else {
		document, _ = json.Marshal(value)
	}
	return string(document) + "\n"
}
