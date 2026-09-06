package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/KalebCole/partiful-cli/internal/auth"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

type exploreOptions struct {
	city, state, country, section, date string
	limit, max                          int
	all, cursorProvided                 bool
	cursor                              string
}

type exploreCursorPayload struct {
	Version      int      `json:"version"`
	FilterHash   string   `json:"filterHash"`
	Mode         string   `json:"mode"`
	RemoteCursor *string  `json:"remoteCursor,omitempty"`
	SeenIDs      []string `json:"seenIds,omitempty"`
	Digest       string   `json:"digest,omitempty"`
	Offset       int      `json:"offset"`
}

type exploreData struct {
	Items []exploreEvent `json:"items"`
}
type exploreEvent struct {
	EventID                string          `json:"eventId"`
	URL                    string          `json:"url"`
	Title                  *string         `json:"title"`
	Start                  *string         `json:"start"`
	End                    *string         `json:"end"`
	Timezone               *string         `json:"timezone"`
	LocationLines          []string        `json:"locationLines"`
	GoogleMapsURL          *string         `json:"googleMapsUrl"`
	GoingGuestCount        *int            `json:"goingGuestCount"`
	MaybeGuestCount        *int            `json:"maybeGuestCount"`
	InterestedGuestCount   *int            `json:"interestedGuestCount"`
	Mutuals                []exploreMutual `json:"mutuals"`
	MutualsPreviewComplete any             `json:"mutualsPreviewComplete"`
}
type exploreMutual struct {
	DisplayName *string `json:"displayName"`
	Status      *string `json:"status"`
}

func exploreListInputSchema() jsonSchema {
	schema := collectionInputSchema(false)
	one := 1
	schema.Required = []string{"city", "state", "country"}
	schema.Properties["city"] = jsonSchema{Type: "string", MinLength: &one, Pattern: `\S`}
	schema.Properties["state"] = jsonSchema{Type: "string", MinLength: &one, Pattern: `\S`}
	schema.Properties["country"] = jsonSchema{Type: "string", MinLength: &one, Pattern: `\S`}
	schema.Properties["section"] = jsonSchema{Type: "string", Enum: []string{"trending", "friends", "open-invite", "followed"}}
	schema.Properties["date"] = jsonSchema{Type: "string", Enum: []string{"anytime", "today", "tomorrow", "this-week", "this-weekend"}}
	return schema
}

func exploreListSuccessSchema() jsonSchema {
	nullableString := jsonSchema{Type: []string{"string", "null"}}
	nullableInt := jsonSchema{Type: []string{"integer", "null"}}
	mutual := objectSchema([]string{"displayName", "status"}, map[string]jsonSchema{"displayName": nullableString, "status": jsonSchema{OneOf: []jsonSchema{{Type: "string", Enum: eventReadRsvpValues()}, {Type: "null"}}}})
	item := objectSchema([]string{"eventId", "url", "title", "start", "end", "timezone", "locationLines", "googleMapsUrl", "goingGuestCount", "maybeGuestCount", "interestedGuestCount", "mutuals", "mutualsPreviewComplete"}, map[string]jsonSchema{
		"eventId": {Type: "string"}, "url": {Type: "string", Format: "uri"}, "title": nullableString, "start": nullableString, "end": nullableString, "timezone": nullableString,
		"locationLines": {Type: "array", Items: &jsonSchema{Type: "string"}}, "googleMapsUrl": nullableString, "goingGuestCount": nullableInt, "maybeGuestCount": nullableInt, "interestedGuestCount": nullableInt,
		"mutuals": {Type: "array", Items: &mutual}, "mutualsPreviewComplete": {Type: "null"},
	})
	return objectSchema([]string{"items"}, map[string]jsonSchema{"items": {Type: "array", Items: &item}})
}

