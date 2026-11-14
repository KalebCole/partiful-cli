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

GoReleaser injects the tagged release version with ldflags. Source builds default to `3.1.0` until a release tag overrides it. Each native archive bundles `README.md` and `LICENSE`.

## License

MIT
