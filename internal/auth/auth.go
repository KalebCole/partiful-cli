package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

type Session struct {
	AccessToken        string
	ExpiresAt          time.Time
	AccountFingerprint string
}

type credentialRecord struct {
	AccessToken        string    `json:"accessToken"`
	RefreshToken       string    `json:"refreshToken"`
	AccountFingerprint string    `json:"accountFingerprint,omitempty"`
	ExpiresAt          time.Time `json:"expiresAt"`
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
	expiresAt := now.Add(session.ExpiresIn).UTC()
	err = SaveCredentials(files, path, Credentials{
		AccessToken:        session.IDToken,
		RefreshToken:       session.RefreshToken,
		AccountFingerprint: accountFingerprint(session.IDToken),
		ExpiresAt:          expiresAt,
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
	now time.Time,
	client remote.AuthClient,
) (State, error) {
	_, state, err := refreshCredentials(ctx, files, path, now, client)
	return state, err
}

func AcquireSession(
	ctx context.Context,
	files FileSystem,
	path string,
	now time.Time,
	client remote.AuthClient,
) (Session, error) {
	credentials, state, err := refreshCredentials(ctx, files, path, now, client)
	if err != nil {
		return Session{}, err
	}
	if state.TokenState == "missing" {
		return Session{}, ErrRequired
	}
	if state.TokenState == "expired" || !state.Authenticated || state.ExpiresAt == nil {
		return Session{}, ErrSessionExpired
	}
	return Session{
		AccessToken:        credentials.AccessToken,
		ExpiresAt:          state.ExpiresAt.UTC(),
		AccountFingerprint: credentials.AccountFingerprint,
	}, nil
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
	now time.Time,
	client remote.AuthClient,
) (credentialRecord, State, error) {
	if files == nil || path == "" {
		return credentialRecord{}, State{}, ErrNotConfigured
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
		state = stateFromCredentials(credentials, now)
		if state.TokenState == "healthy" || credentials.RefreshToken == "" {
			return
		}
		var refreshed remote.RefreshTokenResponse
		refreshed, operationErr = client.RefreshToken(ctx, credentials.RefreshToken)
		if operationErr != nil {
			return
		}
		expiresAt := now.Add(refreshed.ExpiresIn).UTC()
		credentials = credentialRecord{
			AccessToken:  refreshed.IDToken,
			RefreshToken: refreshed.RefreshToken,
			AccountFingerprint: refreshedFingerprint(
				credentials.AccountFingerprint,
				refreshed.IDToken,
			),
			ExpiresAt: expiresAt,
		}
		operationErr = saveCredentialsUnlocked(files, path, Credentials{
			AccessToken:        credentials.AccessToken,
			RefreshToken:       credentials.RefreshToken,
			AccountFingerprint: credentials.AccountFingerprint,
			ExpiresAt:          credentials.ExpiresAt,
		})
		if operationErr != nil {
			operationErr = ErrPersistence
			return
		}
		state = stateFromCredentials(credentials, now)
	}); err != nil {
		return credentialRecord{}, State{}, ErrUnavailable
	}
	if operationErr != nil {
		return credentialRecord{}, State{}, operationErr
	}
	return credentials, state, nil
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
	if credentials.AccountFingerprint == "" {
		credentials.AccountFingerprint = accountFingerprint(credentials.AccessToken)
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

func refreshedFingerprint(current, idToken string) string {
	if fingerprint, ok := accountIdentityFingerprint(idToken); ok {
		return fingerprint
	}
	if current != "" {
		return current
	}
	return opaqueTokenFingerprint(idToken)
}

func accountFingerprint(idToken string) string {
	if fingerprint, ok := accountIdentityFingerprint(idToken); ok {
		return fingerprint
	}
	return opaqueTokenFingerprint(idToken)
}

func accountIdentityFingerprint(idToken string) (string, bool) {
	segments := strings.Split(idToken, ".")
	if len(segments) != 3 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return "", false
	}
	var claims struct {
		Subject string `json:"sub"`
		UserID  string `json:"user_id"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return "", false
	}
	identity := claims.UserID
	if identity == "" {
		identity = claims.Subject
	}
	if identity == "" {
		return "", false
	}
	return fingerprint("partiful-account:v1:", identity), true
}

func opaqueTokenFingerprint(idToken string) string {
	return fingerprint("partiful-session:v1:", idToken)
}

func fingerprint(domain, value string) string {
	hash := sha256.Sum256([]byte(domain + value))
	return hex.EncodeToString(hash[:])
}
