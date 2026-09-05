package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/KalebCole/partiful-cli/internal/auth"
	"github.com/KalebCole/partiful-cli/internal/remote"
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
	Confirmer            Confirmer
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
	guestsListCommand
	guestsInviteCommand
	eventsListCommand
	eventsGetCommand
	eventsCreateCommand
	eventsUpdateCommand
	eventsCancelCommand
	blastsSendCommand
	rsvpGetCommand
	rsvpSetCommand
	cohostsInviteCommand
	cohostsRevokeInviteCommand
	cohostsRemoveCommand
	cohostsLinkCreateCommand
	cohostsLinkRevokeCommand
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
			Kind:        "local-mutation",
			Destructive: false,
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
			Kind:        "local-mutation",
			Destructive: false,
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
			Kind:        "local-mutation",
			Destructive: false,
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
	{
		path:       "guests.list",
		invocation: []string{"guests", "list"},
		kind:       guestsListCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags:         collectionFlagDefinitions(),
		inputSchema:   guestListInputSchema(),
		successSchema: guestsCollectionSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"permission.denied",
			"resource.not_found",
			"state.conflict",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: readOnlySafety(),
	},
	{
		path:       "guests.invite",
		invocation: []string{"guests", "invite"},
		kind:       guestsInviteCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags: append([]flagDefinition{
			{Name: "--contact", Description: "Resolvable contact display name.", Required: true, TakesValue: true},
		}, mutationFlagDefinitions()...),
		inputSchema:   guestInviteInputSchema(),
		successSchema: guestInviteSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"permission.denied",
			"resource.not_found",
			"match.ambiguous",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: consequentialActionSafety(),
	},
	{
		path:        "events.list",
		invocation:  []string{"events", "list"},
		kind:        eventsListCommand,
		positionals: []positionalDefinition{},
		flags: append([]flagDefinition{{
			Name:        "--when",
			Required:    true,
			Description: "Select upcoming or past events.",
			TakesValue:  true,
		}}, collectionFlagDefinitions()...),
		inputSchema:   eventCollectionInputSchema(),
		successSchema: eventCollectionSuccessSchema(),
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
	{
		path:       "events.get",
		invocation: []string{"events", "get"},
		kind:       eventsGetCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags:         []flagDefinition{},
		inputSchema:   eventGetInputSchema(),
		successSchema: eventGetSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"resource.not_found",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: readOnlySafety(),
	},
	{
		path:        "events.create",
		invocation:  []string{"events", "create"},
		kind:        eventsCreateCommand,
		positionals: []positionalDefinition{},
		flags: append([]flagDefinition{
			{Name: "--input", Description: "Read one structured JSON input object.", TakesValue: true},
			{Name: "--title", Description: "Event title.", TakesValue: true},
			{Name: "--start", Description: "RFC 3339 start time.", TakesValue: true},
			{Name: "--end", Description: "RFC 3339 end time.", TakesValue: true},
			{Name: "--timezone", Description: "IANA timezone.", TakesValue: true},
			{Name: "--description", Description: "Optional description.", TakesValue: true},
			{Name: "--location", Description: "Optional free-form location.", TakesValue: true},
			{Name: "--visibility", Description: "private or public.", TakesValue: true},
			{Name: "--guest-limit", Description: "Positive guest limit.", TakesValue: true},
			{Name: "--link", Description: "Link in label=url form; this flag can repeat.", TakesValue: true},
			{Name: "--poster-id", Description: "Exact built-in poster ID.", TakesValue: true},
		}, mutationFlagDefinitions()...),
		inputSchema:   eventCreateInputSchema(),
		successSchema: eventCreateSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"resource.not_found",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: standardMutationSafety(),
	},
	{
		path:       "events.update",
		invocation: []string{"events", "update"},
		kind:       eventsUpdateCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags: append([]flagDefinition{
			{Name: "--input", Description: "Read one structured JSON input object.", TakesValue: true},
			{Name: "--title", Description: "Event title.", TakesValue: true},
			{Name: "--description", Description: "Optional description.", TakesValue: true},
			{Name: "--start", Description: "RFC 3339 start time.", TakesValue: true},
			{Name: "--end", Description: "RFC 3339 end time or null in structured input.", TakesValue: true},
			{Name: "--timezone", Description: "IANA timezone.", TakesValue: true},
			{Name: "--guest-limit", Description: "Positive guest limit.", TakesValue: true},
			{Name: "--link", Description: "Link in label=url form; this flag can repeat.", TakesValue: true},
			{Name: "--poster-id", Description: "Exact built-in poster ID.", TakesValue: true},
		}, mutationFlagDefinitions()...),
		inputSchema:   eventUpdateInputSchema(),
		successSchema: eventUpdateSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"permission.denied",
			"resource.not_found",
			"state.conflict",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: standardMutationSafety(),
	},
	{
		path:       "events.cancel",
		invocation: []string{"events", "cancel"},
		kind:       eventsCancelCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags: append([]flagDefinition{
			{Name: "--input", Description: "Read one structured JSON input object.", TakesValue: true},
			{Name: "--message", Description: "Optional cancellation message.", TakesValue: true},
			{Name: "--notify-guests", Description: "true or false.", TakesValue: true},
		}, mutationFlagDefinitions()...),
		inputSchema:   eventCancelInputSchema(),
		successSchema: eventCancelSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"permission.denied",
			"resource.not_found",
			"state.conflict",
			"safety.confirmation_required",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: destructiveActionSafety(),
	},
	{
		path:       "blasts.send",
		invocation: []string{"blasts", "send"},
		kind:       blastsSendCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags: append([]flagDefinition{
			{Name: "--audience", Description: "Only all-guests is supported.", TakesValue: true},
			{Name: "--message-file", Description: "Read the private message from a file path or - for stdin.", TakesValue: true},
			{Name: "--show-on-event-page", Description: "Show the blast in the event activity feed."},
		}, mutationFlagDefinitions()...),
		inputSchema:   blastSendInputSchema(),
		successSchema: blastSendSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"permission.denied",
			"resource.not_found",
			"state.conflict",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: consequentialActionSafety(),
	},
	{
		path:       "rsvp.get",
		invocation: []string{"rsvp", "get"},
		kind:       rsvpGetCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags:         []flagDefinition{},
		inputSchema:   eventGetInputSchema(),
		successSchema: rsvpReadSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: readOnlySafety(),
	},
	{
		path:       "rsvp.set",
		invocation: []string{"rsvp", "set"},
		kind:       rsvpSetCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags: append([]flagDefinition{
			{Name: "--input", Description: "Read one structured JSON input object.", TakesValue: true},
			{Name: "--status", Description: "Writable RSVP intent.", TakesValue: true},
			{Name: "--display-name", Description: "Attendee display name.", TakesValue: true},
			{Name: "--party-size", Description: "Total attendee count.", TakesValue: true},
			{Name: "--plus-one", Description: "Named plus one; this flag can repeat.", TakesValue: true},
			{Name: "--message", Description: "Optional RSVP message.", TakesValue: true},
			{Name: "--timezone", Description: "IANA timezone.", TakesValue: true},
			{Name: "--questionnaire-response", Description: "Questionnaire response JSON.", TakesValue: true},
		}, mutationFlagDefinitions()...),
		inputSchema:   rsvpSetInputSchema(),
		successSchema: rsvpSetSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"resource.not_found",
			"state.conflict",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: standardMutationSafety(),
	},
	{
		path:       "cohosts.invite",
		invocation: []string{"cohosts", "invite"},
		kind:       cohostsInviteCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags: append([]flagDefinition{
			{Name: "--contact", Description: "Resolvable contact display name.", TakesValue: true, Required: true},
		}, mutationFlagDefinitions()...),
		inputSchema:   cohostContactInputSchema(),
		successSchema: cohostInviteSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"permission.denied",
			"resource.not_found",
			"match.ambiguous",
			"state.conflict",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: consequentialActionSafety(),
	},
	{
		path:       "cohosts.revoke-invite",
		invocation: []string{"cohosts", "revoke-invite"},
		kind:       cohostsRevokeInviteCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags: append([]flagDefinition{
			{Name: "--contact", Description: "Resolvable contact display name.", TakesValue: true, Required: true},
		}, mutationFlagDefinitions()...),
		inputSchema:   cohostContactInputSchema(),
		successSchema: cohostRevokeInviteSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"permission.denied",
			"resource.not_found",
			"match.ambiguous",
			"state.conflict",
			"safety.confirmation_required",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: destructiveActionSafety(),
	},
	{
		path:       "cohosts.remove",
		invocation: []string{"cohosts", "remove"},
		kind:       cohostsRemoveCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags: append([]flagDefinition{
			{Name: "--contact", Description: "Resolvable contact display name.", TakesValue: true, Required: true},
		}, mutationFlagDefinitions()...),
		inputSchema:   cohostContactInputSchema(),
		successSchema: cohostRemoveSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"permission.denied",
			"resource.not_found",
			"match.ambiguous",
			"state.conflict",
			"safety.confirmation_required",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: destructiveActionSafety(),
	},
	{
		path:       "cohosts.link.create",
		invocation: []string{"cohosts", "link", "create"},
		kind:       cohostsLinkCreateCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags:         mutationFlagDefinitions(),
		inputSchema:   emptyInputSchema(),
		successSchema: cohostLinkCreateSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"permission.denied",
			"resource.not_found",
			"state.conflict",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: consequentialActionSafety(),
	},
	{
		path:       "cohosts.link.revoke",
		invocation: []string{"cohosts", "link", "revoke"},
		kind:       cohostsLinkRevokeCommand,
		positionals: []positionalDefinition{{
			Name:        "event-id",
			Required:    true,
			Description: "Event identifier.",
		}},
		flags:         mutationFlagDefinitions(),
		inputSchema:   emptyInputSchema(),
		successSchema: cohostLinkRevokeSuccessSchema(),
		failureTypes: []string{
			"auth.required",
			"auth.expired",
			"permission.denied",
			"resource.not_found",
			"state.conflict",
			"safety.confirmation_required",
			"remote.unavailable",
			"contract.protocol_changed",
			"internal.failure",
		},
		safety: destructiveActionSafety(),
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
	AdditionalProperties any                   `json:"additionalProperties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	Properties           map[string]jsonSchema `json:"properties,omitempty"`
	Enum                 []string              `json:"enum,omitempty"`
	Const                any                   `json:"const,omitempty"`
	Format               string                `json:"format,omitempty"`
	Minimum              *int                  `json:"minimum,omitempty"`
	Maximum              *int                  `json:"maximum,omitempty"`
	MinLength            *int                  `json:"minLength,omitempty"`
	MaxLength            *int                  `json:"maxLength,omitempty"`
	Pattern              string                `json:"pattern,omitempty"`
	DependentRequired    map[string][]string   `json:"dependentRequired,omitempty"`
	Items                *jsonSchema           `json:"items,omitempty"`
	OneOf                []jsonSchema          `json:"oneOf,omitempty"`
	AllOf                []jsonSchema          `json:"allOf,omitempty"`
	Not                  *jsonSchema           `json:"not,omitempty"`
}

type safetyDefinition struct {
	Kind        string `json:"kind"`
	Destructive bool   `json:"destructive"`
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
		[]string{"kind", "destructive"},
		map[string]jsonSchema{
			"kind": {
				Type: "string",
				Enum: []string{"read-only", "local-mutation", "standard-mutation", "consequential-action"},
			},
			"destructive": {Type: "boolean"},
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

func mutationFlagDefinitions() []flagDefinition {
	return []flagDefinition{
		{Name: "--dry-run", Description: "Preview the validated mutation without dispatching it."},
		{Name: "--force", Description: "Skip the confirmation prompt for a destructive command."},
		{Name: "--no-input", Description: "Never prompt; fail if confirmation is required."},
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

func eventCollectionInputSchema() jsonSchema {
	schema := collectionInputSchema(false)
	schema.Required = append(schema.Required, "when")
	schema.Properties["when"] = jsonSchema{
		Type: "string",
		Enum: []string{"upcoming", "past"},
	}
	return schema
}

func eventGetInputSchema() jsonSchema {
	one := 1
	return objectSchema(
		[]string{"eventId"},
		map[string]jsonSchema{
			"eventId": {Type: "string", MinLength: &one},
		},
	)
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

func eventCollectionSuccessSchema() jsonSchema {
	nullableString := jsonSchema{Type: []string{"string", "null"}}
	nullableEnum := func(values []string) jsonSchema {
		return jsonSchema{OneOf: []jsonSchema{
			{Type: "string", Enum: values},
			{Type: "null"},
		}}
	}
	summary := objectSchema(
		[]string{
			"eventId",
			"title",
			"start",
			"end",
			"timezone",
			"state",
			"userRole",
			"myRsvp",
		},
		map[string]jsonSchema{
			"eventId":  {Type: "string"},
			"title":    nullableString,
			"start":    nullableString,
			"end":      nullableString,
			"timezone": nullableString,
			"state":    nullableEnum([]string{"active", "cancelled"}),
			"userRole": nullableEnum(
				[]string{"host", "cohost", "attendee", "none"},
			),
			"myRsvp": nullableEnum(eventReadRsvpValues()),
		},
	)
	items := jsonSchema{Type: "array", Items: &summary}
	return objectSchema([]string{"items"}, map[string]jsonSchema{"items": items})
}

func eventGetSuccessSchema() jsonSchema {
	nullableString := jsonSchema{Type: []string{"string", "null"}}
	nullableEnum := func(values []string) jsonSchema {
		return jsonSchema{OneOf: []jsonSchema{
			{Type: "string", Enum: values},
			{Type: "null"},
		}}
	}
	return objectSchema(
		[]string{
			"eventId",
			"title",
			"start",
			"end",
			"timezone",
			"state",
			"userRole",
			"myRsvp",
			"description",
			"location",
			"address",
			"visibility",
			"guestLimit",
			"poster",
			"links",
		},
		map[string]jsonSchema{
			"eventId":     {Type: "string"},
			"title":       nullableString,
			"start":       nullableString,
			"end":         nullableString,
			"timezone":    nullableString,
			"state":       nullableEnum([]string{"active", "cancelled"}),
			"userRole":    {Type: "null"},
			"myRsvp":      {Type: "null"},
			"description": {Type: "null"},
			"location":    {Type: "null"},
			"address":     {Type: "null"},
			"visibility":  {Type: "null"},
			"guestLimit":  {Type: "null"},
			"poster":      {Type: "null"},
			"links":       {Type: "null"},
		},
	)
}

func rsvpReadSuccessSchema() jsonSchema {
	return objectSchema(
		[]string{"eventId", "status"},
		map[string]jsonSchema{
			"eventId": {Type: "string"},
			"status": {
				OneOf: []jsonSchema{
					{Type: "string", Enum: eventReadRsvpValues()},
					{Type: "null"},
				},
			},
		},
	)
}

func readOnlySafety() safetyDefinition {
	return safetyDefinition{
		Kind:        "read-only",
		Destructive: false,
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
	Limit            int     `json:"limit"`
	NextCursor       *string `json:"nextCursor"`
	HasMore          bool    `json:"hasMore"`
	Truncated        bool    `json:"truncated,omitempty"`
	TruncationReason string  `json:"truncationReason,omitempty"`
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
	if target, requested := helpTarget(request.Argv); requested {
		return renderHelp(target)
	}

	argv := make([]string, 0, len(request.Argv))
	pretty := slices.Contains(request.Argv, "--pretty")
	execution := mutationExecution{}
	force := false
	noInput := false
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
			noInput = true
			continue
		case "--dry-run":
			if seenGlobalFlags[argument] {
				return repeatedFlagFailure(commandName(request.Argv), argument, pretty)
			}
			seenGlobalFlags[argument] = true
			execution.DryRun = true
			continue
		case "--force":
			if seenGlobalFlags[argument] {
				return repeatedFlagFailure(commandName(request.Argv), argument, pretty)
			}
			seenGlobalFlags[argument] = true
			force = true
			continue
		case "--no-input":
			if seenGlobalFlags[argument] {
				return repeatedFlagFailure(commandName(request.Argv), argument, pretty)
			}
			seenGlobalFlags[argument] = true
			noInput = true
			continue
		}
		argv = append(argv, argument)
	}
	execution.confirmation = func(
		definition commandDefinition,
		eventTitle *string,
		pretty bool,
	) *Result {
		return requireCLIConfirmation(
			definition,
			eventTitle,
			force,
			noInput,
			dependencies,
			pretty,
		)
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
				invocation, inputError := parseCLIProductInvocation(
					request,
					definition,
					argv,
					dependencies,
					execution,
				)
				if inputError != nil {
					return failure(definition.path, exitCodeForType(inputError.Type), *inputError, pretty)
				}
				return invokeProductOperation(ctx, invocation, dependencies, pretty)
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
		definition.kind == contactsListCommand ||
		definition.kind == guestsListCommand ||
		definition.kind == guestsInviteCommand ||
		definition.kind == eventsListCommand ||
		definition.kind == eventsGetCommand ||
		definition.kind == eventsCreateCommand ||
		definition.kind == eventsUpdateCommand ||
		definition.kind == eventsCancelCommand ||
		definition.kind == blastsSendCommand ||
		definition.kind == rsvpGetCommand ||
		definition.kind == rsvpSetCommand ||
		definition.kind == cohostsInviteCommand ||
		definition.kind == cohostsRevokeInviteCommand ||
		definition.kind == cohostsRemoveCommand ||
		definition.kind == cohostsLinkCreateCommand ||
		definition.kind == cohostsLinkRevokeCommand) &&
		len(argv) >= len(definition.invocation) {
		return slices.Equal(argv[:len(definition.invocation)], definition.invocation)
	}
	return slices.Equal(argv, definition.invocation)
}

func helpTarget(argv []string) ([]string, bool) {
	if len(argv) > 0 && argv[0] == "help" {
		return normalizeHelpPath(argv[1:]), true
	}

	target := make([]string, 0, len(argv))
	requested := false
	for _, argument := range argv {
		switch argument {
		case "-h", "--help":
			requested = true
		case "--pretty", "--non-interactive", "--dry-run", "--force", "--no-input":
			// Global presentation and interaction flags do not affect help.
		default:
			target = append(target, argument)
		}
	}
	return normalizeHelpPath(target), requested
}

func normalizeHelpPath(path []string) []string {
	if len(path) == 1 && path[0] == "version" {
		return []string{"--version"}
	}
	if len(path) == 1 && strings.Contains(path[0], ".") {
		return strings.Split(path[0], ".")
	}
	return path
}

func renderHelp(target []string) Result {
	if len(target) == 0 {
		return Result{Stdout: rootHelp()}
	}
	if definition, ok := findDefinitionByInvocation(target); ok {
		return Result{Stdout: leafHelp(definition)}
	}
	if hasCommandPrefix(target) {
		return Result{Stdout: groupHelp(target)}
	}
	return Result{Stdout: fmt.Sprintf("Unknown command path: %s\nRun 'partiful help' for available commands.\n", strings.Join(target, " ")), ExitCode: 2}
}

func findDefinitionByInvocation(invocation []string) (commandDefinition, bool) {
	for _, definition := range commandCatalog {
		if slices.Equal(definition.invocation, invocation) {
			return definition, true
		}
	}
	return commandDefinition{}, false
}

func hasCommandPrefix(prefix []string) bool {
	for _, definition := range commandCatalog {
		if len(definition.invocation) > len(prefix) && slices.Equal(definition.invocation[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

func rootHelp() string {
	groups := map[string]bool{}
	for _, definition := range commandCatalog {
		if len(definition.invocation) > 0 && !strings.HasPrefix(definition.invocation[0], "-") {
			groups[definition.invocation[0]] = true
		}
	}
	names := make([]string, 0, len(groups))
	for group := range groups {
		names = append(names, group)
	}
	slices.Sort(names)
	return "Usage: partiful <command>\n\nCommands:\n" + formatHelpList(names) + "\nRun 'partiful help <command path>' for command-group or leaf-command help.\n"
}

func groupHelp(prefix []string) string {
	commands := make([]string, 0)
	for _, definition := range commandCatalog {
		if len(definition.invocation) > len(prefix) && slices.Equal(definition.invocation[:len(prefix)], prefix) {
			commands = append(commands, strings.Join(definition.invocation[len(prefix):], " "))
		}
	}
	slices.Sort(commands)
	return fmt.Sprintf("Usage: partiful %s <command>\n\nCommands:\n%s\nRun 'partiful help %s <command>' for leaf-command help.\n", strings.Join(prefix, " "), formatHelpList(commands), strings.Join(prefix, " "))
}

func formatHelpList(items []string) string {
	var builder strings.Builder
	for _, item := range items {
		fmt.Fprintf(&builder, "  %s\n", item)
	}
	return builder.String()
}

func leafHelp(definition commandDefinition) string {
	usage := "partiful " + strings.Join(definition.invocation, " ")
	for _, positional := range definition.positionals {
		if positional.Required {
			usage += " <" + positional.Name + ">"
		} else {
			usage += " [" + positional.Name + "]"
		}
	}
	if len(definition.flags) > 0 {
		usage += " [flags]"
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "Usage: %s\n\nPurpose:\n  %s\n", usage, commandPurpose(definition))
	if len(definition.flags) > 0 {
		builder.WriteString("\nFlags:\n")
		for _, flag := range definition.flags {
			name := flag.Name
			if flag.TakesValue {
				name += " <value>"
			}
			fmt.Fprintf(&builder, "  %-24s %s\n", name, flag.Description)
		}
	}
	if required := requiredFields(definition); len(required) > 0 {
		builder.WriteString("\nRequired fields:\n")
		for _, field := range required {
			fmt.Fprintf(&builder, "  %s\n", field)
		}
	}
	fmt.Fprintf(&builder, "\nExamples:\n  %s\n", helpExample(definition))
	builder.WriteString("\nExit behavior:\n  Help is local-only and exits 0. Command execution reports documented JSON success or failure envelopes.\n")
	builder.WriteString("\nMutation safety:\n  ")
	builder.WriteString(helpSafety(definition))
	builder.WriteByte('\n')
	return builder.String()
}

func commandPurpose(definition commandDefinition) string {
	return "Shows the reviewed interface for " + strings.ReplaceAll(definition.path, ".", " ") + "."
}

func requiredFields(definition commandDefinition) []string {
	required := make([]string, 0, len(definition.positionals)+len(definition.inputSchema.Required))
	for _, positional := range definition.positionals {
		if positional.Required {
			required = append(required, positional.Name)
		}
	}
	required = append(required, definition.inputSchema.Required...)
	return required
}

func helpExample(definition commandDefinition) string {
	example := "partiful " + strings.Join(definition.invocation, " ")
	for _, positional := range definition.positionals {
		if positional.Required {
			example += " <" + positional.Name + ">"
		}
	}
	for _, field := range definition.inputSchema.Required {
		if slices.ContainsFunc(definition.flags, func(flag flagDefinition) bool { return flag.Name == "--"+field && flag.TakesValue }) {
			example += " --" + field + " <value>"
		}
	}
	return example
}

func helpSafety(definition commandDefinition) string {
	if definition.safety.Kind == "read-only" || definition.safety.Kind == "local-mutation" {
		return "This command is " + definition.safety.Kind + "; review its schema before execution."
	}
	if definition.safety.Destructive {
		return "Use --dry-run to preview the request. Execution prompts only on a terminal; use --force to skip the prompt or --no-input to fail instead of prompting."
	}
	return "Use --dry-run to preview the request. Without --dry-run, one validated invocation dispatches the mutation once."
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
		if argument != "--pretty" &&
			argument != "--non-interactive" &&
			argument != "--dry-run" &&
			argument != "--force" &&
			argument != "--no-input" {
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
