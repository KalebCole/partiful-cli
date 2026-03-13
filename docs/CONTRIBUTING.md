# Contributing to Partiful CLI

## Development Tools

This project uses two AI-assisted development tools that share the same workspace:

### Claude Code (Linux/Mac)
- **Config:** `.claude/` directory in repo root
- **Plans:** `docs/plans/*.md` (persistent, committed)
- **Skills:** `~/.claude/skills/partiful/` (local to machine)
- **Worktrees:** `.claude/worktrees/` for parallel branch work
- **Best for:** Long-running sessions, parallel worktree development

### GitHub Copilot CLI (Windows)
- **Session state:** `~/.copilot/session-state/` (ephemeral per-session)
- **Task tracking:** SQL-based todos within sessions
- **Skills:** Copilot skill `partiful` (auto-loaded)
- **GitHub integration:** Native MCP server for issues/PRs
- **Best for:** Issue triage, quick implementations, GitHub workflow

## Shared Conventions

### Source of Truth
- **GitHub Issues** are the single source of truth for work items
- Both tools read/write the same `partiful` source file
- Both tools use the same Git repo and branch model

### Commit Messages
Follow conventional commits with issue references:

```
feat(#23): add clone command for duplicating events
fix(#28): strip markdown from descriptions
docs: update skill with new commands
```

Always include attribution trailers:
- Claude Code: `Co-authored-by: Claude <noreply@anthropic.com>`
- Copilot CLI: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`

### Branch Strategy
- `main` — stable, all work merges here
- Feature branches: `feat/<issue-number>-<short-name>`
- Both tools can create branches and PRs

### Plan Documents
- Claude Code plans: `docs/plans/YYYY-MM-DD-<topic>.md` (committed for reference)
- Copilot CLI plans: session-scoped (not committed, ephemeral)
- For cross-tool plans, use `docs/plans/` so both can reference them

### Code Style
- Single file: `partiful` (Node.js, zero dependencies)
- Pure stdlib only (no npm packages)
- Functions follow the pattern: parse args → load config → get token → API call → display
- Use `console.error()` for progress messages, `console.log()` for output
- Strip markdown from user-facing text via `stripMarkdown()`

### Testing
No formal test suite — manual testing against live Partiful API:
```bash
node partiful help                    # Verify help text
node partiful list                    # Test API connectivity
node partiful get <eventId>           # Test event fetch
node partiful clone <eventId> --date "tomorrow 7pm"  # Test clone
```

### API Reference
- REST API: `api.partiful.com` (POST endpoints, wrapped payload format)
- Firestore: `firestore.googleapis.com` (direct document read/write)
- Auth: Firebase token refresh via `securetoken.googleapis.com`
- Some features (DMs, blasts) are blocked by Firestore security rules → browser fallback

## Issue Workflow

1. Issues are created on GitHub with priority labels: `[P1]`, `[P2]`, `[P3]`
2. Pick an issue → create branch (or work on main for small changes)
3. Implement → test manually → commit with issue reference
4. Push → create PR (or push to main for small changes)
