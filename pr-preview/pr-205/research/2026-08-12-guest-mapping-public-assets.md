# Guest list and host-invite mapping in current public web assets

## Scope and provenance

This note records privacy-safe research from Partiful's current public web
assets. No authentication, credential, account-scoped response, or mutation
was used. Public assets are research evidence, not remote contract authority.

The current authenticated web shell is the Next.js `/login` route. On
2026-08-12 it referenced build `Sf-HOOx63XpPtr5pPkTvg` and deployment
`dpl_D7TPPj16g1fU46JSHSyrsRURxrK9`. The guest work in this note uses:

- [`pages/_app-b0dc833855a84321.js`, module `95722`](https://partiful.com/_next/static/chunks/pages/_app-b0dc833855a84321.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9)
- [`6652-bc845eb80e835b16.js`, module `68219`](https://partiful.com/_next/static/chunks/6652-bc845eb80e835b16.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9)
- [`pages/e/[event]-f833fec21304964a.js`, module `52105`](https://partiful.com/_next/static/chunks/pages/e/%5Bevent%5D-f833fec21304964a.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9)

## Current callable wrapper

Current `_app` module `95722` still wraps every callable request with the
available `deviceInfo`, `amplitudeDeviceId`, optional
`amplitudeSessionId`, optional administrator flag, and optional `userId`.
It strips undefined members from `params`, then calls the Firebase callable
wrapper against `https://api.partiful.com`.

This confirms that current guest callables still use the same reviewed
top-level callable transport envelope as the other dated August 2026
callables.

## `getGuests` request and pagination

Current event chunk module `68219` defines:

```js
async function l(eventId, cursor, maxResults = 500, password) {
  return await wF("getGuests", {
    params: {
      eventId,
      includeInvitedGuests: true,
      password,
    },
    paging: {
      cursor,
      maxResults,
    },
  });
}
```

The same module paginates with:

```js
do {
  let page = await l(eventId, paging?.nextCursor, 500, password);
  ({ paging, data } = page);
  onPage(data, pageIndex);
} while (paging?.nextCursor != null && pageCount < 20);
```

Current public assets therefore establish these request facts:

- operation name `getGuests`;
- `params.eventId` string;
- `params.includeInvitedGuests: true`;
- optional `params.password`;
- sibling `paging.cursor`;
- sibling `paging.maxResults` with current client default `500`; and
- client-side traversal that stops when `paging.nextCursor` is nullish or after
  20 pages.

Unlike current contacts traversal, the current guests client consumes the
final page's `data` before it stops. It does **not** require an empty
terminal sentinel page.

## `getGuests` response and guest fields

Module `68219` destructures the callable payload as:

```js
({ paging, data } = page);
```

So the callable payload is a JSON object with `data` and `paging`. Under the
Firebase callable protocol, the raw HTTP response is the reviewed callable
envelope whose decoded payload is that object.

The same current guest-list code treats a record as a top-level guest when:

```js
guest.anchorGuestId == null
```

Current event page module `52105` whitelists these guest fields for the active
guest object:

```js
[
  "id",
  "name",
  "status",
  "findATimeResponse",
  "count",
  "plusOnes",
  "questionnaireResponse",
  "invitedMutualsCount",
  "invitedViaPhoneContacts",
  "invitedByUsers",
  "invitedBy",
  "anchorGuestId",
  "checkInCode"
]
```

Current event chunk module `68219` also indexes guest state by `id`, projects
`userId` and `name`, and uses `status` and `count` in guest-state UI. This
supports the narrow reviewed guest fields required by the Go host guest list:

- string `id`;
- string `name`;
- reviewed `status`;
- numeric `count`;
- null-or-string `anchorGuestId`; and
- optional string `userId`.

The public assets do not establish a complete operation-wide guest object
schema, other fields, failure bodies, rate limiting, inaccessible-event
behavior, or whether guest records can omit fields outside this reviewed
narrow subset.

## `addInvitedGuestsAsHost` request

Current event chunk module `68219` defines:

```js
async function u(
  eventId,
  userIdsToInvite,
  invitationMessage,
  otherMutualsCount,
  phoneContactsToInvite,
  emailsToInvite,
) {
  return await wF("addInvitedGuestsAsHost", {
    params: {
      eventId,
      userIdsToInvite,
      invitationMessage,
      otherMutualsCount,
      phoneContactsToInvite,
      emailsToInvite,
    },
  });
}
```

This corrects the old historical `guests` array inference. The current
request parameters are:

- `eventId`;
- `userIdsToInvite`;
- `invitationMessage`;
- `otherMutualsCount`;
- `phoneContactsToInvite`; and
- `emailsToInvite`.

The same assets do not establish a reviewed non-empty element schema for
`phoneContactsToInvite` or `emailsToInvite`. The Go contract therefore keeps
their array members open when it documents the transport and uses empty arrays
for the reviewed contact-invite CLI slice.

## `addInvitedGuestsAsHost` completion

Current guest-invite UI awaits the promise from `addInvitedGuestsAsHost` and
then shows success UI. It does not inspect any returned business field before
success.

This establishes only generic callable completion for the current invite
client. It does **not** establish a reviewed persisted invite state, delivery,
or any response property stronger than successful callable completion.
