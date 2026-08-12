package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	maximumCohostResponseBytes = 1 << 20
	maximumCohostRequestCount  = 100
)

type CohostTargetParams struct {
	EventID      string `json:"eventId"`
	TargetUserID string `json:"targetUserId"`
}

type CohostLinkParams struct {
	EventID string `json:"eventId"`
}

type CohostRequest struct {
	TargetUserID string
	Status       string
}

type CohostLink struct {
	Present bool
	Path    string
}

type callableCohostRequest[T any] struct {
	Data callableCohostRequestData[T] `json:"data"`
}

type callableCohostRequestData[T any] struct {
	Params            T   `json:"params"`
	AmplitudeDeviceID string `json:"amplitudeDeviceId"`
	UserID            any `json:"userId"`
}

type firestoreDocument struct {
	Name   string
	Fields map[string]json.RawMessage
}

func (client Client) CreateCohostRequest(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	userID string,
	params CohostTargetParams,
) error {
	completion, err := callCohostMutation(
		client,
		ctx,
		accessToken,
		amplitudeDeviceID,
		userID,
		"createCohostRequest",
		params,
	)
	if err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(completion), []byte("null")) {
		return fmt.Errorf("%w: cohost invite completion", ErrProtocolChanged)
	}
	return nil
}

func (client Client) DeleteCohostRequest(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	userID string,
	params CohostTargetParams,
) error {
	_, err := callCohostMutation(
		client,
		ctx,
		accessToken,
		amplitudeDeviceID,
		userID,
		"deleteCohostRequest",
		params,
	)
	return err
}

func (client Client) RemoveCohost(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	userID string,
	params CohostTargetParams,
) error {
	_, err := callCohostMutation(
		client,
		ctx,
		accessToken,
		amplitudeDeviceID,
		userID,
		"removeCohost",
		params,
	)
	return err
}

func (client Client) GenerateEventCohostLink(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	userID string,
	params CohostLinkParams,
) (string, error) {
	completion, err := callCohostMutation(
		client,
		ctx,
		accessToken,
		amplitudeDeviceID,
		userID,
		"generateEventCohostLink",
		params,
	)
	if err != nil {
		return "", err
	}
	root, err := decodeEventObject(completion)
	if err != nil {
		return "", fmt.Errorf("%w: cohost link completion", ErrProtocolChanged)
	}
	data, err := eventObjectField(root, "data")
	if err != nil {
		return "", fmt.Errorf("%w: cohost link completion", ErrProtocolChanged)
	}
	path, present, err := eventStringField(data, "path", false)
	if err != nil || !present || strings.TrimSpace(*path) == "" {
		return "", fmt.Errorf("%w: cohost link completion", ErrProtocolChanged)
	}
	return *path, nil
}

func (client Client) RevokeEventCohostLink(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	userID string,
	params CohostLinkParams,
) error {
	_, err := callCohostMutation(
		client,
		ctx,
		accessToken,
		amplitudeDeviceID,
		userID,
		"revokeEventCohostLink",
		params,
	)
	return err
}

func (client Client) GetCohostRequests(
	ctx context.Context,
	accessToken string,
	eventID string,
) ([]CohostRequest, error) {
	root, err := client.getFirestoreCollection(
		ctx,
		accessToken,
		"events/"+eventID+"/cohostRequests",
		maximumCohostRequestCount,
	)
	if err != nil {
		return nil, err
	}
	if token, ok := root["nextPageToken"]; ok && !bytes.Equal(bytes.TrimSpace(token), []byte(`""`)) {
		return nil, fmt.Errorf("%w: cohost requests pagination", ErrProtocolChanged)
	}
	rawDocuments, ok := root["documents"]
	if !ok {
		return []CohostRequest{}, nil
	}
	if !isEventJSONKind(rawDocuments, '[') {
		return nil, fmt.Errorf("%w: cohost requests documents", ErrProtocolChanged)
	}
	var documents []json.RawMessage
	if err := json.Unmarshal(rawDocuments, &documents); err != nil {
		return nil, fmt.Errorf("%w: cohost requests documents", ErrProtocolChanged)
	}
	if len(documents) > maximumCohostRequestCount {
		return nil, fmt.Errorf("%w: cohost requests bound", ErrProtocolChanged)
	}
	requests := make([]CohostRequest, 0, len(documents))
	for _, rawDocument := range documents {
		document, err := decodeFirestoreDocument(rawDocument)
		if err != nil {
			return nil, err
		}
		targetUserID, err := firestoreReferenceUserID(document.Fields, "target")
		if err != nil {
			return nil, fmt.Errorf("%w: cohost request target", ErrProtocolChanged)
		}
		status, present, err := firestoreStringField(document.Fields, "status")
		if err != nil || !present || !validCohostRequestStatus(*status) {
			return nil, fmt.Errorf("%w: cohost request status", ErrProtocolChanged)
		}
		requests = append(requests, CohostRequest{
			TargetUserID: targetUserID,
			Status:       *status,
		})
	}
	return requests, nil
}

