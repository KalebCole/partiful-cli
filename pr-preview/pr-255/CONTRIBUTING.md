# Contributing to Partiful CLI

## Source of truth

The shipped CLI is the Go program in `cmd/partiful` and `internal/`.
Remote calls and their evidence are defined in [`spec/partiful.openapi.json`](../spec/partiful.openapi.json).
Read [Architecture](ARCHITECTURE.md) when adding or correcting a call.

## Core commands

```bash
go mod verify
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Use the native release verification script when you change build metadata,
release packaging, command contracts, or archive contents:

```bash
GOTOOLCHAIN=go1.22.12 go install github.com/goreleaser/goreleaser/v2@v2.2.0
./scripts/verify-native-release.sh
```

## Safe smoke checks

Only these commands belong in automated local smoke checks:

```bash
partiful --version
partiful schema
partiful doctor
```

They do not require live Partiful credentials or remote mutations.

## Public contract

- Command definitions and Go tests enforce the user-facing rules.
- [`spec/partiful.openapi.json`](../spec/partiful.openapi.json) is the remote transport authority.
- [Architecture](ARCHITECTURE.md) explains authentication and contract checks.
- `partiful schema [command.path]` is the machine-readable command catalog.
- `partiful --version` returns the CLI version plus product and remote contract revisions.

## Release rules

- Ship native archives for darwin, linux, and windows on amd64 and arm64.
- Bundle `README.md` and `LICENSE` in every archive.
- Publish one SHA-256 checksum file that covers every archive.
- Use the pinned GoReleaser tool in workflow and local release rehearsal.
- Do not add a release workflow that invokes Node or npm.

## Privacy and safety

- Do not use live Partiful credentials in tests or release verification.
- Do not expose phone numbers, email addresses, access tokens, or Partiful user IDs.
- Keep black-box command tests at the `app.Execute` seam.

## Reviewing public behavior changes

Run `go test ./...` to check command envelopes, error mappings, privacy fixtures,
mutation safety, and the complete schema snapshot. The existing native CI runs
these tests with the race detector as well as native packaging checks.

For an intentional public schema or error mapping change, inspect the behavior
and update the reviewed snapshots explicitly:

```bash
go test ./internal/app -update-product-goldens
git diff -- internal/app/testdata
```

Keep `productContractRevision` in `internal/app/buildinfo.go` aligned with deliberate
changes to the executable product baseline. The schema golden records that revision
in its metadata. Error goldens pin observed type/code/exit mappings;
command fixtures exercise the actual failure paths. See [Architecture](ARCHITECTURE.md#why-these-product-rules-exist)
for the rationale and excluded features.
