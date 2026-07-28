---
name: partiful
description: Use when managing Partiful from the CLI, including authentication, events, RSVPs, guests, invitations, cohosts, contacts, posters, images, templates, exports, bulk operations, and text blasts.
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
| Login, auth status, diagnostics, output formats, global flags, schema, errors, security | [Auth, output, and safety](references/auth-output-and-safety.md) |
| List, inspect, create, update, cancel, clone, template, or bulk-manage events | [Events](references/events.md) |
| RSVP, express interest, list/export/watch guests, invite people, find contacts, or manage cohosts | [Guests and RSVPs](references/guests-and-rsvps.md) |
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
