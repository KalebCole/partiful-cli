# Partiful CLI — Agentic CLI Quality Audit

**Date:** 2026-03-23
**Grading rubric:** [cli-architect SKILL.md](../../skills/cli-architect/SKILL.md) quality checklist
**File under review:** `/tmp/partiful-cli/partiful` (1600 lines, single-file Node.js CLI)

---

## 1. Overall Rating: 3/10

| Category | Score | Notes |
|----------|-------|-------|
| **Structured JSON output** | 2/10 | Most commands output human-readable text to stdout. `--json` flag exists on *some* commands but is opt-in, not default. Many commands mix `console.log` (stdout) human text with `console.error` (stderr) messages. No consistent JSON envelope (`{status, data, metadata}`). |
| **Exit codes** | 2/10 | Only uses `0` and `1`. No structured exit codes (the spec requires 0–5 mapping: success/api/auth/validation/not-found/internal). All failures exit with `process.exit(1)`. |
| **Error handling** | 2/10 | Errors go to `console.error` as plain strings, not JSON objects. No structured `{code, type, message}` error envelope. Catch-all at bottom (L1598) outputs bare `Error:` string. |
| **--help/--dry-run/--yes/--force** | 3/10 | `--help` exists. `--force` exists on cancel only. **No `--dry-run`** on any write command. **No `--yes`** flag. **No `--format`** flag (just `--json` boolean on some commands). No `--verbose`, `--output`, `--no-color`. |
| **HTTP client** | 2/10 | Raw `https.request` with zero retry logic, zero backoff, zero rate-limit awareness. No timeout configuration. No pagination controls (hardcoded `pageSize=100`). |
| **Auth design** | 4/10 | Token refresh with expiry check is solid. But: no credential precedence chain (no env var `PARTIFUL_TOKEN`), no `.env` support, hardcoded config path, no `auth status` JSON output. |
| **Skill/documentation readiness** | 2/10 | No SKILL.md files. No skills/ directory. README exists but doesn't follow GWS pattern. No schema introspection command. |
| **Testability** | 1/10 | Zero tests. Single monolithic file (1600 lines). No separation of concerns — HTTP, CLI parsing, output formatting, and business logic all interleaved. Custom arg parser instead of a library. |

---

## 2. What's Good

- **Auth flow is clever.** The bookmarklet-based IndexedDB extraction for Firebase auth (L283–380) is creative and works around Partiful's lack of a public API. Token refresh with expiry buffer (L77–93) is correctly implemented.
- **Feature coverage is solid.** List, get, create, update, cancel, clone, guests, invite, contacts, share, blast, watch — this covers the real workflows users need.
- **`cancel` has a confirmation gate** (L395–420) that fetches event details and prompts before destructing. This is good UX.
- **`clone` preserves duration** (L470–481) — smart detail.
- **Date parsing is robust** (L200–260) — handles relative dates ("tomorrow", "next Friday 7pm"), missing years, AM/PM.
- **Guest pagination** (L540–560) auto-paginates Firestore documents correctly.
- **CSV export** for guests (L580–592) — useful for data workflows.
- **`--watch` mode** for RSVP polling (L618–660) — nice for event hosts.

---

## 3. What's Bad

### Critical: No JSON-first output
Almost every command defaults to human-readable `console.log` output. An agent parsing this CLI has to pass `--json` (where supported) and still gets inconsistent shapes.

- **L330 (`authStatus`)**: Outputs `Auth Status:\n  User: ...` — plain text, no JSON option at all.
- **L350 (`listEvents`)**: Default output is emoji-decorated text. `--json` gives raw API response, not a `{status, data}` envelope.
- **L383 (`createEvent`)**: Success outputs `✓ Event created!\n  URL: ...` — no JSON by default.
- **L440 (`getEvent`)**: Same pattern — human text default, `--json` dumps raw API response.
- **L710 (`authLogout`)**: `console.log('✓ Logged out.')` — text only.

### Critical: No retry/backoff in HTTP client
- **L100–130 (`apiRequest`)**: Bare `https.request` with no retry on 429 or 5xx. A single transient failure kills the command.
- **L135–170 (`firestoreRequest`)**: Same — no retry.
- **L172–195 (`firestoreListDocuments`)**: Same.

### Critical: Exit code 1 for everything
- Every error path calls `process.exit(1)` regardless of whether it's an auth error, validation error, API error, or not-found. An agent cannot distinguish failure types without parsing stderr text.

### Major: No `--dry-run`
- `create`, `update`, `cancel`, `invite` — none support `--dry-run`. An agent cannot preview what will happen before committing.

