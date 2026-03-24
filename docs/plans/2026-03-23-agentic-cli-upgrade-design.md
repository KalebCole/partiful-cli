# Partiful CLI — Agentic Upgrade Design Spec

**Date:** 2026-03-23
**Status:** Approved
**Audit:** `docs/agentic-cli-audit.md` (rated 3/10)
**Target:** 8+/10 on cli-architect quality checklist
**Reference:** cli-architect SKILL.md (gws pattern)

---

## Goal

Transform the Partiful CLI from a human-only prototype into a deterministic, JSON-first, agent-friendly CLI. Keep the existing feature set — every current command still works — but restructure internals for structured output, proper error handling, retry logic, and agent discoverability.

---

## Architecture Overview

```
partiful <resource> <method> [flags]
partiful <resource> +<helper> [flags]
partiful auth login|logout|status
partiful schema <resource>.<method>
partiful version
```

### Command Tree

```
partiful
├── auth
│   ├── login           # Bookmarklet flow (existing)
│   ├── logout          # Remove credentials
│   └── status          # Check token validity
├── events
│   ├── list            # List upcoming/past events
│   ├── get             # Get single event by ID
│   ├── create          # Create new event
│   ├── update          # Update event fields (Firestore PATCH)
│   ├── cancel          # Cancel event (destructive)
│   ├── +clone          # Get source → create with overrides → preserve duration
│   └── +share          # Get event → return share URL
├── guests
│   ├── list            # List guests for an event (Firestore paginated)
│   ├── invite          # Send invites by phone/user-id
│   ├── +watch          # Poll for RSVP changes (long-running)
│   └── +export         # Guests → CSV/JSON file
├── contacts
│   ├── list            # List/search contacts
├── blasts
│   ├── send            # Text blast (stub → web UI redirect)
├── schema              # Introspect command params
│   └── <resource>.<method>
└── version             # CLI version info
```

**Key change:** Commands move from `partiful list` to `partiful events list`. This is the `<resource> <method>` pattern. The old shortcuts (`partiful list`, `partiful get <id>`) will be kept as aliases during a transition period (1 version), printing a deprecation warning to stderr.

---

## Project Structure

```
partiful-cli/
├── package.json
├── .env.example
├── README.md
├── bin/
│   └── partiful              # Entry point (thin wrapper)
├── src/
│   ├── cli.js                # Commander setup, global flags, routing
│   ├── commands/
│   │   ├── auth.js           # login, logout, status
│   │   ├── events.js         # list, get, create, update, cancel
│   │   ├── guests.js         # list, invite
│   │   ├── contacts.js       # list
│   │   └── blasts.js         # send
│   ├── helpers/
│   │   ├── clone.js          # events +clone
│   │   ├── share.js          # events +share
│   │   ├── watch.js          # guests +watch
│   │   └── export.js         # guests +export
│   └── lib/
│       ├── http.js           # Retry, backoff, pagination, timeouts
│       ├── firestore.js      # Firestore-specific HTTP (PATCH, list docs)
│       ├── output.js         # JSON envelope, table/csv/yaml formatters
│       ├── auth.js           # Credential chain, token refresh
│       ├── errors.js         # PartifulError classes, exit code mapping
│       └── dates.js          # Date parsing (existing logic extracted)
├── skills/
│   ├── partiful-shared/SKILL.md
│   ├── partiful-events/SKILL.md
│   ├── partiful-guests/SKILL.md
│   └── recipe-clone-event/SKILL.md
├── tests/
│   ├── output.test.js
│   ├── http.test.js
│   ├── auth.test.js
│   ├── events.test.js
│   ├── guests.test.js
│   └── dates.test.js
└── docs/
    ├── agentic-cli-audit.md
    └── plans/
```

**Language:** Node.js (staying consistent with current). Using `commander` for arg parsing. No TypeScript — keep it simple, single `node` dependency.

**Dependencies (new):**
- `commander` — arg parsing
- `dotenv` — .env file loading

**Dev dependencies:**
- `vitest` — test runner

---

## JSON Output Contract

