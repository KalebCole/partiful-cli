package app

import (
	"context"
	"encoding/base64"
	"errors"
	"io"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

func executeContacts(
	ctx context.Context,
	definition commandDefinition,
	options collectionOptions,
	dependencies Dependencies,
	pretty bool,
) Result {
	filterHash := normalizedFilterHash(definition.path, options.query)
	var decodedCursor cursorPayload
	var cursorKey []byte
	var err error
	if options.cursorProvided {
		cursorKey, err = loadCursorKey(dependencies)
		if err != nil {
			return internalFailure(definition.path, pretty)
		}
		var cursorFailure *cursorValidationFailure
		decodedCursor, cursorFailure = decodeCursor(options.cursor, filterHash, cursorKey)
		if cursorFailure != nil {
			return failure(definition.path, cursorFailure.exitCode, cursorFailure.body, pretty)
		}
	}
	session, sessionFailure := acquireProtectedSession(
		ctx,
		definition.path,
		dependencies,
		pretty,
	)
	if sessionFailure != nil {
		return *sessionFailure
	}
	deviceID, err := randomDeviceID(dependencies.AuthRandom)
	if err != nil {
		return internalFailure(definition.path, pretty)
	}
	catalog, err := (remote.Client{HTTP: dependencies.HTTP}).GetContacts(
		ctx,
		session.AccessToken,
		deviceID,
	)
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return contactsUnavailableFailure(definition.path, pretty)
		}
		if errors.Is(err, remote.ErrUnauthenticated) {
			return authenticationExpiredFailure(
				definition.path,
				"REMOTE_SESSION_UNAUTHENTICATED",
				"Stored authentication is no longer accepted. Log in again.",
				pretty,
			)
		}
		return contactsProtocolChangedFailure(definition.path, pretty)
	}
	filteredContacts := filterContacts(catalog.Contacts, options.query)
	offset := 0
	if options.cursorProvided {
		var cursorFailure *cursorValidationFailure
		offset, cursorFailure = cursorSnapshotOffset(
			decodedCursor,
			catalog.PayloadSHA256,
			len(filteredContacts),
			"The contact catalog changed after this cursor was issued.",
		)
		if cursorFailure != nil {
			return failure(definition.path, cursorFailure.exitCode, cursorFailure.body, pretty)
		}
	}
	end := min(offset+options.limit, len(filteredContacts))
	items := make([]contact, 0, end-offset)
	for _, remoteContact := range filteredContacts[offset:end] {
		items = append(items, contact{
			DisplayName:      remoteContact.Name,
			SharedEventCount: remoteContact.SharedEventCount,
		})
	}
	var cursor *string
	hasMore := end < len(filteredContacts)
	if hasMore {
		if cursorKey == nil {
			cursorKey, err = loadCursorKey(dependencies)
			if err != nil {
				return internalFailure(definition.path, pretty)
			}
		}
		value, err := nextCursor(
			catalog.PayloadSHA256,
			filterHash,
			end,
			cursorKey,
			dependencies.CursorRandom,
		)
		if err != nil {
			return internalFailure(definition.path, pretty)
		}
		cursor = &value
	}
	return collectionSuccess(definition.path, contactData{Items: items}, pageMeta{
		Limit:            options.limit,
		NextCursor:       cursor,
		HasMore:          hasMore,
		Truncated:        options.serverLimited && hasMore,
		TruncationReason: collectionTruncationReason(options, hasMore),
	}, pretty)
}

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
