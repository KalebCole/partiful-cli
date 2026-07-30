# Text Blasts

Text blasts send real SMS messages. Partiful prepends its own host/event attribution.

## Draft and Preview

Messages are limited to 480 characters. Valid target statuses: `GOING`, `MAYBE`, `DECLINED`, `SENT`, `INTERESTED`, `WAITLIST`, `APPROVED`, and `RESPONDED_TO_FIND_A_TIME`.

```bash
partiful guests list <event-id> --status GOING
partiful blasts send <event-id> \
  --message "See you tonight. Parking is on the left." \
  --to GOING,MAYBE \
  --dry-run
```

Before sending, show the exact message, event, target statuses, and dry-run result. Get explicit approval. Never infer approval from a request to draft or preview.

## Send

```bash
partiful blasts send <event-id> \
  --message "See you tonight. Parking is on the left." \
  --to GOING,MAYBE \
  --yes
```

Blasts appear on the event page by default. Use `--no-show-on-event-page` to hide one from the activity feed.

## Safety Checklist

- Correct event ID and title
- Exact final text, at most 480 characters
- Correct target statuses
- Dry run completed
- Explicit approval received
- `--yes` added only after approval

Report delivery result and aggregate targeting, but do not expose recipient phone numbers or Partiful user IDs.