Every command outputs exactly one JSON object to stdout. No exceptions.

### Success
```json
{
  "status": "success",
  "data": { ... },
  "metadata": {
    "count": 5,
    "nextPageToken": null
  }
}
```

### Error
```json
{
  "status": "error",
  "error": {
    "code": 1,
    "type": "api_error",
    "message": "Partiful API returned 500: Internal Server Error",
    "details": { "statusCode": 500, "endpoint": "/getEvent" }
  }
}
```

### `--format` flag

| Format | Behavior |
|--------|----------|
| `json` (default) | Full JSON envelope to stdout |
| `table` | Human-readable table to stdout, JSON still available via pipe |
| `csv` | CSV with headers to stdout |
| `ndjson` | One JSON object per line (for paginated/streaming results) |

When `--format table`, the `data` field is rendered as a table. The JSON envelope is suppressed.

**Human text (progress, warnings, deprecation notices) → always stderr.**

---

## Exit Codes

| Code | Constant | When |
|------|----------|------|
| 0 | `SUCCESS` | Command completed |
| 1 | `API_ERROR` | Partiful/Firestore returned 4xx/5xx after retries |
| 2 | `AUTH_ERROR` | No credentials, expired token, refresh failed |
| 3 | `VALIDATION_ERROR` | Bad args, missing required flags, unknown command |
| 4 | `NOT_FOUND` | Event/guest/contact doesn't exist |
| 5 | `INTERNAL_ERROR` | Bug in CLI itself |

---

