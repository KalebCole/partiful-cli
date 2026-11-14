# RSVPs and Interest

```bash
partiful rsvp get <event-id>
partiful rsvp remove <event-id> [--dry-run] [--force] [--no-input]
partiful rsvp set <event-id> --status interested
partiful rsvp set <event-id> --status going   --display-name "Example"   --party-size 1   --timezone America/Los_Angeles
partiful rsvp set <event-id> --status not-going   --display-name "Example"   --party-size 1   --timezone America/Los_Angeles
```

`rsvp get` returns the reviewed RSVP state or `null`. It does not expose guest
or account IDs.

Add `--dry-run` to `rsvp set` for a redacted normalized request preview.
Without `--dry-run`, the command performs current event and guest checks, then
dispatches once without prompting.

For going RSVPs, use `--plus-one` repeatedly for named plus-ones. For larger or
structured input, pass `--input <path-or->` and inspect
`partiful schema rsvp.set` for the exact JSON shape.

Paid ticketing (`ticketing.type` `standard`), a required chip-in on a going
RSVP, application, protected, at-capacity going, and unsupported questionnaire
flows fail closed. The error names the event property. An optional chip-in or
suggested donation (`ticketing.mode` `optional`, type absent or `chip_in`)
uses the normal RSVP. Going sends `ticketPaid: true` without taking payment;
not-going omits `ticketPaid`. Stripe ticket sales never receive `ticketPaid`.

## Decline or leave

`rsvp set --status not-going` means **Can't Go** and keeps you listed as a guest.
It continues to send `addGuest` with `rsvp.status: DECLINED`.

`rsvp remove <event-id>` leaves the event entirely; it will no longer appear on
your homepage. The CLI resolves only your own guest ID with the same current
guest precondition as `rsvp set`. No guest record returns `state.conflict` /
`RSVP_GUEST_REQUIRED` (exit 6). `--dry-run` performs read-only resolution and
shows `removeGuest` with `guestId` redacted. Execution requires TTY confirmation;
use `--force` after user approval. `--no-input` and `--non-interactive` disable
prompting and fail with `CONFIRMATION_REQUIRED` (exit 7) unless forced.
The mutation dispatches once, without retries or follow-up reads.
