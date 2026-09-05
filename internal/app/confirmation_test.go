package app

import "testing"

func TestDestructiveConfirmationPromptNamesAction(t *testing.T) {
	title := "Current title"
	tests := []struct {
		command string
		want    string
	}{
		{"events.cancel", "Cancel event \"Current title\"? [y/N] "},
		{"cohosts.remove", "Remove a cohost from \"Current title\"? [y/N] "},
		{"cohosts.revoke-invite", "Revoke a cohost invite for \"Current title\"? [y/N] "},
		{"cohosts.link.revoke", "Revoke the cohost invite link for \"Current title\"? [y/N] "},
	}

	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			if got := destructiveConfirmationPrompt(test.command, &title); got != test.want {
				t.Fatalf("prompt = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDestructiveConfirmationPromptSanitizesUntrustedEventTitles(t *testing.T) {
	tests := []struct {
		name  string
		title *string
		want  string
	}{
		{"controls", pointerTo("Fremont\n\x1b[2J Oktoberfest"), "Fremont�[2J Oktoberfest"},
		{"bidi override", pointerTo("before\u202eafter"), "before�after"},
		{"zero width format", pointerTo("before\u200bafter"), "before�after"},
		{"line separator", pointerTo("before\u2028after"), "before�after"},
		{"paragraph separator", pointerTo("before\u2029after"), "before�after"},
		{"quotes and backslashes", pointerTo(`The "best" \ party`), `The \"best\" \\ party`},
		{"unsafe only", pointerTo(" \n\x1b\u202e\u200b\u2028\u2029 "), "Untitled event"},
		{"empty", pointerTo(""), "Untitled event"},
		{"missing", nil, "Untitled event"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := "Cancel event \"" + test.want + "\"? [y/N] "
			if got := destructiveConfirmationPrompt("events.cancel", test.title); got != want {
				t.Fatalf("prompt = %q, want %q", got, want)
			}
		})
	}
}

func pointerTo(value string) *string {
	return &value
}
