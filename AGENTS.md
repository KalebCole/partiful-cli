# AGENTS.md — Partiful CLI

Use the Go CLI in `cmd/partiful` and `internal/`.

## Release and validation

The native release path is Go-only.

```bash
go mod verify
go test ./...
go test -race ./...
go vet ./...
go build ./...
./scripts/verify-native-release.sh
```

Do not add a release step that needs Node or npm.

## Safe local smoke commands

Use only these non-mutating commands in automated verification:

```text
partiful --version
partiful schema
partiful doctor
```

`schema` is the machine-readable command catalog. `doctor` is the safe local
check for authentication state.

## Approved public commands

`schema` lists the full approved catalog:

- `auth.login`
- `auth.logout`
- `auth.status`
- `blasts.send`
- `cohosts.invite`
- `cohosts.link.create`
- `cohosts.link.revoke`
- `cohosts.remove`
- `cohosts.revoke-invite`
- `contacts.list`
- `doctor`
- `events.cancel`
- `events.create`
- `events.get`
- `events.list`
- `events.update`
- `guests.invite`
- `guests.list`
- `posters.list`
- `posters.search`
- `rsvp.get`
- `rsvp.set`
- `schema`
- `version`

## Privacy and safety

- Never expose phone numbers, email addresses, access tokens, or Partiful user IDs.
- Never use live credentials or mutate real events in automated validation.
- Use `--dry-run` for a redacted mutation preview.
- `events.cancel`, `cohosts.remove`, `cohosts.revoke-invite`, and
  `cohosts.link.revoke` require a TTY confirmation unless `--force` is set.
- Use `--no-input` or `--non-interactive` when prompting is not allowed.
- Mutation commands dispatch at most once and never retry automatically.
