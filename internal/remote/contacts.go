package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"
)

const (
	contactPageSize         = 1000
	maximumContactPageBytes = 4 << 20
)

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
			ID               string `json:"id"`
			Name             string `json:"name"`
			SharedEventCount int    `json:"sharedEventCount"`
		} `json:"data"`
		Paging struct {
			NextCursor *string `json:"nextCursor"`
		} `json:"paging"`
	} `json:"result"`
}

func (client Client) GetContacts(
	ctx context.Context,
	accessToken string,
	amplitudeDeviceID string,
) (ContactCatalog, error) {
	var (
		cursor   *string
		contacts []Contact
	)
	for {
		page, err := client.getContactsPage(ctx, accessToken, amplitudeDeviceID, cursor)
		if err != nil {
			return ContactCatalog{}, err
		}
		for _, item := range page.Result.Data {
			if item.SharedEventCount < 0 {
				return ContactCatalog{}, fmt.Errorf("%w: contact", ErrProtocolChanged)
			}
			contacts = append(contacts, Contact{
				ID:               item.ID,
				Name:             item.Name,
				SharedEventCount: item.SharedEventCount,
			})
		}
		if page.Result.Paging.NextCursor == nil {
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
		cursor = page.Result.Paging.NextCursor
	}
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
	if err := json.Unmarshal(body, &page); err != nil || page.Result.Data == nil {
		return contactsResponse{}, fmt.Errorf("%w: contacts response body", ErrProtocolChanged)
	}
	return page, nil
}
