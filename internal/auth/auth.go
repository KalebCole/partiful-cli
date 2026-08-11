package auth

import (
	"encoding/json"
	"errors"
	"io/fs"
	"time"
)

var (
	ErrNotConfigured = errors.New("credential storage is not configured")
	ErrUnavailable   = errors.New("credential storage is unavailable")
	ErrInvalid       = errors.New("credential record is invalid")
)

type FileSystem interface {
	ReadFile(string) ([]byte, error)
	Remove(string) error
}

type State struct {
	Authenticated bool       `json:"authenticated"`
	TokenState    string     `json:"tokenState"`
	ExpiresAt     *time.Time `json:"expiresAt"`
}

type credentialRecord struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func Status(files FileSystem, path string, now time.Time) (State, error) {
	if files == nil || path == "" {
		return State{}, ErrNotConfigured
	}
	document, err := files.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return State{
			Authenticated: false,
			TokenState:    "missing",
			ExpiresAt:     nil,
		}, nil
	}
	if err != nil {
		return State{}, ErrUnavailable
	}

	var credentials credentialRecord
	if err := json.Unmarshal(document, &credentials); err != nil ||
		credentials.AccessToken == "" ||
		credentials.ExpiresAt.IsZero() {
		return State{}, ErrInvalid
	}
	if credentials.ExpiresAt.After(now.Add(5 * time.Minute)) {
		expiresAt := credentials.ExpiresAt.UTC()
		return State{
			Authenticated: true,
			TokenState:    "healthy",
			ExpiresAt:     &expiresAt,
		}, nil
	}

	if credentials.ExpiresAt.After(now) {
		expiresAt := credentials.ExpiresAt.UTC()
		return State{
			Authenticated: true,
			TokenState:    "expiring",
			ExpiresAt:     &expiresAt,
		}, nil
	}
	expiresAt := credentials.ExpiresAt.UTC()
	return State{
		Authenticated: true,
		TokenState:    "expired",
		ExpiresAt:     &expiresAt,
	}, nil
}

func Logout(files FileSystem, path string) (State, error) {
	if files == nil || path == "" {
		return State{}, ErrNotConfigured
	}
	err := files.Remove(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return State{}, ErrUnavailable
	}
	return State{
		Authenticated: false,
		TokenState:    "missing",
		ExpiresAt:     nil,
	}, nil
}
