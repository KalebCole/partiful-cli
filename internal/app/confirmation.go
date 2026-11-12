package app

import (
	"strings"
	"unicode"
)

type Confirmer interface {
	IsTerminal() bool
	Confirm(string) (bool, error)
}

type mutationExecution struct {
	DryRun  bool
	Force   bool
	NoInput bool
}

func requireDestructiveConfirmation(
	definition commandDefinition,
	eventTitle *string,
	execution mutationExecution,
	dependencies Dependencies,
	pretty bool,
) *Result {
	if !definition.safety.Destructive || execution.DryRun || execution.Force {
		return nil
	}
	if execution.NoInput ||
		dependencies.Confirmer == nil ||
		!dependencies.Confirmer.IsTerminal() {
		result := confirmationRequiredFailure(definition.path, pretty)
		return &result
	}
	confirmed, err := dependencies.Confirmer.Confirm(
		destructiveConfirmationPrompt(definition.path, eventTitle),
	)
	if err != nil || !confirmed {
		result := confirmationRequiredFailure(definition.path, pretty)
		return &result
	}
	return nil
}

func destructiveConfirmationPrompt(command string, eventTitle *string) string {
	action := map[string]string{
		"events.cancel":         "Cancel event",
		"cohosts.remove":        "Remove a cohost from",
		"cohosts.revoke-invite": "Revoke a cohost invite for",
		"cohosts.link.revoke":   "Revoke the cohost invite link for",
	}[command]
	return action + " \"" + sanitizedEventTitle(eventTitle) + "\"? [y/N] "
}

func sanitizedEventTitle(eventTitle *string) string {
	if eventTitle == nil {
		return "Untitled event"
	}

	var builder strings.Builder
	hasDisplayText := false
	previousWasUnsafe := false
	for _, r := range *eventTitle {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			if !previousWasUnsafe {
				builder.WriteRune('�')
			}
			previousWasUnsafe = true
			continue
		}
		builder.WriteRune(r)
		if !unicode.IsSpace(r) {
			hasDisplayText = true
		}
		previousWasUnsafe = false
	}
	if !hasDisplayText {
		return "Untitled event"
	}

	title := strings.TrimSpace(builder.String())
	if title == "" {
		return "Untitled event"
	}
	return strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(title)
}

func confirmationRequiredFailure(command string, pretty bool) Result {
	result := failure(command, 7, errorBody{
		Type:      "safety.confirmation_required",
		Code:      "CONFIRMATION_REQUIRED",
		Message:   "Confirmation is required for this destructive command.",
		Retryable: false,
		Details:   map[string]any{},
	}, pretty)
	result.Stderr = "partiful: confirmation required\n"
	return result
}
