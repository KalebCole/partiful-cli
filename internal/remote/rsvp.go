package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maximumRSVPResponseBytes = 1 << 20

type CurrentGuest struct {
	Present bool
	ID      string
	Status  string
	Count   *float64
}

type currentGuestRequest struct {
	Data currentGuestRequestData `json:"data"`
}

type currentGuestRequestData struct {
	Params            currentGuestParams `json:"params"`
	AmplitudeDeviceID string             `json:"amplitudeDeviceId"`
}

type currentGuestParams struct {
	EventID string `json:"eventId"`
}

type NamedPlusOne struct {
	Name string `json:"name"`
}

type QuestionnaireResponse struct {
	QuestionnaireVersion int               `json:"questionnaireVersion"`
	Answers              map[string]string `json:"answers"`
}

type RSVPDraft struct {
	Name                  string                 `json:"name"`
	Count                 int                    `json:"count"`
	PlusOnes              []NamedPlusOne         `json:"plusOnes"`
	Message               *string                `json:"message,omitempty"`
	Status                string                 `json:"status"`
	GuestID               *string                `json:"guestId,omitempty"`
	Timezone              string                 `json:"timezone"`
	QuestionnaireResponse *QuestionnaireResponse `json:"questionnaireResponse,omitempty"`
	ShouldFollowOrgs      bool                   `json:"shouldFollowOrgs"`
}

type AddGuestParams struct {
	EventID string    `json:"eventId"`
	RSVP    RSVPDraft `json:"rsvp"`
}

type MarkEventInterestParams struct {
	EventID    string `json:"eventId"`
	Interested bool   `json:"interested"`
}

type rsvpMutationRequest[T any] struct {
	Data rsvpMutationRequestData[T] `json:"data"`
}

type rsvpMutationRequestData[T any] struct {
	Params            T      `json:"params"`
	AmplitudeDeviceID string `json:"amplitudeDeviceId"`
}

func (client Client) GetCurrentGuest(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	eventID string,
) (CurrentGuest, error) {
	if client.HTTP == nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest transport", ErrUnavailable)
	}
	payload, _ := json.Marshal(currentGuestRequest{Data: currentGuestRequestData{
		Params:            currentGuestParams{EventID: eventID},
		AmplitudeDeviceID: amplitudeDeviceID,
	}})
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		partifulCallableHost+"/getCurrentGuest",
		bytes.NewReader(payload),
	)
	if err != nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest response", ErrProtocolChanged)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return CurrentGuest{}, fmt.Errorf("%w: current guest status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return CurrentGuest{}, fmt.Errorf("%w: current guest content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumRSVPResponseBytes+1))
	if err != nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest response read", ErrUnavailable)
	}
	if len(body) > maximumRSVPResponseBytes || !utf8.Valid(body) {
		return CurrentGuest{}, fmt.Errorf("%w: current guest response body", ErrProtocolChanged)
	}
	root, err := decodeEventObject(body)
	if err != nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest response body", ErrProtocolChanged)
	}
	result, err := eventObjectField(root, "result")
	if err != nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest result", ErrProtocolChanged)
	}
	data, err := eventObjectField(result, "data")
	if err != nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest data", ErrProtocolChanged)
	}
	rawGuest, ok := data["currentGuest"]
	if !ok {
		return CurrentGuest{}, fmt.Errorf("%w: current guest value", ErrProtocolChanged)
	}
	if bytes.Equal(bytes.TrimSpace(rawGuest), []byte("null")) {
		return CurrentGuest{}, nil
	}
	guest, err := decodeEventObject(rawGuest)
	if err != nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest value", ErrProtocolChanged)
	}
	id, idPresent, idErr := eventStringField(guest, "id", false)
	status, statusPresent, statusErr := eventStringField(guest, "status", false)
	if idErr != nil ||
		!idPresent ||
		strings.TrimSpace(*id) == "" ||
		statusErr != nil ||
		!statusPresent ||
		!validGuestStatus(*status) {
		return CurrentGuest{}, fmt.Errorf("%w: current guest identity", ErrProtocolChanged)
	}
	if _, _, err := eventStringField(guest, "name", false); err != nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest name", ErrProtocolChanged)
	}
	count, err := optionalNumber(guest, "count")
	if err != nil {
		return CurrentGuest{}, fmt.Errorf("%w: current guest count", ErrProtocolChanged)
	}
	return CurrentGuest{
		Present: true,
		ID:      *id,
		Status:  *status,
		Count:   count,
	}, nil
}

