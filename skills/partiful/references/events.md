# Events

## Find and inspect

```bash
partiful events list --when upcoming
partiful events list --when past
partiful events get <event-id>
```

List results include the authenticated user's `myRsvp` and `userRole` when the
reviewed remote data supports them.

## Create

```bash
partiful events create   --title "Game Night"   --start 2026-08-01T19:00:00-07:00   --timezone America/Los_Angeles
```

By default this returns a reviewed plan token. Repeat the same normalized input
with `--apply --plan <token>` to execute it.

Approved create fields are `--title`, `--start`, `--end`, `--timezone`,
`--description`, `--location`, `--visibility`, `--guest-limit`, repeatable
`--link label=url`, and `--poster-id`. Use `--input <file-or->` for structured
JSON instead of field flags.

## Update

```bash
partiful events update <event-id>   --title "New Title"   --start 2026-08-08T19:00:00-07:00   --timezone America/Los_Angeles
```

`events update` uses the same plan flow as `events create`. Approved update
fields are `--title`, `--description`, `--start`, `--end`, `--timezone`,
`--guest-limit`, repeatable `--link label=url`, and `--poster-id`.

## Cancel

```bash
partiful events cancel <event-id> --message "Event cancelled" --notify-guests true
```

Cancellation is a consequential action. Review the plan, get approval, then
repeat the same command with `--apply --confirm <token>`.

## Verify

After any applied write, inspect returned JSON and, when useful, run:

```bash
partiful events get <event-id>
```
