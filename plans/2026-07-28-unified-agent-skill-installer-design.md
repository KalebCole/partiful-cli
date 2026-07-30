# Unified Agent Skill Installer Design

**Date:** 2026-07-28

## Goal

Replace the removed OpenClaw-only setup command with one agent-neutral interface for installing the bundled Partiful skill:

```bash
partiful skill install <hermes|openclaw|copilot|claude>
partiful skill uninstall <hermes|openclaw|copilot|claude>
```

This change is stacked on `feat/singular-partiful-skill` and must not depend on RSVP questionnaire work.

## Decisions

- Use singular `skill`, because the npm package now ships one `partiful` skill.
- Normalize agent names to lowercase.
- Install at each agent's user-level skill root by default:
  - Hermes: `$HERMES_HOME/skills/partiful`, falling back to `~/.hermes/skills/partiful`
  - OpenClaw: `~/.openclaw/skills/partiful`
  - Copilot: `~/.copilot/skills/partiful`
  - Claude Code: `~/.claude/skills/partiful`
- Copy the bundled skill rather than symlink it. This works on Windows without elevated symlink privileges and prevents npm installation paths from becoming runtime dependencies.
- Add a private provenance marker to copied installations. Uninstall refuses to remove an unowned directory.
- Preserve global `--dry-run` and `--force` behavior.
- For OpenClaw uninstall, also clean legacy `partiful-*` symlinks created by `partiful setup openclaw`. Support `--workspace <path>` so custom legacy workspaces can be cleaned.
- Do not add `status`, project-local installation, or auto-detection in this PR.

## Command behavior

### Install

1. Validate agent.
2. Resolve bundled source and agent destination.
3. If destination is absent, recursively copy the skill and marker.
4. If an owned installation already matches, report it as already installed.
5. Refuse to overwrite any existing destination unless `--force` is passed.
6. `--dry-run` returns the intended action without filesystem mutation.

### Uninstall

1. Remove the destination only when it is an installer-owned copy or a symlink to the bundled Partiful skill.
2. Refuse to delete an unowned directory.
3. For OpenClaw, additionally inspect the selected legacy workspace and remove only legacy `partiful-*` symlinks whose targets resolve under this package's `skills/` directory.
4. `--dry-run` reports removals without mutation.

## Output and errors

All responses use the CLI JSON envelope. Unsupported agents, unreadable source directories, unsafe overwrite attempts, and unowned uninstall targets return structured errors with existing exit-code conventions.

## Verification

- Unit tests use temporary homes and never touch real agent directories.
- Test every target's resolved path.
- Test install, idempotence, force overwrite, dry-run, safe uninstall, refusal to delete unowned content, and OpenClaw legacy cleanup.
- Run full tests, typecheck, package dry-run, and CLI smoke tests.