func (client Client) AddGuest(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	params AddGuestParams,
) error {
	data, err := callRSVPMutation(
		client,
		ctx,
		accessToken,
		amplitudeDeviceID,
		"addGuest",
		params,
	)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("%w: add guest data", ErrProtocolChanged)
	}
	return nil
}

func (client Client) MarkEventInterest(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	params MarkEventInterestParams,
) error {
	rawData, err := callRSVPMutation(
		client,
		ctx,
		accessToken,
		amplitudeDeviceID,
		"markEventInterest",
		params,
	)
	if err != nil {
		return err
	}
	data, err := decodeEventObject(rawData)
	if err != nil {
		return fmt.Errorf("%w: event interest data", ErrProtocolChanged)
	}
	success, successPresent := data["success"]
	interested, interestedPresent := data["interested"]
	var returnedInterested bool
	if !successPresent ||
		!jsonTruthy(success) ||
		!interestedPresent ||
		json.Unmarshal(interested, &returnedInterested) != nil ||
		returnedInterested != params.Interested {
		return fmt.Errorf("%w: event interest completion", ErrProtocolChanged)
	}
	return nil
}

func callRSVPMutation[T any](
	client Client,
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	operation string,
	params T,
) (json.RawMessage, error) {
	if client.HTTP == nil {
		return nil, fmt.Errorf("%w: RSVP transport", ErrUnavailable)
	}
	payload, _ := json.Marshal(rsvpMutationRequest[T]{
		Data: rsvpMutationRequestData[T]{
			Params:            params,
			AmplitudeDeviceID: amplitudeDeviceID,
		},
	})
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		partifulCallableHost+"/"+operation,
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: RSVP mutation request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: RSVP mutation request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("%w: RSVP mutation response", ErrProtocolChanged)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: RSVP mutation status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("%w: RSVP mutation content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumRSVPResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: RSVP mutation response read", ErrUnavailable)
	}
	if len(body) > maximumRSVPResponseBytes || !utf8.Valid(body) {
		return nil, fmt.Errorf("%w: RSVP mutation response body", ErrProtocolChanged)
	}
	root, err := decodeEventObject(body)
	if err != nil {
		return nil, fmt.Errorf("%w: RSVP mutation response body", ErrProtocolChanged)
	}
	result, err := eventObjectField(root, "result")
	if err != nil {
		return nil, fmt.Errorf("%w: RSVP mutation result", ErrProtocolChanged)
	}
	data, ok := result["data"]
	if !ok {
		return nil, fmt.Errorf("%w: RSVP mutation data", ErrProtocolChanged)
	}
	return bytes.Clone(data), nil
}

func jsonTruthy(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 ||
		bytes.Equal(trimmed, []byte("null")) ||
		bytes.Equal(trimmed, []byte("false")) ||
		bytes.Equal(trimmed, []byte(`""`)) {
		return false
	}
	if trimmed[0] == '"' {
		var value string
		return json.Unmarshal(trimmed, &value) == nil && value != ""
	}
	if trimmed[0] == '-' || trimmed[0] >= '0' && trimmed[0] <= '9' {
		value, _ := strconv.ParseFloat(string(trimmed), 64)
		return value != 0
	}
	return trimmed[0] == '{' || trimmed[0] == '[' || bytes.Equal(trimmed, []byte("true"))
}

func optionalNumber(
	object map[string]json.RawMessage,
	name string,
) (*float64, error) {
	raw, ok := object[name]
	if !ok {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("number is invalid")
	}
	var value float64
	if json.Unmarshal(raw, &value) != nil {
		return nil, fmt.Errorf("number is invalid")
	}
	return &value, nil
}
