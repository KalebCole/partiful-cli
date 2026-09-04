package app

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
		"Proceed with this destructive action? [y/N] ",
	)
	if err != nil || !confirmed {
		result := confirmationRequiredFailure(definition.path, pretty)
		return &result
	}
	return nil
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
