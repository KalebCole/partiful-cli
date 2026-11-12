# Auth

Run `partiful auth status` before other commands, or `partiful doctor` if a check fails. Tokens expire about an hour after login. A stored refresh token is refreshed when the session is within five minutes of expiry or already expired. When `tokenState` is `expired` or `missing` after `auth status`, log in again (`partiful schema auth.status`).

`partiful auth login` is interactive: it prompts `Phone number:` then `Verification code:`. Drive it in tmux so the prompts can be answered. Enter the phone in E.164 (`+1XXXXXXXXXX`); a bare 10-digit number returned `AUTH_PROTOCOL_CHANGED`. Get the SMS code with the `get-otp` skill instead of asking the user. See `partiful help auth login` for flags that disable those prompts; login needs them.
