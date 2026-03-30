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
# 1. Authenticate (grab tokens from partiful.com — see Auth Setup below)
partiful auth save --token <accessToken> --refresh <refreshToken> --user-id <userId>

# 2. Verify setup
partiful doctor

# 3. Create your first event
partiful events create --title "Game Night" --date "Apr 15 7pm" --location "My Place"

# 4. List your events
partiful events list

# 5. Invite and blast
partiful guests list <eventId>
partiful blasts send <eventId> --message "See you tonight!"
```

## Commands

### `auth` — Manage authentication

```bash
partiful auth save --token <token> --refresh <refresh> --user-id <id>
partiful auth status
partiful auth refresh
partiful auth clear
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
partiful contacts list
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

Partiful doesn't have a public API. This CLI uses the same internal API as the web app, authenticated via Firebase tokens.

1. Log into [partiful.com](https://partiful.com)
2. Open DevTools → Application → Local Storage → grab your auth tokens
3. Save them:

```bash
partiful auth save --token <accessToken> --refresh <refreshToken> --user-id <userId>
```

4. Verify: `partiful auth status`

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

Errors return `{ "status": "error", "error": { "code": 1, "type": "api_error", "message": "..." } }`.

Exit codes: `0` success · `1` API error · `2` auth error · `3` validation · `4` not found · `5` internal.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

MIT
