# Events

## Find and inspect

```bash
partiful events list --when upcoming
partiful events list --when past
partiful events get <event-id>
```

List results include the authenticated user's `myRsvp` and `userRole` when the
reviewed remote data supports them.

Use `--all --max-items 1000` for a bounded collection and check
`meta.page.hasMore` before claiming the list is complete. `sent` is an
unanswered invite; a hosted event may have `myRsvp: null`. A `start` value of
`TBD` is not a timestamp. Display dated events in their event timezone.

`events get` includes `visibility` and `guestLimit` when available. Its
`myRsvp` and `userRole` are null; use the list row for role and the list row
or `rsvp get` for RSVP status. Event URLs are `https://partiful.com/e/<event-id>`.

## Create

```bash
partiful events create   --title "Game Night"   --start 2026-08-01T19:00:00-07:00   --timezone America/Los_Angeles
```

Add `--dry-run` to validate and preview the normalized create request without
creating the event. Without `--dry-run`, the command dispatches once.

Approved create fields are `--title`, `--start`, `--end`, `--timezone`,
`--description`, `--location`, `--visibility`, `--guest-limit`, repeatable
`--link label=url`, and `--poster-id`. Use `--input <file-or->` for structured
JSON instead of field flags.

Successful creation returns `data.eventId` and `data.submitted: true`,
including when Partiful answers with a `result.data` event ID. Read the
normalized CLI `data.eventId`; do not expect a `result` envelope on stdout.
Use that ID for `events get`. If a create submission is uncertain and no ID
is returned, list upcoming events with `--all --max-items 1000`, continue
pagination as needed, and match the exact title and normalized start. Require
one match before reading it back; avoid creating a duplicate.

## Update

```bash
partiful events update <event-id>   --title "New Title"   --start 2026-08-08T19:00:00-07:00   --timezone America/Los_Angeles
```

Add `--dry-run` to preview an update after current event checks. Approved
update fields are `--title`, `--description`, `--start`, `--end`,
`--timezone`, `--guest-limit`, repeatable `--link label=url`, and
`--poster-id`.

## Cancel

```bash
partiful events cancel <event-id> --message "Event cancelled" --notify-guests true
```

Cancellation is destructive. Add `--dry-run` to preview it. Normal execution
prompts only on a TTY after validation and current event checks. Use `--force`
only after approval, or `--no-input` to fail rather than prompt.

## Delete

```bash
partiful events delete <event-id>
```

Deletion is destructive and applies only to an owned, published event with
zero guests. An event with guests returns `EVENT_PRECONDITION_FAILED`; use
`events cancel` for it instead. Add `--dry-run` to preview. Normal execution
asks `Delete event "<title>"? [y/N]` on a TTY; the default is no. Use `--force`
only after approval. `--force --no-input` permits an approved unattended
deletion; `--no-input` alone fails with `CONFIRMATION_REQUIRED` (exit 7).
`--dry-run` checks preconditions and returns a preview without prompting or
deleting. Success returns `data.eventId` and `data.submitted: true`.

## Verify

After any write, inspect returned JSON and, when useful, run:

```bash
partiful events get <event-id>
```
