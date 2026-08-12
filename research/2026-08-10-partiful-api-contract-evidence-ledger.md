# Partiful remote API contract evidence ledger

**Owner-reviewed contract revision:** `2026-08-12.2`
**Owner-reviewed baseline:** `2026-08-12.1`
**Status:** Owner-reviewed under the issue #114 delegation
**Contract:** `spec/partiful.openapi.json`
**Machine-readable ledger:** `spec/partiful.api-evidence.json`
**Stable citation sources:** `docs/research/2026-08-10-contract-evidence-sources.md`

## Evidence classes

- **Dated live observation:** the March 24 browser-interception research, the
  August 11 public poster-catalog observation, and the August 11
  owner-attended authentication and read-only event/contact observations.
- **Current first-party public-asset research:** immutable assets from the
  current Partiful deployment, without authentication or account-scoped data.
- **Official protocol specification:** generic Firebase callable status and
  envelope behavior; never endpoint-specific Partiful business success.
- **Reviewed first-party repository research:** reviewed repository source,
  without claiming it proves current server behavior.
- **TypeScript-derived inference:** historical draft or TypeScript transport
  behavior; useful, but not authoritative.
- **Explicit unknown:** a deliberate absence of a remote claim.

## Operation inventory

The recovered 27 operations remain in the remote inventory.

### Dated-live operations

Fourteen operations have at least one operation-level dated live observation:
`createTextBlast`, `firestoreGetEvent`, `firestoreGetGuest`, `getContacts`,
`getCurrentGuest`, `getEventInfo`, `getLoginToken`,
`getMyPastEventsForHomePage`, `getMyUpcomingEventsForHomePage`,
`getPosterCatalog`, `lookupFirebaseUser`, `refreshToken`,
`sendAuthCodeTrusted`, and `signInWithCustomToken`.

Twelve operations have an observed HTTP `200` status. Eleven of those
operations have a typed `200` response body. `sendAuthCodeTrusted` has an
observed `200` status, but its body remains unclaimed. The text-blast operation
retains an unknown response. The Firestore event read has an observed typed
`403` response, not an observed success.

### Current public-asset operations

`addGuest` and `markEventInterest` now have current first-party public-asset
request and completion mappings. Their errors and all non-`200` statuses
remain unknown.

Two additional callable operations have protocol-specified HTTP `200` completion responses.
`addGuest` and `markEventInterest` use the official Firebase callable
status/result envelope plus current first-party client completion behavior.
Neither is a live business-success observation.

### TypeScript-derived operations

The other 11 operations remain TypeScript-derived inferences:

- callable: `createEvent`, `cancelEvent`, `addInvitedGuestsAsHost`,
  `createCohostRequest`, `deleteCohostRequest`, `removeCohost`,
  `generateEventCohostLink`, and `revokeEventCohostLink`;
- Firestore: `firestorePatchEvent` and `firestoreListDocuments`;
- Firebase auxiliary: `uploadEventPhoto`.

Every operation's request and response claim is enumerated by JSON Pointer in
the JSON ledger, including operation, parameter, content-type, security,
schema, constraint, and response claims. Unless a claim is specifically
observed, callable result payloads, status codes, error bodies, permission
rules, and limits are **explicit unknown**. Eleven operations with an observed
HTTP `200` status and two operations with protocol-specified callable
completion have typed response bodies. The schema-free send-code `200` and
OpenAPI `default` responses do not claim a body.

The 1285 material claims are audited by
`tests/remote-api-contract.test.js`. Each ledger citation resolves either to a
JSON Pointer in the committed non-authoritative historical artifact or to a
heading in the committed stable source index. This keeps the audit independent
of unreachable historical Git objects in shallow CI checkouts.

## Poster catalog evidence

The event-image observation remains unrelated to the catalog. The
unauthenticated August 11 observation directly establishes the public
`/posters.json` HTTP `200` response as a complete array representation and
establishes every field needed by the product `Poster`. It also establishes a
bodyless `416` for an unsatisfiable read range, but the implementation will not
send Range requests and this does not establish ordinary failure mapping.
Exact ordinary-request failure statuses remain unknown, so the schema-free
`default` remains explicit unknown.

Only HTTP `200` is recognized as catalog success. Local no-response or network
failures may map to `remote.unavailable`; any received non-`200` status is
unrecognized and must fail closed as `contract.protocol_changed`. Rate
limiting is not claimed. This boundary lets S1 close without guessing while
preserving the implementation gate until owner approval and merge.

The observed array contained 2,114 entries and one repeated ID. Local
pagination must preserve response order and duplicates. Exact resumption can
bind a cursor to the full payload digest, normalized filters, and next offset;
it cannot rely on `If-Match`, which was not enforced by the observed endpoint.

## Authentication evidence

