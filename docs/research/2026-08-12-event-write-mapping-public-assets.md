# Event-write mapping from current public assets

## Scope and provenance

This note records only unauthenticated, first-party public Partiful assets and
official Firebase protocol documents. No account, credential, event ID, guest
ID, request body, or live Partiful write was used. In particular, this research
did not call `createEvent`, `cancelEvent`, any Firestore write, or any upload.

The entry point was <https://partiful.com/login>. On 2026-08-12 it named Next.js
build `2KXQa2wzQWzlyvnJPIrVj` and deployment
`dpl_D9bWXWUaTVfWyHiJz1RT7qdjuzUC`. The exact manifest URL was
<https://partiful.com/_next/static/2KXQa2wzQWzlyvnJPIrVj/_buildManifest.js?dpl=dpl_D9bWXWUaTVfWyHiJz1RT7qdjuzUC>
(SHA-256
`71b824c18ccb5779d024fde62431a302f26f2c0d9ffaafd3640d02d9c86880f3`).
Relevant deployment assets were:

| Asset | SHA-256 | Relevant modules |
|---|---|---|
| <https://partiful.com/_next/static/chunks/pages/create-27dc07c14b6ce0ff.js?dpl=dpl_D9bWXWUaTVfWyHiJz1RT7qdjuzUC> | `a74bf14e957b8c6a70267d519a8973ef6b4d70fac1b90a703b15226f62e3bc19` | `23552`, `25231`, `79372` |
| <https://partiful.com/_next/static/chunks/pages/_app-6d18e4a563898a0d.js?dpl=dpl_D9bWXWUaTVfWyHiJz1RT7qdjuzUC> | `bc5ae6516170e0331b2bac59af77bc92d93f4970a090acbc1e318ac41bc87499` | `18539`, `42919`, `48144`, `50218`, `54257`, `68997`, `92793`, `95722` |
| <https://partiful.com/_next/static/chunks/1585-2532983fb5eac8e0.js?dpl=dpl_D9bWXWUaTVfWyHiJz1RT7qdjuzUC> | `48f1fc96f0aa318eea09d99a89e0f51e1eaf3863bee7e4c43ea045641aa3ac63` | `52630`, location editor |
| <https://partiful.com/_next/static/chunks/6652-bc845eb80e835b16.js?dpl=dpl_D9bWXWUaTVfWyHiJz1RT7qdjuzUC> | `d6b57ad394646129f4702bb7d74aea214d7c43750c3a9898367fb9d4541a71b4` | `30067` |
| <https://partiful.com/_next/static/chunks/2248-0ec69126f468d508.js?dpl=dpl_D9bWXWUaTVfWyHiJz1RT7qdjuzUC> | `c70b07cd5d86f1d6ce8f0e1e02d12fd4a3dbcf6d52a7e023ed01c3dd1f96cde7` | `22248` |
| <https://partiful.com/_next/static/chunks/8317-f3e4abcc21cc60c3.js?dpl=dpl_D9bWXWUaTVfWyHiJz1RT7qdjuzUC> | `b0d9c7bd12cdbe021df80f2c990319af85638e2d6499580aeca79f5fcdbbc5e4` | current event editor |
| <https://partiful.com/_next/static/chunks/8290-fee201d02665178a.js?dpl=dpl_D9bWXWUaTVfWyHiJz1RT7qdjuzUC> | `a16edfddf6c6bf48d7e758f578e5fde09733a5c51dfbb4ce8a06d7aa0686d17d` | `95074` |

The current default configuration was
<https://assets.partiful.com/newEventDefaultsConfig.json> (SHA-256
`29bdacdf84365583c4014f3f52d12e94821f9dbcba6e189c085085ebabf59dbc`).
Its selected poster, theme, and reduced-motion effect vary by date, locale, and
motion preference. They are not stable CLI defaults.
Module `79372` also contains the stable non-international fallback
`theme: "cloudflow"`, `effect: "fireflies"`, `titleFont: "display"`, and
the `Let's Party` poster. The narrow product fixes those exact reviewed values
instead of reproducing environment-dependent selection.

## Callable wrapper and serialization

Module `95722` invokes Firebase callable functions with an object containing
`params`. Its final object always has a `userId` property; the encoder sends
null when that value is unavailable. It can add `deviceInfo`,
`amplitudeDeviceId`, `amplitudeSessionId`, and
`adminAccessRequested: true` when those values apply. It removes direct
`params` properties whose value is `undefined`; therefore analytics and
device properties are not invariant wrapper requirements.

