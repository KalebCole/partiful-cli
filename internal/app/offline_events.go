package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/KalebCole/partiful-cli/internal/eventstore"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

func offlineDefinition(sync bool) commandDefinition {
	d := commandDefinition{path: "events.offline", invocation: []string{"events", "offline"}, kind: eventsOfflineCommand, positionals: []positionalDefinition{}, successSchema: eventCollectionSuccessSchema(), failureTypes: []string{"input.invalid", "internal.failure"}, safety: readOnlySafety()}
	d.flags = []flagDefinition{{Name: "--db", TakesValue: true, Description: "Local SQLite path; defaults to the user config directory under partiful/events.sqlite."}, {Name: "--limit", TakesValue: true, Description: "Maximum items, 1 through 1000; default 50 for offline, 1000 for sync."}}
	props := map[string]jsonSchema{"db": {Type: "string"}, "limit": {Type: "integer", Minimum: offlineInt(1), Maximum: offlineInt(1000)}}
	if sync {
		d.path = "events.sync"
		d.invocation = []string{"events", "sync"}
		d.kind = eventsSyncCommand
		d.safety = safetyDefinition{Kind: "local-mutation"}
		d.flags = append(d.flags, flagDefinition{Name: "--when", Required: true, TakesValue: true, Description: "Select past or upcoming; makes exactly one homepage list call."})
		props["when"] = jsonSchema{Type: "string", Enum: []string{"past", "upcoming"}}
		d.failureTypes = append(d.failureTypes, "auth.required", "auth.expired", "remote.unavailable", "contract.protocol_changed", "state.conflict")
		d.successSchema = objectSchema([]string{"synced", "available", "hasMore"}, map[string]jsonSchema{"synced": {Type: "integer"}, "available": {Type: "integer"}, "hasMore": {Type: "boolean"}})
	} else {
		d.flags = append(d.flags, flagDefinition{Name: "--status", TakesValue: true, Description: "RSVP filter; defaults to going. Use all for every cached status."}, flagDefinition{Name: "--since", TakesValue: true, Description: "Inclusive RFC 3339 start lower bound."}, flagDefinition{Name: "--until", TakesValue: true, Description: "Inclusive RFC 3339 start upper bound."})
		props["status"] = jsonSchema{Type: "string", Enum: append(eventReadRsvpValues(), "all")}
		props["since"] = jsonSchema{Type: "string", Format: "date-time"}
		props["until"] = props["since"]
	}
	required := []string{}
	if sync {
		required = []string{"when"}
	}
	d.inputSchema = objectSchema(required, props)
	return d
}
func offlineInputError(message string, valid any) *errorBody {
	return &errorBody{Type: "input.invalid", Code: "OFFLINE_OPTIONS_INVALID", Message: message, Details: map[string]any{"validValues": valid}}
}

