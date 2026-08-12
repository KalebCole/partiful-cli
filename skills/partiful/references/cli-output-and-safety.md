# CLI Output and Safety

## Discovery

```bash
partiful schema
partiful schema events.create
partiful schema guests.invite
partiful schema blasts.send
```

Treat `schema` output as authoritative when any human note and the installed
CLI differ.

## Global flags

| Flag | Purpose |
|---|---|
| `--pretty` | Indent JSON output without changing fields |
| `--non-interactive` | Disable terminal prompts |
| `--version` | Return the version envelope |

Standard mutations return a plan until you add `--apply --plan <token>`.
Consequential actions return a plan until you add `--apply --confirm <token>`
after approval.

Success and failure use stable JSON envelopes:

```json
{"ok":true,"data":{},"meta":{"command":"schema"}}
```

```json
{"ok":false,"error":{"type":"auth.required","code":"AUTH_REQUIRED","message":"Authentication is required.","retryable":false,"details":{}},"meta":{"command":"events.list"}}
```

## Safety

- Get explicit approval before sending blasts, cancelling events, inviting guests, changing cohosts, or creating/revoking cohost links.
- Keep JSON on stdout and diagnostics on stderr.
- Do not include phone numbers, email addresses, access tokens, or Partiful user IDs in user-facing summaries.
