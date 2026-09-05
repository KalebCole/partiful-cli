package app

import "testing"

func TestDestructiveConfirmationPromptNamesActionAndSanitizesEventTitle(t *testing.T) {
	title := "Fremont\n\x1b[2J Oktoberfest"
	tests := []struct {
		command string
		want    string
	}{
		{"events.cancel", "Cancel event \"Fremont�[2J Oktoberfest\"? [y/N] "},
		{"cohosts.remove", "Remove a cohost from \"Fremont�[2J Oktoberfest\"? [y/N] "},
		{"cohosts.revoke-invite", "Revoke a cohost invite for \"Fremont�[2J Oktoberfest\"? [y/N] "},
		{"cohosts.link.revoke", "Revoke the cohost invite link for \"Fremont�[2J Oktoberfest\"? [y/N] "},
	}

	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			if got := destructiveConfirmationPrompt(test.command, &title); got != test.want {
				t.Fatalf("prompt = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDestructiveConfirmationPromptUsesPlaceholderForEmptyTitle(t *testing.T) {
	empty := "\n\x1b"
	const want = "Cancel event \"Untitled event\"? [y/N] "
	if got := destructiveConfirmationPrompt("events.cancel", &empty); got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}
