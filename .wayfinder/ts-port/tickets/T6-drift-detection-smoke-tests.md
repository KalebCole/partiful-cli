# T6 — Wire drift-detection + real-API smoke tests

**Type:** task (AFK) · **Blocks on:** T3 · **Status:** OPEN

## Question

Keep the spec honest over time. Partiful is unofficial — they change shapes without notice. Two
low-cost mechanisms from the prior-art gold standard (YouTube.js) keep types-as-spec from rotting.

## Scope

- **Drift detection:** since T3 responses use Zod `.passthrough()`, log any unknown fields observed
  at parse time (behind a verbose/debug flag). Over time real traffic reveals the vendor's true
  shape. Decide where logs go.
- **Smoke tests:** 3–5 integration tests hitting the real Partiful API (need valid auth; gate behind
  env like existing `*.integration.test.js`). When Partiful changes something, a smoke test fails
  before users hit it. This IS the spec verifier.
- Decide: CI-run vs. manual (auth secrets in CI is the constraint).

## Done when

Unknown-field logging wired into the Zod parse path; a small real-API smoke suite exists and is
documented (how to run, what auth it needs); drift strategy noted in the port guide.

## Answer

**CLOSED (2026-07-24, commit e45031e).**

**Drift detection** — `src/lib/drift.ts`: `detectDrift(method, response)` diffs a response's
top-level keys against the schema's declared keys (unwraps array element schemas + callable
envelope); `reportDrift()` logs. Wired centrally + `try/catch`-guarded into `apiRequest()` via a
`path→method` reverse map (`checkDrift`) — advisory only, never breaks a request. Logging is
opt-in via `PARTIFUL_DRIFT_LOG` (unset/`0`/`false` = silent; `1`/`true`/`stderr` = stderr line;
any other value = NDJSON append to that path) so the default stdout JSON contract stays clean.
`responseSchemas` registry (method→Zod schema) added to `endpoints.ts`.

**Smoke tests** — `tests/smoke-real-api.test.js`: live-API spec verifier, read-only endpoints,
`describe.skipIf(!PARTIFUL_SMOKE)` so it's off by default (no secrets in CI). Documented run
instructions (token env, optional `PARTIFUL_SMOKE_EVENT_ID`, `PARTIFUL_DRIFT_LOG`). Unit-level
drift suite `tests/drift.test.js` (+6) runs everywhere, no auth.

**CI vs manual:** smoke = manual/secrets-provisioned job; drift unit tests = every run.
**Drift strategy documented** in `docs/TYPESCRIPT-PORT-GUIDE.md` §10.
