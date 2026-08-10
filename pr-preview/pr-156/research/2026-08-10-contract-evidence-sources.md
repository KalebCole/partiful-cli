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
