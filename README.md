![Banner](https://ghrb.waren.build/banner?header=partiful-cli+%F0%9F%8E%89&subheader=Manage+Partiful+events+from+your+terminal&bg=0d1117&color=f0f6fc&support=false)

# partiful-cli

JSON-first native Go CLI for reviewed Partiful event workflows.

## Install native binaries

Download the matching archive from GitHub Releases:

- macOS: `darwin_amd64` or `darwin_arm64`
- Linux: `linux_amd64` or `linux_arm64`
- Windows: `windows_amd64` or `windows_arm64`

Each release includes:

- one archive per target OS and CPU
- `partiful` or `partiful.exe`
- `partiful_<version>_checksums.txt`

Verify and smoke-test a release locally:

```bash
partiful --version
partiful schema
partiful doctor
```

## Development

The native release path is Go-only. Node and npm are not part of release,
archive verification, or smoke testing.

```bash
go mod verify
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Run the full native snapshot validation, including GoReleaser, archive checks,
checksum verification, and local `version/schema/doctor` smoke tests:

```bash
go install github.com/goreleaser/goreleaser/v2@v2.12.7
./scripts/verify-native-release.sh
```

## Approved command catalog

### Discovery and diagnostics

```text
partiful --version
partiful schema [command.path]
partiful doctor
```

### Authentication

```text
partiful auth login
partiful auth status
partiful auth logout
```

### Posters and contacts

```text
partiful posters list
partiful posters search --query <text>
partiful contacts list [--query <text>]
```

### Event and guest reads

```text
partiful events list --when <upcoming|past>
partiful events get <event-id>
partiful guests list <event-id>
partiful rsvp get <event-id>
```

### Reviewed remote mutations

```text
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

## Notes

- `src/` and `tests/` keep historical TypeScript research artifacts. They are
  not part of the native release path.
- Release archives bundle `README.md` and `LICENSE` with the native binary.
- `doctor` is the safe local smoke command for authentication state. It does
  not require live Partiful credentials.

## License

MIT
