# Guests, Invitations, and Cohosts

These workflows are for event hosts. A `403` for an event merely attended is
expected.

## List guests and resolve contacts

```bash
partiful guests list <event-id>
partiful guests list <event-id> --limit 100
partiful guests list <event-id> --all --max-items 1000
partiful contacts list --query "Alex"
```

Contacts return display names and shared-event counts. They do not expose email
addresses or phone numbers.

Host guest lists work even when Partiful's initial guest-list view rejects
the host. The CLI handles that automatically. Results contain `displayName`,
`rsvpStatus`, `partySize`, and `cohost`, with no guest or account IDs.
Use `meta.page.nextCursor` with `--cursor` while `meta.page.hasMore` is true;
`--all --max-items` caps the returned items and can still leave more pages.

Guest reads also have an internal safety cap of 10,000 records. The host
fallback stops at 10 pages of up to 1,000 records each. Exceeding that cap,
repeated remote page tokens, or an unexpected guest shape returns
`GUESTS_PROTOCOL_CHANGED`, rather than a partial successful list. Reducing
`--limit` or `--max-items` does not bypass this read cap. Report the error
without claiming the event has no guests or retrying indefinitely. Non-hosts
still receive `HOST_PERMISSION_REQUIRED`.

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
