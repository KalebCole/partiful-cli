package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"
)

const maximumTextBlastResponseBytes = 1 << 20

type TextBlastMessage struct {
	Text            string   `json:"text"`
	To              []string `json:"to"`
	ShowOnEventPage bool     `json:"showOnEventPage"`
}

type CreateTextBlastParams struct {
	EventID string           `json:"eventId"`
	Message TextBlastMessage `json:"message"`
}

type textBlastRequest struct {
	Data textBlastRequestData `json:"data"`
}

type textBlastRequestData struct {
	Params            CreateTextBlastParams `json:"params"`
	AmplitudeDeviceID string                `json:"amplitudeDeviceId"`
	UserID            string                `json:"userId"`
}

func (client Client) CreateTextBlast(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	userID string,
	params CreateTextBlastParams,
) error {
	if client.HTTP == nil {
		return fmt.Errorf("%w: text blast transport", ErrUnavailable)
	}
	payload, _ := json.Marshal(textBlastRequest{Data: textBlastRequestData{
		Params:            params,
		AmplitudeDeviceID: amplitudeDeviceID,
		UserID:            userID,
	}})
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		partifulCallableHost+"/createTextBlast",
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("%w: text blast request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("%w: text blast request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return fmt.Errorf("%w: text blast response", ErrProtocolChanged)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: text blast status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return fmt.Errorf("%w: text blast content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumTextBlastResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%w: text blast response read", ErrUnavailable)
	}
	if len(body) > maximumTextBlastResponseBytes || !utf8.Valid(body) {
		return fmt.Errorf("%w: text blast response body", ErrProtocolChanged)
	}
	root, err := decodeEventObject(body)
	if err != nil {
		return fmt.Errorf("%w: text blast response body", ErrProtocolChanged)
	}
	if raw, ok := root["data"]; ok {
		if err := validateNonNullCallableCompletion(raw); err != nil {
			return fmt.Errorf("%w: text blast completion", ErrProtocolChanged)
		}
		return nil
	}
	if raw, ok := root["result"]; ok {
		if err := validateNonNullCallableCompletion(raw); err != nil {
			return fmt.Errorf("%w: text blast completion", ErrProtocolChanged)
		}
		return nil
	}
	return fmt.Errorf("%w: text blast completion", ErrProtocolChanged)
}

func validateNonNullCallableCompletion(raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("completion is null")
	}
	return nil
}
