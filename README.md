![Banner](https://ghrb.waren.build/banner?header=partiful-cli+%F0%9F%8E%89&subheader=Manage+Partiful+events+from+your+terminal&bg=0d1117&color=f0f6fc&support=false)

# partiful-cli

JSON-first native Go CLI for reviewed Partiful event workflows.

The first native-only release line starts at `v3.0.0`. The install command
below becomes available after the coordinated cutover release is published.

## Install

Use a native release archive from GitHub Releases, or install the tagged Go
module directly:

```bash
go install github.com/KalebCole/partiful-cli/cmd/partiful@v3.0.0
```

Verify a local install safely:

```bash
partiful --version
partiful schema
partiful doctor
```

## Development

Repository runtime, development, CI, verification, and release flow are
Go-only.

```bash
go mod verify
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Run the full native release rehearsal, including GoReleaser snapshot archives,
checksum verification, and local `version/schema/doctor` smoke tests:

```bash
GOTOOLCHAIN=go1.25.0 go install github.com/goreleaser/goreleaser/v2@v2.2.0
./scripts/verify-native-release.sh
```

## Approved command catalog

```text
partiful --version
partiful schema [command.path]
partiful doctor
partiful auth login
partiful auth status
partiful auth logout
partiful posters list
partiful posters search --query <text>
partiful contacts list [--query <text>]
partiful events list --when <upcoming|past>
partiful events get <event-id>
partiful guests list <event-id>
partiful rsvp get <event-id>
partiful events create [flags]
partiful events update <event-id> [flags]
partiful events cancel <event-id> [flags]
partiful guests invite <event-id> --contact <name>
partiful blasts send <event-id> --audience all-guests --message-file <path-or->
partiful rsvp set <event-id> [flags]
partiful cohosts invite <event-id> --contact <name>
partiful cohosts revoke-invite <event-id> --contact <name>
partiful cohosts remove <event-id> --contact <name>
partiful cohosts link create <event-id>
partiful cohosts link revoke <event-id>
```

All command results are JSON. Use `partiful schema <command.path>` for the
exact input, success, failure, and safety contract for any approved command.

## MCP stdio server

Start the API-derived MCP server from the same installed binary:

```bash
partiful mcp
```

It exposes all implemented event, guest, RSVP, contact, cohost, blast, and
poster tools by default, including writes. Tool calls use the CLI's validation,
authentication, transport, privacy projection, dry-run, and single-attempt
mutation behavior. Interactive authentication remains CLI-only: run
`partiful auth login` before protected MCP calls.

Narrow the startup surface only when desired:

```bash
partiful mcp --read-only
partiful mcp --allow-tool 'events_*,posters_list'
partiful mcp --list-tools --read-only
```

`--allow-tool` accepts exact names or a trailing `*`, may be repeated or
comma-separated, and rejects empty or unknown selections. MCP tool inputs never
include CLI-only `force` or `no-input`; destructive tool calls do not wait for a
TTY confirmation. Mutation tools accept `dryRun` for redacted, no-write previews.
Protocol mode reserves stdout for MCP messages.

## Mutation safety

Mutation commands execute once after validation and required read-before-write
checks. Add `--dry-run` to return a redacted request preview without sending a
write.

`events cancel`, `cohosts remove`, `cohosts revoke-invite`, and
`cohosts link revoke` require an interactive terminal confirmation.
`--force` skips only that prompt. `--no-input` or `--non-interactive` disables
prompting and fails safely unless `--force` is also present.

## Release notes

- GoReleaser injects the tagged release version with ldflags.
- Source builds default to `3.0.0` until a release tag overrides it.
- Each native archive bundles `README.md` and `LICENSE`.
- `doctor` is the safe local smoke command for authentication state.

## License

MIT
