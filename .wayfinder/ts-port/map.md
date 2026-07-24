# Wayfinder Map: TypeScript Port + API Spec-as-Types

`wayfinder:map` · tracker: local-markdown · created 2026-07-24

## Destination

partiful-cli ported from plain JS to TypeScript, with the `src/lib/` API layer typed such that
**endpoint interfaces + Zod response schemas ARE the living API spec** (no separate hand-maintained
spec file). `strict: true` compiles clean via `tsc --noEmit`, all existing tests green, and the
`schema` command exposes API endpoint types under a new `schema api.<method>` namespace.

Success = the API spec is a *byproduct* of typing the code, not a separate artifact that can drift.

## Notes

- **Domain:** github.com/KalebCole/partiful-cli. Plain JS today (29 files, ~4,373 LOC, ESM,
  Commander 13 + Vitest). Deps: commander, dotenv (both ship first-class TS types).
- **This is fully AFK.** All HITL gates collapsed — Kaleb front-loaded every decision (strict from
  day one; Zod `.passthrough()` for API responses, plain interfaces for internal shapes). The
  first-slice "approval" is a machine oracle (compiles + tests green + matches written criteria),
  not a human glance. Normal PR review before merge to main is the only human touch, and that's
  not a ticket.
- **The port IS the spec.** The ~23 untyped `result?.data...` API-response spreads are where
  endpoint interfaces + Zod schemas get authored. Type the lib layer = write the spec.
- **Sequencing is the whole strategy:** lib/ (API layer, spec born here) → commands/ + helpers/
  (consume the types) → schema.js (surface them). Do NOT reorder.
- **Loop oracle:** `tsc --noEmit` clean AND `npm test` green. Machine-checkable done, ideal for
  an autonomous agent grinding file-by-file.
- Skills to consult: prior art = YouTube.js (the gold standard: TS types + Zod + real-API smoke
  tests, no OpenAPI). AGENTS.md in repo root for conventions/boundaries.
- Separate effort: the explore-command map lives in `.wayfinder/` (parent dir). Don't touch it.

## Decisions so far

- T0 CLOSED (2026-07-24): RSVP work merged to main via PR #65 (squash, commit ad7be30); tree clean; main green at 195/195 tests. Port branch to be cut from ad7be30.
- T1 CLOSED (2026-07-24): TS toolchain up. tsx-loader run path (no dist build), tsconfig strict+NodeNext+allowJs, zod added. `npm run typecheck` clean + 195/195 green on still-JS tree.

## Not yet specified

- Exact tsconfig strictness knobs beyond `strict: true` (noUncheckedIndexedAccess?, exactOptionalPropertyTypes?) — resolve inside T2.
- Whether `schema api.*` output format should mirror the existing `schema <command>` CLI-flag shape or diverge — resolve when T5 is reached.
- Drift-detection ergonomics: where unknown-field logs go, whether smoke tests run in CI or manual — resolve in T6.
- CI: add a `tsc --noEmit` + test gate to the repo's CI once ported — graduates after T1.

## Out of scope

- OpenAPI 3.1 / TypeSpec / standalone spec file. Ruled out: repo is 7-host, 3-method, 2-transport
  (Firebase-callable RPC + Firestore document format); types-as-spec fits the actual code, a
  standalone spec would duplicate and drift. See prior-art + repo-grounded research (2026-07-24).
- Rewriting/refactoring API logic during the port. Port is a faithful JS→TS translation; behavior
  changes are a separate effort.
- The explore/discovery command build (its own map).

## Tickets

| # | Title | Type | Blocks on | Status |
|---|---|---|---|---|
| T0 | RSVP work merged to main, tree clean, port branch cut | task (AFK) | — | ✅ CLOSED (PR #65 merged, ad7be30) |
| T1 | TS toolchain setup (tsconfig strict, tsx run, bin, build) | task (AFK) | T0 | ✅ CLOSED (tsx loader, no dist) |
| T2 | Write porting convention doc (strict + Zod pattern) | task (AFK) | T1 | OPEN |
| T3 | Port src/lib/ API layer + author endpoint types/Zod (THE SPEC) | task (AFK) | T2 | OPEN |
| T4 | Port src/commands/ + src/helpers/ | task (AFK) | T3 | OPEN |
| T5 | Rewire schema command → schema api.<method> | task (AFK) | T3 | OPEN |
| T6 | Wire drift-detection + real-API smoke tests | task (AFK) | T3 | OPEN |
