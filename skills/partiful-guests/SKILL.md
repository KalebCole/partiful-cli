---
name: partiful-guests
description: Manage Partiful event guests — list RSVPs, send invites, watch changes, export
---

# Partiful CLI — Guests

## Commands

### List Guests / RSVPs
```bash
partiful guests list <event-id>
partiful guests list <event-id> --status going --format table
partiful guests list <event-id> --status maybe,declined
```

### Send Invites
```bash
partiful guests invite <event-id> --phone "+12065551234"
partiful guests invite <event-id> --phones "+12065551234,+12065555678"
partiful guests invite <event-id> --name "Alex Smith"    # lookup from contacts
```

## Helpers

### Watch for RSVP Changes (+watch)
Stream RSVP changes in real-time:
```bash
partiful guests +watch <event-id>
partiful guests +watch <event-id> --format json    # NDJSON stream
partiful guests +watch <event-id> --interval 30    # poll every 30s
```
Outputs new RSVPs and status changes as they arrive.

### Export Guest List (+export)
```bash
partiful guests +export <event-id> --format csv --output guests.csv
partiful guests +export <event-id> --format json --output guests.json
partiful guests +export <event-id> --status going --format table
```

### Share / Re-invite (+share)
Bulk invite guests from a previous event:
```bash
partiful guests +share <source-event-id> --to <target-event-id>
partiful guests +share <source-event-id> --to <target-event-id> --status going  # only those who went
partiful guests +share <source-event-id> --to <target-event-id> --dry-run
```

## Tips
- Guest statuses: `going`, `maybe`, `declined`, `invited` (no response yet).
- `+watch` is useful for day-of monitoring of RSVPs.
- `+export` respects `--status` to filter before export.
- `+share` combined with `events +clone` enables full event duplication.
