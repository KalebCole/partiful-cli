# Authentication

```bash
partiful auth login
partiful auth status
partiful auth logout
partiful doctor
```

`auth login` requires an interactive private terminal for the phone number
and SMS verification code. E.164 numbers are accepted; a bare 10-digit US
number is normalized to `+1`, and an 11-digit number starting with `1` gains
the `+` prefix. `--no-input` or `--non-interactive` prevents login prompts and
returns `auth.human_required`.

`auth status` can refresh stored credentials when they expire within five
minutes or have already expired. Log in again if credentials are missing or
refresh fails with `auth.expired`. `auth logout` removes local credentials.

Use `partiful doctor` before mutations. It reports whether credentials are
usable, missing, expired, invalid, or unavailable. It checks local state
without refreshing credentials or contacting Partiful.

Never log, display, or persist tokens outside the CLI credential store. Do not
include phone numbers or Partiful user IDs in user-facing summaries.
