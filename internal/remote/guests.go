package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	guestPageSize           = 500
	maximumGuestDataPages   = 20
	maximumGuestCatalogSize = guestPageSize * maximumGuestDataPages
	maximumGuestPageBytes   = 8 << 20
)

type Guest struct {
	ID            string
	UserID        *string
	Name          string
	Status        string
	Count         int
	AnchorGuestID *string
}

type GuestCatalog struct {
	Guests        []Guest
	PayloadSHA256 [sha256.Size]byte
}

type getGuestsRequest struct {
	Data getGuestsRequestData `json:"data"`
}

type getGuestsRequestData struct {
	Params            getGuestsParams `json:"params"`
	AmplitudeDeviceID string          `json:"amplitudeDeviceId"`
	Paging            guestPaging     `json:"paging"`
}

type getGuestsParams struct {
	EventID              string  `json:"eventId"`
	IncludeInvitedGuests bool    `json:"includeInvitedGuests"`
	Password             *string `json:"password,omitempty"`
}

type guestPaging struct {
	Cursor     *string `json:"cursor"`
	MaxResults int     `json:"maxResults"`
}

type getGuestsResponse struct {
	Result struct {
		Data   []json.RawMessage `json:"data"`
		Paging *struct {
			NextCursor json.RawMessage `json:"nextCursor"`
		} `json:"paging"`
	} `json:"result"`
}

type InviteGuestsAsHostParams struct {
	EventID               string           `json:"eventId"`
	UserIDsToInvite       []string         `json:"userIdsToInvite"`
	InvitationMessage     string           `json:"invitationMessage"`
	OtherMutualsCount     int              `json:"otherMutualsCount"`
	PhoneContactsToInvite []map[string]any `json:"phoneContactsToInvite"`
	EmailsToInvite        []map[string]any `json:"emailsToInvite"`
}

type guestMutationRequest[T any] struct {
	Data guestMutationRequestData[T] `json:"data"`
}

type guestMutationRequestData[T any] struct {
	Params            T      `json:"params"`
	AmplitudeDeviceID string `json:"amplitudeDeviceId"`
	UserID            any    `json:"userId"`
}

func (client Client) GetGuests(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	eventID string,
) (GuestCatalog, error) {
	var (
		cursor        *string
		guests        []Guest
		dataPageCount int
	)
	seenCursors := make(map[string]struct{})
	for {
		page, err := client.getGuestsPage(
			ctx,
			accessToken,
			amplitudeDeviceID,
			eventID,
			cursor,
		)
		if errors.Is(err, errGuestsHostView) && cursor == nil {
			return client.getHostGuests(ctx, accessToken, eventID)
		}
		if err != nil {
			return GuestCatalog{}, err
		}
		nextCursor, err := decodeGuestNextCursor(page.Result.Paging.NextCursor)
		if err != nil {
			return GuestCatalog{}, err
		}
		if len(page.Result.Data) > 0 {
			dataPageCount++
			if dataPageCount > maximumGuestDataPages ||
				len(page.Result.Data) > maximumGuestCatalogSize-len(guests) {
				return GuestCatalog{}, fmt.Errorf("%w: guests traversal bound", ErrProtocolChanged)
			}
		}
		for _, rawGuest := range page.Result.Data {
			guest, err := decodeGuest(rawGuest)
			if err != nil {
				return GuestCatalog{}, fmt.Errorf("%w: guest", ErrProtocolChanged)
			}
			guests = append(guests, guest)
		}
		if nextCursor == nil {
			document, _ := json.Marshal(guests)
			return GuestCatalog{
				Guests:        guests,
				PayloadSHA256: sha256.Sum256(document),
			}, nil
		}
		if len(page.Result.Data) == 0 {
			return GuestCatalog{}, fmt.Errorf("%w: empty guests page", ErrProtocolChanged)
		}
		if _, repeated := seenCursors[*nextCursor]; repeated {
			return GuestCatalog{}, fmt.Errorf("%w: repeated guests cursor", ErrProtocolChanged)
		}
		seenCursors[*nextCursor] = struct{}{}
		cursor = nextCursor
	}
}

