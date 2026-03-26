# Distribution Plan — partiful-cli

**Date:** 2026-03-26
**Status:** Draft
**Goal:** One-command install for humans and AI agents

---

## Overview

Three distribution channels, each building on the last:

1. **npm** — `npm install -g partiful-cli` (core, do first)
2. **OpenClaw skills** — `partiful setup openclaw` (agent integration)
3. **GitHub Release** — tagged releases with changelogs

---

## Phase 1: npm Publish

### What needs to happen

**package.json updates:**
```json
{
  "name": "partiful-cli",
  "version": "2.0.0",
  "description": "CLI for creating and managing Partiful events — JSON-first, agent-friendly",
  "keywords": ["partiful", "events", "cli", "party", "rsvp", "agent", "ai"],
  "repository": {
    "type": "git",
    "url": "https://github.com/KalebCole/partiful-cli"
  },
  "license": "MIT",
  "engines": {
    "node": ">=18.0.0"
  },
  "files": [
    "bin/",
    "src/",
    "skills/",
    ".env.example"
  ]
}
```

**New files needed:**
- `LICENSE` — MIT license file
- `.npmignore` — exclude tests, docs, fixtures from published package

**.npmignore:**
```
tests/
docs/
.env
.git/
node_modules/
*.test.js
```

**Pre-publish checklist:**
- [ ] Add `files` field to package.json (whitelist approach, safer than .npmignore alone)
- [ ] Add `repository`, `keywords`, `engines`, `license` fields
- [ ] Create LICENSE file
- [ ] Verify `npm pack --dry-run` includes only intended files
- [ ] Run full test suite
- [ ] `npm publish` (requires npm account — Kaleb needs to `npm login`)

### Install experience after publish
```bash
npm install -g partiful-cli
partiful auth login +1XXXXXXXXXX
partiful doctor  # verify setup
partiful events list  # go
```

---

## Phase 2: OpenClaw Setup Command

### `partiful setup openclaw`

Auto-detect and configure OpenClaw skill integration:

```bash
partiful setup openclaw
# 1. Detect OpenClaw workspace (check OPENCLAW_WORKSPACE or ~/.openclaw/workspace)
# 2. Symlink skills/ into workspace/skills/
# 3. Verify auth status
# 4. Run doctor
# 5. Print success message with next steps
```

**Implementation:**
- New command: `src/commands/setup.js`
- Detects workspace via: `$OPENCLAW_WORKSPACE` → `~/.openclaw/workspace` → prompt
- Symlinks each `skills/partiful-*` directory
- Idempotent — safe to run multiple times
- `--uninstall` flag to remove symlinks

**Agent install experience:**
```bash
npm install -g partiful-cli
partiful setup openclaw
partiful auth login +1XXXXXXXXXX
# Done — agents now see partiful-events, partiful-guests, etc.
```

### Standalone agent use (no OpenClaw)

Skills are just markdown files. Any agent framework can read them:
```bash
# Point your agent at the skill files directly
cat $(npm root -g)/partiful-cli/skills/partiful-events/SKILL.md
```

---

## Phase 3: GitHub Releases

### Tagging strategy
- Semantic versioning: `v2.0.0`, `v2.1.0`, etc.
- `npm version patch/minor/major` → auto-tags
- GitHub Actions: on tag push → create release with changelog

### GitHub Actions workflow (`.github/workflows/release.yml`)
```yaml
on:
  push:
    tags: ['v*']
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          registry-url: 'https://registry.npmjs.org'
      - run: npm ci
      - run: npm test
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
      - uses: softprops/action-gh-release@v2
        with:
          generate_release_notes: true
```

---

## Phase 4: README & Docs Polish (for public launch)

### README structure
1. **Hero** — one-liner + demo GIF
2. **Quick Start** — 3 commands (install, auth, create)
3. **Features** — what it does, with examples
4. **AI Agent Integration** — OpenClaw setup + skill docs
5. **Commands** — full reference
6. **Contributing** — link to CONTRIBUTING.md

### Demo GIF/video
- Record with `asciinema` or `vhs` (Charm CLI)
- Show: auth → create event → check guests → template workflow
- Keep under 30 seconds

---

## Implementation Order

| # | Task | Effort | Blocked by |
|---|------|--------|------------|
| 1 | package.json + LICENSE + .npmignore | 15 min | Nothing |
| 2 | `partiful setup openclaw` command + tests | 1-2 hrs | Nothing |
| 3 | npm publish | 10 min | Kaleb npm login |
| 4 | GitHub Actions release workflow | 30 min | npm publish working |
| 5 | README rewrite | 1 hr | Nothing |
| 6 | Demo recording | 30 min | README done |

**Critical path for demo:** Steps 1-3. Steps 4-6 are polish.

---

## Open Questions

1. **npm org scope?** Publish as `partiful-cli` (available) or `@kalebcole/partiful-cli`? Unscoped is cleaner for install UX.
2. **Node version floor?** 18+ is safe (LTS). Could go 20+ to use newer APIs.
3. **ClawhHub?** Is there a registry for OpenClaw skills? If so, publish there too.
4. **Brew formula?** Worth a Homebrew tap for Mac users? Low priority.
