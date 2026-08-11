package app

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/KalebCole/partiful-cli/internal/remote"
)

const (
	defaultCollectionLimit = 25
	maximumCollectionLimit = 100
)

type collectionOptions struct {
	limit  int
	cursor string
	all    bool
	max    int
	query  string
}

type cursorPayload struct {
	Version    int    `json:"version"`
	Digest     string `json:"digest"`
	FilterHash string `json:"filterHash"`
	Offset     int    `json:"offset"`
}

type cursorValidationFailure struct {
	exitCode int
	body     errorBody
}

func parseCollectionOptions(definition commandDefinition, argv []string) (collectionOptions, *errorBody) {
	values, parseError := parseCommandFlags(definition, argv)
	if parseError != nil {
		return collectionOptions{}, parseError
	}
	options := collectionOptions{limit: defaultCollectionLimit}
	if value, ok := values["--limit"]; ok {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > maximumCollectionLimit {
			return collectionOptions{}, &errorBody{
				Type:      "input.invalid",
				Code:      "LIMIT_INVALID",
				Message:   "Limit must be an integer from 1 to 100.",
				Retryable: false,
				Details:   map[string]any{},
			}
		}
		options.limit = limit
	}
	options.cursor = values["--cursor"]
	_, options.all = values["--all"]
	if definition.kind == postersSearchCommand {
		options.query = strings.ToLower(strings.TrimSpace(values["--query"]))
		if options.query == "" {
			return collectionOptions{}, &errorBody{
				Type:      "input.invalid",
				Code:      "QUERY_REQUIRED",
				Message:   "Search query must not be empty.",
				Retryable: false,
				Details:   map[string]any{},
			}
		}
	}
	if value, ok := values["--max-items"]; ok {
		maximum, err := strconv.Atoi(value)
		if err != nil || maximum < 1 || maximum > 1000 {
			return collectionOptions{}, &errorBody{
				Type:      "input.invalid",
				Code:      "MAX_ITEMS_INVALID",
				Message:   "Max items must be an integer from 1 to 1000.",
				Retryable: false,
				Details:   map[string]any{},
			}
		}
		options.max = maximum
	}
	if options.all && options.max == 0 {
		return collectionOptions{}, &errorBody{
			Type:      "input.invalid",
			Code:      "MAX_ITEMS_REQUIRED",
			Message:   "--all requires --max-items.",
			Retryable: false,
			Details:   map[string]any{},
		}
	}
	if !options.all && options.max != 0 {
		return collectionOptions{}, &errorBody{
			Type:      "input.invalid",
			Code:      "ALL_REQUIRED",
			Message:   "--max-items requires --all.",
			Retryable: false,
			Details:   map[string]any{},
		}
	}
	if options.all {
		options.limit = options.max
	}
	return options, nil
}

func parseCommandFlags(definition commandDefinition, argv []string) (map[string]string, *errorBody) {
	allowed := make(map[string]flagDefinition, len(definition.flags))
	for _, flag := range definition.flags {
		allowed[flag.Name] = flag
	}
	values := make(map[string]string)
	for index := len(definition.invocation); index < len(argv); index++ {
		name := argv[index]
		flag, ok := allowed[name]
		if !ok {
			return nil, &errorBody{
				Type:      "input.invalid",
				Code:      "FLAG_UNKNOWN",
				Message:   "The command contains an unknown flag.",
				Retryable: false,
				Details:   map[string]any{},
			}
		}
		if _, repeated := values[name]; repeated {
			return nil, &errorBody{
				Type:      "input.invalid",
				Code:      "FLAG_REPEATED",
				Message:   "A scalar flag cannot be repeated.",
				Retryable: false,
				Details:   map[string]any{"flag": name},
			}
		}
		if !flag.TakesValue {
			values[name] = "true"
			continue
		}
		if index+1 >= len(argv) {
			return nil, &errorBody{
				Type:      "input.invalid",
				Code:      "FLAG_VALUE_REQUIRED",
				Message:   "A flag value is required.",
				Retryable: false,
				Details:   map[string]any{"flag": name},
			}
		}
		index++
		values[name] = argv[index]
	}
	return values, nil
}

