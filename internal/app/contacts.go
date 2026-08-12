package app

import (
	"encoding/base64"
	"errors"
	"io"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

func randomDeviceID(random io.Reader) (string, error) {
	if random == nil {
		return "", errors.New("device identity random source is unavailable")
	}
	value := make([]byte, 16)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", errors.New("device identity random source is unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func filterContacts(contacts []remote.Contact, query string) []remote.Contact {
	seen := make(map[string]struct{}, len(contacts))
	filtered := make([]remote.Contact, 0, len(contacts))
	for _, contact := range contacts {
		if _, exists := seen[contact.ID]; exists {
			continue
		}
		seen[contact.ID] = struct{}{}
		if query == "" || containsFold(contact.Name, query) {
			filtered = append(filtered, contact)
		}
	}
	return filtered
}
