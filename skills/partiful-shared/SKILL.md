---
name: partiful-shared
description: Shared auth, global flags, and security rules for the Partiful CLI
---

# Partiful CLI — Shared Patterns

## Authentication

### Login
```bash
partiful auth login
```
Opens a bookmarklet-based flow to capture your Partiful session token.

### Check Status
```bash
partiful auth status
```
Prints current auth state (logged in user, token expiry).

### Credential Resolution (priority order)
1. `PARTIFUL_TOKEN` environment variable
2. `~/.config/partiful/auth.json`

## Global Flags

| Flag | Description |
|------|-------------|
| `--format <json\|table\|csv>` | Output format (default: `json`) |
| `--dry-run` | Preview what would happen without making changes |
| `--yes` | Skip confirmation prompts |
| `--force` | Override safety checks |
| `--verbose` | Verbose logging |
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

## Security

- **Never** log or print tokens in output.
- Use `--dry-run` to preview destructive operations.
- Credentials stored in `~/.config/partiful/` with `0600` permissions.
- Token is excluded from `--verbose` output.
