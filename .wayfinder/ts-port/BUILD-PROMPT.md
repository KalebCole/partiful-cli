# BUILD-PROMPT: partiful-cli JavaScript → TypeScript Port (API Spec-as-Types)

> **You are an autonomous porting orchestrator.** Work in a loop until the Definition of Done is met.
> Do not stop at a plan or a stub — keep going until EVERYTHING is complete: the full port, all
> review gates passed, and all bot review threads resolved. This is fully AFK: no human will answer
> questions mid-run. When a decision is ambiguous, follow the conventions in this file and the map;
> if still ambiguous, choose the option that keeps behavior identical to the current JS and note it.

## Operating rules (read first — these govern the whole run)

1. **Drive off the map.** `.wayfinder/ts-port/map.md` and `.wayfinder/ts-port/tickets/T*.md` are the
   source of truth for scope, sequencing, and per-ticket detail. Work the tickets T1→T6 in order.
   Mark each closed in the map's "Decisions so far" as you finish it. Do not invent scope outside
   the map.
2. **Complete EVERYTHING.** Done is not "most of it." Every ticket closed, every gate green, every
   review comment resolved. Do not hand back a partial port.
3. **Test-Driven Development.** For every fix, new validation, and new typed surface: write the
   failing test FIRST (RED), make it pass (GREEN), then refactor. Follow the `test-driven-development`
   skill. Never add behavior without a test that would fail without it. Never weaken existing tests.
4. **Adversarial self-review before every PR.** Before opening any PR, spawn an adversarial code
   review pass (follow the `adversarial-code-review` skill — VERDICT format, diff+context, attack
   the change). Fix everything it flags as blocking, then re-review until it returns a clean verdict.
   Do this yourself (or via a review subagent) BEFORE requesting external review.
5. **Poll the review bots and handle them.** After opening the PR, external review bots (CodeRabbit,
   etc.) run asynchronously. Enter a **poll loop** (follow the `pr-review-bots` skill):
   - Wait, then re-check the PR for new bot reviews/comments (`gh pr view`, `gh pr checks`).
   - For each actionable comment: verify it against current code, FIX still-valid issues (TDD:
     test first), and SKIP invalid ones with a one-line rationale posted as a reply.
   - Push fixes, which re-triggers the bots. Repeat.
   - **Termination criteria for the loop (stop when ANY holds):** (a) the latest bot review has zero
     actionable comments AND all checks pass; OR (b) two consecutive poll cycles produce only
     comments you've reasoned-declined with no new actionable items; OR (c) 6 poll cycles elapsed —
     then stop and summarize the remaining open threads with your rationale. Never loop forever.
6. **Then done.** Once the port is complete, gates green, adversarial review clean, and the bot-poll
   loop hit a termination criterion, finalize: ensure the PR is green and mergeable, write the final
   report, and STOP.

---

## Mission

Port `github.com/KalebCole/partiful-cli` from plain JavaScript to TypeScript such that the
**`src/lib/` API layer is typed so that endpoint interfaces + Zod response schemas ARE the living
API spec.** The spec must be a *byproduct* of typing the code — never a separate hand-maintained
file that can drift.

Repo today: 29 files, ~4,373 LOC, ESM (`"type": "module"`), Commander 13 + Vitest. Deps:
`commander`, `dotenv` (both ship first-class TS types). Node ≥ 18.

This is a **faithful translation**, NOT a refactor. Do not change runtime behavior, rename
commands, or restructure API logic. Types describe what the code already does.

---

## Setup (do this first)

```bash
cd ~/repos/partiful-cli
git fetch origin
git checkout main && git pull --ff-only        # must be at commit ad7be30 or later
git checkout -b feat/typescript-port           # cut the port branch from clean main
npm test                                        # confirm baseline: expect 195/195 green
```

Read `AGENTS.md` in the repo root before touching anything — it holds hard conventions and
boundaries (no build step historically, ESM, etc.). Also skim `src/lib/http.js`, `src/lib/auth.js`,
and one command file to internalize the current patterns.

---

## Locked conventions (Kaleb already decided these — do not deviate)

1. **`strict: true` from day one.** Full strict mode in `tsconfig.json`. Do not start loose and
   tighten later. If `noUncheckedIndexedAccess` / `exactOptionalPropertyTypes` cause excessive
   churn, you MAY leave those two off, but `strict` itself stays on. Note whatever you chose.
