# Remote API contract

`spec/partiful.openapi.json` is proposed revision `2026-08-11.5` of the remote
transport snapshot. Independent review is required before it can become
owner-reviewed. It describes only network operations and wire shapes. It does
not prescribe commands, output, credentials, mutation safeguards, or
implementation architecture.

## Authority and change process

Live, privacy-safe observations can initiate a correction but do not authorize
implementation by themselves. A proposed change must receive owner or
delegated orchestrator approval, update the contract and
`spec/partiful.api-evidence.json`, then run
`npm test -- tests/remote-api-contract.test.js`. TypeScript, historical drafts,
tests, and endpoint notes are research evidence, never automatic authority.
The CLI product contract is separate and remains the authority for user-facing
behavior.

The evidence ledger records the classification and source for each operation,
each request/response claim, and each component schema. `explicit-unknown`
means the contract intentionally declines to claim precision. Never replace it
with guessed fields, captured credentials, real identifiers, or personal data.
`docs/research/2026-08-10-partiful-api-contract-evidence-ledger.md` is the
human-readable companion to the machine-readable ledger.

## Proposed read evidence revision

Revision `2026-08-11.5` proposes dated response and status evidence for
`getMyUpcomingEventsForHomePage`, `getMyPastEventsForHomePage`,
`getEventInfo`, `getCurrentGuest`, `firestoreGetEvent`,
`firestoreGetGuest`, and `getContacts`. The sanitized source is
`spec/research/read-evidence-redacted-20260811.json`.

The two event lists were observed as complete arrays in one response, at
`result.data.upcomingEvents` and `result.data.pastEvents`. No remote list
pagination was observed, so pagination remains unknown. One selected event
was readable both authenticated and signed out. This does not make all events
public. A synthetic missing event returned `404 NOT_FOUND`. That one detail
object does not establish operation-wide field presence, nullability, or
alternate variants. `EventInfo` therefore has no required field list.
Related event-list representations support only optional `endDate`
string/null and `image` object/null unions; unsupported selected-only variants
remain unconstrained.

The current guest callable and its Firestore guest document returned `200`,
and their guest status matched. The one current-guest object does not
establish operation-wide field presence or variants, so `CurrentGuest` has no
required field list. `count`, `plusOnes`, and `userId` remain unconstrained;
ordinary non-null plus-one shape is unknown. Firestore event GET returned
`403 PERMISSION_DENIED` for both the selected readable ID and a synthetic ID
with the observed authenticated request context. This does not establish
attendee denial or Firestore not-found behavior.

`getContacts` used sibling empty `params` and cursor `paging`. Two traversals
returned 1000, 1000, and 451 items followed by an empty terminal sentinel.
Name filtering and first-occurrence ID deduplication are client-side over the
traversed catalog. They do not establish server ordering or duplicate behavior.
Private identity is modeled only as internal transport data; public product
output remains display name and shared-event count. Signed-out access returned
`401 UNAUTHENTICATED`.

Unsupported statuses, ordering, snapshots, invalid cursors, cursor lifetime,
`useAuthUser`, rate limiting, future catalog completeness, inaccessible-event
permission behavior, and other unobserved variants remain explicit unknowns.

## Historical provenance

The 27-operation historical draft in commit
`17e9800753ada577408074bbbcadbae8cc8eacf0` is preserved at
`spec/research/historical-27-operation-draft.json` as a non-authoritative,
stable research artifact. It was not copied as an approved contract and its
product extensions are excluded from the canonical contract. The nested
`createTextBlast.message` object comes instead from the dated observation recorded in
`docs/research/2026-03-24-text-blast-endpoint.md`; the historical string
message/`recipientStatuses` representation is superseded.