func (client Client) GetCohostLink(
	ctx context.Context,
	accessToken string,
	eventID string,
) (CohostLink, error) {
	document, found, err := client.getFirestoreDocument(
		ctx,
		accessToken,
		"events/"+eventID+"/private/cohostSecret",
	)
	if err != nil {
		return CohostLink{}, err
	}
	if !found {
		return CohostLink{}, nil
	}
	path, present, err := firestoreStringField(document.Fields, "path")
	if err != nil || !present || strings.TrimSpace(*path) == "" {
		return CohostLink{}, fmt.Errorf("%w: cohost link path", ErrProtocolChanged)
	}
	return CohostLink{Present: true, Path: *path}, nil
}

func callCohostMutation[T any](
	client Client,
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	userID string,
	operation string,
	params T,
) (json.RawMessage, error) {
	if client.HTTP == nil {
		return nil, fmt.Errorf("%w: cohost transport", ErrUnavailable)
	}
	var encodedUserID any
	if userID != "" {
		encodedUserID = userID
	}
	payload, _ := json.Marshal(callableCohostRequest[T]{
		Data: callableCohostRequestData[T]{
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
		return nil, fmt.Errorf("%w: cohost request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: cohost request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("%w: cohost response", ErrProtocolChanged)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: cohost status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("%w: cohost content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumCohostResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: cohost response read", ErrUnavailable)
	}
	if len(body) > maximumCohostResponseBytes || !utf8.Valid(body) {
		return nil, fmt.Errorf("%w: cohost response body", ErrProtocolChanged)
	}
	root, err := decodeEventObject(body)
	if err != nil {
		return nil, fmt.Errorf("%w: cohost response body", ErrProtocolChanged)
	}
	if data, ok := root["data"]; ok {
		return bytes.Clone(data), nil
	}
	if result, ok := root["result"]; ok {
		return bytes.Clone(result), nil
	}
	return nil, fmt.Errorf("%w: cohost completion", ErrProtocolChanged)
}

func (client Client) getFirestoreCollection(
	ctx context.Context,
	accessToken string,
	collectionPath string,
	pageSize int,
) (map[string]json.RawMessage, error) {
	if client.HTTP == nil {
		return nil, fmt.Errorf("%w: firestore transport", ErrUnavailable)
	}
	query := url.Values{}
	if pageSize > 0 {
		query.Set("pageSize", fmt.Sprintf("%d", pageSize))
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		firestoreDocumentsHost+"/v1/projects/getpartiful/databases/(default)/documents/"+escapeFirestorePath(collectionPath)+"?"+query.Encode(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: firestore request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: firestore request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("%w: firestore response", ErrProtocolChanged)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: firestore status", ErrProtocolChanged)
	}
	body, err := readFirestoreJSONBody(response)
	if err != nil {
		return nil, err
	}
	root, err := decodeEventObject(body)
	if err != nil {
		return nil, fmt.Errorf("%w: firestore list response", ErrProtocolChanged)
	}
	return root, nil
}

func (client Client) getFirestoreDocument(
	ctx context.Context,
	accessToken string,
	documentPath string,
) (firestoreDocument, bool, error) {
	if client.HTTP == nil {
		return firestoreDocument{}, false, fmt.Errorf("%w: firestore transport", ErrUnavailable)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		firestoreDocumentsHost+"/v1/projects/getpartiful/databases/(default)/documents/"+escapeFirestorePath(documentPath),
		nil,
	)
	if err != nil {
		return firestoreDocument{}, false, fmt.Errorf("%w: firestore request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := client.HTTP.Do(request)
	if err != nil {
		return firestoreDocument{}, false, fmt.Errorf("%w: firestore request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return firestoreDocument{}, false, fmt.Errorf("%w: firestore response", ErrProtocolChanged)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		body, err := readFirestoreJSONBody(response)
		if err != nil {
			return firestoreDocument{}, false, err
		}
		if !validFirestoreNotFound(body) {
			return firestoreDocument{}, false, fmt.Errorf("%w: firestore not found", ErrProtocolChanged)
		}
		return firestoreDocument{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return firestoreDocument{}, false, fmt.Errorf("%w: firestore status", ErrProtocolChanged)
	}
	body, err := readFirestoreJSONBody(response)
	if err != nil {
		return firestoreDocument{}, false, err
	}
	document, err := decodeFirestoreDocument(body)
	if err != nil {
		return firestoreDocument{}, false, err
	}
	return document, true, nil
}

func readFirestoreJSONBody(response *http.Response) ([]byte, error) {
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("%w: firestore content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumEventWriteBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: firestore response read", ErrUnavailable)
	}
	if len(body) > maximumEventWriteBodyBytes || !utf8.Valid(body) {
		return nil, fmt.Errorf("%w: firestore response body", ErrProtocolChanged)
	}
	return body, nil
}

func decodeFirestoreDocument(raw []byte) (firestoreDocument, error) {
	root, err := decodeEventObject(raw)
	if err != nil {
		return firestoreDocument{}, fmt.Errorf("%w: firestore document", ErrProtocolChanged)
	}
	document := firestoreDocument{}
	if name, present, err := eventStringField(root, "name", false); err != nil {
		return firestoreDocument{}, fmt.Errorf("%w: firestore document name", ErrProtocolChanged)
	} else if present {
		document.Name = *name
	}
	if rawFields, ok := root["fields"]; ok {
		fields, err := decodeEventObject(rawFields)
		if err != nil {
			return firestoreDocument{}, fmt.Errorf("%w: firestore document fields", ErrProtocolChanged)
		}
		for _, key := range []string{"createTime", "updateTime"} {
			if value, ok := root[key]; ok {
				var timestamp string
				if json.Unmarshal(value, &timestamp) != nil {
					return firestoreDocument{}, fmt.Errorf("%w: firestore document timestamp", ErrProtocolChanged)
				}
			}
		}
		for _, rawValue := range fields {
			if err := validateFirestoreValue(rawValue); err != nil {
				return firestoreDocument{}, fmt.Errorf("%w: firestore field", ErrProtocolChanged)
			}
		}
		document.Fields = fields
	} else {
		document.Fields = map[string]json.RawMessage{}
	}
	return document, nil
}

func firestoreStringField(fields map[string]json.RawMessage, name string) (*string, bool, error) {
	raw, ok := fields[name]
	if !ok {
		return nil, false, nil
	}
	object, err := decodeEventObject(raw)
	if err != nil {
		return nil, true, err
	}
	return eventStringField(object, "stringValue", false)
}

func firestoreReferenceUserID(fields map[string]json.RawMessage, name string) (string, error) {
	raw, ok := fields[name]
	if !ok {
		return "", fmt.Errorf("reference is absent")
	}
	object, err := decodeEventObject(raw)
	if err != nil {
		return "", err
	}
	value, present, err := eventStringField(object, "referenceValue", false)
	if err != nil || !present {
		return "", fmt.Errorf("reference is invalid")
	}
	const prefix = "projects/getpartiful/databases/(default)/documents/users/"
	if !strings.HasPrefix(*value, prefix) {
		return "", fmt.Errorf("reference is invalid")
	}
	userID := strings.TrimPrefix(*value, prefix)
	if strings.TrimSpace(userID) == "" || strings.Contains(userID, "/") {
		return "", fmt.Errorf("reference is invalid")
	}
	return userID, nil
}

func validFirestoreNotFound(body []byte) bool {
	root, err := decodeEventObject(body)
	if err != nil {
		return false
	}
	failure, err := eventObjectField(root, "error")
	if err != nil {
		return false
	}
	status, present, err := eventStringField(failure, "status", false)
	return err == nil && present && *status == "NOT_FOUND"
}

func validCohostRequestStatus(value string) bool {
	return value == "PENDING" || value == "ACCEPTED" || value == "DECLINED"
}

func escapeFirestorePath(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
