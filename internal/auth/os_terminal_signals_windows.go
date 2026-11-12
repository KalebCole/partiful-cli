//go:build windows

package auth

import (
	"os"
	"syscall"
)

func passwordSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func terminateAfterRestore(os.Signal) error {
	os.Exit(130)
	return nil
}
