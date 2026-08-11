# Partiful remote API contract evidence ledger

**Owner-reviewed contract revision:** `2026-08-11.1`
**Contract:** `spec/partiful.openapi.json`
**Machine-readable ledger:** `spec/partiful.api-evidence.json`
**Stable citation sources:** `docs/research/2026-08-10-contract-evidence-sources.md`

## Evidence classes

- **Dated live observation:** the March 24 browser-interception research and
  the August 11 public poster-catalog observation.
- **Reviewed first-party repository research:** reviewed repository source,
  without claiming it proves current server behavior.
- **TypeScript-derived inference:** historical draft or TypeScript transport
  behavior; useful, but not authoritative.
- **Explicit unknown:** a deliberate absence of a remote claim.

## Operation inventory

The recovered 27 operations remain in the remote inventory.

### Dated-live operations

Four operations have dated live observations: `createTextBlast`,
`getLoginToken`, `signInWithCustomToken`, and `getPosterCatalog`.

### TypeScript-derived operations

The other 23 operations remain TypeScript-derived inferences:

- callable: `createEvent`, `cancelEvent`, `getEventInfo`, `getContacts`,
  `addInvitedGuestsAsHost`, `createCohostRequest`, `deleteCohostRequest`,
  `removeCohost`, `generateEventCohostLink`, `revokeEventCohostLink`,
  `getMyUpcomingEventsForHomePage`, `getMyPastEventsForHomePage`, `addGuest`,
  `markEventInterest`, and `getCurrentGuest`;
- Firestore: `firestoreGetEvent`, `firestorePatchEvent`, `firestoreGetGuest`,
  and `firestoreListDocuments`;
- Firebase and auxiliary: `refreshToken`, `sendAuthCodeTrusted`,
  `lookupFirebaseUser`, and `uploadEventPhoto`.

Every operation's request and response claim is enumerated by JSON Pointer in
the JSON ledger, including operation, parameter, content-type, security,
schema, constraint, and response claims. Unless a claim is specifically
observed, callable result payloads, status codes, error bodies, permission
rules, and limits are **explicit unknown**. Consequently, this proposal uses
schema-free OpenAPI `default` responses rather than inventing a success status
or applying a success body to errors.

The 833 material claims are audited by
`tests/remote-api-contract.test.js`. Each ledger citation resolves either to a
JSON Pointer in the committed non-authoritative historical artifact or to a
heading in the committed stable source index. This keeps the audit independent
of unreachable historical Git objects in shallow CI checkouts.

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

## Resolved conflict

The historical draft described `createTextBlast.params.message` as a string and
added `recipientStatuses`. The dated browser interception instead observed a
nested `message` object with `text`, `to`, `showOnEventPage`, and optional
`images`. The contract adopts the observation and excludes
`recipientStatuses`; no contradictory hybrid schema is retained.

## Open questions

No authenticated probe was performed for this reviewed revision. The poster
observation used only the public catalog endpoint and performed no mutation or
upload. Future safe observations should update both ledgers, preserve
privacy-safe evidence, and receive owner review before changing the revision.
