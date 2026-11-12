package auth

import (
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/term"
)

type OSTerminal struct {
	Input  *os.File
	Output io.Writer
}

var errTerminalInterrupted = errors.New("terminal input interrupted")

const maximumSecretInputBytes = 4 << 10

func (terminal OSTerminal) ReadSecret(prompt string) (string, error) {
	if terminal.Input == nil ||
		terminal.Output == nil ||
		!term.IsTerminal(int(terminal.Input.Fd())) {
		return "", ErrHumanRequired
	}
	if _, err := io.WriteString(terminal.Output, prompt); err != nil {
		return "", ErrUnavailable
	}
	value, err := readSecretWithSignalRestore(terminal.Input)
	_, _ = io.WriteString(terminal.Output, "\n")
	if errors.Is(err, ErrInputInvalid) {
		return "", ErrInputInvalid
	}
	if err != nil {
		return "", ErrUnavailable
	}
	return strings.TrimSpace(string(value)), nil
}

func readSecretWithSignalRestore(input *os.File) ([]byte, error) {
	fileDescriptor := int(input.Fd())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, passwordSignals()...)
	defer signal.Stop(signals)

	state, err := term.MakeRaw(fileDescriptor)
	if err != nil {
		signal.Stop(signals)
		select {
		case received := <-signals:
			return nil, terminateAfterRestore(received)
		default:
		}
		return nil, err
	}
	var restoreOnce sync.Once
	restore := func() {
		restoreOnce.Do(func() {
			_ = term.Restore(fileDescriptor, state)
		})
	}
	defer restore()

	done := make(chan struct{})
	go func() {
		select {
		case received := <-signals:
			restore()
			if err := terminateAfterRestore(received); err != nil {
				os.Exit(1)
			}
		case <-done:
		}
	}()
	value, readErr := readRawSecret(input)
	restore()
	signal.Stop(signals)
	select {
	case received := <-signals:
		return nil, terminateAfterRestore(received)
	default:
	}
	close(done)
	if errors.Is(readErr, errTerminalInterrupted) {
		return nil, terminateAfterRestore(os.Interrupt)
	}
	return value, readErr
}

func readRawSecret(input io.Reader) ([]byte, error) {
	value := make([]byte, 0, 32)
	oneByte := []byte{0}
	oversized := false
	for {
		count, err := input.Read(oneByte)
		if count > 0 {
			switch oneByte[0] {
			case '\r', '\n':
				if oversized {
					return nil, ErrInputInvalid
				}
				return value, nil
			case 3:
				return nil, errTerminalInterrupted
			case 8, 127:
				if !oversized && len(value) > 0 {
					_, size := utf8.DecodeLastRune(value)
					value = value[:len(value)-size]
				}
			case 21:
				value = value[:0]
				oversized = false
			default:
				if oversized {
					break
				}
				value = append(value, oneByte[0])
				if len(value) >= maximumSecretInputBytes {
					oversized = true
				}
			}
		}
		if err != nil {
			if oversized {
				return nil, ErrInputInvalid
			}
			return nil, err
		}
	}
}
