# Stable sources for the proposed remote contract

This index makes the evidence ledger resolvable in a clean checkout. It is a
research index, not an approval of any claim. The canonical remote contract
does not import product metadata from these sources.

## Historical transport draft

`spec/research/historical-27-operation-draft.json` is an exact, clearly
non-authoritative copy of `17e9800753ada577408074bbbcadbae8cc8eacf0`'s
27-operation draft. JSON-pointer citations into that local artifact are
validated by the contract test. Its extensions and product policy are research
only and are not part of `spec/partiful.openapi.json`.

## Unknown status decision

No safe live observation in this reconstruction establishes an HTTP success
status for every operation. The proposal therefore uses OpenAPI `default`
responses and records each response-status claim as `explicit-unknown`.
This decision supports both the response **status key** and the explicit
unknown response/body classification. The proposal deliberately attaches no
content schema to `default`; reusable response shapes remain unattached
research evidence until an applicable status/class is observed.

## Update-mask serialization

The historical draft's `firestorePatchEvent` parameter is an array of strings,
and `9e6ed15:src/lib/http.ts#L132-L134` repeats
`updateMask.fieldPaths` once for every field. The proposal represents it as a
form-style, exploded query array; this is a TypeScript-derived transport
inference, not a live observation.

## Dated text-blast observation

The March 24, 2026 browser-interception note
`docs/research/2026-03-24-text-blast-endpoint.md`, under “Request Payload,”
records the nested `message` object used by `createTextBlast`. It is the
primary evidence that supersedes the historical string-message draft.

## Dated authentication observation

The March 24, 2026 note
`docs/research/2026-03-24-auth-flow-endpoints.md`, under “Auth Endpoints,”
records the observed login-token and custom-token exchange wire shapes.

## TypeScript callable and auth research

Reviewed source evidence is preserved from
`9e6ed15:src/lib/api/endpoints.ts#L1-L560`,
`9e6ed15:src/lib/auth.ts#L1-L150`, and
`9e6ed15:src/commands/auth.ts#L145-L340`. It supports inferred callable
registration and authentication behavior; it is not a live-server claim.

## TypeScript Firestore research

Reviewed source evidence is preserved from
`9e6ed15:src/lib/http.ts#L100-L230`, which constructs the Firestore document,
patch, and list requests. It supports inferred transport shapes only.

## TypeScript upload research

Reviewed source evidence is preserved from
`9e6ed15:src/lib/upload.ts#L35-L115`, which constructs multipart `file`
uploads to `uploadPhoto` and reads `uploadData.url`. It supports inferred
transport shapes only.

## Poster interface

`9e6ed15:src/lib/posters.ts#L5-L16` defines the broad `Poster` interface:
optional ID, name, URL, content type, dimensions, tags, categories, and extra
fields. These are TypeScript-derived catalog schema inferences, not live
catalog observations.

## Poster catalog fetch

`9e6ed15:src/lib/posters.ts#L38-L55` fetches
`https://assets.getpartiful.com/posters.json`, checks the response, and casts
the JSON array to `Poster[]`. This supports the inferred catalog endpoint and
array transport only. The dated event-image observation is intentionally not
used for this claim.

## Dated authentication observation (August 11, 2026)

The August 11, 2026 owner-attended authentication observation
`docs/research/2026-08-11-auth-observation.md`, under "Scope and provenance,"
records the observed success status (HTTP 200) and response shapes for
`sendAuthCodeTrusted`, `getLoginToken`, `signInWithCustomToken`,
`refreshToken`, and `lookupFirebaseUser`. The sanitized evidence artifact at
`spec/research/auth-evidence-redacted-20260811.json` retains only HTTP
metadata and JSON paths/types; no credentials, phone numbers, codes, tokens,
or user IDs are present. Privacy-safe negative probes with fake tokens
confirmed error envelope shapes without using real credentials.

This supersedes the earlier March 24 observation reference for response
evidence. The March 24 note remains valid for the request shapes it recorded.

## Firebase web API key value

The March 24, 2026 browser-interception note
`docs/research/2026-03-24-auth-flow-endpoints.md`, under "Firebase API Key,"
records Partiful's public Firebase web API key
(`AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k`). This value is public
configuration embedded in every Partiful web client; it is not a credential.
It is required as the `key` query parameter on all Firebase Identity Toolkit
and Secure Token endpoints.

## Firebase API key referrer restriction

The August 11, 2026 authentication observation
`docs/research/2026-08-11-auth-observation.md`, under "Firebase API key
referrer restriction," records that Firebase Identity Toolkit and Secure Token
endpoints require an HTTP `Referer` header matching an allowed pattern.
Requests without `Referer: https://partiful.com/` receive HTTP 403
`API_KEY_HTTP_REFERRER_BLOCKED`. This was verified by agent privacy-safe
negative probes that succeeded with the Referer header and failed without it.
Origin was not required — probes without an Origin header received valid error
responses.

## Dated poster catalog observation

The unauthenticated, read-only observation in
`docs/research/2026-08-11-poster-catalog-observation.md` establishes the
catalog's HTTP `200` success, complete JSON-array response, required product
fields and their observed types, a `416` specific to an unsatisfiable Range
request, and the facts needed to bind bounded local pagination to one response
representation. It does not claim global catalog exhaustiveness, ordinary
request failure statuses, or rate limiting.
