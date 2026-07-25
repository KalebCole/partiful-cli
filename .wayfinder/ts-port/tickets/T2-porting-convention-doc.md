# T2 — Write porting convention doc

**Type:** task (AFK) · **Blocks on:** T1 · **Status:** ✅ CLOSED

## Question

Write the single convention doc every subsequent port ticket follows, so an agent porting 29 files
makes consistent choices. This is the "porting guide" — a build-spec, not a tutorial. All decisions
are already made (below); this ticket just writes them down as enforceable rules.

## Decisions (already settled by Kaleb — do NOT relitigate)

- **Strictness:** `strict: true` from day one.
- **API responses:** wrap in Zod schemas using `.passthrough()` so unknown vendor fields don't throw
  (Partiful is an unofficial API we don't own; responses are known-fields-non-exhaustive). Types are
  inferred from the Zod schema via `z.infer<>`.
- **Internal shapes** (config, CLI options, helpers): plain TS `interface`/`type`, no Zod.
- **Requests:** fully typed interfaces (we control what we send — specify completely).
- **Envelope:** the shared `{data:{params:{...}, amplitudeDeviceId}}` RPC envelope is ONE reusable
  generic type, referenced per-endpoint — never re-specified.
- **Transports:** three tagged groups — firebase-callable (POST api.partiful.com), firestore
  (GET/PATCH firestore.googleapis.com, typed-document format), firebase-auth (Google endpoints).
- **Import extensions:** keep `.js` in import specifiers (NodeNext ESM requirement even for .ts).
- **No behavior changes** — faithful translation only.

## Done when

`docs/TYPESCRIPT-PORT-GUIDE.md` exists capturing the above as file-by-file rules + a worked example
of one typed endpoint (envelope + request interface + Zod `.passthrough()` response + z.infer type).

## Answer

**CLOSED 2026-07-24.** Guide committed at `docs/TYPESCRIPT-PORT-GUIDE.md`. Captures all
settled decisions as file-by-file rules: per-file gate (typecheck + test), `.js` import
specifiers under NodeNext, `import type` discipline (verbatimModuleSyntax), no behavior
changes, internal shapes = plain interfaces, requests = fully-typed interfaces, responses =
Zod `.passthrough()` + `z.infer`, the ONE reusable `CallableEnvelope<P>`/`CallableResult<D>`
generic, three tagged transports (firebase-callable / firestore / firebase-auth), spec lives
in `src/lib/api/` (envelope.ts + endpoints.ts registry). Includes the worked `createEvent`
example: envelope reuse + request interface + Zod passthrough response + z.infer type +
introspectable metadata record.
