# Contact pagination in current public web assets

## Scope

This note records privacy-safe research from Partiful's current public web
assets. No authentication, credential, account-scoped request, identity-bearing
response, or mutation was used. Public assets are research evidence, not remote
contract authority.

## Exact callable argument

Partiful's `_app` asset, module `41886`, calls the Partiful callable wrapper as:

```js
wF("getContacts", {
  params: { useAuthUser },
  paging: {
    maxResults: 1000,
    cursor: previousPage?.paging?.nextCursor,
  },
});
```

Source:
[`pages/_app-05be110884cfdc20.js`, module `41886`](https://partiful.com/_next/static/chunks/pages/_app-05be110884cfdc20.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL).

The ordinary contacts provider omits `useAuthUser`. A separate organization
administrator flow explicitly passes `useAuthUser: true`.

Sources:
[`8317-e80fd0b80a07ac2a.js`, module `28339`](https://partiful.com/_next/static/chunks/8317-e80fd0b80a07ac2a.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL);
[`pages/_app-05be110884cfdc20.js`, module `41886`](https://partiful.com/_next/static/chunks/pages/_app-05be110884cfdc20.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL).

## Partiful wrapper and wire envelope

Partiful wrapper module `95722` adds available device, Amplitude, administrator,
and user metadata as siblings of `params` and `paging`. It removes undefined
members from `params`, calls Firebase `httpsCallable`, and returns
`sdkResult.data` after date normalization.

Source:
[`pages/_app-05be110884cfdc20.js`, module `95722`](https://partiful.com/_next/static/chunks/pages/_app-05be110884cfdc20.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL).

Firebase SDK module `68997` recursively encodes undefined as null, wraps the
callable argument under top-level `data`, accepts a response payload under
`data` or `result`, and returns the decoded payload as `sdkResult.data`.
Therefore the first relevant request is:

```json
{
  "data": {
    "params": {},
    "paging": {
      "maxResults": 1000,
      "cursor": null
    }
  }
}
```

Later requests send the previous decoded `paging.nextCursor`.

Source:
[`pages/_app-05be110884cfdc20.js`, module `68997`](https://partiful.com/_next/static/chunks/pages/_app-05be110884cfdc20.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL);
[Firebase callable protocol](https://firebase.google.com/docs/functions/callable-reference).

## Response and traversal

After the SDK and Partiful wrappers, module `41886` expects:

```text
page.data
page.paging.nextCursor
```

It makes sequential cursor requests. A non-null `nextCursor` emits that page's
`data` and starts the next request. A nullish `nextCursor` emits an empty array
and stops, so the public client expects a terminal sentinel response whose data
is not consumed. It does not use offsets, page numbers, total counts, response
length, retries, or repeated-cursor detection.

Source:
[`pages/_app-05be110884cfdc20.js`, module `41886`](https://partiful.com/_next/static/chunks/pages/_app-05be110884cfdc20.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL).

The raw official-style response paths are `result.data` and
`result.paging.nextCursor`. The bundled SDK also accepts top-level `data`.
Public assets do not prove which top-level field the current server emits.

## Ordering, duplicates, and filtering

Provider module `28339` appends emitted pages in order. `_app` module `62629`
deduplicates cumulatively by contact `id`, keeping the first occurrence.
Neither asset establishes the backend ordering key or why duplicates can
occur.

Sources:
[`8317-e80fd0b80a07ac2a.js`, module `28339`](https://partiful.com/_next/static/chunks/8317-e80fd0b80a07ac2a.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL);
[`pages/_app-05be110884cfdc20.js`, module `62629`](https://partiful.com/_next/static/chunks/pages/_app-05be110884cfdc20.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL).

Text filtering is local. Modules `2914` and `3710` normalize and substring-match
contact names. Phone-contact matching can also inspect phone-contact names and
numbers inside the browser, but the CLI product contract does not expose those
private values. Prefix matches sort before other matches without a documented
secondary key. Past-guest, phone-contact, and follower filters are also local.

Source:
[`pages/_app-05be110884cfdc20.js`, modules `2914` and `3710`](https://partiful.com/_next/static/chunks/pages/_app-05be110884cfdc20.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL).

Invite module `6238` calls `getContactsFilteredByEvent({eventId})`, converts the
result to an identity set, and intersects it with the already loaded contact
catalog locally.

Source:
[`1585-abd7081ec2f9f79f.js`, module `6238`](https://partiful.com/_next/static/chunks/1585-abd7081ec2f9f79f.js?dpl=dpl_4w28tFBmSUwoToDpQB8CU8gt16sL).

## Repository mismatch

The current TypeScript CLI sends empty `params`, reads one `result.data`, filters
names locally, and truncates locally. It neither sends `paging` nor follows
`nextCursor`.

Sources:
`src/commands/contacts.ts:19-44`;
`src/lib/cohosts.ts:75-98`;
`src/lib/api/endpoints.ts:158-174`;
`src/lib/api/endpoints.ts:391-399`.

The owner-reviewed remote contract still permits only empty request `params`
and keeps response/status behavior unknown.

Sources:
`spec/partiful.openapi.json#/paths/~1getContacts/post`;
`spec/partiful.api-evidence.json#/operationClaims/getContacts`.

## Remaining authenticated-only facts

Owner-attended authenticated observation is still required to establish:

- actual page lengths and whether 1,000 is a request size, server cap, or both;
- the current raw top-level response field;
- cursor type, lifetime, reuse, invalid-cursor behavior, and terminal response;
- ordering stability, duplicates within/across pages, and snapshot behavior;
- the behavior of `useAuthUser`;
- success and failure status/body mappings; and
- whether the public client's inferred traversal produces a complete catalog.

Generic Firestore cursor rules do not apply without evidence that this callable
uses a corresponding Firestore query.

Sources:
[Firestore order and limit](https://firebase.google.com/docs/firestore/query-data/order-limit-data);
[Firestore query cursors](https://firebase.google.com/docs/firestore/query-data/query-cursors).
