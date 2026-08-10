# Partiful remote API contract evidence ledger

**Proposed contract revision:** `2026-08-10.1` (pending explicit owner approval)
**Contract:** `spec/partiful.openapi.json`
**Machine-readable ledger:** `spec/partiful.api-evidence.json`
**Stable citation sources:** `docs/research/2026-08-10-contract-evidence-sources.md`

## Evidence classes

- **Dated live observation:** the March 24 browser-interception research.
- **Reviewed first-party repository research:** reviewed repository source,
  without claiming it proves current server behavior.
- **TypeScript-derived inference:** historical draft or TypeScript transport
  behavior; useful, but not authoritative.
- **Explicit unknown:** a deliberate absence of a remote claim.

## Operation inventory

The recovered 27 operations remain in the remote inventory. `createTextBlast`,
`getLoginToken`, and `signInWithCustomToken` have dated live observations.
`sendAuthCodeTrusted` and `lookupFirebaseUser` have reviewed repository
research. The remaining operations are TypeScript-derived inferences:

- callable: `createEvent`, `cancelEvent`, `getEventInfo`, `getContacts`,
  `addInvitedGuestsAsHost`, cohost lifecycle operations, both home-page
  queries, `addGuest`, `markEventInterest`, and `getCurrentGuest`;
- Firestore: event and guest reads, event patch, and document listing;
- Firebase and auxiliary: refresh, photo upload, and poster catalog.

Every operation's request and response claim is enumerated by JSON Pointer in
the JSON ledger, including operation, parameter, content-type, security,
schema, constraint, and response claims. Unless a claim is specifically
observed, callable result payloads, status codes, error bodies, permission
rules, and limits are **explicit unknown**. Consequently, this proposal uses
OpenAPI `default` responses rather than inventing a success status.

The 379 material claims are audited by
`tests/remote-api-contract.test.js`. Each ledger citation resolves either to a
JSON Pointer in the committed non-authoritative historical artifact or to a
heading in the committed stable source index. This keeps the audit independent
of unreachable historical Git objects in shallow CI checkouts.

The event-image observation establishes fields used for an event's selected
poster image; it does **not** establish that `/posters.json` returns a catalog
array or that its entries have that shape. The catalog operation and `Poster`
schema are therefore TypeScript-derived inferences with citations to
`src/lib/posters.ts`, not dated live observations.

## Resolved conflict

The historical draft described `createTextBlast.params.message` as a string and
added `recipientStatuses`. The dated browser interception instead observed a
nested `message` object with `text`, `to`, `showOnEventPage`, and optional
`images`. The contract adopts the observation and excludes
`recipientStatuses`; no contradictory hybrid schema is retained.

## Open questions

No authenticated live probe was performed for this proposed revision: `partiful doctor`
reported no local credentials. Mutation probes were deliberately omitted.
Future safe observations should update both ledgers, preserve synthetic-only
examples, and receive owner review before changing the revision.
