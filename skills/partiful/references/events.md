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
  --description $'🎮 Game Night!\n\nBring a favorite game.' \
  --dry-run
```

`--title` and `--date` are required unless supplied by a template. Common options: `--end-date`, `--address`, `--capacity`, `--rsvp-deadline`, `--private`, `--theme`, `--effect`, `--poster`, `--poster-search`, `--image`, `--link`, `--link-text`, `--cohost`, `--template`, and `--var`.

`--rsvp-deadline` accepts the same natural-language or ISO-style dates as `--date`, interpreted in `--timezone`. Responses after the deadline are disabled. For updates, pass `--timezone` explicitly when the deadline is not Pacific time:

```bash
partiful events update <event-id> \
  --rsvp-deadline "2026-08-20 12pm" \
  --timezone "America/Los_Angeles" \
  --dry-run
```

Add cohosts by unique Partiful contact name. This previews a post-create canonical cohost request, not a direct `cohostIds` write:

```bash
partiful events create --title "Game Night" --date "2026-08-01T19:00" \
  --cohost "Alex Smith" --dry-run
partiful events update <event-id> --cohost "Alex Smith" --dry-run
```

Get approval before executing because the request notifies another person and grants event controls after acceptance. See [Guests, invitations, and cohosts](guests-invitations-and-cohosts.md) for direct-request states and invite-link workflows.

Dates must include a full year. The default timezone is `America/Los_Angeles`. Descriptions are plain text, not Markdown.

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
partiful events clone <event-id> --dry-run
partiful events clone <event-id> --shift 14 --dry-run
partiful events clone <event-id> --date "2026-09-01T19:00" --dry-run
partiful events get <event-id>
partiful template list
partiful template save --name <name> --title "Game Night" --location "My Place"
partiful events create --template <name> --date "2026-09-01T19:00"
partiful bulk --help
```

`events clone` copies event details, not guests. Without `--date`, it shifts the source date forward seven days; use `--shift` to choose a different number of days. `events get` includes the shareable URL. `+export` remains a top-level helper command. Templates are assembled from explicit fields rather than imported from an event. Inspect `partiful events clone --help`, `partiful template --help`, `partiful bulk --help`, or schema output before less common flows. Preview bulk operations and get approval before execution.

## Verify

After any write, inspect returned JSON and, when useful, run:

```bash
partiful events get <event-id>
```

Confirm title, full timestamp, timezone, location, visibility, and URL rather than relying only on exit status.
