# AGENTS.md — Partiful CLI

JSON-first CLI for managing Partiful events. No official API — uses Partiful's internal Firebase/Firestore API with reverse-engineered auth.

Run `partiful --help` and `partiful <command> --help` for command reference. Run `partiful schema [command.path]` for parameter introspection (e.g., `partiful schema events.create`).

## Things you will get wrong without reading this

### `myRsvp: SENT` means an inbound invite awaiting the user's reply
`events list` returns the authenticated user's raw Partiful RSVP state in `myRsvp`:

| `myRsvp` | Meaning | Category |
|---|---|---|
| `GOING` | User accepted the invitation | Going |
| `MAYBE` | User replied maybe | Maybe |
| `INTERESTED` | User marked interest | Interested |
| `DECLINED` | User declined the invitation | Declined |
| `SENT` | Invitation sent to the authenticated user; no RSVP reply yet | Awaiting RSVP |

`SENT` does not mean you sent an invitation. It is an inbound invitation awaiting your RSVP. When categorizing event lists, `isHost === true` takes precedence and means **Hosting**; otherwise use the `myRsvp` mapping. A `null` value means no personal guest RSVP record is present. Run `partiful schema events.list` for the machine-readable contract.

### Contacts don't expose emails or phone numbers
`contacts list` returns names, user IDs, and shared event counts. It does **not** return email addresses or phone numbers. This is a Partiful privacy constraint. Don't try to extract contact details — they aren't available through any endpoint.

### To invite someone by name, you need two steps
There is no `--name` flag on `guests invite`. You must resolve names to user IDs first:
1. `partiful contacts list "name"` → get the `id` field from the result
2. `partiful guests invite <eventId> --user-id <id>`

### Guest lists are permission-gated
`guests list <eventId>` only works for events the authenticated user **hosts**. Attending an event doesn't grant access — you'll get a 403. This is expected, not an error.

### Image upload fails silently — use posters instead
`--image <path>` on `events create` often fails with a 404 upload error. Fall back to `--poster <posterId>` or `--poster-search "query"` to use Partiful's built-in poster library. The library is extensive and searchable via `posters search`.

### Auth: userId can be null and things still work
`partiful doctor` may flag `userId: null`. The CLI authenticates via Firebase token, not userId — most operations work fine. Don't treat this as a blocking error.

On the next token refresh the CLI now **backfills `userId` from the token** automatically (decoded from the Firebase JWT), so `myRsvp` and `isHost` on `events list` are correct even for older `auth.json` files that predate userId capture. No re-login needed.

### Destructive commands require confirmation
`events cancel` and `blasts send` prompt for confirmation before executing. Pass `-y` or `--yes` to skip in automated/agent flows. Use `--dry-run` on any command to preview what would happen without side effects.

### Default timezone is America/Los_Angeles
`--date` accepts ISO 8601 or natural language (`Mar 28 7pm`). If the user isn't in Pacific time, pass `--timezone` explicitly or you'll create events at the wrong time.

## Testing

```bash
npm test           # vitest run
npm run test:watch # vitest watch
```

Integration tests (`tests/*.integration.test.js`) hit real Partiful APIs and need valid auth. Unit tests don't.

## Code conventions

- TypeScript strict mode; executed through the tsx loader (no checked-in build output)
- Commander.js CLI framework, Vitest for tests
- One file per command group in `src/commands/`
- Structured error objects: `{ status, error: { code, type, message } }`
- Do not add generated API crates or SDK wrappers — all API interaction goes through `src/lib/`

## Boundaries

- ✅ **Always:** Run `partiful doctor` before assuming auth works. Pass `-y` for agent flows. Use default JSON output.
- ⚠️ **Ask first:** Before sending text blasts (messages real humans), cancelling events, or running bulk operations.
- 🚫 **Never:** Hardcode auth tokens in source. Expose phone numbers or Partiful user IDs in user-facing output. Skip confirmation on destructive actions without explicit user consent.
