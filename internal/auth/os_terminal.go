package auth

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type OSTerminal struct {
	Input  *os.File
	Output io.Writer
}

func (terminal OSTerminal) ReadSecret(prompt string) (string, error) {
	if terminal.Input == nil ||
		terminal.Output == nil ||
		!term.IsTerminal(int(terminal.Input.Fd())) {
		return "", ErrHumanRequired
	}
	if _, err := io.WriteString(terminal.Output, prompt); err != nil {
		return "", ErrUnavailable
	}
	value, err := term.ReadPassword(int(terminal.Input.Fd()))
	_, _ = io.WriteString(terminal.Output, "\n")
	if err != nil {
		return "", ErrUnavailable
	}
	return strings.TrimSpace(string(value)), nil
}
