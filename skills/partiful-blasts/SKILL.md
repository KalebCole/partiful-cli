---
name: partiful-blasts
description: Send text blasts to event guests via Partiful
---

# Partiful CLI — Text Blasts

> ⚠️ **Real SMS messages.** Blasts send actual text messages to real people's phones. Partiful prepends: *"The host of {Event} sent a Text Blast —"* before your message.

## Commands

### Send a Text Blast
```bash
partiful blasts send <eventId> --message "See you tonight! Parking is on the left." --to GOING,MAYBE
partiful blasts send <eventId> --message "Updated address: 123 Main St" --to GOING --show-on-event-page
```

| Flag | Description |
|------|-------------|
| `--message <text>` | Message body (**max 480 characters**) |
| `--to <statuses>` | Comma-separated guest statuses to target (see below) |
| `--show-on-event-page` | Also display the blast on the event page |
| `--yes` | Skip the safety confirmation prompt |
| `--dry-run` | Preview recipients and message without sending |

### Valid `--to` Values

| Status | Description |
|--------|-------------|
| `GOING` | Confirmed guests |
| `MAYBE` | Tentative guests |
| `DECLINED` | Guests who declined |
| `SENT` | Invited but no response |
| `INTERESTED` | Expressed interest |
| `WAITLIST` | On the waitlist |
| `APPROVED` | Approved from waitlist |
| `RESPONDED_TO_FIND_A_TIME` | Responded to scheduling poll |

## Tips

- **Always use `--dry-run` first** to verify the recipient list and message.
- The 480-character limit is enforced by Partiful's API — the CLI will reject longer messages before sending.
- Without `--yes`, the CLI shows a confirmation prompt with the recipient count and message preview.
- Combine with `partiful guests list <eventId> --status going` to preview who will receive the blast.
