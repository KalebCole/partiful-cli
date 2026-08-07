# RSVPs and Interest

```bash
partiful events rsvp get <event-id>
partiful events rsvp set <event-id> --dry-run
partiful events rsvp set <event-id> --status going
partiful events rsvp set <event-id> --status going --plus-one "Alex Smith"
partiful events rsvp set <event-id> --status maybe --message "I may be late"
partiful events rsvp set <event-id> --status declined
partiful events interested <event-id>
partiful events interested <event-id> --remove
```

`events rsvp get` reads your saved status and questionnaire answers without changing the RSVP. `explore rsvp get`, `explore rsvp set`, and `explore interested` are equivalent aliases under the discovery command group.

For questionnaire events, pass one repeatable answer per question. Keys may be the question ID or its exact text:

```bash
partiful events rsvp set <event-id> --answer "<question-id>=<value>"
partiful events rsvp set <event-id> --answer "Dietary restrictions?=None" --answer "Song request?=Anything"
```

Required answers are validated before submission. Successful writes read the Firestore guest document back to verify the saved status and questionnaire answers. Plain `--dry-run` remains offline; combining `--answer` with `--dry-run` performs read-only guest and event lookups to validate the live questionnaire preview. Ticketed or paid events remain unsupported because the CLI cannot purchase tickets.