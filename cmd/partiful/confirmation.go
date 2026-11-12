package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

var errConfirmationUnavailable = errors.New("confirmation input is unavailable")

type terminalConfirmer struct {
	input  *os.File
	output io.Writer
}

func (confirmer terminalConfirmer) IsTerminal() bool {
	return confirmer.input != nil &&
		confirmer.output != nil &&
		term.IsTerminal(int(confirmer.input.Fd()))
}

func (confirmer terminalConfirmer) Confirm(prompt string) (bool, error) {
	if !confirmer.IsTerminal() {
		return false, errConfirmationUnavailable
	}
	if _, err := io.WriteString(confirmer.output, prompt); err != nil {
		return false, err
	}
	line, prefix, err := bufio.NewReaderSize(confirmer.input, 64).ReadLine()
	if err != nil || prefix {
		if err != nil {
			return false, err
		}
		return false, errConfirmationUnavailable
	}
	answer := strings.ToLower(strings.TrimSpace(string(line)))
	return answer == "y" || answer == "yes", nil
}
