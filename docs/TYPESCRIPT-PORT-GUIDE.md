# TypeScript Port Guide — partiful-cli

> **Status:** normative build-spec for the JS→TS port (wayfinder `ts-port`, tickets T3–T6).
> Every porting decision below was settled up front. This is the single set of rules an
> agent follows while flipping 29 files `.js`→`.ts`. It is **not** a tutorial and it is
> **not** open for relitigation — if a rule feels wrong, that is a separate conversation,
> not a port-time deviation.

## 0. The one-line thesis

**The port IS the spec.** Typing the `src/lib/` API layer — endpoint request interfaces +
Zod `.passthrough()` response schemas — produces the living API spec as a *byproduct* of
the code. There is no separate hand-maintained spec file to drift. Type the code = write
the spec.

## 1. Toolchain (settled in T1)

- Compiler: `typescript@7`, checked via `npm run typecheck` = `tsc --noEmit`.
- Runtime: `tsx` ESM loader. `bin/partiful` registers `tsx/esm`; **no dist build**. Sources
  run directly whether `.js` or `.ts`.
- `tsconfig.json`: `strict: true` **from day one** (+ `noUncheckedIndexedAccess`,
  `noImplicitOverride`, `noFallthroughCasesInSwitch`), `module`/`moduleResolution` =
  `NodeNext`, `allowJs: true` so JS and TS coexist file-by-file during the migration,
  `noEmit: true`, `verbatimModuleSyntax: true`.
- Runtime schema dep: `zod` (used only for API **responses** — see §4).

## 2. Per-file gate (non-negotiable)

Keep the tree green **file-by-file**. After every file you flip:

1. `npm run typecheck` — `tsc --noEmit` clean under strict.
2. `npm test` — all existing tests green.

Never batch a stack of un-typechecked files. One `.js`→`.ts` flip, gate, next.

## 3. Rules that apply to every file

### 3.1 Import extensions stay `.js`
NodeNext ESM requires the runtime specifier, not the source extension. Even when
`auth.ts` imports from `errors.ts`, the specifier is **`./errors.js`**. Do not rewrite
import paths to `.ts` — that breaks resolution.

### 3.2 `import type` for type-only imports
`verbatimModuleSyntax` is on. Anything used only in type position must be
`import type { Foo } from './x.js'` (or inline `import { type Foo }`). A value import used
only as a type is a compile error — fix it, don't loosen the tsconfig.

### 3.3 No behavior changes
This is a **faithful translation**. Same control flow, same field names, same runtime
output, same error messages. If you're tempted to "fix" or refactor logic, stop — that's a
separate effort (see map "Out of scope"). Types describe what the code already does.

### 3.4 Internal shapes = plain `interface`/`type`, no Zod
Config objects, CLI option bags, helper return shapes, Firestore field-format maps —
things we construct in-process and fully control — get plain TS interfaces/types. **No
runtime validation** on internal data; we own it, the compiler is enough.

### 3.5 Requests = fully-typed interfaces
We control every byte we send. Request params and the payloads passed to `apiRequest` are
specified **completely** as interfaces — no `.passthrough()`, no `any`, no optional-escape
hatches beyond what the real payload actually allows.

### 3.6 Prefer `unknown` + narrowing over `any`
`strict` is on for a reason. When a value is genuinely dynamic (parsed JSON before schema
validation, `catch` bindings), type it `unknown` and narrow. Reserve `any` for documented,
commented, unavoidable escape hatches — expect to need approximately zero.

## 4. API responses = Zod `.passthrough()` + `z.infer` (THE SPEC)

Partiful is an **unofficial API we don't own**. Responses carry known fields but are
*non-exhaustive* — the vendor adds/removes fields without notice. Therefore:

- Every endpoint response gets a **Zod schema** built with **`.passthrough()`** so unknown
  vendor fields flow through instead of throwing.
- The TS type is **inferred** from the schema: `type CreateEventResponse = z.infer<typeof CreateEventResponseSchema>`. Never hand-write a response interface in parallel with its
  schema — the schema is the single source, the type is derived.
- Parsing happens at the lib-layer boundary (where the raw `fetch` JSON is unwrapped).
- `.passthrough()` is also what makes **drift detection** (T6) possible: unknown keys are
  observable at parse time.

## 5. The RPC envelope is ONE reusable generic

Every firebase-callable endpoint wraps its params in the same shape:

```ts
data: { params: <P>, amplitudeDeviceId: string, amplitudeSessionId?: number, userId?: string | null }
```

Specify it **once** as a generic and reference it per-endpoint. Never re-inline the
envelope shape.

