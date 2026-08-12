# Guests, Invitations, and Cohosts

These workflows are for event hosts. A `403` for an event merely attended is
expected.

## List guests and resolve contacts

```bash
partiful guests list <event-id>
partiful contacts list --query "Alex"
```

Contacts return display names and shared-event counts. They do not expose email
addresses or phone numbers.

## Invite a guest

```bash
partiful guests invite <event-id> --contact "Alex Smith"
```

This command resolves the contact by display name and returns a consequential
plan by default. After approval, repeat it with `--apply --confirm <token>`.
If name resolution is ambiguous, stop and resolve that ambiguity first.

## Manage cohosts

```bash
partiful cohosts invite <event-id> --contact "Alex Smith"
partiful cohosts revoke-invite <event-id> --contact "Alex Smith"
partiful cohosts remove <event-id> --contact "Alex Smith"
```

Each cohost change is consequential. Review the plan, get approval, then repeat
with `--apply --confirm <token>`.

## Manage cohost links

```bash
partiful cohosts link create <event-id>
partiful cohosts link revoke <event-id>
```

The create and revoke flows are also consequential. The created URL is a
capability secret. Do not place it in logs, issues, or public messages.
