---
name: partiful
description: Use when the user wants to check Partiful, discover events to attend, RSVP or express interest, create or manage an event, invite or review guests, choose event imagery, or send a text blast.
---

# Partiful CLI

Operate Partiful through the JSON-first `partiful` CLI. Load only the reference needed for the current task.

## Start Here

1. Run `partiful doctor` before assuming authentication works.
2. Keep default JSON output for agent workflows. Use `partiful schema <command.path>` or `<command> --help` when exact parameters matter.
3. Preview writes with `--dry-run` whenever supported.
4. Ask for confirmation before cancelling events, sending text blasts, inviting or modifying guests in bulk, or any other action that affects real people. Use `--yes` only after approval.

## Route by Task

| Task | Read |
|---|---|
| Login, auth status, credential resolution, or authentication diagnostics | [Authentication](references/authentication.md) |
| Output formats, global flags, schema discovery, errors, or cross-command safety | [CLI output and safety](references/cli-output-and-safety.md) |
| List, inspect, create, update, cancel, clone, template, or bulk-manage events | [Events](references/events.md) |
| RSVP to an event or express interest as an attendee | [RSVPs and interest](references/rsvps-and-interest.md) |
| List/export/watch guests, invite people, find contacts, or manage cohosts as a host | [Guests, invitations, and cohosts](references/guests-invitations-and-cohosts.md) |
| Browse posters, select event imagery, or upload a custom image | [Posters and images](references/posters-and-images.md) |
| Draft, preview, or send a text blast | [Text blasts](references/text-blasts.md) |

Read multiple references only when a task crosses those boundaries.

## Universal Rules

- Dates must include the full year. Default timezone is `America/Los_Angeles`; pass `--timezone` explicitly elsewhere. Verify the parsed date, time, and AM/PM in output.
- Event descriptions are plain text, not Markdown. Use line breaks and emoji rather than Markdown headings, bullets, or emphasis.
- Never print authentication tokens. Do not expose phone numbers or Partiful user IDs in user-facing summaries.
- Guest lists are host-only. A `403` while attending but not hosting is expected.
- Contacts expose names and user IDs, not email addresses or phone numbers.
- Prefer built-in posters when custom image upload fails.