func parseExploreOptions(definition commandDefinition, argv []string) (exploreOptions, *errorBody) {
	values, errBody := parseCommandFlags(definition, argv)
	if errBody != nil {
		return exploreOptions{}, errBody
	}
	o := exploreOptions{limit: defaultCollectionLimit, date: "anytime"}
	for flag, target := range map[string]*string{"--city": &o.city, "--state": &o.state, "--country": &o.country} {
		*target = strings.TrimSpace(values[flag])
		if *target == "" {
			return exploreOptions{}, exploreInputFailure("AREA_REQUIRED", "City, state, and country are required.")
		}
	}
	o.section = strings.ToLower(strings.TrimSpace(values["--section"]))
	if o.section != "" && o.section != "trending" && o.section != "friends" && o.section != "open-invite" && o.section != "followed" {
		return exploreOptions{}, exploreInputFailure("SECTION_INVALID", "--section must be trending, friends, open-invite, or followed.")
	}
	if v, ok := values["--date"]; ok {
		o.date = strings.ToLower(strings.TrimSpace(v))
	}
	if o.date != "anytime" && o.date != "today" && o.date != "tomorrow" && o.date != "this-week" && o.date != "this-weekend" {
		return exploreOptions{}, exploreInputFailure("DATE_INVALID", "--date must be anytime, today, tomorrow, this-week, or this-weekend.")
	}
	if v, ok := values["--limit"]; ok {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > maximumCollectionLimit {
			return exploreOptions{}, exploreInputFailure("LIMIT_INVALID", "Limit must be an integer from 1 to 100.")
		}
		o.limit = n
	}
	o.cursor, o.cursorProvided = values["--cursor"]
	if o.cursorProvided && o.cursor == "" {
		f := invalidCursorFailure()
		return exploreOptions{}, &f.body
	}
	_, o.all = values["--all"]
	if v, ok := values["--max-items"]; ok {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 1000 {
			return exploreOptions{}, exploreInputFailure("MAX_ITEMS_INVALID", "Max items must be an integer from 1 to 1000.")
		}
		o.max = n
	}
	if o.all && o.max == 0 {
		return exploreOptions{}, exploreInputFailure("MAX_ITEMS_REQUIRED", "--all requires --max-items.")
	}
	if !o.all && o.max != 0 {
		return exploreOptions{}, exploreInputFailure("ALL_REQUIRED", "--max-items requires --all.")
	}
	if o.all {
		if _, ok := values["--limit"]; ok {
			return exploreOptions{}, exploreInputFailure("LIMIT_WITH_ALL", "--limit cannot be combined with --all.")
		}
		o.limit = o.max
	}
	return o, nil
}
func exploreInputFailure(code, message string) *errorBody {
	return &errorBody{Type: "input.invalid", Code: code, Message: message, Retryable: false, Details: map[string]any{}}
}
func exploreFilterHash(o exploreOptions) [sha256.Size]byte {
	return normalizedFilterHash("explore.list", strings.Join([]string{o.city, o.state, o.country, o.section, o.date}, "\x00"))
}

func executeExploreList(ctx context.Context, definition commandDefinition, argv []string, dependencies Dependencies, pretty bool) Result {
	o, inputErr := parseExploreOptions(definition, argv)
	if inputErr != nil {
		return failure(definition.path, 2, *inputErr, pretty)
	}
	filterHash := exploreFilterHash(o)
	var decoded exploreCursorPayload
	var key []byte
	var err error
	if o.cursorProvided {
		key, err = loadCursorKey(dependencies)
		if err != nil {
			return internalFailure(definition.path, pretty)
		}
		var bad *cursorValidationFailure
		decoded, bad = decodeExploreCursor(o.cursor, filterHash, key)
		if bad != nil {
			return failure(definition.path, bad.exitCode, bad.body, pretty)
		}
	}
	session, sessionFailure := acquireProtectedSession(ctx, definition.path, dependencies, pretty)
	if sessionFailure != nil {
		return *sessionFailure
	}
	client := remote.Client{HTTP: dependencies.HTTP}
	area := remote.DynamicArea{CountryCode: o.country, State: o.state, City: o.city}
	if o.section != "" {
		return executeExploreSection(ctx, definition, dependencies, pretty, o, filterHash, key, decoded, session, client, area)
	}
	return executeExploreFeed(ctx, definition, dependencies, pretty, o, filterHash, key, decoded, session, client, area)
}

