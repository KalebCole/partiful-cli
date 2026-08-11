package auth

import (
	"encoding/json"
	"time"
)

type Credentials struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

func SaveCredentials(files FileSystem, path string, credentials Credentials) error {
	if files == nil || path == "" {
		return ErrNotConfigured
	}
	var saveError error
	if err := files.WithLock(path, func() {
		saveError = saveCredentialsUnlocked(files, path, credentials)
	}); err != nil {
		return ErrUnavailable
	}
	return saveError
}

func saveCredentialsUnlocked(files FileSystem, path string, credentials Credentials) error {
	document, err := json.Marshal(credentials)
	if err != nil {
		return ErrUnavailable
	}
	return files.WriteFileAtomic(path, document)
}