func (client Client) getGuestsPage(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	eventID string,
	cursor *string,
) (getGuestsResponse, error) {
	if client.HTTP == nil {
		return getGuestsResponse{}, fmt.Errorf("%w: guests transport", ErrUnavailable)
	}
	payload, _ := json.Marshal(getGuestsRequest{Data: getGuestsRequestData{
		Params: getGuestsParams{
			EventID:              eventID,
			IncludeInvitedGuests: true,
		},
		AmplitudeDeviceID: amplitudeDeviceID,
		Paging: guestPaging{
			Cursor:     cursor,
			MaxResults: guestPageSize,
		},
	}})
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		partifulCallableHost+"/getGuests",
		bytes.NewReader(payload),
	)
	if err != nil {
		return getGuestsResponse{}, fmt.Errorf("%w: guests request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return getGuestsResponse{}, fmt.Errorf("%w: guests request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return getGuestsResponse{}, fmt.Errorf("%w: guests response", ErrProtocolChanged)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusBadRequest && isHostGuestListRejection(response) {
		return getGuestsResponse{}, errGuestsHostView
	}
	if response.StatusCode != http.StatusOK {
		return getGuestsResponse{}, fmt.Errorf("%w: guests status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return getGuestsResponse{}, fmt.Errorf("%w: guests content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumGuestPageBytes+1))
	if err != nil {
		return getGuestsResponse{}, fmt.Errorf("%w: guests response read", ErrUnavailable)
	}
	if len(body) > maximumGuestPageBytes || !utf8.Valid(body) {
		return getGuestsResponse{}, fmt.Errorf("%w: guests response body", ErrProtocolChanged)
	}
	var page getGuestsResponse
	if err := decodeSingleGuestJSON(body, &page); err != nil ||
		page.Result.Data == nil ||
		page.Result.Paging == nil {
		return getGuestsResponse{}, fmt.Errorf("%w: guests response body", ErrProtocolChanged)
	}
	return page, nil
}

func decodeGuestNextCursor(value json.RawMessage) (*string, error) {
	if len(value) == 0 {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(value)
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var cursor string
	if err := decodeSingleGuestJSON(trimmed, &cursor); err != nil ||
		strings.TrimSpace(cursor) == "" {
		return nil, fmt.Errorf("%w: guests cursor", ErrProtocolChanged)
	}
	return &cursor, nil
}

func decodeGuest(raw json.RawMessage) (Guest, error) {
	object, err := decodeEventObject(raw)
	if err != nil {
		return Guest{}, err
	}
	id, present, err := eventStringField(object, "id", false)
	if err != nil || !present || strings.TrimSpace(*id) == "" {
		return Guest{}, errors.New("guest id is invalid")
	}
	name, present, err := eventStringField(object, "name", false)
	if err != nil || !present || strings.TrimSpace(*name) == "" {
		return Guest{}, errors.New("guest name is invalid")
	}
	status, present, err := eventStringField(object, "status", false)
	if err != nil || !present || !validGuestStatus(*status) {
		return Guest{}, errors.New("guest status is invalid")
	}
	count, err := requiredGuestCount(object, "count")
	if err != nil {
		return Guest{}, err
	}
	userID, err := optionalGuestString(object, "userId")
	if err != nil {
		return Guest{}, err
	}
	anchorGuestID, err := optionalGuestString(object, "anchorGuestId")
	if err != nil {
		return Guest{}, err
	}
	return Guest{
		ID:            *id,
		UserID:        userID,
		Name:          *name,
		Status:        *status,
		Count:         count,
		AnchorGuestID: anchorGuestID,
	}, nil
}

func requiredGuestCount(
	object map[string]json.RawMessage,
	name string,
) (int, error) {
	raw, ok := object[name]
	if !ok {
		return 0, errors.New("guest count is missing")
	}
	var value float64
	if json.Unmarshal(raw, &value) != nil ||
		value < 0 ||
		value > 1<<53-1 ||
		math.Trunc(value) != value {
		return 0, errors.New("guest count is invalid")
	}
	integer := int64(value)
	if strconv.IntSize == 32 && integer > math.MaxInt32 {
		return 0, errors.New("guest count is invalid")
	}
	return int(integer), nil
}

func optionalGuestString(
	object map[string]json.RawMessage,
	name string,
) (*string, error) {
	raw, ok := object[name]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return nil, errors.New("guest string is invalid")
	}
	return &value, nil
}

func (client Client) InviteGuestsAsHost(
	ctx context.Context,
	accessToken string,
	userID string,
	amplitudeDeviceID string,
	params InviteGuestsAsHostParams,
) error {
	_, err := callGuestMutation(
		client,
		ctx,
		accessToken,
		userID,
		amplitudeDeviceID,
		"addInvitedGuestsAsHost",
		params,
	)
	return err
}

func callGuestMutation[T any](
	client Client,
	ctx context.Context,
	accessToken string,
	userID string,
	amplitudeDeviceID string,
	operation string,
	params T,
) (json.RawMessage, error) {
	if client.HTTP == nil {
		return nil, fmt.Errorf("%w: guest mutation transport", ErrUnavailable)
	}
	var encodedUserID any
	if userID != "" {
		encodedUserID = userID
	}
	payload, _ := json.Marshal(guestMutationRequest[T]{
		Data: guestMutationRequestData[T]{
			Params:            params,
			AmplitudeDeviceID: amplitudeDeviceID,
			UserID:            encodedUserID,
		},
	})
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		partifulCallableHost+"/"+operation,
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: guest mutation request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: guest mutation request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("%w: guest mutation response", ErrProtocolChanged)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: guest mutation status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("%w: guest mutation content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumEventWriteBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: guest mutation response read", ErrUnavailable)
	}
	if len(body) > maximumEventWriteBodyBytes || !utf8.Valid(body) {
		return nil, fmt.Errorf("%w: guest mutation response body", ErrProtocolChanged)
	}
	root, err := decodeEventObject(body)
	if err != nil {
		return nil, fmt.Errorf("%w: guest mutation response body", ErrProtocolChanged)
	}
	result, ok := root["result"]
	if !ok {
		return nil, fmt.Errorf("%w: guest mutation completion", ErrProtocolChanged)
	}
	if _, err := decodeEventObject(result); err != nil {
		return nil, fmt.Errorf("%w: guest mutation completion", ErrProtocolChanged)
	}
	return bytes.Clone(result), nil
}

