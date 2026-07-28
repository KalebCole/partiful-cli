# Guests and RSVPs

## RSVP and Interest

```bash
partiful events rsvp <event-id> --dry-run
partiful events rsvp <event-id> --status going
partiful events rsvp <event-id> --status going --plus-one "Alex Smith"
partiful events rsvp <event-id> --status maybe --message "I may be late"
partiful events rsvp <event-id> --status declined
partiful events interested <event-id>
partiful events interested <event-id> --remove
```

`explore rsvp` and `explore interested` are equivalent aliases. Ticketed events and host questionnaires cannot be completed through the CLI; use Partiful directly.

## List, Watch, and Export Guests

```bash
partiful guests list <event-id>
partiful guests list <event-id> --status GOING
partiful +watch <event-id> --interval 30 --duration 60
partiful +export <event-id> --format csv --output guests.csv
```

`+watch` and `+export` are top-level helper commands. Guest lists work only for events the authenticated user hosts. A `403` for an event merely attended is expected. Statuses include `GOING`, `MAYBE`, `SENT`, `DECLINED`, and `WAITLIST`.

## Invite People

```bash
partiful guests invite <event-id> --phone +12065551234 --dry-run
partiful guests invite <event-id> --user-id <partiful-user-id> --dry-run
partiful guests invite <event-id> --user-id <id> --message "Hope you can make it"
```

There is no direct `--name` invite. Resolve a name first:

```bash
partiful contacts list "Alex"
partiful guests invite <event-id> --user-id <resolved-id> --dry-run
```

Contacts return names, IDs, and shared-event counts. They do not expose email addresses or phone numbers. Get approval before sending invites, especially in bulk.

## Cohosts

```bash
partiful cohosts list <event-id>
partiful cohosts add <event-id> --name "Alex Smith" --dry-run
partiful cohosts add <event-id> --user-id <partiful-user-id> --dry-run
partiful cohosts remove <event-id> --user-id <partiful-user-id> --dry-run
```

Resolve ambiguous contact names before adding cohosts. Verify the event and proposed changes, then get approval before adding or removing cohosts.

## Privacy

Phone numbers and Partiful user IDs may be needed as command inputs. Do not echo them in user-facing completion messages or logs.
