package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"
)

var (
	ErrAuthCodeRejected = errors.New("authentication code rejected")
	ErrAuthExpired      = errors.New("authentication expired")
)

const (
	partifulCallableHost = "https://api.partiful.com"
	firebaseIdentityHost = "https://identitytoolkit.googleapis.com"
	firebaseTokenHost    = "https://securetoken.googleapis.com"
	firebaseProjectKey   = "AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k"
	maximumAuthBodyBytes = 64 << 10
)

type AuthClient struct {
	HTTP HTTPClient
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

type GetLoginTokenResponse struct {
	Token string
}

type SignInWithCustomTokenResponse struct {
	IDToken      string
	RefreshToken string
	ExpiresIn    time.Duration
}

type RefreshTokenResponse struct {
	IDToken      string
	RefreshToken string
	ExpiresIn    time.Duration
}

func (client AuthClient) SendAuthCode(ctx context.Context, req SendAuthCodeRequest) error {
	body := callableBody{
		Data: callableData{
			Params: sendAuthCodeParams{
				DisplayName:             "",
				PhoneNumber:             req.PhoneNumber,
				Silent:                  false,
				ChannelPreference:       "sms",
				CaptchaToken:            nil,
				UseAppleBusinessUpdates: false,
			},
			AmplitudeDeviceID:  req.AmplitudeDeviceID,
			AmplitudeSessionID: req.AmplitudeSessionID,
		},
	}
	response, err := client.callablePost(ctx, partifulCallableHost+"/sendAuthCodeTrusted", body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrProtocolChanged, response.StatusCode)
	}
	return nil
}

func (client AuthClient) GetLoginToken(ctx context.Context, req GetLoginTokenRequest) (GetLoginTokenResponse, error) {
	body := callableBody{
		Data: callableData{
			Params: getLoginTokenParams{
				PhoneNumber: req.PhoneNumber,
				AuthCode:    req.AuthCode,
				AffiliateID: nil,
				UTMs:        map[string]any{},
			},
			AmplitudeDeviceID:  req.AmplitudeDeviceID,
			AmplitudeSessionID: req.AmplitudeSessionID,
		},
	}
	response, err := client.callablePost(ctx, partifulCallableHost+"/getLoginToken", body)
	if err != nil {
		return GetLoginTokenResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusForbidden {
			data, err := readAuthJSON(response)
			if err != nil || !validCallableAuthError(data) {
				return GetLoginTokenResponse{}, fmt.Errorf("%w: response body", ErrProtocolChanged)
			}
			return GetLoginTokenResponse{}, ErrAuthCodeRejected
		}
		return GetLoginTokenResponse{}, fmt.Errorf("%w: status %d", ErrProtocolChanged, response.StatusCode)
	}
	data, err := readAuthJSON(response)
	if err != nil {
		return GetLoginTokenResponse{}, err
	}
	var result loginTokenResult
	if err := json.Unmarshal(data, &result); err != nil || result.Result.Data.Token == "" {
		return GetLoginTokenResponse{}, fmt.Errorf("%w: response body", ErrProtocolChanged)
	}
	return GetLoginTokenResponse{Token: result.Result.Data.Token}, nil
}