Firebase Functions SDK module `68997` wraps the argument in top-level `data`,
encodes a JavaScript `Date` with `toISOString()`, recursively encodes
`undefined` as `null`, sends bearer authorization when available, and rejects
a success body that has neither top-level `data` nor legacy top-level
`result`. This is generic callable protocol behavior, not a Partiful
business-success predicate.

## createEvent request

Module `23552` constructs a new event. Module `50218` supplies `title`,
`startDate`, `timezone`, all-zero `guestStatusCounts`, `displaySettings`, and
`status: "UNSAVED"`. The create page adds:

- `showHostList`, `showGuestCount`, `showGuestList`,
  `showActivityTimestamps`, `displayInviteButton`,
  `allowGuestPhotoUpload`, `enableGuestReminders`, `rsvpsEnabled`, and
  `allowGuestsToInviteMutuals`, all `true`;
- `visibility: "public"`;
- `rsvpButtonGlyphType: "emojis"`;
- `image` from the selected built-in poster; and
- a browser IANA timezone, replacing the base default.

The current `guestStatusCounts` keys in module `54257` are
`READY_TO_SEND`, `SENDING`, `SENT`, `SEND_ERROR`, `DELIVERY_ERROR`,
`INTERESTED`, `MAYBE`, `GOING`, `DECLINED`, `WAITLIST`,
`PENDING_APPROVAL`, `APPROVED`, `WITHDRAWN`,
`RESPONDED_TO_FIND_A_TIME`, `WAITLISTED_FOR_APPROVAL`, and `REJECTED`.
Every new-event value is zero.

Module `25231` sends
`createEvent({params:{event,cohostIds,...optionalTicketingValues}})`.
`event` and `cohostIds` are always present in this call. Ticket types,
promotion codes, and affiliate values are separate product areas and are
omitted when absent. Before sending, it removes `canceledBy`,
`_lastInvitedBy`, and `questionnaire`; it also removes private author or
selector properties inside questionnaire-version and find-a-time values.

Current create representations relevant to the narrow product are:

| Product concept | Current event property |
|---|---|
| title | `title` |
| start/end | `startDate` and optional `endDate`, encoded as UTC ISO strings |
| timezone | IANA string in `timezone` |
| description | `description` |
| discoverability | public creation adds `isPublic: true`; the default private flow leaves it absent; fixed `visibility: "public"` is separate |
| free-form address | `locationInfo: {type:"freeform",value:<address>}` |
| guest limit | `maxCapacity`; the editor sends `enableWaitlist` with it |
| links | `customFields` entries with `icon`, `value`, and, for link entries, `url` |
| built-in poster | `image` as described below |

The create product projection sets `cohostIds: []`; public cohost input is not
part of issue #167. Optional product values are omitted when not supplied;
product null has the same meaning as omission on create.
The current callable serializer would turn an explicitly nested `undefined`
into `null`, so the product must construct the event without such properties.

## createEvent completion

Module `25231` returns decoded callable `.data`. The caller uses that value as
an event ID in analytics and in `/e/<value>` navigation. It does not validate a
complete Event, and it does not perform a post-write event read. The only
runtime completion requirement below the caller is the callable SDK's generic
successful HTTP/result-or-data envelope. Consequently, the reviewed product
can report only the normalized submitted input. It cannot report a persisted
Event or claim persisted state.

## Built-in poster representation

Module `42919` maps one current poster-catalog entry to:

```text
{
  source: "partiful_posters",
  poster: <the complete selected catalog entry>,
  url, blurHash, contentType, name, height, width
}
```

The last six properties are copied from the selected entry. The product must
resolve an exact built-in poster ID to exactly one catalog entry and bind that
entry's digest into its plan. A missing or duplicate match fails closed.
Uploaded images are not registered by this revision.

On 2026-08-12, privacy-safe public GETs to
<https://assets.partiful.com/posters.json> and the already registered
<https://assets.getpartiful.com/posters.json> each returned 1,125,932 bytes
with SHA-256
`35e22005b19dd5795cecf582dee4c4fe4ddc5349e3142f0aae8014f4e471cc6e`.
The current representations are byte-identical. This revision does not change
the registered catalog operation or host.

Module `95074` contains the exact built-in fallback entry with ID and name
`Let's Party`, JPEG content type, 2000 by 2000 dimensions, and the public
catalog URL ending in `/posters/Let's%20Party`. The stable product default can
select this exact current first-party ID instead of reproducing the
date/locale/motion-dependent configuration.

## Firestore event update

Module `52630` is the generic event update helper. It uses the Firebase
Firestore client to update the `events/{eventId}` document and adds
`updatedBy` as a document reference to the current user. It removes derived
event fields and top-level `id`, `createdAt`, and `ref`. A top-level
`null` or `undefined` update value becomes Firestore field deletion.

