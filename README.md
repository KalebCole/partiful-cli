![Banner](https://ghrb.waren.build/banner?header=partiful-cli+%F0%9F%8E%89&subheader=Manage+Partiful+events+from+your+terminal&bg=0d1117&color=f0f6fc&support=false)

# partiful-cli

> Manage [Partiful](https://partiful.com) events from your terminal. JSON-first, script-friendly.

[![npm version](https://img.shields.io/npm/v/partiful-cli)](https://www.npmjs.com/package/partiful-cli)
[![license](https://img.shields.io/npm/l/partiful-cli)](LICENSE)
[![node](https://img.shields.io/node/v/partiful-cli)](package.json)

## Try it now

```bash
npx partiful-cli --help
```

## Install

```bash
# Install globally
npm install -g partiful-cli

# Or run without installing
npx partiful-cli <command>

# Or clone and link
git clone https://github.com/KalebCole/partiful-cli && cd partiful-cli
npm install && npm link
```

### Install the Partiful agent skill

The package includes one Partiful skill. Install it globally for whichever agent you use:

```bash
partiful skill install hermes
partiful skill install openclaw
partiful skill install copilot
partiful skill install claude
```

Remove an installer-owned copy with `partiful skill uninstall <agent>`. For OpenClaw, uninstall also cleans legacy `partiful-*` links created by older releases; pass `--workspace <path>` when the old workspace was not at `~/.openclaw/workspace`.

Use `--dry-run` to preview filesystem changes. During installation, existing or locally modified skill directories are preserved unless `--force` is supplied. Uninstall never removes an unowned destination; an installer-owned copy with local edits also requires `--force`.

## Features

- 🎉 **Events** — create, list, get, update, cancel
- 👥 **Guests** — list RSVPs, send invites
- 📱 **Blasts** — text all your guests at once
- 🎨 **Posters** — browse and attach poster images
- 📋 **Templates** — save and reuse event configs
- 📦 **Bulk** — batch create/update from JSON
- 👀 **Watch** — live RSVP polling with NDJSON output
- 🔄 **Clone** — duplicate events to new dates
- 📤 **Export** — event + guests to JSON or CSV
- 🩺 **Doctor** — diagnose auth and setup issues

## Quick Start

```bash
# 1. Authenticate
partiful auth login +12065551234

# 2. Verify setup
partiful doctor

# 3. Create your first event
partiful events create --title "Game Night" --date "Apr 15 7pm" --location "My Place"

# 4. List your events
partiful events list

# 5. Invite and blast (use an eventId from step 4)
partiful guests list <eventId>
partiful blasts send <eventId> --message "See you tonight!"
```

## Commands

### `auth` — Manage authentication

```bash
partiful auth login +12065551234    # SMS-based auth
partiful auth status                # Check current auth
partiful auth logout                # Clear credentials
```

### `events` — Manage events

```bash
partiful events list                # Upcoming events
partiful events list --past         # Past events
partiful events get <id>            # Event details
partiful events create --title "Party" --date "May 1 8pm" --location "Rooftop"
partiful events update <id> --title "New Title"
partiful events cancel <id>
```

### `events rsvp` / `events interested` — RSVP to events

```bash
partiful events rsvp <id>                          # RSVP going (default)
partiful events rsvp <id> --status maybe           # going | maybe | declined
partiful events rsvp <id> --plus-one Maddie --plus-one Justin   # bring guests
partiful events rsvp <id> --name "Kaleb Cole" --message "Stoked!"
partiful events interested <id>                    # mark interest
partiful events interested <id> --remove           # remove interest
```

RSVP does a read-before-write: it updates your existing guest record if you
already RSVP'd, otherwise creates one. Ticketed and questionnaire-gated events
are refused with a clear error (use the app for those). The same verbs are
available under `explore` (`partiful explore rsvp <id>`) for the discovery flow.

### `guests` — Manage event guests

```bash
partiful guests list <eventId>      # All guests with RSVP status
partiful guests invite <eventId>    # Send invites
```

### `blasts` — Text guests

```bash
partiful blasts send <eventId> --message "Doors open at 7!"
```

### `contacts` — Manage contacts

```bash
partiful contacts list              # All contacts
partiful contacts list "alex"       # Search by name
```

### `cohosts` — Manage co-hosts

```bash
partiful cohosts list <eventId>
partiful cohosts add <eventId> --name "Alex" --name "Jordan"
partiful cohosts remove <eventId>
```

### `posters` — Browse poster catalog

```bash
partiful posters list
partiful posters search "birthday"
partiful posters get <posterId>
```

### `template` — Event templates

```bash
partiful template list                          # List saved templates
partiful template show <name>                   # Show template details
partiful template save --name "Game Night"       # Save a template
partiful template edit <name>                   # Edit a template
partiful template delete <name>                 # Delete a template
```

### `bulk` — Batch operations

```bash
partiful bulk create events.json                              # Create from JSON file
partiful bulk update --filter "title contains Game" --location "New Spot"  # Bulk update
```

### `schema` — Introspect command parameters

```bash
partiful schema events.create       # Show params for events create
```

### `doctor` — Health check

```bash
partiful doctor                     # Check auth, connectivity, setup
```

### Helper commands

Helpers use the `+` prefix:

```bash
# Clone an event to next week
partiful +clone <eventId>
partiful +clone <eventId> --date "May 1 8pm" --title "Game Night v2"

# Watch RSVPs in real-time (NDJSON stream)
partiful +watch <eventId> --interval 15 --duration 30

# Export event + guest list
partiful +export <eventId> --format json --output party.json
partiful +export <eventId> --format csv

# Get shareable link
partiful +share <eventId>
```

## Global Flags

`--format <fmt>` (json/table/csv/ndjson) · `--dry-run` · `-y, --yes` · `--force` · `-v, --verbose` · `-o, --output <path>` · `--no-color`

## Auth Setup

Partiful doesn't have a public API. This CLI authenticates via SMS verification through Firebase.

```bash
partiful auth login +12065551234   # sends SMS code, completes auth
partiful auth status               # verify you're logged in
```

**How it works:**
1. You provide your phone number (E.164 format, e.g. `+12065551234`)
2. Partiful sends an SMS verification code to that number
3. On macOS with [`imsg`](https://github.com/nicktomlin/imsg) installed, the code is auto-retrieved from Messages
4. Otherwise, you'll be prompted to enter it manually
5. Credentials are saved to `~/.config/partiful/auth.json`

Tokens auto-refresh when expired. Run `partiful doctor` if anything seems off.

## JSON Output

All commands support `--format json`. Responses follow a consistent envelope:

```json
{
  "status": "success",
  "data": { ... },
  "metadata": {}
}
```

`events list` returns one summary object per event. Each includes your **own** RSVP and host status:

```json
{
  "id": "BiwCtA9kRMh8Od5TvuPq",
  "title": "Skills & Drills",
  "startDate": "2026-07-08T01:30:00.000Z",
  "endDate": null,
  "location": null,
  "status": "PUBLISHED",
  "isHost": false,
  "myRsvp": "GOING",
  "going": 12,
  "maybe": 14,
  "url": "https://partiful.com/e/BiwCtA9kRMh8Od5TvuPq"
}
```

- **`myRsvp`** — your personal RSVP: `GOING`, `MAYBE`, `DECLINED`, `SENT` (invited, no reply yet), or `null` on events you host. Filter on this to sync only the events you've accepted, e.g. `partiful events list | jq '.data[] | select(.myRsvp == "GOING")'`.
- **`isHost`** — `true` for events you own. (`going`/`maybe` are aggregate counts across all guests, not your status.)

Errors return `{ "status": "error", "error": { "code": 1, "type": "api_error", "message": "..." } }`.

Exit codes: `0` success · `1` API error · `2` auth error · `3` validation · `4` not found · `5` internal.

## Contributing

```bash
git clone https://github.com/KalebCole/partiful-cli && cd partiful-cli
npm install
npm link          # makes `partiful` available globally for testing
npm test          # run tests
```

## License

MIT
