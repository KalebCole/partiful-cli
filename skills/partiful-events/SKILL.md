---
name: partiful-events
description: Manage Partiful events — list, create, update, cancel, clone, set posters/images
---

# Partiful CLI — Events

## Commands

### List Events
```bash
partiful events list
partiful events list --past
partiful events list --past --include-cancelled
```

### Get Event Details
```bash
partiful events get <event-id>
partiful events get <event-id> --format table
```

### Create Event
```bash
partiful events create --title "Game Night" --date "2026-04-01T19:00" --location "My Place"
partiful events create --title "Birthday" --date "2026-05-15T20:00" --description "Bring snacks" --dry-run
```

| Flag | Description |
|------|-------------|
| `--title <title>` | Event title (required) |
| `--date <date>` | Start date/time (required) |
| `--end-date <date>` | End date/time |
| `--location <name>` | Location name |
| `--address <addr>` | Street address |
| `--description <text>` | Event description |
| `--capacity <n>` | Guest limit |
| `--private` | Make event private |
| `--timezone <tz>` | Timezone (default: `America/Los_Angeles`) |
| `--theme <theme>` | Color theme (default: `oxblood`) |
| `--effect <effect>` | Visual effect (default: `sunbeams`) |
| `--poster <id>` | Built-in poster ID (see `posters search`) |
| `--poster-search <query>` | Fuzzy-search poster catalog, use best match |
| `--image <path\|url>` | Upload a custom image (local file or URL) |

### Update Event
```bash
partiful events update <event-id> --title "New Title"
partiful events update <event-id> --date "2026-04-02T19:00" --location "New Venue"
partiful events update <event-id> --poster "piscesairbrush.png"
partiful events update <event-id> --image ./new-flyer.png
```
Accepts all the same flags as `create`.

### Cancel Event
```bash
partiful events cancel <event-id>
partiful events cancel <event-id> --yes          # skip confirmation
```

## Posters & Images

Three ways to set event imagery (all require Partiful auth since they're used on `events create/update`):

| Method | Flag | Extra Auth | Notes |
|--------|------|------------|-------|
| Built-in poster (by ID) | `--poster <id>` | None | Catalog is public; use `posters search` to find IDs |
| Built-in poster (fuzzy) | `--poster-search <query>` | None | Picks best match automatically |
| Custom upload | `--image <path\|url>` | Firebase token | File path or URL, 10MB limit |

```bash
# Find a poster
partiful posters search "birthday"

# Create with that poster
partiful events create --title "Birthday Bash" --date "2026-06-15T19:00" --poster "birthdaypresent.png"

# Or let fuzzy search pick one
partiful events create --title "Birthday Bash" --date "2026-06-15T19:00" --poster-search "birthday cake"

# Upload your own image
partiful events create --title "Party" --date "2026-06-15T19:00" --image ./flyer.png
partiful events create --title "Party" --date "2026-06-15T19:00" --image "https://example.com/poster.jpg"
```

## Helpers

### Clone Event (+clone)
```bash
partiful events +clone <event-id> --date "2026-06-01T19:00"
partiful events +clone <event-id> --date "2026-06-01T19:00" --title "Game Night v2"
```
Copies title, description, location, and settings. Guests are NOT copied (use `guests +share`).

## ⚠️ Formatting Rules

### Dates — Always Include Full Year
```text
✅ 2026-04-01T19:00
✅ 2026-04-01 7pm
❌ Apr 1 7pm
❌ 04/01 7pm
```
The CLI defaults to `America/Los_Angeles`. Pass `--timezone` explicitly if the event is in another timezone.

### Descriptions — Plain Text Only
Partiful renders descriptions as **plain text**. No markdown.
```text
✅ "🎮 Game Night!\n\nBring your favorite board games.\nSnacks provided.\n\n📍 Parking on the left side"
❌ "**Game Night!**\n\n- Bring your favorite board games\n- Snacks provided"
```
Use emoji for visual breaks. Use `\n` for line breaks. No `**`, no `-` lists, no `#` headers.

### Times — Double-Check AM vs PM
Early morning events (e.g., brunch at 10am) — verify you didn't accidentally set 10pm. The CLI will echo back the parsed time in its response.

## Tips
- Use `--dry-run` on create/update/cancel to preview changes without executing.
- Pipe `events list --format json` to `jq` for scripting.
- Event IDs are returned in list output and can be used across all commands.
