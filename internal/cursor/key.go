package cursor

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const KeySize = 32

type KeyProvider interface {
	Key() ([]byte, error)
}

type FileKeyProvider struct {
	Path   string
	Random io.Reader
}

func (provider FileKeyProvider) Key() ([]byte, error) {
	if provider.Path == "" {
		return nil, errors.New("cursor key path is unavailable")
	}
	key, err := os.ReadFile(provider.Path)
	if err == nil {
		return validateKey(key)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(provider.Path), 0o700); err != nil {
		return nil, err
	}
	random := provider.Random
	if random == nil {
		random = rand.Reader
	}
	key = make([]byte, KeySize)
	if _, err := io.ReadFull(random, key); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(provider.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		existing, readErr := os.ReadFile(provider.Path)
		if readErr != nil {
			return nil, readErr
		}
		return validateKey(existing)
	}
	if err != nil {
		return nil, err
	}
	removeIncomplete := true
	defer func() {
		_ = file.Close()
		if removeIncomplete {
			_ = os.Remove(provider.Path)
		}
	}()
	if _, err := file.Write(key); err != nil {
		return nil, err
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	removeIncomplete = false
	return append([]byte(nil), key...), nil
}

func DefaultKeyPath() (string, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDirectory, "partiful", "cursor.key"), nil
}

func validateKey(key []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, errors.New("cursor key is invalid")
	}
	return append([]byte(nil), key...), nil
}
