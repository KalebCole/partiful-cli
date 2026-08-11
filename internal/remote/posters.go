package remote

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
)

const posterCatalogURL = "https://assets.getpartiful.com/posters.json"

var (
	ErrUnavailable     = errors.New("remote unavailable")
	ErrProtocolChanged = errors.New("remote protocol changed")
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	HTTP HTTPClient
}

type Poster struct {
	ID          string
	Name        string
	URL         string
	ContentType string
	Width       *int
	Height      *int
	Tags        []string
	Categories  []string
}

type PosterCatalog struct {
	Posters       []Poster
	PayloadSHA256 [sha256.Size]byte
}

func (client Client) GetPosterCatalog(ctx context.Context) (PosterCatalog, error) {
	if client.HTTP == nil {
		return PosterCatalog{}, ErrUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, posterCatalogURL, nil)
	if err != nil {
		return PosterCatalog{}, fmt.Errorf("%w: request", ErrUnavailable)
	}
	response, err := client.HTTP.Do(request)
	if err != nil {
		return PosterCatalog{}, fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	if response == nil || response.Body == nil {
		return PosterCatalog{}, fmt.Errorf("%w: missing response", ErrProtocolChanged)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return PosterCatalog{}, fmt.Errorf("%w: status", ErrProtocolChanged)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return PosterCatalog{}, fmt.Errorf("%w: content type", ErrProtocolChanged)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return PosterCatalog{}, fmt.Errorf("%w: response read", ErrUnavailable)
	}
	posters, err := decodePosters(body)
	if err != nil {
		return PosterCatalog{}, fmt.Errorf("%w: catalog body", ErrProtocolChanged)
	}
	return PosterCatalog{
		Posters:       posters,
		PayloadSHA256: sha256.Sum256(body),
	}, nil
}

func decodePosters(body []byte) ([]Poster, error) {
	var documents []map[string]json.RawMessage
	if err := json.Unmarshal(body, &documents); err != nil || documents == nil {
		return nil, ErrProtocolChanged
	}
	posters := make([]Poster, 0, len(documents))
	for _, document := range documents {
		poster, err := decodePoster(document)
		if err != nil {
			return nil, err
		}
		posters = append(posters, poster)
	}
	return posters, nil
}

func decodePoster(document map[string]json.RawMessage) (Poster, error) {
	var poster Poster
	if err := decodeRequired(document, "id", &poster.ID); err != nil {
		return Poster{}, err
	}
	if err := decodeRequired(document, "name", &poster.Name); err != nil {
		return Poster{}, err
	}
	if err := decodeRequired(document, "url", &poster.URL); err != nil {
		return Poster{}, err
	}
	parsedURL, err := url.Parse(poster.URL)
	if err != nil || !parsedURL.IsAbs() {
		return Poster{}, ErrProtocolChanged
	}
	if err := decodeRequired(document, "contentType", &poster.ContentType); err != nil {
		return Poster{}, err
	}
	if _, ok := document["blurHash"]; ok {
		var blurHash string
		if err := decodeRequired(document, "blurHash", &blurHash); err != nil {
			return Poster{}, err
		}
	}
	if err := decodeNullableInteger(document, "width", &poster.Width); err != nil {
		return Poster{}, err
	}
	if err := decodeNullableInteger(document, "height", &poster.Height); err != nil {
		return Poster{}, err
	}
	if err := decodeRequired(document, "tags", &poster.Tags); err != nil || poster.Tags == nil {
		return Poster{}, ErrProtocolChanged
	}
	if err := decodeRequired(document, "categories", &poster.Categories); err != nil || poster.Categories == nil {
		return Poster{}, ErrProtocolChanged
	}
	return poster, nil
}

func decodeRequired(document map[string]json.RawMessage, field string, destination any) error {
	value, ok := document[field]
	if !ok || string(value) == "null" {
		return ErrProtocolChanged
	}
	if err := json.Unmarshal(value, destination); err != nil {
		return ErrProtocolChanged
	}
	return nil
}

func decodeNullableInteger(document map[string]json.RawMessage, field string, destination **int) error {
	value, ok := document[field]
	if !ok {
		return ErrProtocolChanged
	}
	if string(value) == "null" {
		*destination = nil
		return nil
	}
	var integer int
	if err := json.Unmarshal(value, &integer); err != nil {
		return ErrProtocolChanged
	}
	*destination = &integer
	return nil
}