func decodeSingleGuestJSON(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("guest response has trailing JSON")
	}
	return nil
}

// errGuestsHostView marks the getGuests rejection Partiful returns when the
// caller hosts the event (observed 2026-09-29: HTTP 400,
// {"error":{"message":"User is host","status":"FAILED_PRECONDITION"}}).
// Hosts read the event's guests subcollection instead.
var errGuestsHostView = errors.New("guests callable rejects host")

func isHostGuestListRejection(response *http.Response) bool {
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumGuestPageBytes+1))
	if err != nil || len(body) > maximumGuestPageBytes {
		return false
	}
	var rejection struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	return json.Unmarshal(body, &rejection) == nil &&
		rejection.Error.Status == "FAILED_PRECONDITION" &&
		rejection.Error.Message == "User is host"
}

func (client Client) getHostGuests(
	ctx context.Context,
	accessToken string,
	eventID string,
) (GuestCatalog, error) {
	documents, err := client.listFirestoreDocuments(
		ctx,
		accessToken,
		firestoreCollectionPath("events", eventID, "guests"),
		maximumGuestFirestorePages,
	)
	if err != nil {
		return GuestCatalog{}, err
	}
	guests := make([]Guest, 0, len(documents))
	for _, document := range documents {
		guest, err := decodeGuestDocument(document)
		if err != nil {
			return GuestCatalog{}, fmt.Errorf("%w: guest document", ErrProtocolChanged)
		}
		guests = append(guests, guest)
	}
	encoded, _ := json.Marshal(guests)
	return GuestCatalog{Guests: guests, PayloadSHA256: sha256.Sum256(encoded)}, nil
}

// decodeGuestDocument reads the same reviewed guest subset as decodeGuest from
// a Firestore events/{eventId}/guests document.
func decodeGuestDocument(document firestoreDocumentRecord) (Guest, error) {
	id := path.Base(document.Name)
	if strings.TrimSpace(id) == "" || id == "." || id == "/" {
		return Guest{}, errors.New("guest id is invalid")
	}
	name, err := firestoreRequiredString(document.Fields, "name")
	if err != nil || strings.TrimSpace(name) == "" {
		return Guest{}, errors.New("guest name is invalid")
	}
	status, err := firestoreRequiredString(document.Fields, "status")
	if err != nil || !validGuestStatus(status) {
		return Guest{}, errors.New("guest status is invalid")
	}
	count, err := firestoreGuestCount(document.Fields, "count")
	if err != nil {
		return Guest{}, err
	}
	userID, _, err := firestoreOptionalNullableString(document.Fields, "userId")
	if err != nil {
		return Guest{}, errors.New("guest user is invalid")
	}
	anchorGuestID, _, err := firestoreOptionalNullableString(document.Fields, "anchorGuestId")
	if err != nil {
		return Guest{}, errors.New("guest anchor is invalid")
	}
	return Guest{
		ID:            id,
		UserID:        userID,
		Name:          name,
		Status:        status,
		Count:         count,
		AnchorGuestID: anchorGuestID,
	}, nil
}

func firestoreGuestCount(fields map[string]json.RawMessage, name string) (int, error) {
	raw, ok := fields[name]
	if !ok {
		return 0, errors.New("guest count is missing")
	}
	var value struct {
		IntegerValue *string `json:"integerValue"`
	}
	if json.Unmarshal(raw, &value) != nil || value.IntegerValue == nil {
		return 0, errors.New("guest count is invalid")
	}
	count, err := strconv.Atoi(*value.IntegerValue)
	if err != nil || count < 0 {
		return 0, errors.New("guest count is invalid")
	}
	return count, nil
}
