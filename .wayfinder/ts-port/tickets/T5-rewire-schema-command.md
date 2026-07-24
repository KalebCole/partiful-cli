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

**CLOSED (2026-07-24, commit 06eeb68).** Added a `schema api.<method>` namespace driven off
the `apiEndpoints` registry in `src/lib/api/endpoints.ts` (the T3 types-as-spec source of truth):
- `schema api` → lists every spec'd method.
- `schema api.<method>` → prints `{ method, transport, host, httpMethod, path, requestParams,
  responseFields }` derived from the registry + Zod schemas.
- Bare `schema` now lists both CLI `commands` and `api.*` methods; existing `schema <command>`
  CLI-flag lookup is unchanged (different layer, kept).
- Output shape: mirrors the existing JSON-envelope format (jsonOutput/jsonError), diverging only
  in the payload keys (endpoint metadata vs. CLI-flag params).
- Tests: `tests/schema-api.test.js` (+5) cover list, per-endpoint spec, firestore transport, and
  the unknown-method not_found path.
