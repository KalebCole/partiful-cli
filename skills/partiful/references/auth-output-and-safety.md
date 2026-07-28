# Auth, Output, and Safety

## Authentication

```bash
partiful auth login +12065551234
partiful auth login +12065551234 --code 123456
partiful auth login +12065551234 --no-auto
partiful auth status
partiful doctor
```

Login sends an SMS verification code. The CLI may retrieve it automatically on supported platforms; otherwise it prompts. Prefer E.164 phone numbers.

Credentials resolve in this order:

1. `PARTIFUL_TOKEN`
2. Credential file selected by `PARTIFUL_CREDENTIALS_FILE`
3. `~/.config/partiful/auth.json`

A `userId: null` diagnostic does not necessarily block operations because Firebase token authentication remains valid. A later refresh can backfill the user ID.

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
| `--format <json|table|csv|ndjson>` | Output format; JSON is default |
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

- Never log, display, or persist tokens outside the CLI credential store.
- Use `--dry-run` before mutations when available.
- Get explicit approval before sending blasts, cancelling events, inviting guests, or bulk actions.
- Keep JSON on stdout and diagnostics on stderr for reliable automation.
- Do not include phone numbers or Partiful user IDs in user-facing summaries.
