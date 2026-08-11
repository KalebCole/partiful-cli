//go:build !windows

package auth

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func acquireFileLock(file *os.File) (func(), error) {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
	}, nil
}

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

func syncDirectory(directory string) {
	handle, err := os.Open(directory)
	if err != nil {
		return
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return
	}
}
