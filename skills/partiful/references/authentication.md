# Authentication

```bash
partiful auth login +120****1234
partiful auth login +120****1234 --code 123456
partiful auth login +120****1234 --no-auto
partiful auth status
partiful doctor
```

Login sends an SMS verification code. The CLI may retrieve it automatically on supported platforms; otherwise it prompts. Prefer E.164 phone numbers.

Credentials resolve in this order:

1. `PARTIFUL_TOKEN`
2. Credential file selected by `PARTIFUL_CREDENTIALS_FILE`
3. `~/.config/partiful/auth.json`

A `userId: null` diagnostic does not necessarily block operations because Firebase token authentication remains valid. A later refresh can backfill the user ID.

Never log, display, or persist tokens outside the CLI credential store. Do not include phone numbers or Partiful user IDs in user-facing summaries.