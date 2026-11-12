# Reading invites and RSVPs

The default `events list` page is 25 items. `meta.page.hasMore` is often true and is not in the success schema, so pass `--all --max-items` when the job is every invite (see `partiful help events list`).

`sent` means an unanswered invite. A hosted event may have `myRsvp` null. `start` can be the literal string `TBD`. Convert ISO times into the event's `timezone` before showing them.

`events get` is for event details. See `partiful schema events.get`: `myRsvp` and `userRole` are typed null. Take `userRole` from the `events list` row, and RSVP status from that row or from `rsvp get` (which returns `status`, not `userRole`).

The CLI lists only events tied to the authenticated account.