func executeExploreFeed(ctx context.Context, definition commandDefinition, d Dependencies, pretty bool, o exploreOptions, hash [sha256.Size]byte, key []byte, cursor exploreCursorPayload, session auth.Session, client remote.Client, area remote.DynamicArea) Result {
	if o.cursorProvided && cursor.Mode != "feed" {
		f := invalidCursorFailure()
		return failure(definition.path, f.exitCode, f.body, pretty)
	}
	seen := make(map[string]bool, len(cursor.SeenIDs))
	for _, id := range cursor.SeenIDs {
		seen[id] = true
	}
	remoteCursor := cursor.RemoteCursor
	retained := make([]remote.DiscoverEvent, 0, o.limit)
	terminal := false
	for len(retained) < o.limit {
		page, err := client.GetDiscoverFeedV2(ctx, session.AccessToken, area, min(maximumCollectionLimit, o.limit-len(retained)), remoteCursor)
		if err != nil {
			return exploreRemoteFailure(definition.path, pretty, err)
		}
		progress := false
		for _, event := range page.Events {
			if !seen[event.ID] {
				seen[event.ID] = true
				progress = true
				if exploreDateMatch(event, o.date, nowFrom(d)) {
					retained = append(retained, event)
					if len(retained) == o.limit {
						break
					}
				}
			}
		}
		if page.NextCursor == nil {
			terminal = true
			break
		}
		if remoteCursor != nil && *page.NextCursor == *remoteCursor {
			return exploreProtocolFailure(definition.path, pretty)
		}
		if !progress {
			return exploreProtocolFailure(definition.path, pretty)
		}
		remoteCursor = page.NextCursor
	}
	items, err := decorateExplore(ctx, client, session.AccessToken, session.UserID, retained)
	if err != nil {
		return exploreRemoteFailure(definition.path, pretty, err)
	}
	hasMore := !terminal
	var next *string
	if hasMore {
		if key == nil {
			key, err = loadCursorKey(d)
			if err != nil {
				return internalFailure(definition.path, pretty)
			}
		}
		ids := mapKeys(seen)
		value, e := encodeExploreCursor(exploreCursorPayload{Version: 1, FilterHash: hex.EncodeToString(hash[:]), Mode: "feed", RemoteCursor: remoteCursor, SeenIDs: ids}, key, d.CursorRandom)
		if e != nil {
			return internalFailure(definition.path, pretty)
		}
		next = &value
	}
	complete := terminal || o.date == "anytime"
	return collectionSuccess(definition.path, exploreData{Items: items}, pageMeta{Limit: o.limit, NextCursor: next, HasMore: hasMore, FilterComplete: &complete}, pretty)
}

func executeExploreSection(ctx context.Context, definition commandDefinition, d Dependencies, pretty bool, o exploreOptions, hash [sha256.Size]byte, key []byte, cursor exploreCursorPayload, session auth.Session, client remote.Client, area remote.DynamicArea) Result {
	if o.cursorProvided && cursor.Mode != "section" {
		f := invalidCursorFailure()
		return failure(definition.path, f.exitCode, f.body, pretty)
	}
	sections, err := client.GetDiscoverSectionsV2(ctx, session.AccessToken, session.UserID, area)
	if err != nil {
		return exploreRemoteFailure(definition.path, pretty, err)
	}
	wanted := map[string]string{"trending": "area-dynamic-trending-events", "friends": "area-mutual-events", "open-invite": "area-open-invite", "followed": "area-followed-events"}[o.section]
	var events []remote.DiscoverEvent
	for _, section := range sections {
		if section.ID == wanted {
			events = section.Events
			break
		}
	}
	unique := make([]remote.DiscoverEvent, 0, len(events))
	seen := map[string]bool{}
	for _, event := range events {
		if !seen[event.ID] {
			seen[event.ID] = true
			if exploreDateMatch(event, o.date, nowFrom(d)) {
				unique = append(unique, event)
			}
		}
	}
	digest := exploreDigest(unique)
	offset := 0
	if o.cursorProvided {
		if cursor.Digest != digest {
			return failure(definition.path, 6, errorBody{Type: "state.conflict", Code: "CURSOR_SNAPSHOT_CHANGED", Message: "The Explore section changed after this cursor was issued.", Retryable: false, Details: map[string]any{}}, pretty)
		}
		if cursor.Offset < 0 || cursor.Offset > len(unique) {
			f := invalidCursorFailure()
			return failure(definition.path, f.exitCode, f.body, pretty)
		}
		offset = cursor.Offset
	}
	end := min(offset+o.limit, len(unique))
	items, err := decorateExplore(ctx, client, session.AccessToken, session.UserID, unique[offset:end])
	if err != nil {
		return exploreRemoteFailure(definition.path, pretty, err)
	}
	hasMore := end < len(unique)
	var next *string
	if hasMore {
		if key == nil {
			key, err = loadCursorKey(d)
			if err != nil {
				return internalFailure(definition.path, pretty)
			}
		}
		value, e := encodeExploreCursor(exploreCursorPayload{Version: 1, FilterHash: hex.EncodeToString(hash[:]), Mode: "section", Digest: digest, Offset: end}, key, d.CursorRandom)
		if e != nil {
			return internalFailure(definition.path, pretty)
		}
		next = &value
	}
	complete := true
	return collectionSuccess(definition.path, exploreData{Items: items}, pageMeta{Limit: o.limit, NextCursor: next, HasMore: hasMore, FilterComplete: &complete}, pretty)
}

