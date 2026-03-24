# Partiful CLI

JSON-first, agent-friendly CLI for managing [Partiful](https://partiful.com) events from the command line.

## Installation

```bash
npm install -g .
# or link for development
npm link
```

Requires **Node.js 18+**.

## Auth Setup

Partiful doesn't offer a public API. This CLI uses the same internal API that the web app uses, authenticated via Firebase tokens.

### Bookmarklet Flow

1. Log into [partiful.com](https://partiful.com) in your browser
2. Run the bookmarklet extractor or use browser DevTools to capture your auth tokens
3. Save credentials:

```bash
partiful auth save --token <accessToken> --refresh <refreshToken> --user-id <userId>
```

4. Verify:

```bash
partiful auth status
```

Tokens auto-refresh when expired.

## Quick Start

```bash
# List upcoming events
partiful events list

# Get event details
partiful events get <eventId>

# Create an event
partiful events create --title "Game Night" --date "Apr 15 7pm" --location "My Place"

# List guests
partiful guests list <eventId>

# Clone an event to next week
partiful +clone <eventId>

# Export event + guests
partiful +export <eventId> --format json --output event.json

# Share link
partiful +share <eventId>
```

## Command Reference

### Global Options

| Option | Description |
|--------|-------------|
| `--format <fmt>` | Output format: `json` (default), `table`, `csv`, `ndjson` |
| `--dry-run` | Preview request without executing |
| `-y, --yes` | Skip confirmation prompts |
| `--force` | Skip confirmation and overwrite protection |
| `-v, --verbose` | Show request details on stderr |
| `-o, --output <path>` | Write output to file |
| `--no-color` | Disable colored output |

### Commands

#### `auth` — Manage authentication

| Subcommand | Description |
|------------|-------------|
| `auth save` | Save auth credentials (`--token`, `--refresh`, `--user-id`) |
| `auth status` | Check current auth status |
| `auth refresh` | Force token refresh |
| `auth clear` | Remove saved credentials |

#### `events` — Manage events

| Subcommand | Description |
|------------|-------------|
| `events list` | List upcoming events (`--past` for past events) |
| `events get <id>` | Get event details |
| `events create` | Create a new event (`--title`, `--date`, `--location`, etc.) |
| `events update <id>` | Update event via Firestore (`--title`, `--date`, etc.) |
| `events cancel <id>` | Cancel an event |

#### `guests` — Manage event guests

| Subcommand | Description |
|------------|-------------|
| `guests list <eventId>` | List guests with RSVP status |
| `guests summary <eventId>` | Guest count summary by status |

#### `contacts` — Manage contacts

| Subcommand | Description |
|------------|-------------|
| `contacts list` | List your Partiful contacts |

#### `blasts` — Text blasts to event guests

| Subcommand | Description |
|------------|-------------|
| `blasts send <eventId>` | Send a text blast (`--message`, `--filter`) |
| `blasts history <eventId>` | View blast history |

### Helper Commands

Helpers use the `+` prefix to distinguish from core CRUD commands.

| Command | Description |
|---------|-------------|
| `+clone <eventId>` | Clone an event with shifted date (`--title`, `--date`, `--shift <days>`) |
| `+watch <eventId>` | Poll for guest RSVP changes as NDJSON (`--interval <s>`, `--duration <m>`) |
| `+export <eventId>` | Export event + guests to file (`--format json\|csv`, `--output <path>`) |
| `+share <eventId>` | Generate shareable event link |

#### `+clone` Examples

```bash
# Clone to next week (default: +7 days)
partiful +clone FDwyIXK42phoWEZgFin5

# Clone with specific date
partiful +clone FDwyIXK42phoWEZgFin5 --date "May 1 8pm"

# Clone with new title
partiful +clone FDwyIXK42phoWEZgFin5 --title "Game Night v2" --shift 14
```

#### `+watch` Example

```bash
# Watch for 30 minutes, polling every 15 seconds
partiful +watch FDwyIXK42phoWEZgFin5 --interval 15 --duration 30
```

Output (NDJSON):
```json
{"type":"rsvp_change","guest":{"name":"Alex","count":1},"from":"SENT","to":"GOING","timestamp":"..."}
{"type":"new_guest","guest":{"name":"Jordan","count":2},"from":null,"to":"GOING","timestamp":"..."}
```

#### `+export` Example

```bash
# Export as JSON
partiful +export FDwyIXK42phoWEZgFin5 --output party.json

# Export guest list as CSV
partiful +export FDwyIXK42phoWEZgFin5 --format csv
```

## JSON Envelope Format

All JSON output follows a consistent envelope:

### Success

```json
{
  "status": "success",
  "data": { ... },
  "metadata": {}
}
```

### Error

```json
{
  "status": "error",
  "error": {
    "code": 1,
    "type": "api_error",
    "message": "Description of what went wrong"
  }
}
```

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | API error |
| `2` | Authentication error |
| `3` | Validation error |
| `4` | Not found |
| `5` | Internal error |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and contribution guidelines.

## License

MIT
