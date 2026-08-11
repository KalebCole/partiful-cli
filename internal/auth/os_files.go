package auth

import (
	"os"
	"path/filepath"
)

type OSFileSystem struct{}

func (OSFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (OSFileSystem) Remove(path string) error {
	return os.Remove(path)
}

func (OSFileSystem) WithLock(path string, operation func()) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := secureDirectory(directory); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Close()
	}()
	if err := lock.Chmod(0o600); err != nil {
		return err
	}
	if err := secureFile(lock.Name()); err != nil {
		return err
	}
	release, err := acquireFileLock(lock)
	if err != nil {
		return err
	}
	defer release()
	operation()
	return nil
}

func (OSFileSystem) WriteFileAtomic(path string, document []byte) (resultError error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := secureDirectory(directory); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".credentials-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if resultError != nil {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if err := secureFile(temporaryPath); err != nil {
		return err
	}
	if _, err := temporary.Write(document); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return err
	}
	syncDirectory(directory)
	return nil
}

func DefaultCredentialsPath() (string, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDirectory, "partiful", "credentials.json"), nil
}
