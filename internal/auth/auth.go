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
)

var (
	ErrNotConfigured         = errors.New("credential storage is not configured")
	ErrUnavailable           = errors.New("credential storage is unavailable")
	ErrInvalid               = errors.New("credential record is invalid")
	ErrHumanRequired         = errors.New("a private terminal is required")
	ErrInputInvalid          = errors.New("authentication input is invalid")
	ErrPersistence           = errors.New("credential persistence failed")
	ErrAuthCodeRejected      = errors.New("authentication code rejected")
	ErrRemoteTokenExpired    = errors.New("remote authentication token expired")
	ErrRemoteProtocolChanged = errors.New("remote authentication protocol changed")
	ErrRemoteUnavailable     = errors.New("remote authentication service unavailable")
)

type FileSystem interface {
	ReadFile(string) ([]byte, error)
	Remove(string) error
	WriteFileAtomic(string, []byte) error
	WithLock(string, func()) error
}

type PrivateTerminal interface {
	ReadSecret(string) (string, error)
}

type State struct {
	Authenticated bool       `json:"authenticated"`
	TokenState    string     `json:"tokenState"`
	ExpiresAt     *time.Time `json:"expiresAt"`
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
	now func() time.Time,
	random io.Reader,
	client RemoteAuth,
) (State, error) {
	if files == nil || path == "" || now == nil || random == nil {
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
	sessionID := now().UnixMilli()
	if err := client.SendAuthCode(ctx, SendAuthCodeRequest{
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
	customToken, err := client.GetLoginToken(ctx, GetLoginTokenRequest{
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
	expiresAt := now().Add(session.ExpiresIn).UTC()
	err = saveCredentials(files, path, credentialRecord{
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
	credentials, err := loadCredentials(files, path)
	if errors.Is(err, fs.ErrNotExist) {
		return missingState(), nil
	}
	if err != nil {
		return State{}, err
	}
	return stateFromCredentials(credentials, now), nil
}

func StatusWithRefresh(
	ctx context.Context,
	files FileSystem,
	path string,
	now func() time.Time,
	client RemoteAuth,
) (State, error) {
	state, err := refreshCredentials(ctx, files, path, now, client)
	return state, err
}

func Logout(files FileSystem, path string) (State, error) {
	if files == nil || path == "" {
		return State{}, ErrNotConfigured
	}
	var removeError error
	if err := files.WithLock(path, func() {
		removeError = files.Remove(path)
	}); err != nil {
		return State{}, ErrUnavailable
	}
	if removeError != nil && !errors.Is(removeError, fs.ErrNotExist) {
		return State{}, ErrUnavailable
	}
	return missingState(), nil
}

func refreshCredentials(
	ctx context.Context,
	files FileSystem,
	path string,
	now func() time.Time,
	client RemoteAuth,
) (State, error) {
	if files == nil || path == "" || now == nil {
		return State{}, ErrNotConfigured
	}
	var (
		credentials  credentialRecord
		state        State
		operationErr error
	)
	if err := files.WithLock(path, func() {
		credentials, operationErr = loadCredentials(files, path)
		if errors.Is(operationErr, fs.ErrNotExist) {
			operationErr = nil
			state = missingState()
			return
		}
		if operationErr != nil {
			return
		}
		state = stateFromCredentials(credentials, now())
		if state.TokenState == "healthy" || credentials.RefreshToken == "" {
			return
		}
		var refreshed RefreshResponse
		refreshed, operationErr = client.RefreshToken(ctx, credentials.RefreshToken)
		if operationErr != nil {
			return
		}
		completedAt := now()
		expiresAt := completedAt.Add(refreshed.ExpiresIn).UTC()
		credentials = credentialRecord{
			AccessToken:  refreshed.IDToken,
			RefreshToken: refreshed.RefreshToken,
			ExpiresAt:    expiresAt,
		}
		operationErr = saveCredentialsUnlocked(files, path, credentials)
		if operationErr != nil {
			operationErr = ErrPersistence
			return
		}
		state = stateFromCredentials(credentials, completedAt)
	}); err != nil {
		return State{}, ErrUnavailable
	}
	if operationErr != nil {
		return State{}, operationErr
	}
	return state, nil
}

func loadCredentials(files FileSystem, path string) (credentialRecord, error) {
	document, err := files.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return credentialRecord{}, fs.ErrNotExist
		}
		return credentialRecord{}, ErrUnavailable
	}
	var credentials credentialRecord
	if json.Unmarshal(document, &credentials) != nil ||
		credentials.AccessToken == "" ||
		credentials.ExpiresAt.IsZero() {
		return credentialRecord{}, ErrInvalid
	}
	return credentials, nil
}

func stateFromCredentials(credentials credentialRecord, now time.Time) State {
	expiresAt := credentials.ExpiresAt.UTC()
	if credentials.ExpiresAt.After(now.Add(5 * time.Minute)) {
		return State{Authenticated: true, TokenState: "healthy", ExpiresAt: &expiresAt}
	}
	if credentials.ExpiresAt.After(now) {
		return State{Authenticated: true, TokenState: "expiring", ExpiresAt: &expiresAt}
	}
	return State{Authenticated: false, TokenState: "expired", ExpiresAt: &expiresAt}
}

func missingState() State {
	return State{Authenticated: false, TokenState: "missing", ExpiresAt: nil}
}