func executeOfflineEvents(ctx context.Context, d commandDefinition, argv []string, deps Dependencies, pretty bool) Result {
	flags, inputErr := parseCommandFlags(d, argv)
	if inputErr != nil {
		return failure(d.path, 2, *inputErr, pretty)
	}
	sync := d.kind == eventsSyncCommand
	limit := 50
	if sync {
		limit = 1000
	}
	if v, ok := flags["--limit"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			return failure(d.path, 2, *offlineInputError("Limit must be 1 through 1000.", []int{1, 1000}), pretty)
		}
		limit = n
	}
	if sync && !slices.Contains([]string{"past", "upcoming"}, flags["--when"]) {
		return failure(d.path, 2, *offlineInputError("Select --when past or upcoming.", []string{"past", "upcoming"}), pretty)
	}
	status := "going"
	if v, ok := flags["--status"]; ok {
		status = v
	}
	if !sync && status != "all" && !slices.Contains(eventReadRsvpValues(), status) {
		return failure(d.path, 2, *offlineInputError("Invalid RSVP status.", append(eventReadRsvpValues(), "all")), pretty)
	}
	var since, until time.Time
	for _, entry := range []struct {
		flag string
		dest *time.Time
	}{{"--since", &since}, {"--until", &until}} {
		if v, ok := flags[entry.flag]; ok {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return failure(d.path, 2, *offlineInputError("Date bounds must be RFC 3339 timestamps.", []string{"2026-01-01T00:00:00Z"}), pretty)
			}
			*entry.dest = t
		}
	}
	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return failure(d.path, 2, *offlineInputError("Since must be at or before until.", []string{"since <= until"}), pretty)
	}
	path, provided := flags["--db"]
	if provided && strings.TrimSpace(path) == "" {
		return failure(d.path, 2, *offlineInputError("Database path must be nonempty.", []string{"filesystem path"}), pretty)
	}
	if !provided {
		dir, err := os.UserConfigDir()
		if err != nil {
			return configurationDirectoryFailure(d.path, pretty)
		}
		path = filepath.Join(dir, "partiful", "events.sqlite")
	}
	if !sync {
		store, err := eventstore.Open(path, true)
		if err != nil {
			return offlineStoreFailure(d.path, pretty)
		}
		defer store.Close()
		if status == "all" {
			status = ""
		}
		items, err := store.List(ctx, eventstore.Filter{Status: status, Since: flags["--since"], Until: flags["--until"], Limit: limit})
		if err != nil {
			return offlineStoreFailure(d.path, pretty)
		}
		return success(d.path, eventData{Items: items}, pretty)
	}
	session, sessionErr := acquireProtectedSessionWithPersistence(ctx, d.path, deps, pretty, false)
	if sessionErr != nil {
		return *sessionErr
	}
	device, err := randomDeviceID(deps.AuthRandom)
	if err != nil {
		return internalFailure(d.path, pretty)
	}
	client := remote.Client{HTTP: deps.HTTP}
	var catalog remote.EventCatalog
	if flags["--when"] == "past" {
		catalog, err = client.GetPastEvents(ctx, session.AccessToken, device)
	} else {
		catalog, err = client.GetUpcomingEvents(ctx, session.AccessToken, device)
	}
	if err != nil {
		if errors.Is(err, remote.ErrUnavailable) {
			return eventListUnavailableFailure(d.path, pretty)
		}
		if errors.Is(err, remote.ErrEventListBoundExceeded) {
			return eventListBoundFailure(d.path, pretty)
		}
		return eventListProtocolChangedFailure(d.path, pretty)
	}
	items := make([]eventstore.Event, 0, len(catalog.Events))
	for _, e := range catalog.Events {
		item, err := projectEventSummary(e, session.UserID)
		if err != nil {
			return eventListProtocolChangedFailure(d.path, pretty)
		}
		for _, v := range []*string{item.Start, item.End} {
			if v != nil {
				if _, err := time.Parse(time.RFC3339, *v); err != nil {
					return eventListProtocolChangedFailure(d.path, pretty)
				}
			}
		}
		items = append(items, item)
	}
	available := len(items)
	items = items[:min(limit, available)]
	store, err := eventstore.Open(path, false)
	if err != nil {
		return offlineStoreFailure(d.path, pretty)
	}
	defer store.Close()
	fingerprint := sha256.Sum256([]byte(session.UserID))
	err = store.Upsert(ctx, hex.EncodeToString(fingerprint[:]), items)
	if errors.Is(err, eventstore.ErrAccountMismatch) {
		return failure(d.path, 6, errorBody{Type: "state.conflict", Code: "OFFLINE_ACCOUNT_MISMATCH", Message: "This database belongs to another account. Select a different --db path.", Details: map[string]any{}}, pretty)
	}
	if err != nil {
		return offlineStoreFailure(d.path, pretty)
	}
	return success(d.path, struct {
		Synced    int  `json:"synced"`
		Available int  `json:"available"`
		HasMore   bool `json:"hasMore"`
	}{len(items), available, len(items) < available}, pretty)
}
func offlineStoreFailure(command string, pretty bool) Result {
	return failure(command, 10, errorBody{Type: "internal.failure", Code: "OFFLINE_STORE_UNAVAILABLE", Message: "Local event database could not be read or written. Sync first, or check the --db path and permissions.", Details: map[string]any{}}, pretty)
}

func offlineInt(v int) *int { return &v }
