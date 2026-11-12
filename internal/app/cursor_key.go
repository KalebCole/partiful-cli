package app

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const CursorKeySize = 32

type CursorKeyProvider interface {
	Key() ([]byte, error)
}

type FileCursorKeyProvider struct {
	Path   string
	Random io.Reader
}

func (provider FileCursorKeyProvider) Key() ([]byte, error) {
	if provider.Path == "" {
		return nil, errors.New("cursor key path is unavailable")
	}
	key, err := os.ReadFile(provider.Path)
	if err == nil {
		return validateCursorKey(key)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	directory := filepath.Dir(provider.Path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	random := provider.Random
	if random == nil {
		random = rand.Reader
	}
	key = make([]byte, CursorKeySize)
	if _, err := io.ReadFull(random, key); err != nil {
		return nil, err
	}

	temporary, err := os.CreateTemp(directory, ".cursor-key-*")
	if err != nil {
		return nil, err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return nil, err
	}
	if _, err := temporary.Write(key); err != nil {
		return nil, err
	}
	if err := temporary.Sync(); err != nil {
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	closed = true

	if err := os.Link(temporaryPath, provider.Path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		existing, readErr := os.ReadFile(provider.Path)
		if readErr != nil {
			return nil, readErr
		}
		return validateCursorKey(existing)
	}
	return append([]byte(nil), key...), nil
}

func DefaultCursorKeyPath() (string, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDirectory, "partiful", "cursor.key"), nil
}

func validateCursorKey(key []byte) ([]byte, error) {
	if len(key) != CursorKeySize {
		return nil, errors.New("cursor key is invalid")
	}
	return append([]byte(nil), key...), nil
}
