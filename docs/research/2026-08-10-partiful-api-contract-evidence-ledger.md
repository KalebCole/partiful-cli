# Partiful remote API contract evidence ledger

**Contract revision:** `2026-08-10.1`
**Contract:** `spec/partiful.openapi.json`
**Machine-readable ledger:** `spec/partiful.api-evidence.json`

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

Every operation's request and response claim is enumerated in the JSON ledger.
Unless a claim is specifically observed, callable result payloads, status
codes, error bodies, permission rules, and limits are **explicit unknown**.

## Resolved conflict

The historical draft described `createTextBlast.params.message` as a string and
added `recipientStatuses`. The dated browser interception instead observed a
nested `message` object with `text`, `to`, `showOnEventPage`, and optional
`images`. The contract adopts the observation and excludes
`recipientStatuses`; no contradictory hybrid schema is retained.

## Open questions

No authenticated live probe was performed for this revision: `partiful doctor`
reported no local credentials. Mutation probes were deliberately omitted.
Future safe observations should update both ledgers, preserve synthetic-only
examples, and receive owner review before changing the revision.
