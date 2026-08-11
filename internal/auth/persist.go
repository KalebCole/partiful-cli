package auth

import (
	"encoding/json"
	"time"
)

type Credentials struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	UserID       string    `json:"userId,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

func SaveCredentials(files FileSystem, path string, credentials Credentials) error {
	if files == nil || path == "" {
		return ErrNotConfigured
	}
	document, err := json.Marshal(credentials)
	if err != nil {
		return ErrUnavailable
	}
	return files.WriteFileAtomic(path, document)
}
