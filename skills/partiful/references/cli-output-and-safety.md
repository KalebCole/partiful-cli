# CLI Output and Safety

## Discovery

```bash
partiful --help
partiful events create --help
partiful schema events.create
partiful schema guests.invite
```

Treat live help and schema output as authoritative when this reference and the installed CLI differ.

## Global Flags

| Flag | Purpose |
|---|---|
| `--format <json, table, csv, or ndjson>` | Output format; JSON is default |
| `--dry-run` | Preview without executing |
| `-y, --yes` | Skip confirmation after approval |
| `--force` | Override confirmation or overwrite protection after approval |
| `-v, --verbose` | Write request diagnostics to stderr |
| `-o, --output <path>` | Write output to a file |
| `--no-color` | Disable color |

Success and failure use structured envelopes:

```json
{"status":"success","data":{},"metadata":{}}
```

```json
{"status":"error","error":{"code":2,"type":"auth_error","message":"Token expired"}}
```

Exit codes: `0` success, `1` API, `2` auth, `3` validation, `4` not found, `5` internal.

## Safety

- Use `--dry-run` before mutations when available.
- Get explicit approval before sending blasts, cancelling events, inviting guests, or bulk actions.
- Keep JSON on stdout and diagnostics on stderr for reliable automation.
- Do not include phone numbers or Partiful user IDs in user-facing summaries.