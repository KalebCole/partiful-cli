# Event and contact read observation

## Scope and provenance

On August 11, 2026, the repository owner attended authenticated, read-only
calls for event and contact reads. The agent did not handle credentials,
identities, names, event IDs, or contact details. No mutation or additional
live request was made.

The sanitized source is
`spec/research/read-evidence-redacted-20260811.json`. It contains only HTTP
metadata, normalized paths and types, counts, equality facts, and allowlisted
error codes. Revision `2026-08-11.5` is a proposal. This observation does not
make it owner-reviewed.

## Event list observations

`getMyUpcomingEventsForHomePage` and
`getMyPastEventsForHomePage` each returned HTTP `200` JSON callable
envelopes. The exact array paths were
`result.data.upcomingEvents` and `result.data.pastEvents`. One response held
35 upcoming items and one response held 294 past items.

An immediate repeat returned the same count, identity sequence, and identity
set for each operation. No duplicate identity occurred in either observed
sequence. This is stability evidence for two observations only. It does not
establish an ordering key or snapshot behavior.

The observations establish the named item fields and types in the proposed
schemas. Only `id` was present on every item by an explicit aggregate check.
The selected upcoming item also had a guest object with a string status. This
supports the event ID projection and one selected RSVP-status projection. It
does not establish the remote status-to-product-state mapping or a complete
`userRole` mapping.

No list paging request or response was observed. The arrays were complete
representations in each observed response. Remote pagination, limits, and
future completeness remain unknown.

## Event detail observations

`getEventInfo` returned HTTP `200` for one selected readable event. The
response used `result.data.event` and had the named fields and types in
`EventInfo`. The same selected event returned HTTP `200` while signed out.
This is a fact about that selected event only. It does not establish that all
events are public.

A synthetic missing ID returned HTTP `404` with callable error status
`NOT_FOUND`. No known inaccessible event was supplied. An authenticated
callable permission denial was not observed and is not claimed.

## Guest and Firestore observations

`getCurrentGuest` returned HTTP `200` for the selected event. The observed
path was `result.data.currentGuest`. It was an object with string `id`,
`name`, `status`, and `userId`, integer `count`, and null `plusOnes`.
No null `currentGuest` or other variant was observed.

`firestoreGetGuest` returned HTTP `200` for the document selected by the
observed current guest ID. The document had `name`, `fields`, `createTime`,
and `updateTime`. The named `count`, `createdAt`, `name`, and `status` fields
were typed Firestore value objects with string children. The document ID
matched the current guest ID, and its status matched the callable guest
status. The complete Firestore typed-value grammar remains unknown.

With the observed authenticated credential, `firestoreGetEvent` returned
HTTP `403` and `PERMISSION_DENIED` for both the selected readable event ID and
a synthetic missing ID. This exact operation and credential behavior does not
show attendee denial, resource existence, or Firestore not-found behavior.

## Contact observations

The public-asset request evidence is in
`docs/research/2026-08-11-contacts-pagination-public-assets.md`. It establishes
sibling `params` and `paging` with `maxResults: 1000`; normal loading uses
empty `params`, and `cursor` is null on the first request and a string on later
data-page requests. A separate administrator flow can send boolean
`useAuthUser`; its behavior remains unknown.

The authenticated observation traversed 2,451 contacts twice. Both traversals
returned pages of 1000, 1000, and 451 items, then an empty terminal sentinel.
Each data page had a string `nextCursor`. The terminal response omitted
`nextCursor`. Both traversals had the same private identity sequence and set,
and no duplicate identity was observed.

Every observed contact had a string private `id`, string `name`, and
nonnegative integer `sharedEventCount`. The private identity is transport
data for internal resolution only. Public product output remains
`displayName` and `sharedEventCount`.

The current first-party client traverses the cursor sequence and then filters
names locally. Thus the product name filter applies to the complete traversed
catalog, not to a remote filter parameter. The client also deduplicates by
contact `id` and keeps the first occurrence. This does not establish server
duplicate or ordering behavior. Signed-out `getContacts` returned HTTP `401`
with callable error status `UNAUTHENTICATED`.

## Privacy boundary

The committed artifact contains no raw body values, credentials, identities,
names, IDs, phone numbers, email addresses, or tokens. Tests strictly walk
every aggregate key and string value against an exact allowlist, including
mutation checks for unknown keys and arbitrary values. They also reject
identity or credential value keys, JWT-like values, phone numbers, and email
addresses. Only allowlisted metadata, paths, types, counts, equality facts,
and stable error codes can remain.

## Remaining unknowns

Unsupported statuses and error bodies remain unknown. No claim is made for
rate limiting, request retries, invalid cursors, cursor lifetime or reuse,
backend ordering, snapshot behavior, `useAuthUser`, duplicates outside the
two contact traversals, future catalog completeness, list pagination, or null
and alternate current-guest variants. No inaccessible-event permission probe
exists.
