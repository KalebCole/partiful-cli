# Text-blast mappings from current public web assets

## Scope and provenance

This note records only unauthenticated, first-party public Partiful assets and
official Firebase and Firestore protocol documents. No account, credential,
real event, guest list, text blast, phone number, or live mutation was used.
In particular, this research did not call `createTextBlast` or any Firestore
write.

The public login page at <https://partiful.com/login> exposed Next build
`Sf-HOOx63XpPtr5pPkTvg` with deployment query
`dpl_D7TPPj16g1fU46JSHSyrsRURxrK9` on 2026-08-12.
The exact manifest URL was
<https://partiful.com/_next/static/Sf-HOOx63XpPtr5pPkTvg/_buildManifest.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9>
with SHA-256
`2de1d4a26e8d4e9e2370ef970cafab2cd1cd6f5c8402fd54a5ea6f150488cbf6`.
Relevant assets were:

| Asset | SHA-256 | Relevant modules |
|---|---|---|
| <https://partiful.com/_next/static/chunks/2583.bd7537e8be64d6a1.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9> | `b0905f97d8fd2998a957884539c923809f2838ba502398ed6ae7841c24eddfdd` | `12583`, `8012` |
| <https://partiful.com/_next/static/chunks/502-a1c7122349c97d59.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9> | `011b2bb1bc2050b6516a465854a39d3cb3c2cc774952db984100cff92d0bd9d1` | `5851` |
| <https://partiful.com/_next/static/chunks/1585-abd7081ec2f9f79f.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9> | `437bf13262a46ca96ffb19f625c3ee2a5aaa1996ef9e97ae5b92731341d94487` | `75076`, `69865` |
| <https://partiful.com/_next/static/chunks/pages/_app-b0dc833855a84321.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9> | `812edcea27949b86471cfc5f970cb5b3b961e27a6eea80e7cf6d99aad6623f41` | `17959`, `47186`, `48144`, `82262`, `99181` |
| <https://firebase.google.com/docs/functions/callable-reference> | n/a | callable protocol |
| <https://firebase.google.com/docs/firestore/reference/rest/v1/projects.databases.documents/list> | n/a | list-documents protocol |

The current assets establish client request construction, precondition checks,
and completion handling. They do not establish endpoint business success,
delivery, authorization, or unseen error bodies.

## `createTextBlast` helper and completion

Dynamic chunk `2583` module `12583` defines:

```js
async function ea(e) {
  try {
    return (await wF("createTextBlast", { params: e })).data
  } catch (e) {
    console.error("Error creating text blast", e)
    throw e
  }
}
```

The same module's send button awaits `ea(...)`, marks the modal done, and closes
it. It does not inspect a business-success field, recipient report, or post-send
read. A rejected promise sets the modal error state.

The reviewed completion facts are therefore:

- callable protocol success uses HTTP `200` with the official Firebase callable
  result envelope;
- the decoded callable value must be non-nullish, because the helper reads its
  `.data` property before returning; and
- no reviewed nested business field is required after that helper access.

A recognized completion means only that the exact submitted request reached the
current client success path. It does not prove stored state, SMS delivery,
email delivery, push delivery, or any recipient result.

## Reviewed current client request facts for this slice

The same `12583` module builds the modal request with:

```js
{ eventId, message: { text, to, showOnEventPage, ... } }
```

and sets `showOnEventPage` from the current checkbox state. The current modal
also appends `notificationChannels` and optional `images`, but issue #170 keeps
the dated nested `createTextBlast.message` mapping as the transport baseline.
This note promotes only the reviewed `all-guests` `to` representation and the
current page-display flag.

The `Select all` button does **not** send a sentinel, empty array, or expanded
identity list. Module `12583` sets the audience to:

```js
V.map(e => e.group)
```

where `V` is the enabled, non-empty subset of the current group list. The
request then sends that exact ordered array as `message.to`.

## Group derivation for `all-guests`

Module `5851` constructs the ordered group list as:

1. `"invited"`
2. `"checkedIn"`
3. one of these status sets:
   - `APPROVED`, `PENDING_APPROVAL`, `WAITLISTED_FOR_APPROVAL`, `WITHDRAWN`,
     `REJECTED` when `guestAction === "APPLY"`;
   - `RESPONDED_TO_FIND_A_TIME` when the current event satisfies the active
     find-a-time predicate; or
   - `GOING`, `MAYBE`, `DECLINED`, and conditional `WAITLIST` for ordinary RSVP
     events.

Chunk `_app` module `82262` defines the recipient predicate used for these
counts. `"invited"` matches the five invited statuses
`READY_TO_SEND`, `SENDING`, `SENT`, `SEND_ERROR`, and `DELIVERY_ERROR`.
`"checkedIn"` matches any guest with non-null `checkIn`. Exact status groups
match the same string value.

For `all-guests`, only enabled groups with a positive count are included in the
submitted array. Module `5851` also marks invited groups as excluded from
`Select all` when the invited count exceeds `100`. The current bundle contains
one private hard-coded owner exception for that exclusion. This note does not
promote that private identifier. A privacy-safe implementation must therefore
fail closed or conservatively exclude the invited group when it cannot verify an
exemption.

## Host-only precondition reads

Current host tooling reads the event guest collection and host-message
collection before enabling send:

- `_app` module `99181` builds `events/{eventId}/guests` and
  `events/{eventId}/hostMessages` Firestore collection references;
- `_app` module `17959` keeps guest `status` and `checkIn` when it normalizes a
  guest document, and keeps optional host-message `type` when it normalizes a
  host-message document; and
- chunk `2583` module `12583` passes the loaded guest list length and current
  text-blast list length into module `5851`'s `canSendTextBlast` helper.

That helper disables send for exactly three reviewed reasons:

- `event_expired`
- `max_text_blasts_per_event_reached`
- `no_guests`

The expiry helper uses `_app` modules `48144` and `26969`. It treats an event
as old when `endDate + 67 days` is in the past, or when `startDate + 6 hours +
67 days` is in the past for an event without `endDate`.

Current host-message counting filters the collection to documents whose `type`
is missing, null, or `TEXT_BLAST`. No recipient failure detail is consulted
before submit.

## Submitted-only boundary

Because the reviewed current client does not inspect a business-success field or
perform a post-send read, a safe CLI result can report only submitted intent.
The correct public result is the exact submitted event ID, audience,
`showOnEventPage`, `submitted: true`, and `recipientStatus:
"not-reported"`.
