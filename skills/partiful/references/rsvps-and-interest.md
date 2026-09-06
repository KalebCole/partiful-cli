# RSVPs and Interest

```bash
partiful rsvp get <event-id>
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

Ticketed, application, protected, at-capacity going, and unsupported
questionnaire flows fail closed.
