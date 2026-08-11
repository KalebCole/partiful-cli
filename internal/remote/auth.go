package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/KalebCole/partiful-cli/internal/auth"
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

var _ auth.RemoteAuth = AuthClient{}

func (client AuthClient) SendAuthCode(ctx context.Context, req auth.SendAuthCodeRequest) error {
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
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", auth.ErrRemoteProtocolChanged, response.StatusCode)
	}
	if _, err := readBoundedAuthBody(response); err != nil {
		return err
	}
	return nil
}

func (client AuthClient) GetLoginToken(
	ctx context.Context,
	req auth.GetLoginTokenRequest,
) (auth.LoginTokenResponse, error) {
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
		return auth.LoginTokenResponse{}, err
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusForbidden {
			data, err := readAuthJSON(response)
			if err != nil {
				return auth.LoginTokenResponse{}, err
			}
			if !validCallableAuthError(data) {
				return auth.LoginTokenResponse{}, fmt.Errorf("%w: response body", auth.ErrRemoteProtocolChanged)
			}
			return auth.LoginTokenResponse{}, auth.ErrAuthCodeRejected
		}
		return auth.LoginTokenResponse{}, fmt.Errorf("%w: status %d", auth.ErrRemoteProtocolChanged, response.StatusCode)
	}
	data, err := readAuthJSON(response)
	if err != nil {
		return auth.LoginTokenResponse{}, err
	}
	var result loginTokenResult
	if err := json.Unmarshal(data, &result); err != nil || result.Result.Data.Token == "" {
		return auth.LoginTokenResponse{}, fmt.Errorf("%w: response body", auth.ErrRemoteProtocolChanged)
	}
	return auth.LoginTokenResponse{Token: result.Result.Data.Token}, nil
}

func (client AuthClient) SignInWithCustomToken(
	ctx context.Context,
	token string,
) (auth.SignInResponse, error) {
	if client.HTTP == nil {
		return auth.SignInResponse{}, fmt.Errorf("%w: authentication transport", auth.ErrRemoteUnavailable)
	}
	payload, _ := json.Marshal(signInRequest{Token: token, ReturnSecureToken: true})
	endpoint := firebaseIdentityHost + "/v1/accounts:signInWithCustomToken?key=" + firebaseProjectKey
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return auth.SignInResponse{}, fmt.Errorf("%w: request", auth.ErrRemoteUnavailable)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Referer", "https://partiful.com/")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return auth.SignInResponse{}, fmt.Errorf("%w: request failed", auth.ErrRemoteUnavailable)
	}
	if response == nil || response.Body == nil {
		return auth.SignInResponse{}, fmt.Errorf("%w: missing response", auth.ErrRemoteProtocolChanged)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode == http.StatusBadRequest {
		data, err := readAuthJSON(response)
		if err != nil {
			return auth.SignInResponse{}, err
		}
		if !validFirebaseValidationError(data) {
			return auth.SignInResponse{}, fmt.Errorf("%w: response body", auth.ErrRemoteProtocolChanged)
		}
		return auth.SignInResponse{}, auth.ErrRemoteTokenExpired
	}
	if response.StatusCode != http.StatusOK {
		return auth.SignInResponse{}, fmt.Errorf("%w: status %d", auth.ErrRemoteProtocolChanged, response.StatusCode)
	}
	data, err := readAuthJSON(response)
	if err != nil {
		return auth.SignInResponse{}, err
	}
	var result signInResult
	if err := json.Unmarshal(data, &result); err != nil ||
		result.IDToken == "" ||
		result.RefreshToken == "" ||
		result.ExpiresIn == "" ||
		!validOptionalString(result.Kind) {
		return auth.SignInResponse{}, fmt.Errorf("%w: response body", auth.ErrRemoteProtocolChanged)
	}
	expiresIn, err := parseExpiresIn(result.ExpiresIn)
	if err != nil {
		return auth.SignInResponse{}, fmt.Errorf("%w: expiresIn", auth.ErrRemoteProtocolChanged)
	}
	return auth.SignInResponse{
		IDToken:      result.IDToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

func (client AuthClient) RefreshToken(
	ctx context.Context,
	refreshToken string,
) (auth.RefreshResponse, error) {
	if client.HTTP == nil {
		return auth.RefreshResponse{}, fmt.Errorf("%w: authentication transport", auth.ErrRemoteUnavailable)
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
		return auth.RefreshResponse{}, fmt.Errorf("%w: request", auth.ErrRemoteUnavailable)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Referer", "https://partiful.com/")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return auth.RefreshResponse{}, fmt.Errorf("%w: request failed", auth.ErrRemoteUnavailable)
	}
	if response == nil || response.Body == nil {
		return auth.RefreshResponse{}, fmt.Errorf("%w: missing response", auth.ErrRemoteProtocolChanged)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode == http.StatusBadRequest {
		data, err := readAuthJSON(response)
		if err != nil {
			return auth.RefreshResponse{}, err
		}
		if !validFirebaseTokenError(data) {
			return auth.RefreshResponse{}, fmt.Errorf("%w: response body", auth.ErrRemoteProtocolChanged)
		}
		return auth.RefreshResponse{}, auth.ErrRemoteTokenExpired
	}
	if response.StatusCode != http.StatusOK {
		return auth.RefreshResponse{}, fmt.Errorf("%w: status %d", auth.ErrRemoteProtocolChanged, response.StatusCode)
	}
	data, err := readAuthJSON(response)
	if err != nil {
		return auth.RefreshResponse{}, err
	}
	var result refreshResult
	if json.Unmarshal(data, &result) != nil ||
		result.AccessToken == "" ||
		result.IDToken == "" ||
		result.RefreshToken == "" ||
		result.ExpiresIn == "" ||
		result.TokenType == "" ||
		!validOptionalString(result.ProjectID) ||
		!validOptionalString(result.UserID) {
		return auth.RefreshResponse{}, fmt.Errorf("%w: response body", auth.ErrRemoteProtocolChanged)
	}
	expiresIn, err := parseExpiresIn(result.ExpiresIn)
	if err != nil {
		return auth.RefreshResponse{}, fmt.Errorf("%w: expiresIn", auth.ErrRemoteProtocolChanged)
	}
	return auth.RefreshResponse{
		IDToken:      result.IDToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

func parseExpiresIn(value string) (time.Duration, error) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return 0, auth.ErrRemoteProtocolChanged
	}
	if seconds > math.MaxInt64/int64(time.Second) {
		return 0, auth.ErrRemoteProtocolChanged
	}
	return time.Duration(seconds) * time.Second, nil
}

func (client AuthClient) callablePost(
	ctx context.Context,
	endpoint string,
	body any,
) (*http.Response, error) {
	if client.HTTP == nil {
		return nil, fmt.Errorf("%w: authentication transport", auth.ErrRemoteUnavailable)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: encode", auth.ErrRemoteUnavailable)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: request", auth.ErrRemoteUnavailable)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: request failed", auth.ErrRemoteUnavailable)
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("%w: missing response", auth.ErrRemoteProtocolChanged)
	}
	return response, nil
}

func readAuthJSON(response *http.Response) ([]byte, error) {
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, fmt.Errorf("%w: content type", auth.ErrRemoteProtocolChanged)
	}
	data, err := readBoundedAuthBody(response)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) || !json.Valid(data) {
		return nil, fmt.Errorf("%w: response body", auth.ErrRemoteProtocolChanged)
	}
	return data, nil
}

