# Text Blasts

Text blasts send real SMS messages.

## Preview the blast

```bash
partiful blasts send <event-id>   --audience all-guests   --message-file blast.txt   --dry-run
```

The preview includes the message SHA-256 and length, not the raw message body.
Show the exact message from the local file before asking for approval.

## Send after approval

Run the command without `--dry-run` after explicit approval. It validates the
current event and audience, then makes one send attempt without prompting or
automatically retrying. Omit `--show-on-event-page` when the blast should stay
off the event activity feed.

## Safety checklist

- Correct event ID and title
- Exact final message text in the local file or stdin stream
- Approval received before the non-dry-run invocation
- No phone numbers, email addresses, or Partiful user IDs in summaries
