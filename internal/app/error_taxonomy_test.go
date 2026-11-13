package app

import "testing"

func TestFailureTypeExitCodes(t *testing.T) {
	for failureType, want := range map[string]int{
		"usage.invalid": 2, "input.invalid": 2, "match.ambiguous": 2,
		"auth.required": 3, "auth.expired": 3, "auth.human_required": 3,
		"permission.denied": 4, "resource.not_found": 5, "state.conflict": 6,
		"safety.confirmation_required": 7, "remote.unavailable": 8, "remote.rate_limited": 8,
		"contract.protocol_changed": 9, "internal.failure": 10, "unknown": 10,
	} {
		t.Run(failureType, func(t *testing.T) {
			if got := exitCodeForType(failureType); got != want {
				t.Fatalf("exit = %d, want %d", got, want)
			}
		})
	}
}