func readBoundedAuthBody(response *http.Response) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumAuthBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read body", auth.ErrRemoteUnavailable)
	}
	if len(data) > maximumAuthBodyBytes {
		return nil, fmt.Errorf("%w: response body", auth.ErrRemoteProtocolChanged)
	}
	return data, nil
}

func validCallableAuthError(data []byte) bool {
	var failure struct {
		Error *struct {
			Message *string         `json:"message"`
			Status  *string         `json:"status"`
			Details json.RawMessage `json:"details"`
		} `json:"error"`
	}
	return json.Unmarshal(data, &failure) == nil &&
		failure.Error != nil &&
		failure.Error.Message != nil &&
		failure.Error.Status != nil &&
		validOptionalCallableDetails(failure.Error.Details)
}

func validFirebaseValidationError(data []byte) bool {
	var failure struct {
		Error *struct {
			Code    *float64        `json:"code"`
			Message *string         `json:"message"`
			Errors  json.RawMessage `json:"errors"`
		} `json:"error"`
	}
	return json.Unmarshal(data, &failure) == nil &&
		failure.Error != nil &&
		failure.Error.Code != nil &&
		failure.Error.Message != nil &&
		validOptionalFirebaseValidationErrors(failure.Error.Errors)
}

func validOptionalFirebaseValidationErrors(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return false
	}
	for _, entry := range entries {
		if bytes.Equal(bytes.TrimSpace(entry), []byte("null")) {
			return false
		}
		var item struct {
			Domain  json.RawMessage `json:"domain"`
			Message json.RawMessage `json:"message"`
			Reason  json.RawMessage `json:"reason"`
		}
		if json.Unmarshal(entry, &item) != nil ||
			!validOptionalString(item.Domain) ||
			!validOptionalString(item.Message) ||
			!validOptionalString(item.Reason) {
			return false
		}
	}
	return true
}

func validFirebaseTokenError(data []byte) bool {
	var failure struct {
		Error *struct {
			Code    *float64        `json:"code"`
			Message *string         `json:"message"`
			Status  json.RawMessage `json:"status"`
		} `json:"error"`
	}
	return json.Unmarshal(data, &failure) == nil &&
		failure.Error != nil &&
		failure.Error.Code != nil &&
		failure.Error.Message != nil &&
		validOptionalString(failure.Error.Status)
}

func validOptionalCallableDetails(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	var details struct {
		AuthErrorCode json.RawMessage `json:"authErrorCode"`
	}
	return json.Unmarshal(raw, &details) == nil &&
		validOptionalString(details.AuthErrorCode)
}

func validOptionalString(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	var value string
	return json.Unmarshal(raw, &value) == nil
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
	IDToken      string          `json:"idToken"`
	RefreshToken string          `json:"refreshToken"`
	ExpiresIn    string          `json:"expiresIn"`
	Kind         json.RawMessage `json:"kind"`
}

type refreshResult struct {
	AccessToken  string          `json:"access_token"`
	IDToken      string          `json:"id_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresIn    string          `json:"expires_in"`
	TokenType    string          `json:"token_type"`
	ProjectID    json.RawMessage `json:"project_id"`
	UserID       json.RawMessage `json:"user_id"`
}
