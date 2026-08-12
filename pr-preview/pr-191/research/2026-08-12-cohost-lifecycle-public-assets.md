# Cohost lifecycle mappings in current public web assets

## Scope and provenance

This note records privacy-safe research from current first-party Partiful web
assets and the official Firebase callable protocol. The assets were fetched
without authentication on 2026-08-12. No credential, account-scoped request,
real event ID, real user ID, cohost mutation, or access-link mutation was
used.

The public [login page](https://partiful.com/login) exposed Next build
`Sf-HOOx63XpPtr5pPkTvg` and deployment query
`dpl_D7TPPj16g1fU46JSHSyrsRURxrK9`.

Exact public sources:

| Asset | SHA-256 | Relevant modules |
| --- | --- | --- |
| <https://partiful.com/_next/static/Sf-HOOx63XpPtr5pPkTvg/_buildManifest.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9> | `2de1d4a26e8d4e9e2370ef970cafab2cd1cd6f5c8402fd54a5ea6f150488cbf6` | route/chunk map |
| <https://partiful.com/_next/static/chunks/pages/_app-b0dc833855a84321.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9> | `812edcea27949b86471cfc5f970cb5b3b961e27a6eea80e7cf6d99aad6623f41` | `95722`, `99181`, `47186`, `35104`, `61713`, `17959`, `13680`, `70820`, `26813` |
| <https://partiful.com/_next/static/chunks/6652-bc845eb80e835b16.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9> | `d6b57ad394646129f4702bb7d74aea214d7c43750c3a9898367fb9d4541a71b4` | `73188`, `81977` |
| <https://partiful.com/_next/static/chunks/1585-abd7081ec2f9f79f.js?dpl=dpl_D7TPPj16g1fU46JSHSyrsRURxrK9> | `437bf13262a46ca96ffb19f625c3ee2a5aaa1996ef9e97ae5b92731341d94487` | `31076` |
| <https://firebase.google.com/docs/functions/callable-reference> | n/a | generic callable `200` and `result` envelope |

The manifest, `_app`, shared cohost chunk, and event-shared chunk all reported
`Last-Modified: Wed, 12 Aug 2026 17:42:55 GMT`.

These assets establish current client request construction, current read-path
selection, and current completion checks. They do not establish business
success, authorization failures, non-`200` statuses, or unseen response
variants.

## Callable transport

Shared module `95722` is the current Firebase callable wrapper. It calls the
named function with one object that contains `params`, and it can also attach
`deviceInfo`, `amplitudeDeviceId`, `amplitudeSessionId`,
`adminAccessRequested`, and `userId` when those values are available. It
removes `undefined` direct `params` properties before dispatch and returns the
decoded callable business result.

The official Firebase callable protocol is the authority only for generic
successful HTTP `200` completion and the top-level `result` envelope. It does
not establish a Partiful business side effect.

## Host-only gating and state reads

Shared module `73188` wires cohost mutation and link controls. For a current
host it exposes:

- `createCohostRequest`
- `deleteCohostRequest`
- `removeCohost`
- `generateEventCohostLink`
- `revokeEventCohostLink`

For a non-host it exposes only `claimEventCohostLink`.

Host gating depends on shared module `70820` hook `M_()`, which checks current
owner membership against the loaded event. This agrees with the existing event
ownership research that treats `ownerIds` membership as host access.

Shared module `99181` builds the current Firestore cohost paths:

```text
events/{eventId}/cohostRequests
events/{eventId}/private/cohostSecret
```

Module `35104` subscribes to the request collection. Module `61713` subscribes
to the secret document. Shared module `47186` defines collection `cohostRequests`
and private-document key `cohostSecret`.

## Cohost request document shape

Shared decoder module `17959` maps each cohost-request document to current
client state by:

- decoding the Firestore document;
- taking `eventId` from the parent collection path;
- reading `target.id` as `targetUserId`; and
- reading `status` from the document body.

Shared module `13680` defines the closed cohost-request status enum:

```text
PENDING
ACCEPTED
DECLINED
```

The current client does not expose another request status here.

## Link document shape and token path

Shared subscription module `61713` treats the secret document as optional and
returns `undefined` when it does not exist. The cohost provider stores that
decoded object as `cohostSecret`.

Current link creation consumes a server-returned `path`. The current event
shared module `31076` defines the invite query key constant
`"accept-cohost"`. The client does not synthesize a token path locally.

This note therefore supports only the current optional secret-document state
and the current server-owned `path` field. It does not claim any other secret
field as required.

## Operation requests and completions

Shared module `73188` makes these exact callable requests:

```text
createCohostRequest({ params: { eventId, targetUserId } })
deleteCohostRequest({ params: { eventId, targetUserId } })
removeCohost({ params: { eventId, targetUserId } })
generateEventCohostLink({ params: { eventId } })
revokeEventCohostLink({ params: { eventId } })
```

The current completion use is:

- `createCohostRequest`: reads `response.data` and does not inspect its
  content further;
- `deleteCohostRequest`: inspects no business field;
- `removeCohost`: inspects no business field;
- `generateEventCohostLink`: reads `response.data.path`; and
- `revokeEventCohostLink`: inspects no business field.

Current reviewed completion consequences are therefore:

- the four non-link-returning operations need only a generic callable protocol
  completion at the current client boundary; and
- `generateEventCohostLink` additionally requires a decoded business-result
  object with nested `data.path` string.

No current public asset proves persisted cohost state, invitation delivery,
link delivery, access grant, revocation side effect, or any error mapping.
Every received non-`200` status, endpoint error, and unseen response shape
remains explicit unknown.
