# Explore V1 Design

Status: Approved by the issue owner’s implementation request for issue #102.

Date: 2026-09-05

## Goal

Add one authenticated, read-only `partiful explore list` operation that discovers public events for an explicit dynamic area, supports reviewed social sections and event-local date facets, and preserves the CLI’s bounded JSON collection contract.

## Scope

The only new remote operations are `getDiscoverFeedV2`, `getDiscoverSectionsV2`, and `getDiscoverEventItemDecorators`. The command requires explicit city, state, and country fields and accepts stable section values `trending`, `friends`, `open-invite`, and `followed`, plus date filters `anytime`, `today`, `tomorrow`, `this-week`, and `this-weekend`.

Tags, map pins, date-range callables, arbitrary search, public Next.js fallback, Explore mutations, RSVP/interest aliases, raw transport escape hatches, and retired TypeScript compatibility are excluded.

## Contract authority

Remote and product contract revision `2026-09-05.1` promotes only facts supported by issue #206, its sanitized research synthesis, and the throwaway Explore prototype. The evidence ledger identifies request, response, status, and schema claims individually. Fields or failures not supported by that evidence remain nullable or explicit unknowns. Raw response bodies, credentials, private user/owner/guest identifiers, and real display names never enter the repository.

## Architecture

### Remote transport

`internal/remote/explore.go` owns typed request envelopes, response bounds, strict JSON decoding, and wire-to-remote model projection. It preserves the Firebase callable `{ "data": ... }` envelope and bearer authentication. Feed pages carry a reviewed dynamic area, `DISCOVER_HOME`, rows presentation, a bounded `maxResults`, and an optional opaque cursor. Section lookup uses the reviewed locale and presentation styles and maps generated remote section IDs internally. Decorator requests contain only a bounded event-ID list and the authenticated user ID required by the reviewed shape.

The transport accepts only reviewed HTTP 200 JSON success variants. No response maps to `remote.unavailable`; every unreviewed received status, malformed reviewed field, repeated cursor, non-JSON body, or size/item bound breach maps to `contract.protocol_changed`. The existing session refresh path remains the sole authentication path.

### Application operation

`internal/app/explore.go` owns input parsing, stable product enums, pagination orchestration, deduplication, event-local overlap filtering, privacy-safe projection, and structured failures. `internal/app/app.go` adds one `explore.list` command definition and one dispatch case; schema and help continue to derive from that same catalog entry.

Feed cursors are signed local envelopes containing only the remote continuation state needed for resumption and are bound to the normalized command filters. Section responses are one reviewed generated-section representation and use digest-bound local snapshot pagination. `--all` requires `--max-items`, fetches only until terminal or the hard bound, and keeps the first occurrence of each event ID. A repeated remote cursor or a page that cannot make progress fails closed.

### Output

Each item always contains all documented fields:

- non-null `eventId` and derived public `https://partiful.com/e/{eventId}` URL;
- reviewed title/start/end/timezone values, nullable where presence is not proven;
- privacy-safe location display lines and reviewed Google Maps URL, nullable when hidden or unavailable;
- nullable going, maybe, and interested counts;
- mutual previews containing only display name and normalized closed guest status; and
- `mutualsPreviewComplete: null` until completeness is proven.

No private remote identifiers appear in output or failures. Empty feeds, missing or empty selected sections, and no-mutual responses are successful empty values.

### Date filtering

`anytime` performs no date filtering. Other facets convert each event start/end and the injected current time into the event’s IANA timezone, compare inclusive local calendar dates, and retain events whose local date span overlaps the facet:

- `today`: today through today;
- `tomorrow`: tomorrow through tomorrow;
- `this-week`: later of today or Monday through Sunday;
- `this-weekend`: later of today or Saturday through Sunday.

An absent end uses the start date. A date-filtered feed reports `filterComplete:false` while an unread remote cursor remains and `true` at terminal traversal. Section filtering is complete for the returned one-response section representation.

## Testing

Tests are fixture-backed and test-first. Remote tests cover exact requests, bounds, nullable variants, malformed variants, reviewed status behavior, empty feed/sections/decorators, and cursor parsing. Application tests cover required area fields, aliases, auth failures, output privacy, hidden/approximate locations, mutual uncertainty, date overlap boundaries, section mapping, cursor/filter binding, duplicate prevention, and `--all --max-items`. Catalog tests cover schema and help.

The final branch is verified with all requested Go, race, vet, build, PII, native release, whitespace, branch-built schema/help, and authenticated read-only Seattle checks. No event or RSVP mutation is invoked.
