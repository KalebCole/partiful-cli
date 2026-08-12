package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maximumFirestoreListBytes = 8 << 20
	firestorePageSize         = 1000
)

type EventGuest struct {
	Status    string
	CheckedIn bool
}

type firestoreDocumentRecord struct {
	Name       string
	Fields     map[string]json.RawMessage
	CreateTime *string
	UpdateTime *string
}

func (client Client) ListEventGuests(
	ctx context.Context,
	accessToken string,
	eventID string,
) ([]EventGuest, error) {
	documents, err := client.listFirestoreDocuments(
		ctx,
		accessToken,
		firestoreCollectionPath("events", eventID, "guests"),
	)
	if err != nil {
		return nil, err
	}
	guests := make([]EventGuest, 0, len(documents))
	for _, document := range documents {
		status, err := firestoreRequiredString(document.Fields, "status")
		if err != nil || !validGuestStatus(status) {
			return nil, fmt.Errorf("%w: guest document status", ErrProtocolChanged)
		}
		checkedIn, err := firestoreHasNonNullField(document.Fields, "checkIn")
		if err != nil {
			return nil, fmt.Errorf("%w: guest document check-in", ErrProtocolChanged)
		}
		guests = append(guests, EventGuest{Status: status, CheckedIn: checkedIn})
	}
	return guests, nil
}

func (client Client) CountTextBlasts(
	ctx context.Context,
	accessToken string,
	eventID string,
) (int, error) {
	documents, err := client.listFirestoreDocuments(
		ctx,
		accessToken,
		firestoreCollectionPath("events", eventID, "hostMessages"),
	)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, document := range documents {
		typeValue, present, err := firestoreOptionalNullableString(document.Fields, "type")
		if err != nil {
			return 0, fmt.Errorf("%w: host message type", ErrProtocolChanged)
		}
		if !present || typeValue == nil || *typeValue == "TEXT_BLAST" {
			count++
		}
	}
	return count, nil
}

func (client Client) listFirestoreDocuments(
	ctx context.Context,
	accessToken string,
	collectionPath string,
) ([]firestoreDocumentRecord, error) {
	if client.HTTP == nil {
		return nil, fmt.Errorf("%w: firestore list transport", ErrUnavailable)
	}
	var documents []firestoreDocumentRecord
	pageToken := ""
	for {
		page, nextToken, err := client.listFirestoreDocumentsPage(
			ctx,
			accessToken,
			collectionPath,
			pageToken,
		)
		if err != nil {
			return nil, err
		}
		documents = append(documents, page...)
		if nextToken == "" {
			return documents, nil
		}
		pageToken = nextToken
	}
}

func (client Client) listFirestoreDocumentsPage(
	ctx context.Context,
	accessToken string,
	collectionPath string,
	pageToken string,
) ([]firestoreDocumentRecord, string, error) {
	query := url.Values{}
	query.Set("pageSize", fmt.Sprintf("%d", firestorePageSize))
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		firestoreDocumentsHost+"/v1/projects/getpartiful/databases/(default)/documents/"+collectionPath+"?"+query.Encode(),
		nil,
	)
	if err != nil {
		return nil, "", fmt.Errorf("%w: firestore list request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("%w: firestore list request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return nil, "", fmt.Errorf("%w: firestore list response", ErrProtocolChanged)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("%w: firestore list status", ErrProtocolChanged)
	}
	if !eventJSONContentType(response.Header.Get("Content-Type")) {
		return nil, "", fmt.Errorf("%w: firestore list content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumFirestoreListBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("%w: firestore list response read", ErrUnavailable)
	}
	if len(body) > maximumFirestoreListBytes || !utf8.Valid(body) {
		return nil, "", fmt.Errorf("%w: firestore list response body", ErrProtocolChanged)
	}
	return decodeFirestoreListResponse(body)
}

func decodeFirestoreListResponse(body []byte) ([]firestoreDocumentRecord, string, error) {
	root, err := decodeEventObject(body)
	if err != nil {
		return nil, "", fmt.Errorf("%w: firestore list response body", ErrProtocolChanged)
	}
	nextToken := ""
	if raw, ok := root["nextPageToken"]; ok {
		if json.Unmarshal(raw, &nextToken) != nil {
			return nil, "", fmt.Errorf("%w: firestore list next page token", ErrProtocolChanged)
		}
	}
	rawDocuments, ok := root["documents"]
	if !ok {
		return []firestoreDocumentRecord{}, nextToken, nil
	}
	if !isEventJSONKind(rawDocuments, '[') {
		return nil, "", fmt.Errorf("%w: firestore list documents", ErrProtocolChanged)
	}
	var entries []json.RawMessage
	if json.Unmarshal(rawDocuments, &entries) != nil || entries == nil {
		return nil, "", fmt.Errorf("%w: firestore list documents", ErrProtocolChanged)
	}
	documents := make([]firestoreDocumentRecord, 0, len(entries))
	for _, entry := range entries {
		document, err := decodeFirestoreDocumentRecord(entry)
		if err != nil {
			return nil, "", fmt.Errorf("%w: firestore list document", ErrProtocolChanged)
		}
		documents = append(documents, document)
	}
	return documents, nextToken, nil
}

func decodeFirestoreDocumentRecord(raw json.RawMessage) (firestoreDocumentRecord, error) {
	if err := validateFirestoreDocument(raw); err != nil {
		return firestoreDocumentRecord{}, err
	}
	root, err := decodeEventObject(raw)
	if err != nil {
		return firestoreDocumentRecord{}, err
	}
	document := firestoreDocumentRecord{Fields: map[string]json.RawMessage{}}
	if rawName, ok := root["name"]; ok {
		if json.Unmarshal(rawName, &document.Name) != nil {
			return firestoreDocumentRecord{}, fmt.Errorf("invalid firestore document name")
		}
	}
	for _, field := range []string{"createTime", "updateTime"} {
		if rawTime, ok := root[field]; ok {
			var value string
			if json.Unmarshal(rawTime, &value) != nil {
				return firestoreDocumentRecord{}, fmt.Errorf("invalid firestore timestamp")
			}
			if field == "createTime" {
				document.CreateTime = &value
			} else {
				document.UpdateTime = &value
			}
		}
	}
	if rawFields, ok := root["fields"]; ok {
		fields, err := decodeEventObject(rawFields)
		if err != nil {
			return firestoreDocumentRecord{}, err
		}
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			document.Fields[key] = bytes.Clone(fields[key])
		}
	}
	return document, nil
}

