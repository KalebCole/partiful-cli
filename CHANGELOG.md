# Changelog

## Unreleased

- Add `partiful rsvp remove <event-id>` to remove yourself from an event entirely
  using `removeGuest`, so it no longer appears on your homepage. The CLI resolves
  your own guest record, redacts the guest ID in `--dry-run`, and requires terminal
  confirmation unless `--force` is supplied. Supports `--no-input` and dispatches once.
- Clarify that `rsvp set --status not-going` means Can't Go and keeps you listed
  as a guest; its `addGuest` / `DECLINED` behavior is unchanged.
