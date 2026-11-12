# Authentication

```bash
partiful auth login
partiful auth status
partiful auth logout
partiful doctor
```

`auth login` is the local sign-in entry point. `doctor` is the safe local
smoke check for authentication state.

Use `partiful doctor` before mutations. It reports whether credentials are
present, expiring, expired, invalid, or unavailable without making a live
Partiful change.

Never log, display, or persist tokens outside the CLI credential store. Do not
include phone numbers or Partiful user IDs in user-facing summaries.
