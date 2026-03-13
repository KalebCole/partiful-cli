# Partiful CLI

Create and manage [Partiful](https://partiful.com) events from the command line — no browser needed after initial auth.

## Installation

```bash
# Clone the repo
git clone git@github.com:KalebCole/partiful-cli.git
cd partiful-cli

# Make executable and link
chmod +x partiful
ln -s $(pwd)/partiful ~/.local/bin/partiful
```

## Initial Setup

The CLI needs a Firebase refresh token from an authenticated Partiful session. One-time setup:

1. Log into Partiful in your browser (Chrome)
2. Open DevTools → Application → IndexedDB → `firebaseLocalStorageDb` → `firebaseLocalStorage`
3. Copy the auth data and create `~/.config/partiful/auth.json`:

```json
{
  "apiKey": "AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k",
  "refreshToken": "<your-refresh-token>",
  "userId": "<your-user-id>",
  "displayName": "Your Name",
  "phoneNumber": "+1234567890"
}
```

## Usage

### List Events
```bash
partiful list                    # Upcoming events
partiful list --past             # Past events
partiful list --json             # JSON output
```

### Get Event Details
```bash
partiful get <eventId>           # Human-readable
partiful get <eventId> --json    # JSON output
```

### Create Event
```bash
partiful create --title "Game Night" --date "2026-04-15 7pm" --location "My Place"
partiful create --title "Birthday" --date "May 20 6:30pm" --capacity 20 --waitlist
partiful create --title "Secret" --date "Apr 1 8pm" --private
```

### Clone Event
```bash
partiful clone <eventId> --date "Apr 22 7pm"                    # Clone with new date
partiful clone <eventId> --date "Apr 22 7pm" --title "Vol 2"    # Override title
partiful clone <eventId> --date "Apr 22 7pm" --reinvite going   # List guests to reinvite
```

### Cancel Event
```bash
partiful cancel <eventId>        # Prompts for confirmation
partiful cancel <eventId> -f     # Force (skip confirmation)
```

### Auth Status
```bash
partiful auth-status
```

## Create Options

| Option | Description |
|--------|-------------|
| `--title` | Event name (required) |
| `--date` | Start date/time (required) |
| `--end-date` | End date/time |
| `--location` | Venue name |
| `--address` | Street address |
| `--description` | Event description |
| `--capacity` | Guest limit |
| `--waitlist` | Enable waitlist (default: true) |
| `--private` | Make event private |
| `--timezone` | Timezone (default: America/Los_Angeles) |
| `--theme` | Color theme |
| `--json` | Output full JSON response |

## How It Works

1. Partiful uses Firebase Auth with phone/SMS verification
2. Firebase stores a **refresh token** in IndexedDB that lasts months
3. This CLI uses that refresh token to get fresh access tokens (~1hr life)
4. Access tokens are used to call `api.partiful.com` directly

The refresh token survives browser restarts. You only need to re-authenticate if:
- You explicitly log out of Partiful
- Firebase revokes the token (rare)
- Token expires after extended inactivity (~months)

## API Endpoints Used

- `POST /getMyUpcomingEventsForHomePage` - List upcoming events
- `POST /getMyPastEventsForHomePage` - List past events
- `POST /getEvent` - Get event details
- `POST /createEvent` - Create new event
- `POST /cancelEvent` - Cancel an event

All requests wrap body in `{ data: { params: {...}, userId, amplitudeDeviceId, amplitudeSessionId } }`

## Known Limitations

- **No update/edit** - Partiful uses Firestore directly for edits, not REST API
- **Delete = Cancel** - Events are cancelled, not deleted (Partiful's model)

## License

MIT