func (client AuthClient) SignInWithCustomToken(ctx context.Context, token string) (SignInWithCustomTokenResponse, error) {
	if client.HTTP == nil {
		return SignInWithCustomTokenResponse{}, fmt.Errorf("%w: authentication transport", ErrUnavailable)
	}
	payload, _ := json.Marshal(signInRequest{Token: token, ReturnSecureToken: true})
	url := firebaseIdentityHost + "/v1/accounts:signInWithCustomToken?key=" + firebaseProjectKey
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return SignInWithCustomTokenResponse{}, fmt.Errorf("%w: request", ErrUnavailable)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Referer", "https://partiful.com/")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return SignInWithCustomTokenResponse{}, fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return SignInWithCustomTokenResponse{}, fmt.Errorf("%w: missing response", ErrProtocolChanged)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusBadRequest {
		data, err := readAuthJSON(response)
		if err != nil || !validFirebaseError(data) {
			return SignInWithCustomTokenResponse{}, fmt.Errorf("%w: response body", ErrProtocolChanged)
		}
		return SignInWithCustomTokenResponse{}, ErrAuthExpired
	}
	if response.StatusCode != http.StatusOK {
		return SignInWithCustomTokenResponse{}, fmt.Errorf("%w: status %d", ErrProtocolChanged, response.StatusCode)
	}
	data, err := readAuthJSON(response)
	if err != nil {
		return SignInWithCustomTokenResponse{}, err
	}
	var result signInResult
	if err := json.Unmarshal(data, &result); err != nil ||
		result.IDToken == "" || result.RefreshToken == "" || result.ExpiresIn == "" {
		return SignInWithCustomTokenResponse{}, fmt.Errorf("%w: response body", ErrProtocolChanged)
	}
	expiresIn, err := parseExpiresIn(result.ExpiresIn)
	if err != nil {
		return SignInWithCustomTokenResponse{}, fmt.Errorf("%w: expiresIn", ErrProtocolChanged)
	}
	return SignInWithCustomTokenResponse{
		IDToken:      result.IDToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

func (client AuthClient) RefreshToken(
	ctx context.Context,
	refreshToken string,
) (RefreshTokenResponse, error) {
	if client.HTTP == nil {
		return RefreshTokenResponse{}, fmt.Errorf("%w: authentication transport", ErrUnavailable)
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}.Encode()
	endpoint := firebaseTokenHost + "/v1/token?key=" + firebaseProjectKey
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewBufferString(form),
	)
	if err != nil {
		return RefreshTokenResponse{}, fmt.Errorf("%w: request", ErrUnavailable)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Referer", "https://partiful.com/")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return RefreshTokenResponse{}, fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return RefreshTokenResponse{}, fmt.Errorf("%w: missing response", ErrProtocolChanged)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusBadRequest {
		data, err := readAuthJSON(response)
		if err != nil || !validFirebaseError(data) {
			return RefreshTokenResponse{}, fmt.Errorf("%w: response body", ErrProtocolChanged)
		}
		return RefreshTokenResponse{}, ErrAuthExpired
	}
	if response.StatusCode != http.StatusOK {
		return RefreshTokenResponse{}, fmt.Errorf("%w: status %d", ErrProtocolChanged, response.StatusCode)
	}
	data, err := readAuthJSON(response)
	if err != nil {
		return RefreshTokenResponse{}, err
	}
	var result refreshResult
	if json.Unmarshal(data, &result) != nil ||
		result.AccessToken == "" ||
		result.IDToken == "" ||
		result.RefreshToken == "" ||
		result.ExpiresIn == "" ||
		result.TokenType == "" {
		return RefreshTokenResponse{}, fmt.Errorf("%w: response body", ErrProtocolChanged)
	}
	expiresIn, err := parseExpiresIn(result.ExpiresIn)
	if err != nil {
		return RefreshTokenResponse{}, fmt.Errorf("%w: expiresIn", ErrProtocolChanged)
	}
	return RefreshTokenResponse{
		IDToken:      result.IDToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

func parseExpiresIn(value string) (time.Duration, error) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return 0, ErrProtocolChanged
	}
	duration, err := time.ParseDuration(strconv.FormatInt(seconds, 10) + "s")
	if err != nil {
		return 0, ErrProtocolChanged
	}
	return duration, nil
}

func (client AuthClient) callablePost(ctx context.Context, url string, body any) (*http.Response, error) {
	if client.HTTP == nil {
		return nil, fmt.Errorf("%w: authentication transport", ErrUnavailable)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: encode", ErrUnavailable)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: request", ErrUnavailable)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("%w: missing response", ErrProtocolChanged)
	}
	return response, nil
}

func readAuthJSON(response *http.Response) ([]byte, error) {
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, fmt.Errorf("%w: content type", ErrProtocolChanged)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumAuthBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read body", ErrUnavailable)
	}
	if len(data) > maximumAuthBodyBytes || !utf8.Valid(data) || !json.Valid(data) {
		return nil, fmt.Errorf("%w: response body", ErrProtocolChanged)
	}
	return data, nil
}

func validCallableAuthError(data []byte) bool {
	var failure struct {
		Error *struct {
			Message *string `json:"message"`
			Status  *string `json:"status"`
		} `json:"error"`
	}
	return json.Unmarshal(data, &failure) == nil &&
		failure.Error != nil &&
		failure.Error.Message != nil &&
		failure.Error.Status != nil
}

func validFirebaseError(data []byte) bool {
	var failure struct {
		Error *struct {
			Code    *float64 `json:"code"`
			Message *string  `json:"message"`
		} `json:"error"`
	}
	return json.Unmarshal(data, &failure) == nil &&
		failure.Error != nil &&
		failure.Error.Code != nil &&
		failure.Error.Message != nil
}

type callableBody struct {
	Data callableData `json:"data"`
}

type callableData struct {
	Params             any    `json:"params"`
	AmplitudeDeviceID  string `json:"amplitudeDeviceId"`
	AmplitudeSessionID int64  `json:"amplitudeSessionId"`
}

type sendAuthCodeParams struct {
	DisplayName             string  `json:"displayName"`
	PhoneNumber             string  `json:"phoneNumber"`
	Silent                  bool    `json:"silent"`
	ChannelPreference       string  `json:"channelPreference"`
	CaptchaToken            *string `json:"captchaToken"`
	UseAppleBusinessUpdates bool    `json:"useAppleBusinessUpdates"`
}

type getLoginTokenParams struct {
	PhoneNumber string         `json:"phoneNumber"`
	AuthCode    string         `json:"authCode"`
	AffiliateID *string        `json:"affiliateId"`
	UTMs        map[string]any `json:"utms"`
}

type loginTokenResult struct {
	Result struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	} `json:"result"`
}

type signInRequest struct {
	Token             string `json:"token"`
	ReturnSecureToken bool   `json:"returnSecureToken"`
}

type signInResult struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    string `json:"expiresIn"`
}

type refreshResult struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    string `json:"expires_in"`
	TokenType    string `json:"token_type"`
}
