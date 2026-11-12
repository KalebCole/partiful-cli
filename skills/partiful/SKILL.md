---
name: partiful
description: Use when the user wants to inspect Partiful events, RSVP, create or manage an event, invite or review guests, choose built-in posters, manage cohosts, or send a text blast.
---

# Partiful CLI

Operate Partiful through the JSON-first `partiful` CLI. Load only the
reference needed for the current task.

## Start Here

1. Run `partiful doctor` before assuming authentication works.
2. Keep default JSON output for agent workflows. Use `partiful schema <command.path>` when exact parameters matter.
3. Add `--dry-run` to any mutation for a redacted preview with no remote write.
4. Without `--dry-run`, a mutation validates, performs required read checks, and dispatches once.
5. `events cancel`, `cohosts remove`, `cohosts revoke-invite`, and `cohosts link revoke` prompt on a TTY. Use `--force` only after approval, or `--no-input` to fail rather than prompt.

## Route by Task

| Task | Read |
|---|---|
| Login, auth status, credential resolution, or authentication diagnostics | [Authentication](references/authentication.md) |
| Output envelopes, global flags, schema discovery, errors, or safety flow | [CLI output and safety](references/cli-output-and-safety.md) |
| List, inspect, create, update, or cancel events | [Events](references/events.md) |
| RSVP or express interest as an attendee | [RSVPs and interest](references/rsvps-and-interest.md) |
| List guests, resolve contacts, invite guests, or manage cohosts as a host | [Guests, invitations, and cohosts](references/guests-invitations-and-cohosts.md) |
| Browse built-in posters and apply poster IDs to events | [Posters and images](references/posters-and-images.md) |
| Draft, approve, or send a text blast | [Text blasts](references/text-blasts.md) |

Read multiple references only when a task crosses those boundaries.

## Universal Rules

- Event timestamps use RFC 3339. Event writes also require an IANA timezone such as `America/Los_Angeles`.
- Event descriptions are plain text, not Markdown.
- Never print authentication tokens. Do not expose phone numbers, email addresses, or Partiful user IDs in user-facing summaries.
- Guest lists are host-only. A `403` while attending but not hosting is expected.
- Contacts expose display names and shared-event counts, not email addresses or phone numbers.
- The approved Go CLI supports built-in poster IDs. It does not promise custom image upload in the public command catalog.
