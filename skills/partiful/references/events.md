# Events

## Find and Inspect

```bash
partiful events list
partiful events list --past --include-cancelled
partiful events get <event-id>
```

List results include `myRsvp` and `isHost`; aggregate `going` and `maybe` fields are guest counts, not the authenticated user's status.

## Create

```bash
partiful events create \
  --title "Game Night" \
  --date "2026-08-01T19:00" \
  --timezone "America/Los_Angeles" \
  --location "My Place" \
  --description "🎮 Game Night!\n\nBring a favorite game." \
  --dry-run
```

`--title` and `--date` are required unless supplied by a template. Common options: `--end-date`, `--address`, `--capacity`, `--private`, `--theme`, `--effect`, `--poster`, `--poster-search`, `--image`, `--link`, `--link-text`, `--cohost`, `--template`, and `--var`.

Dates must include a full year. The default timezone is Pacific. Descriptions are plain text, not Markdown.

## Update and Cancel

```bash
partiful events update <event-id> --title "New Title" --dry-run
partiful events update <event-id> --date "2026-08-02T19:00" --location "New Venue" --dry-run
partiful events cancel <event-id> --dry-run
partiful events cancel <event-id> --yes
```

Get approval before cancellation. Verify event ID, title, date, timezone, and target fields first.

## Clone, Share Links, Templates, and Bulk Work

```bash
partiful +clone <event-id> --date "2026-09-01T19:00" --dry-run
partiful +share <event-id>
partiful template list
partiful template save --name <name> --title "Game Night" --location "My Place"
partiful events create --template <name> --date "2026-09-01T19:00"
partiful bulk --help
```

`+clone`, `+share`, and `+export` are top-level helper commands, not subcommands under `events`. Cloning copies event details, not guests. `+share` returns the event's shareable URL. Templates are assembled from explicit fields rather than imported from an event. Inspect `partiful template --help`, `partiful bulk --help`, or schema output before less common flows. Preview bulk operations and get approval before execution.

## Verify

After any write, inspect returned JSON and, when useful, run:

```bash
partiful events get <event-id>
```

Confirm title, full timestamp, timezone, location, visibility, and URL rather than relying only on exit status.
