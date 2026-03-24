---
name: partiful-events
description: Manage Partiful events — list, create, update, cancel, clone
---

# Partiful CLI — Events

## Commands

### List Events
```bash
partiful events list
partiful events list --status upcoming --format table
partiful events list --status past --limit 5
```

### Get Event Details
```bash
partiful events get <event-id>
partiful events get <event-id> --format json
```

### Create Event
```bash
partiful events create --title "Game Night" --date "2026-04-01T19:00" --location "My Place"
partiful events create --title "Birthday" --date "2026-05-15T20:00" --description "Bring snacks" --dry-run
```

### Update Event
```bash
partiful events update <event-id> --title "New Title"
partiful events update <event-id> --date "2026-04-02T19:00" --location "New Venue"
```

### Cancel Event
```bash
partiful events cancel <event-id>
partiful events cancel <event-id> --yes          # skip confirmation
```

## Helpers

### Clone Event (+clone)
Duplicate an existing event with a new date:
```bash
partiful events +clone <event-id> --date "2026-06-01T19:00"
partiful events +clone <event-id> --date "2026-06-01T19:00" --title "Game Night v2"
```
Copies title, description, location, and settings. Guests are NOT copied (use `guests +share` to re-invite).

## Tips
- Use `--dry-run` on create/update/cancel to preview changes.
- Pipe `events list --format json` to `jq` for scripting.
- Event IDs are returned in list output and can be used across all commands.
