# Contributing to Partiful CLI

## Source of truth

The shipped CLI is the Go program in `cmd/partiful` and `internal/`.
Reviewed transport evidence lives under `docs/research/`, `spec/`, and
`spec/research/`.

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
GOTOOLCHAIN=go1.25.0 go install github.com/goreleaser/goreleaser/v2@v2.2.0
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

- `docs/CLI-PRODUCT-CONTRACT.md` is the user-facing command authority.
- `docs/REMOTE-API-CONTRACT.md` is the reviewed transport authority.
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
