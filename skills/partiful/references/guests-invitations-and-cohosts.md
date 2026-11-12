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

This command resolves the contact by display name. Add `--dry-run` for a
redacted preview with no invitation. Without `--dry-run`, it dispatches once.
If name resolution is ambiguous, stop and resolve that ambiguity first.

## Manage cohosts

```bash
partiful cohosts invite <event-id> --contact "Alex Smith"
partiful cohosts revoke-invite <event-id> --contact "Alex Smith"
partiful cohosts remove <event-id> --contact "Alex Smith"
```

Add `--dry-run` to preview any cohost change. `cohosts invite` dispatches
without a prompt. `cohosts revoke-invite` and `cohosts remove` are destructive
and prompt on a TTY unless `--force` is set.

## Manage cohost links

```bash
partiful cohosts link create <event-id>
partiful cohosts link revoke <event-id>
```

Link creation dispatches without a prompt. Link revocation is destructive and
prompts on a TTY unless `--force` is set. Add `--dry-run` to either command for
a no-write preview. The created URL is a capability secret. Do not place it in
logs, issues, or public messages.
