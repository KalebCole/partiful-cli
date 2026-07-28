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

For questionnaire events, pass one repeatable answer per question. Keys may be the question ID or its exact text:

```bash
partiful events rsvp <event-id> --answer "<question-id>=<value>"
partiful events rsvp <event-id> --answer "Dietary restrictions?=None" --answer "Song request?=Anything"
```

Required answers are validated before submission. Ticketed or paid events remain unsupported because the CLI cannot purchase tickets.