On August 11, 2026, the repository owner completed an attended authentication
session. The agent did not enter credentials, send codes, or access live
tokens. A sanitized evidence artifact at
`spec/research/auth-evidence-redacted-20260811.json` retains only HTTP
metadata and JSON paths/types. An independent scan confirmed no phone numbers,
verification codes, tokens, API keys, or user IDs are present. A second
artifact at `spec/research/auth-error-probes-redacted-20260811.json` records
privacy-safe fake-token probes with no real credentials.

All five authentication operations (`sendAuthCodeTrusted`, `getLoginToken`,
`signInWithCustomToken`, `refreshToken`, `lookupFirebaseUser`) returned HTTP
`200` on success. Four operations have promoted error responses with precise
schemas and operation-specific product failure mappings:

- `getLoginToken` `403` → `input.invalid` / `AUTH_CODE_REJECTED`
- `signInWithCustomToken` `400` → `auth.expired`
- `refreshToken` `400` → `auth.expired`
- `lookupFirebaseUser` `400` → `auth.expired` (optional for S2)

`sendAuthCodeTrusted` has an observed `200` with unclaimed body shape (auth
login uses no send-response field). No error status is promoted.

Request-shape provenance is **not** promoted by these observations. All five
authentication request schemas remain TypeScript-derived inferences, matched
to the preserved historical request schemas. The newly observed public
Firebase key and Referer facts remain separately classified as
dated-live observations.

Schema `required` arrays list fields that were present in every observed
success response and that the implementation needs for correct operation
(token extraction, session establishment, user lookup). Their absence in
a future `200` response constitutes `contract.protocol_changed` under the
fail-closed boundary. This is a validation design choice supported by the
observation, not a universal server guarantee.

Schema `additionalProperties: true` on response schemas is a
TypeScript-derived inference, not an observed fact. One success response
cannot prove that additional fields will appear or that the server promises
open-ended extensibility.

`auth.human_required` remains a local no-private-terminal failure, not a
remote status mapping. Every other received status is `contract.protocol_changed`.
No-response/network is `remote.unavailable`. Rate limiting is not claimed.

The Firebase Identity Toolkit and Secure Token endpoints require an HTTP
`Referer` header matching an allowed pattern. This is a Firebase project
configuration fact, not a Partiful callable behavior.

## Event and guest read evidence

The sanitized owner-attended artifact records one-response arrays at the exact
paths `result.data.upcomingEvents` and `result.data.pastEvents`. The observed
counts were 35 and 294. Immediate repeats had the same count, identity
sequence, and identity set, with no duplicate identity. Only item `id`
completeness was checked. Named event fields retain their observed types and
nullability without a broader presence claim.

No remote paging field was observed for either list. Revision
`2026-08-11.5` records the observed complete array representations but keeps
remote pagination, limits, ordering, snapshot behavior, and list failures
unknown. Event ID projection is supported. One selected guest status supports
an RSVP projection for that item. Current public assets close the S3
event-state, owner-membership, and guest-status vocabulary. The client
event-status vocabulary is `UNSAVED`, `PUBLISHED`, and `CANCELED`; only the
latter two have S3 product mappings. Owner membership uses
`event.ownerIds.includes(userId)`. The events UI treats any owner membership
as hosting and does not expose a primary-host/cohost distinction. The 16
current guest statuses remain lossless in the read projection.

`getEventInfo` returned `200` at `result.data.event` for one selected readable
event and returned `404 NOT_FOUND` for a synthetic missing ID. The selected
event also returned `200` signed out. The proposal does not generalize this
fact to other events. No inaccessible event was supplied, and no authenticated
callable permission denial is claimed. One event-detail object cannot
establish operation-wide field presence, nullability, or alternate variants,
so `EventInfo` has no operation-wide top-level type and no required field
list. Related event-list representations support only optional `endDate`
string/null and `image` object/null unions. Selected-only fields without
related variant evidence remain unconstrained.

`getCurrentGuest` returned `200` with an object at
`result.data.currentGuest`. This single object cannot establish
operation-wide field presence, nullability, or alternate variants, so
`CurrentGuest` has no operation-wide top-level type and no required field
list. Cross-source fields retain only their supported types; `count`,
`plusOnes`, and `userId` remain unconstrained. A null current guest, an
ordinary non-null plus-one shape, and other variants remain unknown.

`firestoreGetGuest` returned `200` for the current guest document. Its
document ID and status matched the callable guest. The complete Firestore
typed-value grammar remains unknown. `firestoreGetEvent` returned
`403 PERMISSION_DENIED` for both the selected readable ID and a synthetic
missing ID with the observed authenticated request context. This does not
establish attendee denial, resource existence, or Firestore not-found
behavior.

## RSVP public-asset evidence

Current build `z1npyrEHkwRMn_JlKXQXR` corrects the RSVP request projections.
`getCurrentGuest` accepts only `params.eventId`. Its one live HTTP `200`
object remains the response authority; the public client does not demonstrate
a null callable result.