## Standard Flags (Global)

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--format <F>` | `-f` | Output format: json, table, csv, ndjson | `json` |
| `--dry-run` | | Preview request without executing | off |
| `--yes` | `-y` | Skip confirmation prompts | off |
| `--force` | | Skip confirmation + overwrite protection | off |
| `--verbose` | `-v` | Log request/response details to stderr | off |
| `--output <PATH>` | `-o` | Write output to file instead of stdout | stdout |
| `--no-color` | | Disable ANSI colors in table output | off |

**Interaction rules:**
- `--dry-run` beats `--yes`/`--force` — if dry-run is set, nothing executes
- `--yes` suppresses "Are you sure?" prompts
- `--force` suppresses confirmation AND allows destructive overwrite
- `--verbose` output goes to stderr only

---

## Auth Design

### Credential Precedence

| Priority | Source | How |
|----------|--------|-----|
| 1 | Environment token | `PARTIFUL_TOKEN` (Firebase access token) |
| 2 | Credentials file (env) | `PARTIFUL_CREDENTIALS_FILE` → path to auth.json |
| 3 | Credentials file (default) | `~/.config/partiful/auth.json` |
| 4 | Interactive login | `partiful auth login` (bookmarklet flow) |

### Token lifecycle
- Access tokens live ~1 hour. CLI auto-refreshes using the stored refresh token.
- If refresh fails → exit code 2, JSON error with `"type": "auth_error"`.
- `partiful auth status` returns JSON: `{ "status": "success", "data": { "user": "...", "tokenValid": true, "expiresIn": 3400 } }`

---

## HTTP Module (`src/lib/http.js`)

### Retry Strategy
- **Retryable:** 429, 500, 502, 503, 504
- **Not retryable:** All other 4xx (won't succeed on retry)
- **Max retries:** 3 (configurable via `PARTIFUL_MAX_RETRIES`)
- **Backoff:** Exponential with jitter — `min(30s, 2^attempt + random(0,1))`
- **`Retry-After` header:** Respected when present (both delta-seconds and HTTP-date)
- **Verbose mode:** Logs method, URL, status code, retry delay to stderr

### Timeouts
- **Request timeout:** 30s (configurable via `PARTIFUL_TIMEOUT`)
- **Connect timeout:** 10s

### Pagination (Firestore list)
- Auto-paginate by default (follow `nextPageToken`)
- `--page-size <N>` — documents per request (default: 100)
- `--page-limit <N>` — max pages to fetch (default: 10)
- `--page-all` — fetch all pages, stream as NDJSON

### Verbose logging
```
[stderr] POST api.partiful.com/getMyUpcomingEventsForHomePage (attempt 1)
[stderr] → 200 OK (342ms)
```

---

## Command Specifications

### `partiful events list`

```bash
partiful events list [--past] [--include-cancelled] [--format json|table|csv]
```

**Output shape:**
```json
{
  "status": "success",
  "data": {
    "events": [
      {
        "id": "FDwyIXK42phoWEZgFin5",
        "title": "Game Night",
        "startDate": "2026-04-15T02:00:00.000Z",
        "endDate": null,
        "location": "My Place",
        "status": "ACTIVE",
        "role": "hosting",
        "guestCounts": { "going": 8, "maybe": 2, "invited": 3, "declined": 1 },
        "url": "https://partiful.com/e/FDwyIXK42phoWEZgFin5"
      }
    ]
  },
  "metadata": { "count": 1 }
}
```

**Aliases:** `partiful list` → `partiful events list` (deprecated, stderr warning)

### `partiful events get <eventId>`

```bash
partiful events get <eventId> [--format json|table]
```

Returns full event object. If event not found → exit code 4.

### `partiful events create`

```bash
partiful events create --title "Party" --date "Apr 15 7pm" [options]
```

**Flags:** `--title` (required), `--date` (required), `--end-date`, `--location`, `--address`, `--description`, `--capacity`, `--waitlist`, `--private`, `--timezone`, `--theme`, `--effect`

**`--dry-run` behavior:** Shows the full API payload that would be sent, exits 0.

**Output:**
```json
{
  "status": "success",
  "data": {
    "id": "abc123",
    "title": "Party",
    "url": "https://partiful.com/e/abc123"
  }
}
```

### `partiful events update <eventId>`

```bash
partiful events update <eventId> --title "New Title" [--date] [--location] [--description] [--capacity]
```

**`--dry-run`:** Shows Firestore PATCH payload without sending.

If no update flags provided → exit code 3, validation error.

### `partiful events cancel <eventId>`

```bash
partiful events cancel <eventId> [--yes] [--force] [--dry-run]
```

Without `--yes`: fetches event details, shows title/date/guest count, prompts for confirmation on stderr.

With `--dry-run`: shows what would be cancelled, exits 0.

### `partiful events +clone <sourceEventId>`

```bash
partiful events +clone <sourceEventId> --date "Apr 22 7pm" [--title] [--location] [--reinvite going]
```

1. `GET` source event
2. `CREATE` new event with overrides (preserves duration if `--end-date` not given)
3. If `--reinvite`: lists guests to reinvite (phone/user-id output for piping to `guests invite`)

**Output:** Same as `events create`, plus `source` field.

### `partiful guests list <eventId>`

```bash
partiful guests list <eventId> [--status going|maybe|declined|invited] [--format json|table|csv] [--page-all]
```

**Output:**
```json
{
  "status": "success",
  "data": {
    "eventId": "abc123",
    "eventTitle": "Game Night",
    "counts": { "going": 8, "maybe": 2, "invited": 3, "declined": 1, "waitlist": 0 },
    "total": 14,
    "guests": [
      { "name": "Jane Doe", "status": "GOING", "count": 1, "inviteDate": "2026-04-01T..." }
    ]
  },
  "metadata": { "count": 14 }
}
```

### `partiful guests invite <eventId>`

```bash
partiful guests invite <eventId> --phone "+12065551234" [--phone "+1..."] [--user-id "abc"] [--message "Note"] [--dry-run]
```

### `partiful guests +watch <eventId>`

```bash
partiful guests +watch <eventId> [--interval 60] [--format ndjson]
```

Long-running. Polls every `--interval` seconds. Emits NDJSON change events:
```json
{"timestamp":"2026-04-15T20:00:00Z","type":"rsvp_change","field":"going","from":7,"to":8}
```

### `partiful guests +export <eventId>`

```bash
partiful guests +export <eventId> [--format csv|json] [--output guests.csv]
```

### `partiful contacts list`

```bash
partiful contacts list [<query>] [--limit 20] [--format json|table]
```

### `partiful blasts send <eventId>`

```bash
partiful blasts send <eventId> --message "Hey everyone!"
```

Currently a stub (Partiful uses Firestore SDK for blasts, not REST). Output:
```json
{
  "status": "error",
  "error": {
    "code": 5,
    "type": "not_implemented",
    "message": "Text blasts require Firestore SDK. Use web UI: https://partiful.com/e/<id>"
  }
}
```

### `partiful schema <resource>.<method>`

```bash
partiful schema events.create
```

Returns available parameters, types, required flags, defaults. Enables agent self-discovery.

```json
{
  "status": "success",
  "data": {
    "command": "events create",
    "parameters": {
      "--title": { "type": "string", "required": true, "description": "Event title" },
      "--date": { "type": "string", "required": true, "description": "Start date/time (natural language)" },
      "--end-date": { "type": "string", "required": false },
      "--location": { "type": "string", "required": false },
      "--capacity": { "type": "integer", "required": false },
      "--private": { "type": "boolean", "required": false, "default": false }
    }
  }
}
```

### `partiful version`

```json
{ "status": "success", "data": { "version": "2.0.0", "cli": "partiful" } }
```

---

## Migration & Backwards Compatibility

**Old-style commands get aliases with deprecation warnings:**

| Old | New | Warning |
|-----|-----|---------|
| `partiful list` | `partiful events list` | `[deprecated] Use "partiful events list"` (stderr) |
| `partiful get <id>` | `partiful events get <id>` | same pattern |
| `partiful guests <id>` | `partiful guests list <id>` | |
| `partiful cancel <id>` | `partiful events cancel <id>` | |
| `partiful clone <id>` | `partiful events +clone <id>` | |
| `partiful contacts` | `partiful contacts list` | |

Aliases live for 1 minor version (v2.0 → v2.1), then removed.

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PARTIFUL_TOKEN` | Firebase access token (priority 1) | — |
| `PARTIFUL_CREDENTIALS_FILE` | Path to auth.json (priority 2) | `~/.config/partiful/auth.json` |
| `PARTIFUL_TIMEOUT` | Request timeout in seconds | `30` |
| `PARTIFUL_MAX_RETRIES` | Max retry attempts for 429/5xx | `3` |
| `PARTIFUL_FORMAT` | Default output format | `json` |

