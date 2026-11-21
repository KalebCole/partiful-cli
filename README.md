![Banner](https://ghrb.waren.build/banner?header=partiful-cli+%F0%9F%8E%89&subheader=Manage+Partiful+events+from+your+terminal&bg=0d1117&color=f0f6fc&support=false)

# partiful-cli

A terminal CLI for Partiful events. Commands return JSON so people and agents can use it.
See [VISION.md](VISION.md) for product scope and principles.

## Install

Download and extract a native archive for your platform from [GitHub Releases](https://github.com/KalebCole/partiful-cli/releases), then put the `partiful` executable on your `PATH`. Or install with Go:

```bash
go install github.com/KalebCole/partiful-cli/cmd/partiful@latest
```


Verify your installation and check authentication state:

```bash
partiful --version
partiful schema
partiful doctor
```

## Quick start

Log in, then list your upcoming events:

```bash
partiful auth login
partiful events list --when upcoming
```

Use an event ID from the list to see its details:

```text
partiful events get <event-id>
```

Commands that change data accept `--dry-run` to preview the request.

## Commands

Run `partiful --help` to explore commands or `partiful schema` for the machine-readable catalog.
Use `partiful schema <command.path>` for one command's inputs and outputs, for example `partiful schema events.get`.

## Agent skills

The [Partiful agent skill](skills/partiful/SKILL.md) is part of the agent plugin that teaches agents to use the CLI.

## Contributing and development

See [Contributing](docs/CONTRIBUTING.md) for setup, tests, and release checks, [AGENTS.md](AGENTS.md) for contributor instructions, and [Architecture](docs/ARCHITECTURE.md) for how the CLI works.

## Release notes

See [GitHub Releases](https://github.com/KalebCole/partiful-cli/releases) for versions and changelogs.

## License

MIT

Offline events: `partiful events sync --when past` (or `upcoming`) upserts one homepage list into `partiful/events.sqlite` under the user config directory. `partiful events offline --status going --since 2026-01-01T00:00:00Z --until 2026-12-31T23:59:59Z` queries it without authentication or network access. Both accept `--db` and `--limit` (1 to 1000; defaults: sync 1000, offline 50). Offline defaults to going; `--status all` includes every cached RSVP status. Past GOING is an RSVP, not proof of attendance. Sync retains cached events absent from later lists; filters use inclusive start times, with unknown starts excluded from date filters. Use a separate DB per account.
