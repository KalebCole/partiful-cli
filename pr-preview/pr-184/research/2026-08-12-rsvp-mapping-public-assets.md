# RSVP mappings in current public web assets

## Scope and provenance

This note records privacy-safe research from Partiful's current first-party
public web assets and the official Firebase callable protocol. The assets were
fetched without authentication on August 12, 2026. No account-scoped request,
credential, real identifier, questionnaire answer, message, RSVP mutation, or
SMS was used.

The public [login page](https://partiful.com/login) supplied Next build ID
`z1npyrEHkwRMn_JlKXQXR` and deployment query
`dpl_4w28tFBmSUwoToDpQB8CU8gt16sL`. The build manifest assigns the RSVP UI to
`/e/[event]`.

Exact public sources:

- [build manifest](https://partiful.com/_next/static/z1npyrEHkwRMn_JlKXQXR/_buildManifest.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL)
- [shared `_app` asset](https://partiful.com/_next/static/chunks/pages/_app-05be110884cfdc20.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL)
- [interest asset](https://partiful.com/_next/static/chunks/1945-cbff097107005c38.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL)
- [RSVP controls asset](https://partiful.com/_next/static/chunks/1585-abd7081ec2f9f79f.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL)
- [RSVP flow asset](https://partiful.com/_next/static/chunks/2565-91cc334b3dc48a18.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL)
- [`/e/[event]` page asset](https://partiful.com/_next/static/chunks/pages/e/%5Bevent%5D-f833fec21304964a.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL)
- [official Firebase callable protocol](https://firebase.google.com/docs/functions/callable-reference)

The manifest, `_app`, and `1945` responses reported `Last-Modified: Tue, 11
Aug 2026 21:17:57 GMT`. The other three responses reported `Last-Modified:
Tue, 11 Aug 2026 21:18:03 GMT`.

Public assets establish current client inputs and completion checks. They do
not establish endpoint business success, failure statuses, authorization, or
unseen response variants.

## Callable transport

Shared module `95722` calls Firebase `httpsCallable`. It supplies the caller's
argument as the callable `data` value, removes `undefined` direct properties
from `params`, and returns the decoded callable `result`. The wrapper can add
`deviceInfo`, `amplitudeDeviceId`, `amplitudeSessionId`,
`adminAccessRequested`, and `userId` when those values are available. The RSVP
operation modules supply `params`; they do not inspect these generic metadata
members.

The official Firebase protocol requires a JSON request with one top-level
`data` field. A successful callable trigger has HTTP `200` and a JSON response
with `result`; an `error` member means failure. This generic rule supports the
callable success status and result envelope only. It does not prove that a
Partiful mutation changed state.

## Guest status and product intent

Shared module `54257` contains the same closed 16-value `GuestStatus` enum
recorded in the event-read note. The RSVP buttons use the narrow response set
`GOING`, `MAYBE`, and `DECLINED`; another helper restricts a no-maybe control
to `GOING` and `DECLINED`.

The exact writable product mappings supported here are:

```text
going      -> addGuest rsvp.status = "GOING"
not-going  -> addGuest rsvp.status = "DECLINED"
interested -> markEventInterest interested = true
```

Interest removal uses `interested = false`. It is useful as an exact remote
fact but is not a fourth product intent.

Read status stays separate. A present `CurrentGuest.status` can use the full
16-value lossless `EventReadRsvp` mapping. This does not make all 16 values
writable.

## getCurrentGuest

Page module `77504` sends:

```js
call("getCurrentGuest", { params: { eventId } })
```

Page module `52105` reads `response.data.currentGuest`,
`response.data.anchorGuest`, and `response.data.tickets`. It only makes this
call while augmenting an existing real-time guest that has linked plus ones,
an anchor guest, or tickets. Otherwise it uses the real-time guest directly.
The state update is null-safe, but this callable path does not demonstrate a
null `currentGuest` response.

The owner-reviewed `2026-08-11.5` evidence remains the response authority: one
HTTP `200` had an object at `result.data.currentGuest`, and one Firestore guest
HTTP `200` had the same document ID and status. It does not support callable
nullability, missing-status behavior, ordinary non-null plus-one shapes, or
alternate responses. Those variants stay unknown.

## addGuest request

Module `82565` constructs the RSVP form and completion flow. Module `52105`
adds the current guest ID and page-only values, then sends:

```js
call("addGuest", { params: { eventId, rsvp } })
```

For the product's direct, non-protected, non-discover path, the narrow request
mapping is:

```json
{
  "eventId": "event input",
  "rsvp": {
    "name": "current guest or authenticated profile display name",
    "count": "partySize",
    "plusOnes": [
      { "name": "plus-one display name" }
    ],
    "status": "GOING or DECLINED",
    "timezone": "input IANA timezone",
    "shouldFollowOrgs": false
  }
}
```

The client adds a nonempty message as `message` after trimming it. An absent
message is omitted, not sent as JSON null. Questionnaire answers are strings
keyed by question ID. For a going response, the client adds:

```json
{
  "questionnaireResponse": {
    "questionnaireVersion": "event.questionnaireVersions.length - 1",
    "answers": {
      "question ID": "answer"
    }
  }
}
```

The questionnaire step is skipped for `DECLINED`; the product therefore must
not attach a questionnaire response to `not-going`.

Shared module `7073` recognizes named plus ones as `{name}`. It also recognizes
private linked, phone-contact, and user-contact variants. The product mapping
uses only named plus ones. Module `64949` removes `phoneNumber`,
`channelPreference`, and `captchaToken` before `addGuest`, and removes the
embedded `user` object from a linked plus one.

For an existing guest, module `52105` adds its private `guestId`. For no guest,
`guestId` is `undefined` and is omitted. The same module adds the browser's
timezone. It can add a stored event password; the product has no reviewed
password input, so the narrow mapping omits `password`. Normal product
requests also omit `emailInvitationId`, `image`, and `_discoverSource`.

The client always submits `name`, `count`, `plusOnes`, `status`, and
`shouldFollowOrgs`. Going and not-going therefore have exact wire status
values, but an implementation still needs reviewed pre-read facts for the
name, existing guest ID, questionnaire version, and protected/ticketed event
conditions.

## addGuest completion

Module `52105` destructures decoded `addGuest.data`. This rejects only
null/undefined at the client boundary; JavaScript permits object
destructuring from other JSON value kinds. The client therefore does not
establish an operation-wide `data` type. When data is an object, it splits
optional `previousStatus` and `linkedPlusOneFailures`, then treats the
remaining properties as the updated guest for local state.
Module `82565` uses the optional previous status for analytics and optional
linked-plus-one failures for a warning. It checks no endpoint success boolean.

The narrow remote completion contract is therefore HTTP `200` under the
official callable protocol with:

```json
{
  "result": {
    "data": {}
  }
}
```

`data` must be an object, but no business field is required for the product.
All its properties remain unclaimed. This completion means only that the
submitted callable request completed. It does not prove the stored RSVP,
delivery, notification, or another remote side effect.

## markEventInterest request and completion

Module `64951` sends the supplied params to `markEventInterest` and returns
decoded `response.data`. Module `34679` sends:

```js
{ eventId, interested, source }
```

The direct event page passes its optional `source` from the URL query. When
the URL has no string `source`, the value is `undefined`, module `95722`
removes it from `params`, and the JSON request contains only `eventId` and
`interested`. A direct event-page-equivalent CLI request therefore omits
`source`; it does not invent a tracking value. The same toggle sends
`interested: false` for removal.

The client accepts completion only when decoded `data.success` is truthy and
`data.interested` equals the requested boolean. Otherwise it rolls back its
optimistic local value. This is a JavaScript client predicate, not a remote
field-type claim. A representative accepted completion is:

```json
{
  "result": {
    "data": {
      "success": true,
      "interested": true
    }
  }
}
```

The `interested` member must equal the submitted value, including `false` for
removal. This is a current client completion check, not an observation of
Partiful business state.

## Apply preconditions and irreducible blockers

A safe plan must bind the normalized input, event ID, exact operation and
request projection, a private stable account fingerprint, and a digest of the
reviewed pre-read representation. Apply must reacquire the account and
pre-read once, require the same bindings, consume the single-use plan, and
make at most one mutation request. It must not retry automatically.

The current public and owner-reviewed evidence leaves these blockers:

1. `getCurrentGuest` null and alternate behavior is unknown. The CLI cannot
   safely distinguish create from update for a user with no current guest.
2. No reviewed Partiful profile read guarantees the required `name` when no
   current guest supplies one.
3. The selected-event read contract does not yet guarantee the questionnaire
   version, party-size limits, password requirement, ticketing conditions, or
   other event facts used by module `82565`.
4. No observed `addGuest` or `markEventInterest` response exists. The proposed
   HTTP `200` envelopes are limited to official callable protocol and current
   client completion behavior. Every unobserved status or error stays protocol
   drift.
5. The available post-write reads cannot populate the former shared RSVP
   output without guessing field presence or nullability.

These blockers prevent a releasable Go RSVP read or addGuest-backed mutation.
They do not prevent review of the exact request projections, the interest
completion check, or the mutation safety contract.
