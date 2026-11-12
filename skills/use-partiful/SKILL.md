---
name: use-partiful
description: Use when reading Partiful invites and RSVP status, RSVPing, creating or hosting Partiful events, managing guest lists, or sending text blasts.
---

# Use Partiful

Use the installed `partiful` CLI. Look up flags, required fields, enums, and output shapes with `partiful help <group> <cmd>` and `partiful schema <command.path>` (for example `partiful schema rsvp.set`). Commands print a JSON envelope `{ok, data|error, meta}` by default; do not pass `--format`, which is an unknown flag. Event URLs are `https://partiful.com/e/<eventId>`.

One exception uses the browser: a custom cover image after create (see `references/hosting-events.md`).

RSVP changes, event create/update/cancel, guest invites, cohost changes, and text blasts are visible to hosts and guests, so get one explicit yes first. Show the `--dry-run` result or the exact draft when asking. After that yes, invoke the live command once. Mutation help documents `--dry-run`, `--force`, and `--no-input`; use `--force --no-input` on the live call so it does not hang on a confirmation prompt.

A `contract.protocol_changed` error after a write can mean the remote call already succeeded. Do not retry from that result alone; read back instead.

Read the matching reference when the job needs it:

- `references/auth.md` when status is missing or expired, or login is needed
- `references/reading-invites.md` when listing or inspecting invites, RSVP, or role
- `references/rsvp.md` when setting an RSVP
- `references/hosting-events.md` when creating, updating, or canceling an event, or writing event copy
- `references/guests-and-blasts.md` when listing or inviting guests, changing cohosts, or sending blasts
