# Partiful remote API contract evidence ledger

**Proposed contract revision:** `2026-08-11.3`
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

The 1008 material claims are audited by
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

Request-shape provenance is **not** promoted by these observations. Each
operation's request claims retain their prior evidence class.

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

## Firebase transport configuration

Revision `2026-08-11.3` formalizes two transport configuration facts required
by the Go implementation's Firebase requests:

1. **Firebase web API key value**: The `firebaseApiKey` security scheme now
   carries `x-publicValue: AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k`. This is
   a public value embedded in every Partiful web client, documented in the
   March 24, 2026 browser interception under "Firebase API Key." It is not a
   credential. It is required as the `key` query parameter on
   `signInWithCustomToken`, `refreshToken`, and `lookupFirebaseUser`.

2. **Referer header requirement**: Each Firebase operation now has a required
   `Referer: https://partiful.com/` header parameter. This is documented in
   the August 11, 2026 auth observation under "Firebase API key referrer
   restriction." The observed probe without a Referer received HTTP 403
   `API_KEY_HTTP_REFERRER_BLOCKED`. This is the only observed accepted value;
   the full set of values allowed by the remote restriction remains unknown.

Origin is **not** modelled. Agent negative probes succeeded without an Origin
header, receiving valid error responses (400). Origin is browser CORS
behavior, not a Firebase transport requirement. If the Go implementation sends
Origin, that is an implementation choice, not a contract claim.

The 1008 material claims are audited by `tests/remote-api-contract.test.js`.

## Resolved conflict

The historical draft described `createTextBlast.params.message` as a string and
added `recipientStatuses`. The dated browser interception instead observed a
nested `message` object with `text`, `to`, `showOnEventPage`, and optional
`images`. The contract adopts the observation and excludes
`recipientStatuses`; no contradictory hybrid schema is retained.

## Open questions

Future safe observations should update both ledgers, preserve privacy-safe
evidence, and receive owner review before changing the revision. The remaining
20 TypeScript-derived operations have no observed success status or response
shape.
