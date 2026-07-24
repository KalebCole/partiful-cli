# T5 — Rewire schema command → schema api.<method>

**Type:** task (AFK) · **Blocks on:** T3 · **Status:** OPEN

## Question

Today `src/commands/schema.js` is a static hardcoded dict documenting CLI FLAGS. Make the API
endpoint types authored in T3 introspectable via a new `schema api.<method>` namespace, so the
spec-as-types is queryable from the CLI (the "source of truth" payoff).

## Scope

- Keep existing `schema <command>` (CLI-flag lookup) — still useful, different layer.
- Add `schema api.<method>` (e.g. `schema api.createEvent`) reading from the T3 endpoint
  types/Zod schemas: host, method, transport, request params, known response fields.
- Decide output shape (mirror existing schema format vs. diverge) — this is the "Not yet specified"
  item that graduates here.

## Done when

`partiful schema api.<method>` prints endpoint spec derived from the T3 types for every spec'd
endpoint; existing `schema <command>` unchanged; tests cover the new namespace.

## Answer

<!-- record on close -->
