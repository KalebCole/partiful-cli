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

## TypeScript callable and auth research

Reviewed source evidence is preserved from
`9e6ed15:src/lib/api/endpoints.ts#L1-L560`,
`9e6ed15:src/lib/auth.ts#L1-L150`, and
`9e6ed15:src/commands/auth.ts#L145-L340`. It supports inferred callable
registration and authentication request schemas; it is not a live-server
claim. Authentication request schemas remain TypeScript-derived inferences.

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

This is the privacy-safe source for the reviewed authentication response
evidence.

## Firebase web API key value

The privacy-safe redacted extract
`docs/research/2026-08-11-firebase-public-api-key-redacted.md`, under
"Firebase public API key," records Partiful's public Firebase web API key
(`AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k`). This value is public
configuration embedded in every Partiful web client; it is not a credential.
It is required as the `key` query parameter on all Firebase Identity Toolkit
and Secure Token endpoints.

## Firebase API key referrer restriction

The August 11, 2026 authentication observation
`docs/research/2026-08-11-auth-observation.md`, under "Firebase API key
referrer restriction," records that Firebase Identity Toolkit and Secure Token
endpoints require an HTTP `Referer` header matching an allowed pattern.
A privacy-safe negative probe without a `Referer` header received HTTP 403
`API_KEY_HTTP_REFERRER_BLOCKED`; probes using the observed
`Referer: https://partiful.com/` reached the endpoint and returned ordinary
invalid-input responses. The full allowed Referer set is unknown. Origin is
unmodelled and remains unknown because the reviewed evidence establishes no
Origin request fact.

## Dated poster catalog observation

The unauthenticated, read-only observation in
`docs/research/2026-08-11-poster-catalog-observation.md` establishes the
catalog's HTTP `200` success, complete JSON-array response, required product
fields and their observed types, a `416` specific to an unsatisfiable Range
request, and the facts needed to bind bounded local pagination to one response
representation. It does not claim global catalog exhaustiveness, ordinary
request failure statuses, or rate limiting.

## Dated event and contact read observation

The August 11, 2026 owner-attended read-only observation in
`docs/research/2026-08-11-event-contacts-read-observation.md`, under “Scope
and provenance,” records event list, event detail, current guest, Firestore
guest, Firestore event denial, and contact response evidence. The sanitized
artifact at `spec/research/read-evidence-redacted-20260811.json` contains only
allowlisted metadata, paths, types, counts, equality facts, and error codes.
It contains no raw response values or private identifier values.

The operation-specific sections are the stable human citations for the
proposed response and status claims. The proposal does not promote any
unsupported status, pagination rule, permission behavior, ordering rule, or
snapshot behavior.

## Current public contact pagination assets

The public-asset research in
`docs/research/2026-08-11-contacts-pagination-public-assets.md`, under “Exact
callable argument,” records `getContacts` request paging as a sibling of `params`.
Normal contact loading uses empty `params`; a separate administrator flow can
send boolean `useAuthUser`. Its behavior remains unknown. The assets also
record local cursor traversal, client-side name filtering, and client-side
deduplication by contact `id` with the first occurrence winning. These client
behaviors do not establish server ordering or duplicate behavior. This is
reviewed first-party repository research, not a live-server request-shape
observation.

## Current public event mapping assets

The unauthenticated first-party asset research in
`docs/research/2026-08-12-event-read-mapping-public-assets.md`, under “Scope and
provenance,” records the current `/events` build, exact module IDs, asset
digests, event-status wire values, owner-membership host check, and closed
guest-status enum. It contains no account-scoped response values or private
identifiers. Public assets establish current client field and enum behavior;
they do not establish server statuses, field presence, remote pagination,
ordering, limits, or inaccessible-event behavior.

## Official Firebase callable protocol

The official
[Firebase callable protocol](https://firebase.google.com/docs/functions/callable-reference)
specifies the generic wire format for `https.onCall`: a JSON request with one
top-level `data` member, HTTP `200` for a successful callable trigger, and a
JSON response containing `result`. It also states that an `error` member means
failure. This source supports only generic callable status and envelope facts.
It does not establish endpoint-specific Partiful business success.

## Current public RSVP mapping assets

The unauthenticated first-party asset research in
`docs/research/2026-08-12-rsvp-mapping-public-assets.md`, under “Scope and
provenance,” records the current `/e/[event]` build, exact asset URLs and
module IDs, and privacy-safe integrity metadata.

Its “addGuest request” section supports the narrow `GOING` and `DECLINED`
request projections, named plus-one shape, questionnaire response, omitted
null message, timezone, and stripped private contact fields. Its “addGuest
completion” section records JavaScript destructuring of decoded `data`
without an operation-wide type or required business-field claim.

Its “markEventInterest request and completion” section supports boolean
interest and removal requests, optional `source` with direct-page omission,
and the exact `success`/`interested` completion check. Its
“getCurrentGuest” section supports the event-ID request and documents why
callable null and alternate responses remain unknown. No live mutation or
account-scoped read was used.

The “getCurrentGuest” and “addGuest request” sections also support the
current-client selection predicate used by proposed revision `2026-08-12.3`:
client state access is null-safe, an existing guest ID is included, and
`guestId` is omitted when no guest exists. This supports product
create/update selection only. It does not establish that a remote null or
missing response was observed.