### Major: Monolithic single file
- 1600 lines in one file. HTTP client, arg parsing, date utilities, auth, commands, and output formatting are all interleaved. This makes testing, reuse, and maintenance painful.

### Major: Custom arg parser
- **L734–752 (`parseArgs`)**: Hand-rolled arg parser doesn't handle `=` syntax (`--title="Foo"`), doesn't support short flags properly (only `-f` single char), can't handle repeated flags natively (phone collection at L885 works around this by re-scanning `process.argv`).

### Minor: Hardcoded values
- **L12**: `CONFIG_PATH` hardcoded to `~/.config/partiful/auth.json` — no env var override.
- **L304**: Localhost port `9876` hardcoded — no `--port` flag.
- **L14**: API key `AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k` hardcoded (L308) — should be in config.

### Minor: Inconsistent `_statusCode` injection
- **L120–125**: Response objects get `_statusCode` injected as a field, polluting the data. Should be handled separately.

---

## 4. Actionable Backlog

| Priority | Item | Description |
|----------|------|-------------|
| **P0** | JSON output envelope | Wrap ALL command output in `{status: "success", data: {...}, metadata: {...}}` on stdout. Move all human-readable text to stderr. Make `--json` the default; add `--format table` for human mode. |
| **P0** | Structured exit codes | Implement 0–5 exit code mapping: 0=success, 1=API error, 2=auth error, 3=validation error, 4=not found, 5=internal error. Map every `process.exit(1)` to the correct code. |
| **P0** | JSON error objects | Replace all `console.error('✗ Failed...')` with JSON error objects: `{status: "error", error: {code, type, message}}` on stdout. |
| **P0** | HTTP retry with backoff | Add exponential backoff + jitter for 429/5xx in `apiRequest`, `firestoreRequest`, `firestoreListDocuments`. Respect `Retry-After` header. Max 3 retries, 1s base, 30s cap. |
| **P1** | Standard flags | Add `--format`, `--dry-run`, `--yes`, `--verbose`, `--output`, `--no-color` as global flags. Wire `--dry-run` into all write commands. Wire `--yes` into cancel (alongside `--force`). |
| **P1** | Modularize into files | Split into `src/cli.js`, `src/lib/http.js`, `src/lib/output.js`, `src/lib/auth.js`, `src/lib/errors.js`, `src/commands/*.js`. |
| **P1** | Use a real arg parser | Replace custom `parseArgs` with `commander` or `yargs`. Get proper `--help` per subcommand, type validation, required arg enforcement. |
| **P1** | Auth credential precedence | Support `PARTIFUL_TOKEN` env var (priority 1), `PARTIFUL_CREDENTIALS_FILE` env var (priority 2), then file at default path (priority 3). Support `.env` loading. |
| **P1** | SKILL.md files | Create `skills/partiful-shared/SKILL.md`, `skills/partiful-events/SKILL.md`, `skills/partiful-guests/SKILL.md`, `skills/recipe-clone-event/SKILL.md`. |
| **P1** | Test scaffold | Add tests for: CRUD operations, auth flow, retry behavior, JSON output compliance, exit codes, `--dry-run` behavior. |
| **P2** | Pagination controls | Add `--page-size`, `--page-limit`, `--page-all` (NDJSON streaming) flags for `list`, `guests`, `contacts`. |
| **P2** | `schema` command | Add `partiful schema events.create` to introspect available fields and types — helps agents discover the API. |
| **P2** | `version` command | Add `partiful version` outputting `{status: "success", data: {version: "x.y.z", cli: "partiful"}}`. |
| **P2** | Timeout configuration | Add `PARTIFUL_TIMEOUT` env var and `--timeout` flag. Default 30s request, 10s connect. |
| **P2** | Remove `_statusCode` pollution | Handle HTTP status separately from response data. Don't inject into returned objects. |
| **P2** | GWS-style README | Rewrite README with: install, quick start, auth precedence table, command reference table, exit codes, env vars, architecture. |

---

## Summary

The Partiful CLI is a **functional prototype** — it covers the right features and has some smart design choices (auth flow, date parsing, clone duration preservation). But it was built for human use, not agent consumption. The lack of structured JSON output, meaningful exit codes, retry logic, and `--dry-run` support means an AI agent using this CLI must rely on string parsing and hope for the best. The path to agentic quality requires wrapping output in JSON envelopes (P0), adding structured exit codes (P0), and building retry into the HTTP layer (P0) — then the P1 items for proper flags, modularization, and skill documentation.
