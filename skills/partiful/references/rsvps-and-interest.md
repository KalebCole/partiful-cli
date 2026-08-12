# RSVPs and Interest

```bash
partiful rsvp get <event-id>
partiful rsvp set <event-id> --status going --display-name "Example" \
  --party-size 1 --timezone America/Los_Angeles
partiful rsvp set <event-id> --status going --display-name "Example" \
  --party-size 2 --plus-one "Guest One" --timezone America/Los_Angeles
partiful rsvp set <event-id> --status not-going --display-name "Example" \
  --party-size 1 --timezone America/Los_Angeles
partiful rsvp set <event-id> --status interested
```

`rsvp get` returns the current reviewed RSVP status or `null`. It does not
return guest or account IDs.

`rsvp set` returns a five-minute, single-use plan by default. Review the plan,
then repeat the same normalized input with its token:

```bash
partiful rsvp set <event-id> --status interested \
  --apply --plan "$PLAN_TOKEN"
```

For a structured questionnaire response or larger input, use
`--input <path-or->`. Run `partiful schema rsvp.set` for its exact shape.
Ticketed, application, protected, at-capacity going, and unsupported
questionnaire flows fail closed. Applied success confirms only one submitted
request. It does not confirm persisted RSVP state.