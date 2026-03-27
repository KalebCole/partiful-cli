# AGENTS.md — Partiful CLI

JSON-first CLI for managing Partiful events. No official API exists — this uses Partiful's internal Firebase/Firestore API.

## Commands

```bash
# Health check (run first to verify auth)
partiful doctor

# Auth
partiful auth status
partiful auth login <phone>        # SMS verification flow

# Events
partiful events list               # upcoming events
partiful events list --past        # past events
partiful events get <eventId>
partiful events create --title "..." --date "Mar 28 7pm" --poster-search "game night" --private -y
partiful events update <eventId> --title "New Title"
partiful events cancel <eventId>

# Guests
partiful guests list <eventId>                          # only works for events you HOST
partiful guests invite <eventId> --user-id <id1> <id2>  # invite by Partiful user ID
partiful guests invite <eventId> --phone +1234567890    # invite by phone number

# Contacts (your Partiful network)
partiful contacts list "kevin"     # search by name
partiful contacts list --limit 50  # browse all

# Co-hosts
partiful cohosts add <eventId> --name "Kevin Granados"  # resolved from contacts
partiful cohosts add <eventId> --user-id <userId>

# Text blasts (message all guests)
partiful blasts send <eventId> --message "Running 10 min late!" -y

# Posters
partiful posters search "game night"
partiful posters list

# Utilities
partiful +clone <eventId>          # clone event with new date
partiful +export <eventId>         # export event + guest list
partiful +share <eventId>          # generate shareable link
partiful +watch <eventId>          # poll for RSVP changes (NDJSON stream)
partiful schema [command.path]     # introspect any command's parameters
```

## Testing

```bash
npm test          # vitest run (all tests)
npm run test:watch # vitest watch mode
```

Tests live in `tests/`. Integration tests (`*.integration.test.js`) hit real APIs and require valid auth.

## Project structure

```
src/
├── cli.js              # Entry point, Commander setup
├── commands/           # One file per command group
│   ├── events.js
│   ├── guests.js
│   ├── contacts.js
│   ├── blasts.js
│   ├── cohosts.js
│   ├── posters.js
│   ├── auth.js
│   ├── schema.js
│   ├── templates.js
│   ├── bulk.js
│   ├── setup.js
│   └── doctor.js
├── helpers/            # Shared utilities
└── lib/                # Core library (API client, auth, Firebase)
tests/
├── *.test.js           # Unit tests
├── *.integration.test.js
└── fixtures/
```

## Non-obvious things agents get wrong

### Contacts don't expose emails or phone numbers
The `contacts list` endpoint returns names, user IDs, and shared event counts. It does **not** return email addresses or phone numbers. This is a Partiful privacy constraint, not a bug. Don't waste time trying to extract contact details — they aren't available.

### Guest lists are permission-gated
`guests list <eventId>` only works for events the authenticated user **hosts**. For events you're just attending, you'll get a 403. This is expected behavior.

### Image upload is unreliable
`--image <path>` on `events create` can fail with a 404 upload error. When this happens, fall back to `--poster <posterId>` or `--poster-search "query"` to use Partiful's built-in poster library instead. The poster library is extensive and searchable.

### The invite flow: contacts → user IDs → invite
To invite someone by name:
1. `partiful contacts list "name"` → get their `id` field
2. `partiful guests invite <eventId> --user-id <id>` → send invite

There is no `--name` flag on `guests invite`. You must resolve names to user IDs first via contacts.

### Auth: userId can be null and things still work
`partiful doctor` may flag `userId: null` in the config. The CLI still works for most operations — it authenticates via Firebase token, not userId. Don't treat this as a blocking error.

### Date parsing
`--date` accepts ISO 8601 (`2026-03-28T19:00:00`) or natural language (`Mar 28 7pm`). Default timezone is `America/Los_Angeles`. Always pass `--timezone` if the user is in a different zone.

### Confirmation prompts
Write commands (`create`, `cancel`, `invite`, `blasts send`) prompt for confirmation. Pass `-y` or `--yes` to skip in automated flows.

### Output format
All commands output JSON by default. Use `--format table|csv|ndjson` for alternatives. Agents should use the default JSON — it's structured and parseable.

## Code style

- Plain JavaScript (no TypeScript, no build step)
- Commander.js for CLI framework
- Vitest for testing
- `src/commands/` — one file per command group, each exports a function that registers subcommands
- `src/lib/` — API client, Firebase auth, HTTP helpers
- Favor explicit error handling with structured error objects (`{ status, error: { code, type, message } }`)

## Git workflow

- Branch from `main`
- Branch naming: `feat/description`, `fix/description`
- Run `npm test` before committing
- PR required for merge

## Boundaries

- ✅ **Always:** Run `partiful doctor` before assuming auth works. Pass `-y` for automated flows. Use JSON output.
- ⚠️ **Ask first:** Before sending blasts (messages real people), cancelling events, or bulk operations.
- 🚫 **Never:** Hardcode auth tokens. Expose phone numbers or user IDs in logs/output meant for display. Skip confirmation on destructive actions without user consent.