2. **API responses → Zod schemas with `.passthrough()`.** Every endpoint's response gets a Zod
   schema that documents known fields but does NOT claim exhaustiveness (Partiful is an unofficial
   API we don't own). Derive the TS type via `z.infer<>`. This is the gold-standard pattern from
   YouTube.js — one definition yields runtime validation + compile-time type + drift signal.
3. **Internal (non-API) shapes → plain `interface`/`type`.** Don't wrap internal data in Zod.
   Zod is only for data crossing the network boundary from Partiful.
4. **No behavior changes.** Faithful JS→TS. Behavior-altering improvements are a separate effort.
5. **ESM import extensions:** keep `.js` extensions in relative imports (TS ESM convention — the
   emitted/`tsx`-run code resolves `./foo.js` even when the source is `foo.ts`). Decide build-vs-tsx
   in T1 and be consistent.

---

## Sequencing — DO NOT REORDER

The order is the whole strategy. The spec is born in the lib layer; everything downstream consumes it.

### T1 — Toolchain setup
- Add `typescript`, `tsx`, `@types/node`, `zod` as devDeps (zod is a runtime dep — put it in
  `dependencies`).
- Create `tsconfig.json`: `strict: true`, `module: NodeNext`, `moduleResolution: NodeNext`,
  `target: ES2022`, `noEmit` for the check (or emit to `dist/` if you choose a build over tsx).
- Decide the run path: **`tsx` wrapper** (no build step, matches repo ethos) OR compile to `dist/`.
  Define how the published `bin` invokes the TS entry BEFORE renaming `src/cli.js`. Keep a working
  JS launcher shim if needed so `bin` never breaks.
- Add an `npm run typecheck` script (`tsc --noEmit`) and wire it alongside `npm test`.
- Add a smoke test that the `bin` path resolves and executes (`partiful --version` works).
- **Gate:** `tsc --noEmit` clean on the (still mostly-JS, `allowJs: true`) tree; `npm test` green.

### T2 — Porting convention doc
- Write `docs/PORTING.md` capturing: strict knobs chosen, the Zod-`.passthrough()`-for-responses +
  plain-interface-for-internal pattern (with a concrete before/after example from a real endpoint),
  `.js`-import-extension rule, file-by-file order, and the Definition of Done.
- This doc is the contract every subsequent file follows. Keep it short and prescriptive.

### T3 — Port `src/lib/` (THE SPEC IS BORN HERE) ⭐
- Port every `src/lib/*.js` to `.ts`. This is the keystone.
- For each endpoint touched (createEvent, cancelEvent, getEventInfo, getContacts, createTextBlast,
  addInvitedGuestsAsHost, upcoming/past feeds, addGuest, markEventInterest, getCurrentGuest,
  Firestore GET/PATCH surfaces): define a **request interface** (fully specified) and a **Zod
  response schema** with `.passthrough()` (known fields only). Co-locate them, e.g.
  `src/lib/endpoints/<name>.ts` or a typed registry — your call, but be consistent and importable.
- **FIRST-SLICE ORACLE (self-approval — no human):** before replicating the pattern across all lib
  files, fully port ONE representative endpoint per transport — (a) a Firebase-callable POST via
  `http.js`, (b) a Firestore document GET/PATCH, (c) a Firebase-auth call. Each must: compile under
  strict, have a `.passthrough()` Zod response schema, expose a `z.infer` type, keep the RPC
  envelope typed, and pass its tests. If all three satisfy this, that IS the approved pattern —
  replicate across the rest of `lib/`.
- **Gate:** `tsc --noEmit` clean; `npm test` green.

### T4 — Port `src/commands/` + `src/helpers/`
- Mechanical once T3 exists — these consume the lib types. Port every `.js` → `.ts`.
- **Gate:** `tsc --noEmit` clean; `npm test` green.

### T5 — Rewire `schema` command → `schema api.<method>`
- Today `src/commands/schema.js` is a static hardcoded dict of CLI flags. Keep `schema <command>`
  (CLI-flag lookup) behavior unchanged.
- ADD a `schema api.<method>` namespace that reads from an explicit runtime endpoint registry
  carrying, per method: host, HTTP method, transport, request-parameter descriptors, and the Zod
  response schema (from T3). `schema api.*` introspects THAT registry.
- **Gate:** `tsc --noEmit` clean; `npm test` green; `schema api.addGuest` (etc.) returns sane output.

### T6 — Drift-detection + smoke tests
- **Drift-detection:** when a response has fields not in its Zod schema, surface it — but only
  behind a `--verbose`/debug flag, and log ONLY redacted field paths + value types + bounded counts.
  NEVER log raw unknown values (no phone numbers, user IDs, tokens leaking to output/logs).
- **Smoke tests:** a few real-API tests as the spec verifier, gated behind an explicit opt-in env
  var (match existing integration-test conventions). Use a dedicated throwaway test account/event.
  NO destructive operations without guaranteed cleanup — especially authenticated writes (RSVPs).
- **Gate:** `tsc --noEmit` clean; `npm test` green.

---

## Definition of Done (the loop's exit condition)

- [ ] Every `src/**/*.js` is now `.ts` (no stray JS in `src/`, except an intentional `bin` shim).
- [ ] `npm run typecheck` (`tsc --noEmit`) passes clean under `strict: true`.
- [ ] `npm test` — ALL tests green (≥ the 195 baseline; you'll add more).
- [ ] `src/lib/` endpoints each have a request interface + `.passthrough()` Zod response schema;
      types derive via `z.infer`. The API spec exists AS the typed lib layer.
- [ ] `schema api.<method>` works and reads the endpoint registry.
- [ ] Drift-detection is redaction-safe and flag-gated; smoke tests are opt-in and non-destructive.
- [ ] `docs/PORTING.md` documents the conventions actually used.
- [ ] `partiful --version` and a couple of real commands still run (bin not broken).

## Loop discipline

- Work file-by-file. After each file (or small batch), run `tsc --noEmit` + `npm test`. Never let
  red accumulate. Commit in logical chunks with clear messages.
- If a fix isn't obvious, prefer the change that keeps behavior IDENTICAL to the JS and add a
  `// TODO(port):` note rather than inventing new behavior.
- Do NOT fabricate API responses or test data. If a real call is needed and blocked, gate it and
  say so — don't invent output.
- When done, open a PR from `feat/typescript-port` → `main`, summarize what changed, report exact
  before/after test counts, and list any convention deviations with reasons.

## Reference

- Wayfinder map + per-ticket detail: `.wayfinder/ts-port/map.md` and `.wayfinder/ts-port/tickets/`.
- Prior-art gold standard: YouTube.js (TS types + Zod + real-API smoke tests; no OpenAPI).
- Out of scope: OpenAPI/TypeSpec/standalone spec file; refactoring API logic; the explore-command build.
