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

Or use the browser automation to extract it automatically (see `extract-auth.js`).

## Usage

```bash
# Check auth status
partiful auth-status

# Create an event
partiful create --title "Game Night" --date "2026-04-15 7pm" --location "My Place"

# Create with capacity limit
partiful create --title "Birthday Party" --date "May 20 6:30pm" --capacity 20 --waitlist

# Create private event
partiful create --title "Secret Meeting" --date "Apr 1 8pm" --private

# Get event details
partiful get <eventId>
```

## Options

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

## API Notes

- Token refresh: `POST https://securetoken.googleapis.com/v1/token?key=<apiKey>`
  - Requires `Referer: https://partiful.com/` header
- Create event: `POST https://api.partiful.com/createEvent`
- All requests wrap body in `{ data: { params: {...}, userId, amplitudeDeviceId, amplitudeSessionId } }`

## License

MIT