func decorateExplore(ctx context.Context, client remote.Client, token, userID string, events []remote.DiscoverEvent) ([]exploreEvent, error) {
	if len(events) == 0 {
		return []exploreEvent{}, nil
	}
	ids := make([]string, len(events))
	for i, event := range events {
		ids[i] = event.ID
	}
	decorators, err := client.GetDiscoverEventItemDecorators(ctx, token, userID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]exploreEvent, 0, len(events))
	for _, event := range events {
		item := exploreEvent{EventID: event.ID, URL: "https://partiful.com/e/" + event.ID, Title: event.Title, Start: event.Start, End: event.End, Timezone: event.Timezone, LocationLines: []string{}, GoingGuestCount: event.GoingGuestCount, MaybeGuestCount: event.MaybeGuestCount, InterestedGuestCount: event.InterestedGuestCount, Mutuals: []exploreMutual{}, MutualsPreviewComplete: nil}
		if event.Location != nil {
			item.LocationLines = append(item.LocationLines, event.Location.DisplayAddressLines...)
			item.GoogleMapsURL = event.Location.GoogleMapsURL
		}
		for _, guest := range decorators.ByEventID[event.ID] {
			status, err := projectEventRSVP(&guest.Status)
			if err != nil {
				return nil, err
			}
			item.Mutuals = append(item.Mutuals, exploreMutual{DisplayName: guest.Name, Status: status})
		}
		out = append(out, item)
	}
	return out, nil
}

func nowFrom(d Dependencies) time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}
func exploreDateMatch(event remote.DiscoverEvent, facet string, now time.Time) bool {
	if facet == "anytime" {
		return true
	}
	if event.Start == nil || event.Timezone == nil {
		return false
	}
	location, err := time.LoadLocation(*event.Timezone)
	if err != nil {
		return false
	}
	start, err := time.Parse(time.RFC3339, *event.Start)
	if err != nil {
		return false
	}
	end := start
	if event.End != nil {
		end, err = time.Parse(time.RFC3339, *event.End)
		if err != nil {
			return false
		}
	}
	startDate, endDate, today := dateOnly(start.In(location)), dateOnly(end.In(location)), dateOnly(now.In(location))
	from, to := today, today
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	sunday := monday.AddDate(0, 0, 6)
	saturday := monday.AddDate(0, 0, 5)
	switch facet {
	case "tomorrow":
		from, to = today.AddDate(0, 0, 1), today.AddDate(0, 0, 1)
	case "this-week":
		if today.After(monday) {
			from = today
		} else {
			from = monday
		}
		to = sunday
	case "this-weekend":
		if today.After(saturday) {
			from = today
		} else {
			from = saturday
		}
		to = sunday
	}
	return !endDate.Before(from) && !startDate.After(to)
}
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
func exploreDigest(events []remote.DiscoverEvent) string {
	ids := make([]string, len(events))
	for i, event := range events {
		ids[i] = event.ID
	}
	sum := sha256.Sum256([]byte(strings.Join(ids, "\x00")))
	return hex.EncodeToString(sum[:])
}
func encodeExploreCursor(payload exploreCursorPayload, key []byte, random io.Reader) (string, error) {
	if random == nil {
		return "", errors.New("cursor random unavailable")
	}
	document, _ := json.Marshal(payload)
	aead, err := cursorAEAD(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(random, nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, document, []byte("partiful/explore/cursor/v1"))), nil
}
func decodeExploreCursor(token string, hash [sha256.Size]byte, key []byte) (exploreCursorPayload, *cursorValidationFailure) {
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return exploreCursorPayload{}, invalidCursorFailure()
	}
	aead, err := cursorAEAD(key)
	if err != nil || len(sealed) < aead.NonceSize()+aead.Overhead() {
		return exploreCursorPayload{}, invalidCursorFailure()
	}
	doc, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte("partiful/explore/cursor/v1"))
	if err != nil {
		return exploreCursorPayload{}, invalidCursorFailure()
	}
	var payload exploreCursorPayload
	decoder := json.NewDecoder(strings.NewReader(string(doc)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&payload) != nil || payload.Version != 1 || (payload.Mode != "feed" && payload.Mode != "section") {
		return exploreCursorPayload{}, invalidCursorFailure()
	}
	if payload.FilterHash != hex.EncodeToString(hash[:]) {
		return exploreCursorPayload{}, &cursorValidationFailure{exitCode: 2, body: errorBody{Type: "input.invalid", Code: "CURSOR_FILTER_MISMATCH", Message: "The cursor does not match this command and filters.", Retryable: false, Details: map[string]any{}}}
	}
	return payload, nil
}
func exploreRemoteFailure(command string, pretty bool, err error) Result {
	if errors.Is(err, remote.ErrUnavailable) {
		return remoteUnavailableFailure(command, pretty)
	}
	return exploreProtocolFailure(command, pretty)
}
func exploreProtocolFailure(command string, pretty bool) Result {
	return failure(command, 9, errorBody{Type: "contract.protocol_changed", Code: "EXPLORE_PROTOCOL_CHANGED", Message: "Explore no longer matches the reviewed remote contract.", Retryable: false, Details: map[string]any{}}, pretty)
}
