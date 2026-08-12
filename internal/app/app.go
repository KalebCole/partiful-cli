package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"time"

	"github.com/KalebCole/partiful-cli/internal/auth"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

const (
	Version                 = "1.0.0"
	ProductContractRevision = "2026-08-10.1"
	RemoteContractRevision  = "2026-08-11.4"
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
	HTTP                 remote.HTTPClient
	CursorKeys           CursorKeyProvider
	CursorRandom         io.Reader
	AuthRandom           io.Reader
	Terminal             auth.PrivateTerminal
}

type commandKind uint8

const (
	versionCommand commandKind = iota
	schemaCommand
	authLoginCommand
	authStatusCommand
	authLogoutCommand
	doctorCommand
	postersListCommand
	postersSearchCommand
	contactsListCommand
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
		path:          "auth.login",
		invocation:    []string{"auth", "login"},
		kind:          authLoginCommand,
		positionals:   []positionalDefinition{},
		flags:         []flagDefinition{},
		inputSchema:   emptyInputSchema(),
		successSchema: authStateSuccessSchema(),
		failureTypes: []string{
			"input.invalid",
			"auth.expired",
			"auth.human_required",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: safetyDefinition{
			Kind:                 "local-mutation",
			PlanRequired:         false,
			ConfirmationRequired: false,
		},
	},
	{
		path:          "auth.status",
		invocation:    []string{"auth", "status"},
		kind:          authStatusCommand,
		positionals:   []positionalDefinition{},
		flags:         []flagDefinition{},
		inputSchema:   emptyInputSchema(),
		successSchema: authStateSuccessSchema(),
		failureTypes: []string{
			"auth.expired",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: safetyDefinition{
			Kind:                 "local-mutation",
			PlanRequired:         false,
			ConfirmationRequired: false,
		},
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
	{
		path:          "posters.list",
		invocation:    []string{"posters", "list"},
		kind:          postersListCommand,
		positionals:   []positionalDefinition{},
		flags:         collectionFlagDefinitions(),
		inputSchema:   collectionInputSchema(false),
		successSchema: posterCollectionSuccessSchema(),
		failureTypes: []string{
			"input.invalid",
			"state.conflict",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: readOnlySafety(),
	},
	{
		path:        "posters.search",
		invocation:  []string{"posters", "search"},
		kind:        postersSearchCommand,
		positionals: []positionalDefinition{},
		flags: append([]flagDefinition{{
			Name:        "--query",
			Required:    true,
			Description: "Non-empty poster search text.",
			TakesValue:  true,
		}}, collectionFlagDefinitions()...),
		inputSchema:   collectionInputSchema(true),
		successSchema: posterCollectionSuccessSchema(),
		failureTypes: []string{
			"input.invalid",
			"state.conflict",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: readOnlySafety(),
	},
	{
		path:        "contacts.list",
		invocation:  []string{"contacts", "list"},
		kind:        contactsListCommand,
		positionals: []positionalDefinition{},
		flags: append([]flagDefinition{{
			Name:        "--query",
			Description: "Optional non-empty contact name filter.",
			TakesValue:  true,
		}}, collectionFlagDefinitions()...),
		inputSchema:   contactCollectionInputSchema(),
		successSchema: contactCollectionSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"state.conflict",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: readOnlySafety(),
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
	TakesValue  bool   `json:"-"`
}

type jsonSchema struct {
	Type                 any                   `json:"type,omitempty"`
	AdditionalProperties *bool                 `json:"additionalProperties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	Properties           map[string]jsonSchema `json:"properties,omitempty"`
	Enum                 []string              `json:"enum,omitempty"`
	Format               string                `json:"format,omitempty"`
	Minimum              *int                  `json:"minimum,omitempty"`
	Maximum              *int                  `json:"maximum,omitempty"`
	MinLength            *int                  `json:"minLength,omitempty"`
	Pattern              string                `json:"pattern,omitempty"`
	DependentRequired    map[string][]string   `json:"dependentRequired,omitempty"`
	Items                *jsonSchema           `json:"items,omitempty"`
	OneOf                []jsonSchema          `json:"oneOf,omitempty"`
	AllOf                []jsonSchema          `json:"allOf,omitempty"`
	Not                  *jsonSchema           `json:"not,omitempty"`
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

func collectionFlagDefinitions() []flagDefinition {
	return []flagDefinition{
		{Name: "--limit", Description: "Maximum items in this result.", TakesValue: true},
		{Name: "--cursor", Description: "Opaque cursor from the same command and filters.", TakesValue: true},
		{Name: "--all", Description: "Return multiple pages up to --max-items."},
		{Name: "--max-items", Description: "Hard result limit for --all.", TakesValue: true},
	}
}

func collectionInputSchema(search bool) jsonSchema {
	one := 1
	oneHundred := 100
	oneThousand := 1000
	properties := map[string]jsonSchema{
		"limit":    {Type: "integer", Minimum: &one, Maximum: &oneHundred},
		"cursor":   {Type: "string", MinLength: &one},
		"all":      {Type: "boolean"},
		"maxItems": {Type: "integer", Minimum: &one, Maximum: &oneThousand},
	}
	required := []string{}
	if search {
		properties["query"] = jsonSchema{Type: "string", MinLength: &one, Pattern: `\S`}
		required = append(required, "query")
	}
	schema := objectSchema(required, properties)
	schema.DependentRequired = map[string][]string{
		"all":      {"maxItems"},
		"maxItems": {"all"},
	}
	schema.AllOf = []jsonSchema{{
		Not: &jsonSchema{Required: []string{"all", "limit"}},
	}}
	return schema
}

func contactCollectionInputSchema() jsonSchema {
	schema := collectionInputSchema(false)
	one := 1
	schema.Properties["query"] = jsonSchema{Type: "string", MinLength: &one, Pattern: `\S`}
	return schema
}

func posterCollectionSuccessSchema() jsonSchema {
	nullableInteger := jsonSchema{Type: []string{"integer", "null"}}
	stringItem := jsonSchema{Type: "string"}
	stringList := jsonSchema{Type: "array", Items: &stringItem}
	poster := objectSchema(
		[]string{"posterId", "name", "url", "contentType", "width", "height", "tags", "categories"},
		map[string]jsonSchema{
			"posterId":    {Type: "string"},
			"name":        {Type: "string"},
			"url":         {Type: "string", Format: "uri"},
			"contentType": {Type: "string"},
			"width":       nullableInteger,
			"height":      nullableInteger,
			"tags":        stringList,
			"categories":  stringList,
		},
	)
	items := jsonSchema{Type: "array", Items: &poster}
	return objectSchema([]string{"items"}, map[string]jsonSchema{"items": items})
}

func contactCollectionSuccessSchema() jsonSchema {
	zero := 0
	contact := objectSchema(
		[]string{"displayName", "sharedEventCount"},
		map[string]jsonSchema{
			"displayName":      {Type: "string"},
			"sharedEventCount": {Type: "integer", Minimum: &zero},
		},
	)
	items := jsonSchema{Type: "array", Items: &contact}
	return objectSchema([]string{"items"}, map[string]jsonSchema{"items": items})
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
	Command                 string    `json:"command"`
	CLIVersion              string    `json:"cliVersion"`
	ProductContractRevision string    `json:"productContractRevision"`
	RemoteContractRevision  string    `json:"remoteContractRevision"`
	Warnings                []string  `json:"warnings"`
	Page                    *pageMeta `json:"page,omitempty"`
}

type pageMeta struct {
	Limit      int     `json:"limit"`
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

type posterData struct {
	Items []poster `json:"items"`
}

type poster struct {
	PosterID    string   `json:"posterId"`
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	ContentType string   `json:"contentType"`
	Width       *int     `json:"width"`
	Height      *int     `json:"height"`
	Tags        []string `json:"tags"`
	Categories  []string `json:"categories"`
}

type contactData struct {
	Items []contact `json:"items"`
}

type contact struct {
	DisplayName      string `json:"displayName"`
	SharedEventCount int    `json:"sharedEventCount"`
}

type versionData struct {
	Version                 string `json:"version"`
	ProductContractRevision string `json:"productContractRevision"`
	RemoteContractRevision  string `json:"remoteContractRevision"`
}

func Execute(ctx context.Context, request Request, dependencies Dependencies) Result {
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
			case authLoginCommand:
				if slices.Contains(request.Argv, "--non-interactive") ||
					dependencies.Terminal == nil {
					return privateTerminalRequiredFailure(definition.path, pretty)
				}
				if dependencies.CredentialsPathError != nil {
					return configurationDirectoryFailure(definition.path, pretty)
				}
				clock := time.Now
				if dependencies.Now != nil {
					clock = dependencies.Now
				}
				state, err := auth.Login(
					ctx,
					dependencies.Files,
					dependencies.CredentialsPath,
					dependencies.Terminal,
					clock,
					dependencies.AuthRandom,
					remote.AuthClient{HTTP: dependencies.HTTP},
				)
				if err != nil {
					if errors.Is(err, auth.ErrHumanRequired) {
						return privateTerminalRequiredFailure(definition.path, pretty)
					}
					if errors.Is(err, auth.ErrInputInvalid) {
						return authenticationInputInvalidFailure(definition.path, pretty)
					}
					if errors.Is(err, auth.ErrAuthCodeRejected) {
						return authCodeRejectedFailure(definition.path, pretty)
					}
					if errors.Is(err, auth.ErrRemoteTokenExpired) {
						return authenticationExpiredFailure(
							definition.path,
							"INVALID_CUSTOM_TOKEN",
							"Authentication expired during login. Start login again.",
							pretty,
						)
					}
					if errors.Is(err, auth.ErrRemoteProtocolChanged) {
						return authenticationProtocolChangedFailure(definition.path, pretty)
					}
					if errors.Is(err, auth.ErrRemoteUnavailable) {
						return authenticationUnavailableFailure(definition.path, pretty)
					}
					if errors.Is(err, auth.ErrPersistence) {
						return credentialUnavailableFailure(definition.path, pretty)
					}
					return internalFailure(definition.path, pretty)
				}
				return success(definition.path, state, pretty)
			case authStatusCommand:
				if dependencies.CredentialsPathError != nil {
					return configurationDirectoryFailure(definition.path, pretty)
				}
				clock := time.Now
				if dependencies.Now != nil {
					clock = dependencies.Now
				}
				state, err := auth.StatusWithRefresh(
					ctx,
					dependencies.Files,
					dependencies.CredentialsPath,
					clock,
					remote.AuthClient{HTTP: dependencies.HTTP},
				)
				if err != nil {
					if errors.Is(err, auth.ErrRemoteTokenExpired) {
						return authenticationExpiredFailure(
							definition.path,
							"INVALID_REFRESH_TOKEN",
							"Stored authentication has expired. Log in again.",
							pretty,
						)
					}
					if errors.Is(err, auth.ErrRemoteProtocolChanged) {
						return authenticationProtocolChangedFailure(definition.path, pretty)
					}
					if errors.Is(err, auth.ErrRemoteUnavailable) {
						return authenticationUnavailableFailure(definition.path, pretty)
					}
					if errors.Is(err, auth.ErrPersistence) {
						return credentialUnavailableFailure(definition.path, pretty)
					}
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
			case postersListCommand, postersSearchCommand:
				options, inputError := parseCollectionOptions(definition, argv)
				if inputError != nil {
					return failure(definition.path, 2, *inputError, pretty)
				}
				filterHash := normalizedFilterHash(definition.path, options.query)
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
				catalog, err := (remote.Client{HTTP: dependencies.HTTP}).GetPosterCatalog(ctx)
				if err != nil {
					if errors.Is(err, remote.ErrUnavailable) {
						return remoteUnavailableFailure(definition.path, pretty)
					}
					return protocolChangedFailure(definition.path, pretty)
				}
				filteredPosters := catalog.Posters
				if definition.kind == postersSearchCommand {
					filteredPosters = filterPosters(catalog.Posters, options.query)
				}
				offset := 0
				if options.cursorProvided {
					var cursorFailure *cursorValidationFailure
					offset, cursorFailure = cursorSnapshotOffset(
						decodedCursor,
						catalog.PayloadSHA256,
						len(filteredPosters),
						"The poster catalog changed after this cursor was issued.",
					)
					if cursorFailure != nil {
						return failure(definition.path, cursorFailure.exitCode, cursorFailure.body, pretty)
					}
				}
				end := min(offset+options.limit, len(filteredPosters))
				items := make([]poster, 0, end-offset)
				for _, remotePoster := range filteredPosters[offset:end] {
					items = append(items, poster{
						PosterID:    remotePoster.ID,
						Name:        remotePoster.Name,
						URL:         remotePoster.URL,
						ContentType: remotePoster.ContentType,
						Width:       remotePoster.Width,
						Height:      remotePoster.Height,
						Tags:        remotePoster.Tags,
						Categories:  remotePoster.Categories,
					})
				}
				var cursor *string
				hasMore := end < len(filteredPosters)
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
				return collectionSuccess(definition.path, posterData{Items: items}, pageMeta{
					Limit:      options.limit,
					NextCursor: cursor,
					HasMore:    hasMore,
				}, pretty)
			case contactsListCommand:
				options, inputError := parseCollectionOptions(definition, argv)
				if inputError != nil {
					return failure(definition.path, 2, *inputError, pretty)
				}
				filterHash := normalizedFilterHash(definition.path, options.query)
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
				clock := time.Now
				if dependencies.Now != nil {
					clock = dependencies.Now
				}
				session, err := auth.AcquireSession(
					ctx,
					dependencies.Files,
					dependencies.CredentialsPath,
					clock,
					remote.AuthClient{HTTP: dependencies.HTTP},
				)
				if err != nil {
					if errors.Is(err, auth.ErrRequired) {
						return authenticationRequiredFailure(definition.path, pretty)
					}
					if errors.Is(err, auth.ErrRemoteTokenExpired) {
						return authenticationExpiredFailure(
							definition.path,
							"INVALID_REFRESH_TOKEN",
							"Stored authentication has expired. Log in again.",
							pretty,
						)
					}
					if errors.Is(err, auth.ErrExpired) {
						return authenticationExpiredFailure(
							definition.path,
							"SESSION_EXPIRED",
							"Stored authentication has expired. Log in again.",
							pretty,
						)
					}
					if errors.Is(err, auth.ErrRemoteProtocolChanged) {
						return authenticationProtocolChangedFailure(definition.path, pretty)
					}
					if errors.Is(err, auth.ErrRemoteUnavailable) {
						return authenticationUnavailableFailure(definition.path, pretty)
					}
					return internalFailure(definition.path, pretty)
				}
				deviceID, err := randomDeviceID(dependencies.AuthRandom)
				if err != nil {
					return internalFailure(definition.path, pretty)
				}
				catalog, err := (remote.Client{HTTP: dependencies.HTTP}).GetContacts(
					ctx,
					session.AccessToken,
					deviceID,
				)
				if err != nil {
					if errors.Is(err, remote.ErrUnavailable) {
						return contactsUnavailableFailure(definition.path, pretty)
					}
					if errors.Is(err, remote.ErrUnauthenticated) {
						return authenticationExpiredFailure(
							definition.path,
							"REMOTE_SESSION_UNAUTHENTICATED",
							"Stored authentication is no longer accepted. Log in again.",
							pretty,
						)
					}
					return contactsProtocolChangedFailure(definition.path, pretty)
				}
				filteredContacts := filterContacts(catalog.Contacts, options.query)
				offset := 0
				if options.cursorProvided {
					var cursorFailure *cursorValidationFailure
					offset, cursorFailure = cursorSnapshotOffset(
						decodedCursor,
						catalog.PayloadSHA256,
						len(filteredContacts),
						"The contact catalog changed after this cursor was issued.",
					)
					if cursorFailure != nil {
						return failure(definition.path, cursorFailure.exitCode, cursorFailure.body, pretty)
					}
				}
				end := min(offset+options.limit, len(filteredContacts))
				items := make([]contact, 0, end-offset)
				for _, remoteContact := range filteredContacts[offset:end] {
					items = append(items, contact{
						DisplayName:      remoteContact.Name,
						SharedEventCount: remoteContact.SharedEventCount,
					})
				}
				var cursor *string
				hasMore := end < len(filteredContacts)
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
				return collectionSuccess(definition.path, contactData{Items: items}, pageMeta{
					Limit:      options.limit,
					NextCursor: cursor,
					HasMore:    hasMore,
				}, pretty)
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

func loadCursorKey(dependencies Dependencies) ([]byte, error) {
	if dependencies.CursorKeys == nil {
		return nil, errors.New("cursor key provider is unavailable")
	}
	key, err := dependencies.CursorKeys.Key()
	if err != nil || len(key) != CursorKeySize {
		return nil, errors.New("cursor key is unavailable")
	}
	return key, nil
}

func (definition commandDefinition) matches(argv []string) bool {
	if definition.kind == schemaCommand && len(argv) == 2 {
		return argv[0] == "schema"
	}
	if (definition.kind == postersListCommand ||
		definition.kind == postersSearchCommand ||
		definition.kind == contactsListCommand) &&
		len(argv) >= len(definition.invocation) {
		return slices.Equal(argv[:len(definition.invocation)], definition.invocation)
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
		FailureTypes:  declaredFailureTypes(definition),
		Safety:        definition.safety,
	}
}

func declaredFailureTypes(definition commandDefinition) []string {
	failureTypes := []string{"usage.invalid", "input.invalid"}
	for _, failureType := range definition.failureTypes {
		if !slices.Contains(failureTypes, failureType) {
			failureTypes = append(failureTypes, failureType)
		}
	}
	return failureTypes
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

func privateTerminalRequiredFailure(command string, pretty bool) Result {
	result := failure(command, 3, errorBody{
		Type:      "auth.human_required",
		Code:      "PRIVATE_TERMINAL_REQUIRED",
		Message:   "Authentication login requires a private terminal.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: private terminal required\n"
	return result
}

func authenticationInputInvalidFailure(command string, pretty bool) Result {
	result := failure(command, 2, errorBody{
		Type:      "input.invalid",
		Code:      "AUTH_INPUT_INVALID",
		Message:   "Authentication input is invalid.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: authentication input invalid\n"
	return result
}

func authCodeRejectedFailure(command string, pretty bool) Result {
	result := failure(command, 2, errorBody{
		Type:      "input.invalid",
		Code:      "AUTH_CODE_REJECTED",
		Message:   "The verification code was rejected.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: authentication code rejected\n"
	return result
}

func authenticationExpiredFailure(command, code, message string, pretty bool) Result {
	result := failure(command, 3, errorBody{
		Type:      "auth.expired",
		Code:      code,
		Message:   message,
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: authentication expired\n"
	return result
}

func authenticationRequiredFailure(command string, pretty bool) Result {
	result := failure(command, 3, errorBody{
		Type:      "auth.required",
		Code:      "AUTHENTICATION_REQUIRED",
		Message:   "Authentication is required. Log in and try again.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: authentication required\n"
	return result
}

func authenticationProtocolChangedFailure(command string, pretty bool) Result {
	result := failure(command, 9, errorBody{
		Type:      "contract.protocol_changed",
		Code:      "AUTH_PROTOCOL_CHANGED",
		Message:   "Authentication no longer matches the reviewed remote contract.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: authentication protocol changed\n"
	return result
}

func authenticationUnavailableFailure(command string, pretty bool) Result {
	result := failure(command, 8, errorBody{
		Type:      "remote.unavailable",
		Code:      "AUTH_SERVICE_UNAVAILABLE",
		Message:   "The authentication service is unavailable.",
		Retryable: true,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: authentication service unavailable\n"
	return result
}

func remoteUnavailableFailure(command string, pretty bool) Result {
	result := failure(command, 8, errorBody{
		Type:      "remote.unavailable",
		Code:      "POSTER_CATALOG_UNAVAILABLE",
		Message:   "The poster catalog is unavailable.",
		Retryable: true,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: poster catalog unavailable\n"
	return result
}

func protocolChangedFailure(command string, pretty bool) Result {
	result := failure(command, 9, errorBody{
		Type:      "contract.protocol_changed",
		Code:      "POSTER_CATALOG_PROTOCOL_CHANGED",
		Message:   "The poster catalog no longer matches the reviewed remote contract.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: poster catalog protocol changed\n"
	return result
}

func contactsUnavailableFailure(command string, pretty bool) Result {
	result := failure(command, 8, errorBody{
		Type:      "remote.unavailable",
		Code:      "CONTACTS_UNAVAILABLE",
		Message:   "Contacts are unavailable.",
		Retryable: true,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: contacts unavailable\n"
	return result
}

func contactsProtocolChangedFailure(command string, pretty bool) Result {
	result := failure(command, 9, errorBody{
		Type:      "contract.protocol_changed",
		Code:      "CONTACTS_PROTOCOL_CHANGED",
		Message:   "Contacts no longer match the reviewed remote contract.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: contacts protocol changed\n"
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

func collectionSuccess(command string, data any, page pageMeta, pretty bool) Result {
	document := encode(successEnvelope{
		OK:   true,
		Data: data,
		Meta: successMeta{
			Command:                 command,
			CLIVersion:              Version,
			ProductContractRevision: ProductContractRevision,
			RemoteContractRevision:  RemoteContractRevision,
			Warnings:                []string{},
			Page:                    &page,
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
