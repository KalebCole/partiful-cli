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
| `--no-input` | Disable terminal prompts; alias of `--non-interactive` |
| `--version` | Return the version envelope |

Every remote mutation also accepts `--dry-run`, `--force`, and `--no-input`.
`--dry-run` returns a redacted normalized preview after required read-only
resolution and never sends a write. Without it, the command dispatches once
and never retries automatically.

`events cancel`, `events delete`, `cohosts remove`, `cohosts revoke-invite`,
and `cohosts link revoke` prompt only on a TTY. `--force` skips only that prompt.
`--no-input`, `--non-interactive`, or a non-TTY invocation fails with
`safety.confirmation_required` unless `--force` is present.

Success and failure use stable JSON envelopes:

```json
{"ok":true,"data":{},"meta":{"command":"schema"}}
```

```json
{"ok":false,"error":{"type":"auth.required","code":"AUTH_REQUIRED","message":"Authentication is required.","retryable":false,"details":{}},"meta":{"command":"events.list"}}
```

## Collections

Collection commands return `data.items` and `meta.page` with `limit`,
`nextCursor`, and `hasMore`. The default limit is 25; `--limit` accepts 1–100.
To collect more, pass `--all --max-items <1-1000>` instead of `--limit`.
`--all` alone fails with `MAX_ITEMS_REQUIRED`. A capped result can still have
`hasMore: true`; continue with `--cursor <nextCursor>` using the same command
and filters. If the collection changes and the cursor is rejected, restart
the read instead of treating the old page as complete.

## Errors and safety

- Review `--dry-run` output when the user requests a preview.
- Get explicit approval before using `--force` for a destructive command.
- If a write returns `contract.protocol_changed` or reports an uncertain
  submission, it may already have succeeded. Inspect remote state before
  considering another attempt; never retry the write automatically.
- Keep JSON on stdout and diagnostics on stderr.
- Do not include phone numbers, email addresses, access tokens, or Partiful user IDs in user-facing summaries.
