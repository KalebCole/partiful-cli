# RSVPs and Interest

```bash
partiful events rsvp <event-id> --dry-run
partiful events rsvp <event-id> --status going
partiful events rsvp <event-id> --status going --plus-one "Alex Smith"
partiful events rsvp <event-id> --status maybe --message "I may be late"
partiful events rsvp <event-id> --status declined
partiful events interested <event-id>
partiful events interested <event-id> --remove
```

`explore rsvp` and `explore interested` are equivalent aliases.

The current CLI cannot complete ticket purchases or host questionnaires. Questionnaire response fields are known internally, but `events rsvp` exposes no answer option and deliberately rejects questionnaire-gated events. Use Partiful directly for either flow.