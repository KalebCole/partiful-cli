package auth

import (
	"context"
	"time"
)

type RemoteAuth interface {
	SendAuthCode(context.Context, SendAuthCodeRequest) error
	GetLoginToken(context.Context, GetLoginTokenRequest) (LoginTokenResponse, error)
	SignInWithCustomToken(context.Context, string) (SignInResponse, error)
	RefreshToken(context.Context, string) (RefreshResponse, error)
}

type SendAuthCodeRequest struct {
	PhoneNumber        string
	AmplitudeDeviceID  string
	AmplitudeSessionID int64
}

type GetLoginTokenRequest struct {
	PhoneNumber        string
	AuthCode           string
	AmplitudeDeviceID  string
	AmplitudeSessionID int64
}

type LoginTokenResponse struct {
	Token string
}

type SignInResponse struct {
	IDToken      string
	RefreshToken string
	ExpiresIn    time.Duration
}

type RefreshResponse struct {
	IDToken      string
	RefreshToken string
	ExpiresIn    time.Duration
}
