# Event-read mappings in current public web assets

## Scope and provenance

This note records privacy-safe research from Partiful's current first-party
public web assets. The assets were fetched without authentication on
August 12, 2026. No credential, account-scoped request, response containing
event or user data, or mutation was used.

The public login page supplied build ID `z1npyrEHkwRMn_JlKXQXR` and deployment
query `dpl_4w28tFBmSUwoToDpQB8CU8gt16sL`. Its build manifest assigns
`pages/events-16d5030ecfa4fd91.js` to `/events`.

Exact sources:

- [login page](https://partiful.com/login)
- [build manifest](https://partiful.com/_next/static/z1npyrEHkwRMn_JlKXQXR/_buildManifest.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL)
- [`/events` page asset](https://partiful.com/_next/static/chunks/pages/events-16d5030ecfa4fd91.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL)
- [shared `_app` asset](https://partiful.com/_next/static/chunks/pages/_app-05be110884cfdc20.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL)
- [shared chunk containing the past-operation name](https://partiful.com/_next/static/chunks/8733-cef914451f66c66d.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL)

The immutable asset responses reported `Last-Modified: Tue, 11 Aug 2026
21:17:57 GMT`. The build manifest, events asset, and `_app` asset were 11,476,
65,476, and 2,369,333 bytes. Their SHA-256 digests were, respectively:

```text
6014233108449ef82cd4066e6f29476845e3a637e0adfceb79b7cb17fd7d0159
f06c1b0d78258ebb02ad6a95152ef4f9628c25c7ab54720180ed43db97e9f1fa
46a2fe543a4aa17f2cd5d5f8d0547b8f8e692b43b9d7b7061d78f1a01228c9fe
```

Public assets are research evidence. They do not establish unobserved server
statuses, ordering, limits, pagination, or response-field presence.

## One-response event-list calls

The `/events` asset names the two callable operations and sends empty
`params`. It reads `response.data`, then reads `upcomingEvents` or
`pastEvents`; it sends no paging member.

Faithfully deminified modules `48666`, `17350`, `35932`, and `22920`:

```js
const upcomingOperation = "getMyUpcomingEventsForHomePage";

async function getUpcomingHomePageData() {
  return (await call(upcomingOperation, { params: EMPTY_OBJECT })).data ??
    {
      upcomingEvents: EMPTY_ARRAY,
      initialPastEvents: EMPTY_ARRAY,
      eventCategoryCounts: EMPTY_CATEGORY_COUNTS,
    };
}

const pastOperation = "getMyPastEventsForHomePage";

async function getPastHomePageData() {
  return (await call(pastOperation, { params: EMPTY_OBJECT })).data ??
    { pastEvents: EMPTY_ARRAY };
}
```

The names, empty `params`, and absence of a paging argument agree with the
owner-reviewed `.5` request and one-response observations. The fallback
objects are client behavior and do not describe a successful remote body.

## Event status

Shared module `18539` contains this exact minified enum construction:

```js
let d=((r={}).UNSAVED="UNSAVED",r.LIVE="PUBLISHED",
  r.CANCELED="CANCELED",r)
```

Faithfully deminified:

```js
const EventStatus = {
  UNSAVED: "UNSAVED",
  LIVE: "PUBLISHED",
  CANCELED: "CANCELED",
};
```

Thus the current client symbolic cases `LIVE` and `CANCELED` correspond to the
wire values `PUBLISHED` and `CANCELED`. The `/events` page checks
`event.status === EventStatus.CANCELED` for the canceled label and checks
`event.status === EventStatus.LIVE` when selecting live invited and attended
events. For S3 reads, the lossless product mapping is:

```text
PUBLISHED -> active
CANCELED  -> cancelled
```

`UNSAVED` is a client draft state. The reviewed homepage list observations do
not establish it as a returned value, so it is not promoted for S3.

## Owner membership and hosting

Shared module `50218` exports `Ro`. Its exact minified body is:

```js
function Z(e,t){
  return t.status===l.fb.UNSAVED||null!=e&&t.ownerIds.includes(e)
}
```

Faithfully deminified for non-draft event reads:

```js
function isHost(userId, event) {
  return userId != null && event.ownerIds.includes(userId);
}
```

Equivalently, the product rule is
`event.ownerIds.includes(userId)`. The `/events` page uses this helper to
build `hostedEvents`. It also independently computes:

```js
const isHost = event.ownerIds.some(ownerId => ownerId === currentUserId);
const guest = "guest" in event ? event.guest : undefined;
```

It supplies that boolean as the event card's `isHost` value. Owner membership
therefore means hosting in the current first-party events UI. The asset does
not distinguish a primary host from a cohost. S3 can map any current-user
owner membership to product `host`; product `cohost` stays reserved until a
distinct reviewed field exists.

The route separates non-host records by guest presence:

```js
const hostedEvents = events.filter(event => isHost(currentUserId, event));
const guestEvents = events.filter(
  event => hasGuest(event) || !isHost(currentUserId, event),
);
```

For a reviewed representation with `ownerIds`, S3 can therefore emit
`attendee` when the current user is not an owner and a `guest` object is
present, and `none` when neither condition applies. If `ownerIds` is absent,
the role is unavailable rather than guessed.

## Guest statuses

Shared module `54257` defines this closed current enum:

```js
const GuestStatus = {
  READY_TO_SEND: "READY_TO_SEND",
  SENDING: "SENDING",
  SEND_ERROR: "SEND_ERROR",
  DELIVERY_ERROR: "DELIVERY_ERROR",
  SENT: "SENT",
  INTERESTED: "INTERESTED",
  WAITLIST: "WAITLIST",
  MAYBE: "MAYBE",
  DECLINED: "DECLINED",
  GOING: "GOING",
  PENDING_APPROVAL: "PENDING_APPROVAL",
  APPROVED: "APPROVED",
  WITHDRAWN: "WITHDRAWN",
  WAITLISTED_FOR_APPROVAL: "WAITLISTED_FOR_APPROVAL",
  REJECTED: "REJECTED",
  RESPONDED_TO_FIND_A_TIME: "RESPONDED_TO_FIND_A_TIME",
};
```

The same module's `VO` helper recognizes all 16 values for current event-card
status display. Other helpers form narrower groups for invite delivery,
attendance, application, and response behavior. None supplies one
semantically lossless grouping for a general read result.

S3 therefore uses a separate lossless read enum. It lowercases each exact
value and replaces underscores with hyphens:

| Remote | Product read value |
| --- | --- |
| `READY_TO_SEND` | `ready-to-send` |
| `SENDING` | `sending` |
| `SEND_ERROR` | `send-error` |
| `DELIVERY_ERROR` | `delivery-error` |
| `SENT` | `sent` |
| `INTERESTED` | `interested` |
| `WAITLIST` | `waitlist` |
| `MAYBE` | `maybe` |
| `DECLINED` | `declined` |
| `GOING` | `going` |
| `PENDING_APPROVAL` | `pending-approval` |
| `APPROVED` | `approved` |
| `WITHDRAWN` | `withdrawn` |
| `WAITLISTED_FOR_APPROVAL` | `waitlisted-for-approval` |
| `REJECTED` | `rejected` |
| `RESPONDED_TO_FIND_A_TIME` | `responded-to-find-a-time` |

This read enum does not redefine S5's narrower writable RSVP intent. A null
read value means that no current guest object or status is available. An
unknown present status is protocol drift, not null.

## Pagination boundary

The public assets make one call for each selected homepage representation and
do not supply or consume remote paging for these arrays. The `.5` observation
establishes complete one-response representations of 35 and 294 items,
repeated with identical identity sequences. Their largest observed body was
773,455 bytes.

These facts support local snapshot pagination only. They do not establish
remote order meaning, remote limits, server snapshots, or future completeness.
The product can preserve the received sequence, bind its existing opaque
cursor to a digest of the complete response plus command/filter and next
offset, refetch once on resume, and reject a digest change as a snapshot
conflict.

A local ceiling is product safety, not a remote claim. S3 limits the
one-response representation to 1,000 items (the existing product
`--max-items` ceiling) and 8 MiB. It rejects an exceeded bound without
returning a truncated success.

## Facts not established

These assets do not establish:

- operation-wide presence of optional event fields;
- a primary-host/cohost distinction inside `ownerIds`;
- a universal signed-out event-read policy;
- inaccessible-event behavior or a callable permission response;
- remote paging, ordering keys, limits, snapshot behavior, or future
  completeness;
- null or alternate `getCurrentGuest` variants;
- Firestore event success or not-found behavior; or
- mappings for event address, guest limit, poster, links, or other fields not
  named above.

S3 must not call Firestore event GET: `.5` records `403 PERMISSION_DENIED` for
both selected and synthetic IDs with the observed credential context.
