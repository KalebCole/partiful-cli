# Remote API contract

`spec/partiful.openapi.json` is proposed revision `2026-08-12.2` of the remote
transport snapshot, pending delegated review. Its owner-reviewed baseline is
`2026-08-12.1`, which is based on owner-reviewed revision `2026-08-11.5`. It
describes only network operations and wire shapes. It does not prescribe
commands, output, credentials, mutation safeguards, or implementation
architecture.

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

## Owner-reviewed read evidence baseline

Revision `2026-08-11.5` records dated response and status evidence for
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
alternate variants. `EventInfo` therefore has no operation-wide top-level
type and no required field list.
Related event-list representations support only optional `endDate`
string/null and `image` object/null unions; unsupported selected-only variants
remain unconstrained.

The current guest callable and its Firestore guest document returned `200`,
and their guest status matched. The one current-guest object does not
establish operation-wide field presence or variants, so `CurrentGuest` has no
operation-wide top-level type and no required field list. `count`, `plusOnes`,
and `userId` remain unconstrained; ordinary non-null plus-one shape is
unknown. Firestore event GET returned
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

## Event mapping correction

Revision `2026-08-12.1` adds current first-party public-asset
research from
`docs/research/2026-08-12-event-read-mapping-public-assets.md`. It promotes
only facts used directly by the current `/events` build:

- module `18539` defines the client event-status vocabulary `UNSAVED`,
  `PUBLISHED`, and `CANCELED`; only `PUBLISHED` and `CANCELED` have S3 product
  mappings;
- homepage event `ownerIds` is the private identity collection used by
  `event.ownerIds.includes(userId)` for host membership; and
- module `54257` defines the closed 16-value guest-status enum.

The `HomePageEvent.status` and conditional `EventInfo.status` properties now
reference that three-value client vocabulary. `HomePageEvent.ownerIds` remains
optional. The inline homepage guest status references the closed
`GuestStatus` schema. No new field is required, and the single-sample
`EventInfo` and `CurrentGuest` top-level uncertainty is unchanged.

The public assets send no paging argument for the two homepage list calls.
This agrees with the observed one-response arrays but does not establish
remote order, limits, snapshots, paging, or future completeness. Digest-bound
pagination and local item/body ceilings are CLI product behavior, not remote
claims.

No response status changed in this proposal. `getEventInfo` retains only
reviewed `200` and `404 NOT_FOUND`; an unobserved `403` remains protocol drift.
`firestoreGetEvent` remains unusable as an S3 success or permission path
because its selected and synthetic requests both returned
`403 PERMISSION_DENIED`.

## RSVP mapping proposal

Revision `2026-08-12.2` adds unauthenticated first-party public-asset research
from
`docs/research/2026-08-12-rsvp-mapping-public-assets.md`. Build
`z1npyrEHkwRMn_JlKXQXR` establishes the current client request and completion
behavior without making a live mutation.

The proposal corrects only these request facts:

- `getCurrentGuest` sends `params.eventId`;
- `addGuest` maps product going and not-going to `GOING` and `DECLINED`,
  uses `count`, named plus-one objects, optional trimmed message, IANA
  timezone, optional current guest ID, optional questionnaire response, and
  `shouldFollowOrgs: false`;
- the direct-event product mapping omits phone number, contact channel,
  captcha, image, invitation ID, discovery source, and password; and
- `markEventInterest` sends `eventId` and boolean `interested`. Its `source`
  is optional and is omitted when a direct event URL has no string source.

The current client requires decoded `addGuest.data` to be an object but
requires no property for completion. For `markEventInterest`, it keeps the
optimistic value only when `data.success` is true and `data.interested` equals
the submitted boolean.

The official
[Firebase callable protocol](https://firebase.google.com/docs/functions/callable-reference)
supports HTTP `200` and a `result` envelope for successful callable
completion. It is the source only for that generic wire behavior. The nested
Partiful `data` objects are current-client requirements, not live endpoint
observations. A recognized completion does not claim stored business state,
delivery, or another side effect. Every unobserved error and status remains
the schema-free `default` response and is protocol drift.

`CurrentGuest.status` now references the existing 16-value `GuestStatus`
vocabulary. No current-guest property becomes required. The single live
object and matching Firestore document remain the only response observation;
callable null, alternate, and failure variants remain unknown.

The proposal does not make RSVP releasable. Safe create-versus-update
selection still lacks a reviewed null-current-guest response, and the
selected-event precondition read does not guarantee all facts used by the
current client. Those blockers are recorded in the research note and product
contract.

## Historical provenance

The 27-operation historical draft in commit
`17e9800753ada577408074bbbcadbae8cc8eacf0` is preserved at
`spec/research/historical-27-operation-draft.json` as a non-authoritative,
stable research artifact. It was not copied as an approved contract and its
product extensions are excluded from the canonical contract. The nested
`createTextBlast.message` object comes instead from the dated observation recorded in
`docs/research/2026-03-24-text-blast-endpoint.md`; the historical string
message/`recipientStatuses` representation is superseded.