```ts
// src/lib/api/envelope.ts
/** Shared Firebase-callable RPC request envelope. `P` = the endpoint's params shape. */
export interface CallableEnvelope<P> {
  data: {
    params: P;
    amplitudeDeviceId: string;
    /** Present on identity-scoped calls (rsvp, cohosts). */
    amplitudeSessionId?: number;
    /** Backfilled from the Firebase JWT; may be null on legacy auth files. */
    userId?: string | null;
  };
}

/** Firebase-callable responses nest the real payload under result.data. */
export interface CallableResult<D> {
  result?: { data?: D };
}
```

## 6. Three transport groups — tag every endpoint

Endpoints fall into exactly three tagged groups. Every spec'd endpoint declares which one
it belongs to (a `transport` discriminant on its metadata — see §7):

| transport          | host                          | verb(s)     | body shape                                  |
|--------------------|-------------------------------|-------------|---------------------------------------------|
| `firebase-callable`| `api.partiful.com`            | POST        | `CallableEnvelope<P>` (§5); resp `CallableResult<D>` |
| `firestore`        | `firestore.googleapis.com`    | GET / PATCH | Firestore typed-document format             |
| `firebase-auth`    | Google (`securetoken.…`)      | POST        | form-encoded token refresh                  |

Firebase-callable endpoints to spec (from T3): `createEvent`, `cancelEvent`,
`getEventInfo`, `getContacts`, `createTextBlast`, `addInvitedGuestsAsHost`,
`getMyUpcomingEventsForHomePage`, `getMyPastEventsForHomePage`, `addGuest`,
`markEventInterest`, `getCurrentGuest`. Plus Firestore GET/PATCH document ops and the
firebase-auth token refresh.

## 7. Where the spec lives

Collect endpoint types + schemas under **`src/lib/api/`** so T5's `schema api.<method>`
command can surface them from one place:

- `src/lib/api/envelope.ts` — the shared generics (§5).
- `src/lib/api/endpoints.ts` — one entry per endpoint: request interface, response Zod
  schema + `z.infer` type, and a small metadata record (`{ method, host, transport }`) so
  the spec is introspectable. This registry is what makes types-as-spec *queryable*.

Keep each endpoint's request interface, response schema, and metadata co-located so the
three never drift from each other.

## 8. Worked example — one fully-typed endpoint

`createEvent` (firebase-callable). This is the pattern to replicate across all endpoints.

```ts
// src/lib/api/endpoints.ts
import { z } from 'zod';
import type { CallableEnvelope, CallableResult } from './envelope.js';

// --- request: fully typed, we control what we send (§3.5) ---
export interface CreateEventParams {
  event: EventDraft;          // internal shape, plain interface (§3.4)
  cohostIds: string[];
}
export type CreateEventRequest = CallableEnvelope<CreateEventParams>;

// --- response: Zod .passthrough(), type inferred (§4) ---
export const CreateEventResponseSchema = z
  .object({
    id: z.string(),
    title: z.string().optional(),
    status: z.string().optional(),
  })
  .passthrough(); // unknown vendor fields flow through, don't throw

export type CreateEventData = z.infer<typeof CreateEventResponseSchema>;
export type CreateEventResponse = CallableResult<CreateEventData>;

// --- metadata: makes the endpoint introspectable for `schema api.createEvent` (§7) ---
export const createEventEndpoint = {
  method: 'POST',
  host: 'api.partiful.com',
  path: '/createEvent',
  transport: 'firebase-callable',
  requestParams: ['event', 'cohostIds'],
  responseSchema: CreateEventResponseSchema,
} as const;
```

At the call site (lib layer), parse the raw JSON through the schema so passthrough +
drift-logging (T6) apply:

```ts
const raw = await apiRequest('POST', '/createEvent', token, payload, verbose);
const data = CreateEventResponseSchema.parse(raw.result?.data ?? {});
```

## 9. Sequencing (do not reorder)

`src/lib/` (API layer, spec born here — T3) → `src/commands/` + `src/helpers/` (consume the
types — T4) → `src/commands/schema.ts` (surface them — T5). Drift + smoke tests (T6) wire
onto the T3 parse path. If while porting a command (T4) you find yourself authoring a new
endpoint shape, it belonged in T3 — go back and add it there.

## 10. Drift detection & smoke tests (T6 summary)

- Because responses use `.passthrough()`, unknown keys are observable at parse time. Log
  them behind a verbose/debug flag so real traffic reveals the vendor's true shape over
  time.
- A small real-API smoke suite (gated behind auth env, like existing `*.integration.test.js`)
  is the spec verifier: when Partiful changes a shape, a smoke test fails before users hit
  it.
