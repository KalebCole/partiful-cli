package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

var (
	ErrNotConfigured  = errors.New("credential storage is not configured")
	ErrUnavailable    = errors.New("credential storage is unavailable")
	ErrInvalid        = errors.New("credential record is invalid")
	ErrHumanRequired  = errors.New("a private terminal is required")
	ErrInputInvalid   = errors.New("authentication input is invalid")
	ErrPersistence    = errors.New("credential persistence failed")
	ErrRequired       = errors.New("authentication is required")
	ErrSessionExpired = errors.New("authentication session is expired")
)

type FileSystem interface {
	ReadFile(string) ([]byte, error)
	Remove(string) error
	WriteFileAtomic(string, []byte) error
}

type PrivateTerminal interface {
	ReadSecret(string) (string, error)
}

type State struct {
	Authenticated bool       `json:"authenticated"`
	TokenState    string     `json:"tokenState"`
	ExpiresAt     *time.Time `json:"expiresAt"`
}

type Session struct {
	AccessToken string
	ExpiresAt   time.Time
}

type credentialRecord struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

func Login(
	ctx context.Context,
	files FileSystem,
	path string,
	terminal PrivateTerminal,
	now time.Time,
	random io.Reader,
	client remote.AuthClient,
) (State, error) {
	if files == nil || path == "" || random == nil {
		return State{}, ErrNotConfigured
	}
	if terminal == nil {
		return State{}, ErrHumanRequired
	}
	phoneNumber, err := terminal.ReadSecret("Phone number: ")
	if errors.Is(err, ErrHumanRequired) {
		return State{}, ErrHumanRequired
	}
	if err != nil {
		return State{}, ErrUnavailable
	}
	phoneNumber = strings.TrimSpace(phoneNumber)
	if phoneNumber == "" {
		return State{}, ErrInputInvalid
	}

	deviceBytes := make([]byte, 16)
	if _, err := io.ReadFull(random, deviceBytes); err != nil {
		return State{}, ErrUnavailable
	}
	deviceID := base64.RawURLEncoding.EncodeToString(deviceBytes)
	sessionID := now.UnixMilli()
	if err := client.SendAuthCode(ctx, remote.SendAuthCodeRequest{
		PhoneNumber:        phoneNumber,
		AmplitudeDeviceID:  deviceID,
		AmplitudeSessionID: sessionID,
	}); err != nil {
		return State{}, err
	}

	code, err := terminal.ReadSecret("Verification code: ")
	if errors.Is(err, ErrHumanRequired) {
		return State{}, ErrHumanRequired
	}
	if err != nil {
		return State{}, ErrUnavailable
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return State{}, ErrInputInvalid
	}
	customToken, err := client.GetLoginToken(ctx, remote.GetLoginTokenRequest{
		PhoneNumber:        phoneNumber,
		AuthCode:           code,
		AmplitudeDeviceID:  deviceID,
		AmplitudeSessionID: sessionID,
	})
	if err != nil {
		return State{}, err
	}
	session, err := client.SignInWithCustomToken(ctx, customToken.Token)
	if err != nil {
		return State{}, err
	}
	expiresAt := now.Add(time.Duration(session.ExpiresIn) * time.Second).UTC()
	err = SaveCredentials(files, path, Credentials{
		AccessToken:  session.IDToken,
		RefreshToken: session.RefreshToken,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		return State{}, ErrPersistence
	}
	return State{
		Authenticated: true,
		TokenState:    "healthy",
		ExpiresAt:     &expiresAt,
	}, nil
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
		Authenticated: false,
		TokenState:    "expired",
		ExpiresAt:     &expiresAt,
	}, nil
}

func StatusWithRefresh(
	ctx context.Context,
	files FileSystem,
	path string,
	now time.Time,
	client remote.AuthClient,
) (State, error) {
	state, err := Status(files, path, now)
	if err != nil ||
		state.TokenState == "healthy" ||
		state.TokenState == "missing" {
		return state, err
	}
	document, err := files.ReadFile(path)
	if err != nil {
		return State{}, ErrUnavailable
	}
	var credentials credentialRecord
	if json.Unmarshal(document, &credentials) != nil {
		return State{}, ErrInvalid
	}
	if credentials.RefreshToken == "" {
		return state, nil
	}
	refreshed, err := client.RefreshToken(ctx, credentials.RefreshToken)
	if err != nil {
		return State{}, err
	}
	expiresAt := now.Add(time.Duration(refreshed.ExpiresIn) * time.Second).UTC()
	if err := SaveCredentials(files, path, Credentials{
		AccessToken:  refreshed.IDToken,
		RefreshToken: refreshed.RefreshToken,
		ExpiresAt:    expiresAt,
	}); err != nil {
		return State{}, ErrPersistence
	}
	return State{
		Authenticated: true,
		TokenState:    "healthy",
		ExpiresAt:     &expiresAt,
	}, nil
}

func AcquireSession(
	ctx context.Context,
	files FileSystem,
	path string,
	now time.Time,
	client remote.AuthClient,
) (Session, error) {
	state, err := StatusWithRefresh(ctx, files, path, now, client)
	if err != nil {
		return Session{}, err
	}
	if state.TokenState == "missing" {
		return Session{}, ErrRequired
	}
	if state.TokenState == "expired" || !state.Authenticated || state.ExpiresAt == nil {
		return Session{}, ErrSessionExpired
	}
	document, err := files.ReadFile(path)
	if err != nil {
		return Session{}, ErrUnavailable
	}
	var credentials credentialRecord
	if json.Unmarshal(document, &credentials) != nil || credentials.AccessToken == "" {
		return Session{}, ErrInvalid
	}
	return Session{
		AccessToken: credentials.AccessToken,
		ExpiresAt:   state.ExpiresAt.UTC(),
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