Module `30067` applies the candidate values to local event state before the
remote calls. It awaits the field-specific calls and module `52630`, calls the
success callback without remote data, and restores the previous local fields
on an exception. It performs no event post-read and consumes no update
response body.

The current event editor does **not** use that generic update for every field.
Module `30067` routes `locationInfo` through `setEventLocation`, public
visibility through `makeEventPublic` or `unpublishEvent`, and
`displaySettings` through `updateDisplaySettings`. No general callable
`updateEvent` path was found. Therefore, a `firestorePatchEvent`-only product
must exclude address/location, visibility, and display settings. Its closed
projection is:

| Product field | Firestore field paths and typed values |
|---|---|
| `title` | `title` / `stringValue` |
| `description` | `description` / `stringValue`; null deletes |
| `start` | `startDate` / UTC ISO `stringValue` |
| `end` | `endDate` / UTC ISO `stringValue`; null deletes |
| `timezone` | `timezone` / `stringValue` |
| `guestLimit` | `maxCapacity` / `integerValue`, plus `enableWaitlist` / `booleanValue:false`; null deletes both |
| `links` | `customFields` / `arrayValue` of maps containing `icon:"link"`, `value`, and `url`; null deletes |
| `posterId` | `image` / the built-in poster map as Firestore map/array/scalar values; null deletes |

`updatedBy` is a `referenceValue` to
`projects/getpartiful/databases/(default)/documents/users/{currentUserId}`.
This is private request data and must never be printed.

## Official Firestore PATCH protocol

The official sources are:

- <https://firebase.google.com/docs/firestore/use-rest-api>
- <https://firebase.google.com/docs/firestore/reference/rest/v1/projects.databases.documents/patch>
- <https://firestore.googleapis.com/$discovery/rest?version=v1>

They define bearer Firebase ID-token authorization, the request path
`/v1/projects/getpartiful/databases/(default)/documents/events/{eventId}`,
repeated `updateMask.fieldPaths` query values, `currentDocument.exists` or
`currentDocument.updateTime`, Firestore `Document` input/output, and the
typed-value grammar. The narrow product sends sorted, percent-encoded repeated
field paths, `currentDocument.exists=true`, and only the allowlist above.
A masked field omitted from `Document.fields` is deleted. HTTP `200` with a
Document is protocol completion only; it is not proof of Partiful business
state or a complete product Event.

## Update preconditions

The current EventInfo provider permits editing for a current user in
`ownerIds` (or a separate administrator path that is outside this product).
It has no general event-status check. The date editor additionally prevents a
date change when ticketing is present, and prevents it for an event with
`hasGuests: true` after its end plus two hours (or start plus eight hours when
there is no end). No other endpoint permission is inferred.

A product plan must read `getEventInfo`, bind raw presence/null distinctions,
ownership, status, the current values of every target path, and the date
safeguards when date fields are targeted. Apply re-reads once and rejects any
change as stale before it consumes and dispatches the single attempt.
The current client does not perform that stale comparison; it uses its current
local event and live Firestore subscription. The explicit re-read is product
mutation safety, not an endpoint guarantee.

## cancelEvent request and completion

Module `22248` sends:

```text
{
  eventId,
  cancellationMessage,
  shouldSkipNotifyGuests
}
```

The modal starts with `cancellationMessage: ""` and notifications selected, so
the default is `shouldSkipNotifyGuests: false`. The current client awaits the
decoded callable value but does not inspect a business field and performs no
post-write read. Generic callable completion can therefore support only a
submitted-only product result.

The observed UI cancel choice is for an owned `PUBLISHED` event with a
positive guest count and a future start. UI exposure also has an unrelated
employee-tag branch; it is not promoted to endpoint authorization. The
product re-reads `getEventInfo` and binds ownership, status, start, guest-count
facts, exact message, and notification choice. It must not dispatch without
the exact consequential confirmation value.

## Mutation boundary and remaining unknowns

Create has no existing-event precondition. Create and update use standard
five-minute, account-bound, single-use plans. Cancel uses the same binding plus
exact consequential confirmation. Each plan binds normalized input, exact
request projection, private account fingerprint, and pre-read facts where
applicable. Apply consumes immediately before one attempt and never retries an
ambiguous write.

Partiful endpoint authorization rules, endpoint-specific success meanings,
unobserved response bodies, business errors, and unknown status codes remain
protocol drift. No inaccessible-event permission response was observed or
claimed. This evidence does not establish post-write Event state.
