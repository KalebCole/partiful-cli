package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"
)

// The reviewed traversal used three data pages with a requested maximum of
// 1,000 items, followed by one empty terminal page. Do not exceed three data
// pages or 3,000 retained items until a larger boundary is reviewed.
const (
	contactPageSize           = 1000
	maximumContactDataPages   = 3
	maximumContactCatalogSize = contactPageSize * maximumContactDataPages
	// The observed full page was below 1 MiB; this is a local memory ceiling.
	maximumContactPageBytes = 4 << 20
)

var ErrUnauthenticated = errors.New("remote authentication is required")

type Contact struct {
	ID               string
	Name             string
	SharedEventCount int
}

type ContactCatalog struct {
	Contacts      []Contact
	PayloadSHA256 [sha256.Size]byte
}

type contactsRequest struct {
	Data contactsRequestData `json:"data"`
}

type contactsRequestData struct {
	Params            struct{}       `json:"params"`
	AmplitudeDeviceID string         `json:"amplitudeDeviceId"`
	Paging            contactsPaging `json:"paging"`
}

type contactsPaging struct {
	MaxResults int     `json:"maxResults"`
	Cursor     *string `json:"cursor"`
}

type contactsResponse struct {
	Result struct {
		Data []struct {
			ID               *string `json:"id"`
			Name             *string `json:"name"`
			SharedEventCount *int    `json:"sharedEventCount"`
		} `json:"data"`
		Paging *struct {
			NextCursor json.RawMessage `json:"nextCursor"`
		} `json:"paging"`
	} `json:"result"`
}

func (client Client) GetContacts(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
) (ContactCatalog, error) {
	var (
		cursor        *string
		contacts      []Contact
		dataPageCount int
	)
	seenCursors := make(map[string]struct{})
	for {
		page, err := client.getContactsPage(ctx, accessToken, amplitudeDeviceID, cursor)
		if err != nil {
			return ContactCatalog{}, err
		}
		nextCursor, err := decodeContactNextCursor(page.Result.Paging.NextCursor)
		if err != nil {
			return ContactCatalog{}, err
		}
		if nextCursor == nil {
			if len(page.Result.Data) != 0 {
				return ContactCatalog{}, fmt.Errorf("%w: terminal page", ErrProtocolChanged)
			}
			document, _ := json.Marshal(contacts)
			return ContactCatalog{
				Contacts:      contacts,
				PayloadSHA256: sha256.Sum256(document),
			}, nil
		}
		if len(page.Result.Data) == 0 {
			return ContactCatalog{}, fmt.Errorf("%w: empty data page", ErrProtocolChanged)
		}
		dataPageCount++
		if dataPageCount > maximumContactDataPages ||
			len(page.Result.Data) > maximumContactCatalogSize-len(contacts) {
			return ContactCatalog{}, fmt.Errorf("%w: contacts traversal bound", ErrProtocolChanged)
		}
		for _, item := range page.Result.Data {
			if item.ID == nil ||
				item.Name == nil ||
				item.SharedEventCount == nil ||
				*item.SharedEventCount < 0 {
				return ContactCatalog{}, fmt.Errorf("%w: contact", ErrProtocolChanged)
			}
			contacts = append(contacts, Contact{
				ID:               *item.ID,
				Name:             *item.Name,
				SharedEventCount: *item.SharedEventCount,
			})
		}
		if _, repeated := seenCursors[*nextCursor]; repeated {
			return ContactCatalog{}, fmt.Errorf("%w: repeated contacts cursor", ErrProtocolChanged)
		}
		seenCursors[*nextCursor] = struct{}{}
		cursor = nextCursor
	}
}

func decodeContactNextCursor(value json.RawMessage) (*string, error) {
	if len(value) == 0 {
		return nil, nil
	}
	var cursor *string
	if err := json.Unmarshal(value, &cursor); err != nil || cursor == nil {
		return nil, fmt.Errorf("%w: contacts cursor", ErrProtocolChanged)
	}
	return cursor, nil
}

func (client Client) getContactsPage(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
	cursor *string,
) (contactsResponse, error) {
	if client.HTTP == nil {
		return contactsResponse{}, fmt.Errorf("%w: contacts transport", ErrUnavailable)
	}
	payload, _ := json.Marshal(contactsRequest{Data: contactsRequestData{
		AmplitudeDeviceID: amplitudeDeviceID,
		Paging: contactsPaging{
			MaxResults: contactPageSize,
			Cursor:     cursor,
		},
	}})
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		partifulCallableHost+"/getContacts",
		bytes.NewReader(payload),
	)
	if err != nil {
		return contactsResponse{}, fmt.Errorf("%w: contacts request", ErrUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return contactsResponse{}, fmt.Errorf("%w: contacts request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return contactsResponse{}, fmt.Errorf("%w: contacts response", ErrProtocolChanged)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		if err := validateContactsUnauthenticated(response); err != nil {
			return contactsResponse{}, err
		}
		return contactsResponse{}, ErrUnauthenticated
	}
	if response.StatusCode != http.StatusOK {
		return contactsResponse{}, fmt.Errorf("%w: contacts status", ErrProtocolChanged)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return contactsResponse{}, fmt.Errorf("%w: contacts content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumContactPageBytes+1))
	if err != nil {
		return contactsResponse{}, fmt.Errorf("%w: contacts response read", ErrUnavailable)
	}
	if len(body) > maximumContactPageBytes || !utf8.Valid(body) {
		return contactsResponse{}, fmt.Errorf("%w: contacts response body", ErrProtocolChanged)
	}
	var page contactsResponse
	if err := json.Unmarshal(body, &page); err != nil ||
		page.Result.Data == nil ||
		page.Result.Paging == nil {
		return contactsResponse{}, fmt.Errorf("%w: contacts response body", ErrProtocolChanged)
	}
	return page, nil
}

func validateContactsUnauthenticated(response *http.Response) error {
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("%w: contacts unauthenticated content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumContactPageBytes+1))
	if err != nil {
		return fmt.Errorf("%w: contacts unauthenticated response read", ErrUnavailable)
	}
	if len(body) > maximumContactPageBytes || !utf8.Valid(body) {
		return fmt.Errorf("%w: contacts unauthenticated response body", ErrProtocolChanged)
	}
	var failure struct {
		Error *struct {
			Message *string `json:"message"`
			Status  *string `json:"status"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &failure) != nil ||
		failure.Error == nil ||
		failure.Error.Message == nil ||
		failure.Error.Status == nil ||
		*failure.Error.Status != "UNAUTHENTICATED" {
		return fmt.Errorf("%w: contacts unauthenticated response body", ErrProtocolChanged)
	}
	return nil
}
