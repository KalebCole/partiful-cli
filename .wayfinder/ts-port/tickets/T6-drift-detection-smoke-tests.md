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

<!-- record on close -->
