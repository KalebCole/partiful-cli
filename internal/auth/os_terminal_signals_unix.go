//go:build !windows

package auth

import (
	"os"
	"os/signal"
	"syscall"
)

func passwordSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func terminateAfterRestore(received os.Signal) error {
	signal.Reset(received)
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	if err := process.Signal(received); err != nil {
		return err
	}
	exitCode := 1
	if unixSignal, ok := received.(syscall.Signal); ok {
		exitCode = 128 + int(unixSignal)
	}
	os.Exit(exitCode)
	return ErrUnavailable
}