func firestoreCollectionPath(segments ...string) string {
	encoded := make([]string, 0, len(segments))
	for _, segment := range segments {
		encoded = append(encoded, url.PathEscape(segment))
	}
	return path.Join(encoded...)
}

func firestoreRequiredString(fields map[string]json.RawMessage, name string) (string, error) {
	value, present, err := firestoreOptionalNullableString(fields, name)
	if err != nil || !present || value == nil || *value == "" {
		return "", fmt.Errorf("field is invalid")
	}
	return *value, nil
}

func firestoreOptionalNullableString(fields map[string]json.RawMessage, name string) (*string, bool, error) {
	raw, ok := fields[name]
	if !ok {
		return nil, false, nil
	}
	object, err := decodeEventObject(raw)
	if err != nil || len(object) != 1 {
		return nil, true, fmt.Errorf("field is invalid")
	}
	if rawNull, ok := object["nullValue"]; ok {
		if !bytes.Equal(bytes.TrimSpace(rawNull), []byte("null")) {
			return nil, true, fmt.Errorf("field is invalid")
		}
		return nil, true, nil
	}
	rawString, ok := object["stringValue"]
	if !ok {
		return nil, true, fmt.Errorf("field is invalid")
	}
	var value string
	if json.Unmarshal(rawString, &value) != nil {
		return nil, true, fmt.Errorf("field is invalid")
	}
	return &value, true, nil
}

func firestoreHasNonNullField(fields map[string]json.RawMessage, name string) (bool, error) {
	raw, ok := fields[name]
	if !ok {
		return false, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false, fmt.Errorf("field is invalid")
	}
	object, err := decodeEventObject(trimmed)
	if err != nil || len(object) != 1 {
		return false, fmt.Errorf("field is invalid")
	}
	if rawNull, ok := object["nullValue"]; ok {
		if !bytes.Equal(bytes.TrimSpace(rawNull), []byte("null")) {
			return false, fmt.Errorf("field is invalid")
		}
		return false, nil
	}
	for _, variant := range []string{"booleanValue", "integerValue", "doubleValue", "timestampValue", "stringValue", "bytesValue", "referenceValue", "geoPointValue", "arrayValue", "mapValue"} {
		if _, ok := object[variant]; ok {
			return true, nil
		}
	}
	return false, fmt.Errorf("field is invalid")
}

func invitedGuestStatus(status string) bool {
	return status == "READY_TO_SEND" ||
		status == "SENDING" ||
		status == "SENT" ||
		status == "SEND_ERROR" ||
		status == "DELIVERY_ERROR"
}

func normalizeFirestorePath(value string) string {
	return strings.TrimPrefix(value, "/")
}