---

## Skills

### `skills/partiful-shared/SKILL.md`
Auth precedence, global flags, exit codes, security rules, env vars.

### `skills/partiful-events/SKILL.md`
`events list|get|create|update|cancel`, `+clone`, `+share` — all params, examples, output shapes.

### `skills/partiful-guests/SKILL.md`
`guests list|invite`, `+watch`, `+export` — params, pagination, streaming.

### `skills/recipe-clone-event/SKILL.md`
End-to-end: clone an event with new date, reinvite guests, verify.

---

## Test Coverage

| Area | Tests |
|------|-------|
| Output | JSON envelope shape, table rendering, CSV headers, NDJSON streaming |
| HTTP | Retry on 429/5xx, no retry on 400, backoff timing, timeout |
| Auth | Token refresh, credential precedence, expired token → exit 2 |
| Events | CRUD happy paths, not-found → exit 4, validation → exit 3 |
| Guests | Pagination, status filtering, CSV export |
| Dates | Relative parsing, missing year, timezone, edge cases |
| Flags | `--dry-run` prevents execution, `--yes` skips prompts, `--format` switching |
| Exit codes | Every error type maps to correct code |

---

## Implementation Phases (for writing-plans)

1. **Scaffold** — Project structure, package.json, commander setup, `lib/` modules
2. **Core lib** — output.js, errors.js, http.js (retry), auth.js (credential chain), dates.js
3. **Commands** — Migrate each command to new structure with JSON output
4. **Helpers** — +clone, +share, +watch, +export
5. **Schema + Version** — Introspection commands
6. **Skills** — SKILL.md files
7. **Tests** — Full test suite
8. **Migration** — Aliases with deprecation, README rewrite

---

*This spec is the source of truth. Implementation plan follows via writing-plans skill.*
