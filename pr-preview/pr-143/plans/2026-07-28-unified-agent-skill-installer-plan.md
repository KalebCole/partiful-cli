# Unified Agent Skill Installer Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add a safe, cross-agent `partiful skill install|uninstall <agent>` interface for the singular bundled Partiful skill.

**Architecture:** A new command module owns target resolution, copy provenance, safe removal, and OpenClaw legacy cleanup. Commander registration remains thin in `src/cli.ts`; filesystem behavior is exercised through temporary-directory integration tests.

**Tech Stack:** TypeScript, Commander.js, Node filesystem APIs, Vitest.

---

### Task 1: Lock command contract with failing tests

**Objective:** Specify supported agents, destination paths, dry-run behavior, and structured output.

**Files:**
- Create: `tests/skill-installer.test.js`
- Modify: `tests/skill-structure.test.js`

**Steps:**
1. Add tests invoking the CLI with isolated `HOME` and `HERMES_HOME` values.
2. Assert `skill install` exists for `hermes`, `openclaw`, `copilot`, and `claude`.
3. Assert `--dry-run` reports target paths and makes no changes.
4. Replace the consolidation test asserting `setup` is absent with assertions for the new command contract.
5. Run `npm test -- --run tests/skill-installer.test.js tests/skill-structure.test.js`; expect failure because `skill` is not registered.

### Task 2: Implement installation

**Objective:** Copy the bundled singular skill safely into each agent's user-level skill root.

**Files:**
- Create: `src/commands/skill.ts`
- Modify: `src/cli.ts`
- Test: `tests/skill-installer.test.js`

**Steps:**
1. Add typed target metadata and path resolution.
2. Register `skill install <agent>`.
3. Recursively copy `skills/partiful/` and write a provenance marker.
4. Implement idempotence, `--force`, and global `--dry-run`.
5. Verify the focused tests pass.

### Task 3: Implement safe uninstall and OpenClaw migration

**Objective:** Remove only installer-owned copies and safely clean legacy OpenClaw links.

**Files:**
- Modify: `src/commands/skill.ts`
- Test: `tests/skill-installer.test.js`

**Steps:**
1. Add failing tests for owned removal, unowned-directory refusal, symlink removal, dry-run, and custom `--workspace` cleanup.
2. Implement `skill uninstall <agent>`.
3. Detect old OpenClaw `partiful-*` links without following dangling symlinks.
4. Restrict migration cleanup to symlinks targeting this package's `skills/` directory.
5. Run focused tests and verify all pass.

### Task 4: Document and verify the public interface

**Objective:** Make installation discoverable and prove package behavior.

**Files:**
- Modify: `README.md`
- Modify: `package.json` only if discoverability metadata requires it.

**Steps:**
1. Document all four install commands and uninstall/migration behavior.
2. Run CLI help smoke tests.
3. Run `npm test`, `npm run typecheck`, and `npm pack --dry-run --json`.
4. Confirm the tarball contains exactly `skills/partiful/SKILL.md` plus its references.
5. Run `git diff --check` and an adversarial review.

### Task 5: Publish stacked PR

**Objective:** Push a PR that depends only on the consolidation branch.

**Steps:**
1. Commit implementation and verification changes.
2. Push `feat/unified-skill-installer`.
3. Open PR with base `feat/singular-partiful-skill`, not `main` and not the RSVP branch.
4. Verify local HEAD equals remote HEAD and inspect PR checks.
