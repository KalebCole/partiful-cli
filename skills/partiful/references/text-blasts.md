# Text Blasts

Text blasts send real SMS messages.

## Plan the blast

```bash
partiful blasts send <event-id>   --audience all-guests   --message-file blast.txt
```

This is a consequential action. By default it returns a plan that includes the
message SHA-256 and length, not the raw message body. Show the exact message
from your local file before asking for approval.

## Send after approval

Repeat the same command with `--apply --confirm <token>` after explicit
approval. Use `--show-on-event-page false` when the blast should stay off the
event activity feed.

## Safety checklist

- Correct event ID and title
- Exact final message text in the local file or stdin stream
- Approval received before `--apply --confirm`
- No phone numbers, email addresses, or Partiful user IDs in summaries