The current client maps product going and not-going to `GOING` and
`DECLINED`. The broader remote RSVP status control also demonstrates `MAYBE`.
`addGuest` receives `eventId` and an RSVP object with required name, count,
plus-one array, status, timezone, and `shouldFollowOrgs: false`. The product
projection supplies only named plus ones, while the remote client recognizes
additional private linked/contact variants. Message, existing guest ID, and
questionnaire response are optional. A null product message maps to omission.
The client strips phone number, contact channel, captcha token, and an
embedded linked-plus-one user before the call. The narrow product mapping does
not send image, invitation ID, discovery source, or password.

The direct event page sends boolean `interested` to `markEventInterest` and
omits `source` when the URL has no string source. The same path sends
`interested: false` for removal.

The official Firebase callable protocol supports HTTP `200` with a `result`
envelope for successful callable completion. The current `addGuest` client
does not establish a type or required business property for `result.data`.
For `markEventInterest`, it accepts the optimistic result only when
`result.data.success` is truthy and `result.data.interested` equals the
submitted boolean. These are protocol and client-completion predicates, not
remote response type claims or live observations of stored Partiful state.
Every unobserved error and status remains explicit unknown.

`CurrentGuest.status` references the existing closed `GuestStatus` vocabulary,
but remains optional. Current-guest nullability, no-guest create behavior,
required profile name, selected-event mutation preconditions, endpoint
failure mappings, and post-write business state remain unknown.

## Contact read evidence

Current first-party public assets establish the exact relevant request:
sibling `params` and `paging`, `maxResults: 1000`, and a null or string
`cursor`. Normal loading uses empty `params`. A separate administrator flow
can send boolean `useAuthUser`, but its behavior remains unknown. This request
claim is reviewed first-party repository research. The owner-attended
observation establishes the response and status claims.

Two authenticated traversals each returned page sizes 1000, 1000, 451, and an
empty terminal sentinel. Data pages had string `nextCursor` values. The
terminal response omitted `nextCursor`. Both 2,451-item traversals had the same
private identity sequence and set, with no observed duplicate identity.

Every observed item had string private identity and name fields and a
nonnegative integer `sharedEventCount`. The private identity is an internal
transport field only. Public product output remains `displayName` and
`sharedEventCount`. First-party assets establish client-side name filtering
after cursor traversal. They also establish client-side deduplication by
contact `id`, with the first occurrence winning. These are client behaviors;
they do not establish server duplicate or ordering behavior. Signed-out
`getContacts` returned `401 UNAUTHENTICATED`.

Invalid cursors, cursor lifetime and reuse, backend ordering, snapshot
behavior, `useAuthUser`, rate limiting, unsupported statuses, and duplicates
outside these two observations remain unknown. Future catalog completeness is
also unknown.

## Read evidence privacy

`spec/research/read-evidence-redacted-20260811.json` contains only HTTP
metadata, allowlisted paths and types, counts, equality facts, and stable error
codes. It contains no raw response values, credentials, identities, names,
event ID values, or contact details. Contract tests strictly walk every
aggregate key and string value against an exact allowlist and include
unknown-key and arbitrary-value mutation checks. They also reject unsafe
identity, credential, JWT-like, phone, and email value patterns.

## Firebase transport configuration

Revision `2026-08-11.4` formalizes two transport configuration facts required
by the Go implementation's Firebase requests:

1. **Firebase web API key value**: The `firebaseApiKey` security scheme now
   carries `x-publicValue: AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k`. This is
   a public value embedded in every Partiful web client, documented in the
   privacy-safe redacted Firebase public-key extract. It is not a credential.
   It is required as the `key` query parameter on
   `signInWithCustomToken`, `refreshToken`, and `lookupFirebaseUser`.

2. **Referer header requirement**: Each Firebase operation now has a required
   `Referer: https://partiful.com/` header parameter. This is documented in
   the August 11, 2026 auth observation under "Firebase API key referrer
   restriction." The observed probe without a Referer received HTTP 403
   `API_KEY_HTTP_REFERRER_BLOCKED`. This is the only observed accepted value;
   the full set of values allowed by the remote restriction remains unknown.

Origin is unmodelled and remains unknown because the reviewed evidence
establishes no Origin request fact.

The 1285 material claims are audited by `tests/remote-api-contract.test.js`.

## Resolved conflict

The historical draft described `createTextBlast.params.message` as a string and
added `recipientStatuses`. The dated browser interception instead observed a
nested `message` object with `text`, `to`, `showOnEventPage`, and optional
`images`. The contract adopts the observation and excludes
`recipientStatuses`; no contradictory hybrid schema is retained.

## Open questions

Future safe observations must update both ledgers, preserve privacy-safe
evidence, and receive owner review before changing the revision. The remaining
11 TypeScript-derived operations have no observed response status or shape.
The read-specific unknowns above remain explicit and must not become inferred
implementation behavior.
