# AGENTS.md Schema — Systematic Structure

This documents the schema and rationale behind how we write AGENTS.md files
for CLI tools. Based on research from GitHub Blog (2,500+ repos), agents.md
spec (60k+ repos), and ETH Zurich findings (March 2026).

## Core Principle

> Only include what an agent **cannot infer** from `--help`, source code,
> or standard conventions. Everything else is noise that increases cost
> and reduces performance. (ETH Zurich, 2026)

## Schema

An AGENTS.md for a CLI tool should have these sections, in this order:

### 1. Identity (2-3 lines max)
- What it is, one sentence
- Key constraint the agent needs to know immediately
- Example: "JSON-first CLI for Partiful. No official API — uses internal Firebase API."

### 2. Commands (executable, copy-pasteable)
- Every command the agent might need, with realistic arguments
- Group by resource (events, guests, contacts, etc.)
- Include flags that matter. Skip flags an agent will discover via `--help`
- **This section is the most referenced.** Put it early.

### 3. Testing
- Exact test command(s)
- Where tests live
- Which tests hit real APIs (integration vs unit)

### 4. Project Structure (only if non-obvious)
- File tree, 2 levels deep max
- One-line description per directory
- Skip if it follows standard conventions (e.g., Next.js app)

### 5. Non-obvious Things (THE MOST VALUABLE SECTION)
This is where the real value lives. Document:
- **Permission gotchas** — what fails and why (not bugs, just auth/scope constraints)
- **Data model surprises** — fields that are missing, types that are weird
- **Workarounds** — when Plan A fails, what's Plan B
- **Multi-step flows** — sequences that aren't obvious from individual commands
- **Silent failures** — things that return 200 OK but didn't actually work

Format each as:
```
### <Short title>
<What happens> + <Why> + <What to do instead>
```

### 6. Code Style (only non-inferable conventions)
- Language, framework, test runner
- Naming patterns only if unusual
- Skip if standard (e.g., "use camelCase in JS" is inferable)

### 7. Git Workflow (only if non-standard)
- Branch naming if you have a convention
- Required checks before commit
- Skip if it's just "branch from main, PR to merge"

### 8. Boundaries (three tiers)
- ✅ **Always** — things the agent should do without asking
- ⚠️ **Ask first** — things that affect real people or are destructive
- 🚫 **Never** — hard constraints (security, privacy, data loss)

## Anti-patterns (from research)

1. **Architecture overviews** — agents don't use them to find files faster (ETH Zurich)
2. **Restating what `--help` says** — pure noise, increases token cost
3. **LLM-generated content** — -3% success rate vs no file at all
4. **Vague instructions** — "be careful with auth" vs "run `partiful doctor` first"
5. **Philosophy/motivation** — "we believe in clean code" does nothing

## Token Budget Guidance

- Aim for 2,000-5,000 tokens (roughly 150-400 lines of markdown)
- Under 2,000: probably missing non-obvious gotchas
- Over 5,000: probably including inferable content that hurts performance
- The "non-obvious things" section should be 30-50% of the file

## Validation Checklist

For each line in the AGENTS.md, ask:
- [ ] Could an agent figure this out from `--help` or reading source? → Remove it
- [ ] Has this actually caused an agent to fail/waste time? → Keep it
- [ ] Is this a hard constraint (security, privacy, permissions)? → Keep it
- [ ] Is this a multi-step flow that isn't obvious? → Keep it
- [ ] Is this describing standard conventions? → Remove it