func nextCursor(
	payloadDigest [sha256.Size]byte,
	filterHash [sha256.Size]byte,
	offset int,
	key []byte,
	random io.Reader,
) (string, error) {
	payload := cursorPayload{
		Version:    1,
		Digest:     hex.EncodeToString(payloadDigest[:]),
		FilterHash: hex.EncodeToString(filterHash[:]),
		Offset:     offset,
	}
	document, _ := json.Marshal(payload)
	aead, err := cursorAEAD(key)
	if err != nil || random == nil {
		return "", errors.New("cursor encryption is unavailable")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return "", errors.New("cursor nonce is unavailable")
	}
	sealed := aead.Seal(nonce, nonce, document, cursorAssociatedData())
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decodeCursor(
	token string,
	filterHash [sha256.Size]byte,
	key []byte,
) (cursorPayload, *cursorValidationFailure) {
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return cursorPayload{}, invalidCursorFailure()
	}
	aead, err := cursorAEAD(key)
	if err != nil || len(sealed) < aead.NonceSize()+aead.Overhead() {
		return cursorPayload{}, invalidCursorFailure()
	}
	nonce := sealed[:aead.NonceSize()]
	document, err := aead.Open(nil, nonce, sealed[aead.NonceSize():], cursorAssociatedData())
	if err != nil {
		return cursorPayload{}, invalidCursorFailure()
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	var payload cursorPayload
	if err := decoder.Decode(&payload); err != nil {
		return cursorPayload{}, invalidCursorFailure()
	}
	if payload.Version != 1 || payload.Offset < 0 {
		return cursorPayload{}, invalidCursorFailure()
	}
	if payload.FilterHash != hex.EncodeToString(filterHash[:]) {
		return cursorPayload{}, &cursorValidationFailure{
			exitCode: 2,
			body: errorBody{
				Type:      "input.invalid",
				Code:      "CURSOR_FILTER_MISMATCH",
				Message:   "The cursor does not match this command and filters.",
				Retryable: false,
				Details:   map[string]any{},
			},
		}
	}
	return payload, nil
}

func cursorAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func cursorAssociatedData() []byte {
	return []byte("partiful/posters/cursor/v1")
}

func cursorSnapshotOffset(
	payload cursorPayload,
	payloadDigest [sha256.Size]byte,
	itemCount int,
) (int, *cursorValidationFailure) {
	if payload.Digest != hex.EncodeToString(payloadDigest[:]) {
		return 0, &cursorValidationFailure{
			exitCode: 6,
			body: errorBody{
				Type:      "state.conflict",
				Code:      "CURSOR_SNAPSHOT_CHANGED",
				Message:   "The poster catalog changed after this cursor was issued.",
				Retryable: false,
				Details:   map[string]any{},
			},
		}
	}
	if payload.Offset > itemCount {
		return 0, invalidCursorFailure()
	}
	return payload.Offset, nil
}

func invalidCursorFailure() *cursorValidationFailure {
	return &cursorValidationFailure{
		exitCode: 2,
		body: errorBody{
			Type:      "input.invalid",
			Code:      "CURSOR_INVALID",
			Message:   "The cursor is malformed.",
			Retryable: false,
			Details:   map[string]any{},
		},
	}
}

func normalizedFilterHash(command, query string) [sha256.Size]byte {
	return sha256.Sum256([]byte(command + "\x00" + query))
}

func filterPosters(posters []remote.Poster, query string) []remote.Poster {
	filtered := make([]remote.Poster, 0, len(posters))
	for _, poster := range posters {
		if containsFold(poster.Name, query) ||
			containsAnyFold(poster.Tags, query) ||
			containsAnyFold(poster.Categories, query) {
			filtered = append(filtered, poster)
		}
	}
	return filtered
}

func containsAnyFold(values []string, query string) bool {
	for _, value := range values {
		if containsFold(value, query) {
			return true
		}
	}
	return false
}

func containsFold(value, query string) bool {
	return strings.Contains(strings.ToLower(value), query)
}
