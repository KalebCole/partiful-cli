---
name: partiful-shared
description: Shared auth, global flags, formatting rules, and security for the Partiful CLI
---

# Partiful CLI — Shared Patterns

## Authentication

### Login
```bash
partiful auth login <phone>
partiful auth login +12065551234
```
Phone-based authentication. Prefer **E.164 format** (e.g., `+12065551234`); US numbers without `+1` are normalized automatically.

1. Sends an SMS verification code.
2. If supported on your platform (macOS via `imsg`, Android via `termux-sms-list`), auto-retrieves the code.
3. Otherwise (or on timeout), prompts for manual code entry.

| Flag | Description |
|------|-------------|
| `--code <code>` | Provide verification code directly (skip SMS wait) |
| `--no-auto` | Disable automatic SMS retrieval |

### Check Status
```bash
partiful auth status
```

### Credential Resolution (priority order)
1. `PARTIFUL_TOKEN` environment variable
2. `~/.config/partiful/auth.json`

## Global Flags

| Flag | Description |
|------|-------------|
| `--format <json\|table\|csv>` | Output format (default: `json`) |
| `--dry-run` | Preview without making changes |
| `--yes` | Skip confirmation prompts |
| `--force` | Override safety checks |
| `--verbose` | Verbose logging to stderr |
| `--output <file>` | Write output to file |
| `--no-color` | Disable colored output |

## JSON Envelope

### Success
```json
{ "status": "success", "data": { ... }, "metadata": { "count": 5 } }
```

### Error
```json
{ "status": "error", "error": { "code": 2, "type": "AUTH_ERROR", "message": "Token expired" } }
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | API error |
| 2 | Auth error |
| 3 | Validation error |
| 4 | Not found |
| 5 | Internal error |

## ⚠️ Formatting Rules (Important!)

### Dates — Always Include Full Year
```text
✅ 2026-04-01T19:00
✅ 2026-04-01 7pm
❌ Apr 1 7pm
❌ 04/01 7pm
```
Default timezone: `America/Los_Angeles`. Use `--timezone` for other zones.

### Descriptions — Plain Text Only
Partiful renders plain text. **No markdown.**
```text
✅ "🎮 Game Night!\n\nBring your favorite board games.\nSnacks provided."
❌ "**Game Night!**\n\n- Bring board games\n- Snacks provided"
```
Use emoji for visual breaks. Use `\n` for newlines. No `**`, `-` lists, or `#` headers.

### Times — Verify AM vs PM
Always double-check AM/PM, especially for morning events. The CLI echoes back the parsed time.

## Schema Introspection

Discover command parameters programmatically:
```bash
partiful schema events.create
partiful schema guests.invite
```

## Security

- **Never** log or print tokens in output.
- Use `--dry-run` to preview destructive operations.
- Credentials stored in `~/.config/partiful/` with `0600` permissions.
- Token is excluded from `--verbose` output.
