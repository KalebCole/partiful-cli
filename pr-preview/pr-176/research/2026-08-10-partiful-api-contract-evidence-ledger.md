# Partiful remote API contract evidence ledger

**Proposed contract revision:** `2026-08-11.2`
**Status:** Proposed — pending owner approval
**Contract:** `spec/partiful.openapi.json`
**Machine-readable ledger:** `spec/partiful.api-evidence.json`
**Stable citation sources:** `docs/research/2026-08-10-contract-evidence-sources.md`

## Evidence classes

- **Dated live observation:** the March 24 browser-interception research, the
  August 11 public poster-catalog observation, and the August 11
  owner-attended authentication observation.
- **Reviewed first-party repository research:** reviewed repository source,
  without claiming it proves current server behavior.
- **TypeScript-derived inference:** historical draft or TypeScript transport
  behavior; useful, but not authoritative.
- **Explicit unknown:** a deliberate absence of a remote claim.

## Operation inventory

The recovered 27 operations remain in the remote inventory.

### Dated-live operations

Seven operations have at least one operation-level dated live observation:
`createTextBlast`, `getLoginToken`, `getPosterCatalog`,
`lookupFirebaseUser`, `refreshToken`, `sendAuthCodeTrusted`, and
`signInWithCustomToken`.

Of these seven, six have an observed HTTP 200 success status and typed
response schema (all except createTextBlast, whose response status and body
remain explicit unknown).

### TypeScript-derived operations

The other 20 operations remain TypeScript-derived inferences:

- callable: `createEvent`, `cancelEvent`, `getEventInfo`, `getContacts`,
  `addInvitedGuestsAsHost`, `createCohostRequest`, `deleteCohostRequest`,
  `removeCohost`, `generateEventCohostLink`, `revokeEventCohostLink`,
  `getMyUpcomingEventsForHomePage`, `getMyPastEventsForHomePage`, `addGuest`,
  `markEventInterest`, and `getCurrentGuest`;
- Firestore: `firestoreGetEvent`, `firestorePatchEvent`, `firestoreGetGuest`,
  and `firestoreListDocuments`;
- Firebase auxiliary: `uploadEventPhoto`.

Every operation's request and response claim is enumerated by JSON Pointer in
the JSON ledger, including operation, parameter, content-type, security,
schema, constraint, and response claims. Unless a claim is specifically
observed, callable result payloads, status codes, error bodies, permission
rules, and limits are **explicit unknown**. Operations with an observed HTTP
`200` success have typed response schemas; the schema-free OpenAPI `default`
response is retained for unrecognized statuses.

The 930 material claims are audited by
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
verification codes, tokens, API keys, or user IDs are present.

All five authentication operations (`sendAuthCodeTrusted`, `getLoginToken`,
`signInWithCustomToken`, `refreshToken`, `lookupFirebaseUser`) returned HTTP
`200` on success with typed JSON response bodies. Response schemas are now
included in the contract with observed field paths and types.

Request-shape provenance is **not** promoted by this observation. Each
operation's request claims retain their prior evidence class: `getLoginToken`
and `signInWithCustomToken` retain the March 24 dated observation;
`sendAuthCodeTrusted`, `refreshToken`, and `lookupFirebaseUser` retain their
TypeScript-derived inferences.

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

Error responses were observed (403 for wrong auth code, 400 for invalid
tokens) but are **not** promoted to contract-level status codes because the
full failure space is not characterized. The failure boundary is identical to
the poster catalog: received non-`200` is `contract.protocol_changed`,
no-response/network is `remote.unavailable`, rate limiting is not claimed.

The Firebase Identity Toolkit and Secure Token endpoints require an HTTP
`Referer` header matching an allowed pattern. This is a Firebase project
configuration fact, not a Partiful callable behavior.

## Resolved conflict

The historical draft described `createTextBlast.params.message` as a string and
added `recipientStatuses`. The dated browser interception instead observed a
nested `message` object with `text`, `to`, `showOnEventPage`, and optional
`images`. The contract adopts the observation and excludes
`recipientStatuses`; no contradictory hybrid schema is retained.

## Open questions

Future safe observations should update both ledgers, preserve privacy-safe
evidence, and receive owner review before changing the revision. The remaining
18 TypeScript-derived operations have no observed success status or response
shape.
