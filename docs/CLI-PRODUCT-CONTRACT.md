# CLI product contract

**Status:** Approved product contract

**Product contract revision:** `2026-08-12.7`

**Remote API contract revision:** `2026-08-12.7`

**Owner-reviewed baseline:** product and remote `2026-08-12.6`

**Currently shipped Go revisions:** product and remote `2026-08-12.7`

This document defines the public behavior of the greenfield Go `partiful` CLI.
It is the authority for commands, inputs, JSON outputs, failures, and mutation
safety. [`spec/partiful.openapi.json`](../spec/partiful.openapi.json) remains
the separate authority for remote transport facts.

The TypeScript CLI is research evidence only. Its command names, output,
credential format, helpers, and quirks are not compatibility requirements.

## Product rules

1. Commands represent Partiful tasks, not remote endpoints.
2. Command results are JSON. Diagnostics go to stderr.
3. Failures have stable types and exit codes.
4. Remote mutations support a redacted `--dry-run` preview and otherwise
   execute in one invocation.
5. Destructive actions require a TTY confirmation unless `--force` is set.
6. Authentication secrets, phone numbers, email addresses, and Partiful user
   IDs never appear in command output.
7. Raw Firebase, Firestore, upload, and callable operations are not public.
8. The CLI never guesses when Partiful no longer matches the reviewed remote
   API contract.

## Invocation and input

The command shape is:

```text
partiful <resource> [subresource] <action> [primary-resource] [flags]
```

The primary resource, such as an event ID, is positional. Common scalar input
uses named flags. Commands with structured input accept exactly one of:

```text
--input <file>
--input -
```

`--input -` reads one JSON object from stdin. Structured input and equivalent
field flags cannot be combined. Unknown fields and repeated scalar flags are
input errors.

Dates and times use RFC 3339. Event writes also require an IANA timezone such
as `America/Los_Angeles`. Zone-free dates, natural-language dates, and an
implicit default timezone are not accepted.

The CLI does not have a product-specific output-file flag. It writes one JSON
document to stdout. Callers use shell redirection when they need a file. The
CLI finishes the complete result before it writes to stdout, so a failed
command does not leave a partial JSON result.

Global flags are:

| Flag | Meaning |
| --- | --- |
| `--pretty` | Indent the JSON result without changing its fields. |
| `--non-interactive` | Prohibit terminal prompts. This is the default for every command except `auth login`. |
| `--version` | Return `{"version","productContractRevision","remoteContractRevision"}` in the success envelope with command `version`. Release builds may inject the version string at link time; development builds use the source default. |

`schema` is the machine-readable discovery interface. Help text can guide a
human to `schema`, but scripts must not parse help text.

## JSON envelopes

Every successful command returns:

```json
{
  "ok": true,
  "data": {},
  "meta": {
    "command": "events.get",
    "cliVersion": "3.0.0",
    "productContractRevision": "2026-08-12.7",
    "remoteContractRevision": "2026-08-12.7",
    "warnings": []
  }
}
```

Every failed command returns:

```json
{
  "ok": false,
  "error": {
    "type": "permission.denied",
    "code": "HOST_PERMISSION_REQUIRED",
    "message": "This command requires host access to the event.",
    "retryable": false,
    "details": {
      "requiredRole": "host"
    }
  },
  "meta": {
    "command": "guests.list",
    "cliVersion": "3.0.0",
    "productContractRevision": "2026-08-12.7",
    "remoteContractRevision": "2026-08-12.7"
  }
}
```

`type` is a stable category for control flow. `code` is a stable specific
condition. `message` is for a human. `details` contains only safe structured
context. Remote response bodies are not copied into an error.

Missing optional values are JSON `null`. Commands do not omit documented
fields based on remote response variation.

Command examples below show the value inside `data`. The complete
`{"ok","data","meta"}` envelope always wraps it.

## Failure and exit contract

| Exit | Failure types | Meaning |
| --- | --- | --- |
| `0` | None | Success. A dry-run preview is also a success because no mutation was attempted. |
| `2` | `usage.invalid`, `input.invalid`, `match.ambiguous` | The invocation must change. |
| `3` | `auth.required`, `auth.expired`, `auth.human_required` | A human must establish or repair authentication. |
| `4` | `permission.denied` | The signed-in account lacks the required role. |
| `5` | `resource.not_found` | The selected domain resource does not exist. |
| `6` | `state.conflict` | Remote state changed or violates a command precondition. |
| `7` | `safety.confirmation_required` | A destructive command requires an allowed confirmation path. |
| `8` | `remote.unavailable`, `remote.rate_limited` | Partiful could not complete a known request. `retryable` and retry details are explicit. |
| `9` | `contract.protocol_changed` | Partiful no longer matches a reviewed remote mapping. The CLI stops and tells the user to update it. |
| `10` | `internal.failure` | An unexpected local failure occurred. |

Lack of remote evidence is not a runtime failure. It is an implementation
gate: the affected command cannot ship until its required response and failure
mappings are reviewed into the remote API contract.

## Collection and pagination contract

Collection commands return:

```json
{
  "ok": true,
  "data": {
    "items": []
  },
  "meta": {
    "command": "events.list",
    "cliVersion": "3.0.0",
    "productContractRevision": "2026-08-12.7",
    "remoteContractRevision": "2026-08-12.7",
    "warnings": [],
    "page": {
      "limit": 25,
      "nextCursor": null,
      "hasMore": false
    }
  }
}
```

Collection commands accept:

| Flag | Meaning |
| --- | --- |
| `--limit <n>` | Maximum items in this result. The default is `25`; the maximum is `100`. |
| `--cursor <opaque>` | Continue from a cursor returned by the same command and filters. |
| `--all` | Request multiple pages. Requires `--max-items`. |
| `--max-items <n>` | Hard result limit for `--all`; maximum `1000`. |

Cursors are opaque. A caller must not inspect, edit, or reuse a cursor with
different filters. If the remote capability cannot support this contract
safely, the collection command remains behind its implementation gate.
When `--all` reaches `--max-items` before the collection ends, `hasMore` is
`true` and `nextCursor` lets the caller resume from that exact point.

### Event-list local snapshots

`events list` has no remote paging contract. One invocation fetches exactly
one reviewed upcoming or past one-response representation and paginates that
array locally. The CLI preserves the received sequence without claiming what
the remote order means.

The local safety ceiling is 1,000 items and 8 MiB for the complete response.
If either ceiling is exceeded, the CLI returns
`contract.protocol_changed` / `EVENT_LIST_BOUND_EXCEEDED`; it does not
truncate a successful result.

An event-list cursor is digest-bound to the complete response representation,
the `events.list` command, the normalized `--when` value, and the next array
offset. Resumption refetches the same one-response representation once. If
its digest changed, the CLI returns `state.conflict` /
`CURSOR_SNAPSHOT_CHANGED` rather than skip, repeat, merge, or guess items.
This is local snapshot pagination. It does not claim a remote cursor, limit,
ordering key, snapshot, or future completeness.

## Mutation safety

### Common mutation flags

Every remote mutation accepts these flags, regardless of where they appear in
the argument list:

| Flag | Meaning |
| --- | --- |
| `--dry-run` | Validate input, perform required read-only resolution and precondition checks, and return a stable redacted preview without dispatching a mutation. |
| `--force` | Skip only the TTY prompt for a destructive command. Validation, authorization, precondition checks, and fail-closed handling still run. |
| `--no-input` | Never prompt. A destructive command fails with `safety.confirmation_required` unless `--force` is also present. |

Without `--dry-run`, one invocation validates its normalized input, performs
the required read-before-write checks, and dispatches exactly once. There is
no persisted mutation state, confirmation token, expiry window, or automatic
retry.

A dry-run success contains the operation, normalized public input, redacted
public request, and public precondition markers appropriate to the command.
It can read remote state needed to resolve names, permissions, or request
shape, but it makes zero mutation calls and does not persist refreshed
credentials. Authentication secrets, account identifiers, guest IDs, contact
IDs, message bodies, and other private remote identifiers never appear in the
preview.

### Destructive confirmation

These commands are destructive:

- `events cancel`;
- `cohosts remove`;
- `cohosts revoke-invite`; and
- `cohosts link revoke`.

After validation and required read-before-write checks, a destructive command
prompts only when standard input is a terminal. A positive answer dispatches
the mutation once. Declining, an input error, `--no-input`,
`--non-interactive`, or a non-TTY invocation returns
`safety.confirmation_required` and makes no mutation call. `--force` bypasses
only this prompt.

All other mutations execute without a CLI prompt. Callers can use `--dry-run`
for review before a separate normal invocation.

### Completion uncertainty

The CLI never automatically retries a mutation. A timeout, connection loss,
or other uncertain completion returns a non-retryable
`remote.unavailable` result that tells the caller to inspect remote state
before another attempt.

## Authentication

### `partiful auth login`

This is the only human-interactive command. It privately prompts for the
information required by Partiful and returns:

```json
{
  "authenticated": true,
  "tokenState": "healthy",
  "expiresAt": "2026-08-11T00:00:00Z"
}
```

Phone numbers, verification codes, Firebase tokens, and Partiful user IDs are
never accepted through command-line flags and never printed. If no private
terminal is available, the command returns `auth.human_required`.

### `partiful auth status`

Returns:

```json
{
  "authenticated": true,
  "tokenState": "healthy",
  "expiresAt": "2026-08-11T00:00:00Z"
}
```

`tokenState` is `healthy`, `expiring`, `expired`, or `missing`. The command
does not return identity details or credentials. When stored credentials have
a refresh token and are within five minutes of expiry or already expired,
`auth status` deterministically refreshes and atomically replaces them before
reporting `healthy`. This makes the command a local mutation. A rejected
refresh returns
`auth.expired`; an unavailable authentication service returns
`remote.unavailable`; and an unrecognized released response returns
`contract.protocol_changed`. Credentials without a refresh token continue to
report `expiring` or `expired`.

### `partiful auth logout`

Deletes local credentials and returns:

```json
{
  "authenticated": false,
  "tokenState": "missing",
  "expiresAt": null
}
```

Token refresh, custom-token exchange, and Firebase account lookup are internal
authentication steps. Other commands never start login or prompt. They return
an authentication failure so an agent can hand control to a human.

Authentication response bodies are limited to 64 KiB. A larger successful or
error response does not enter output and fails closed as
`contract.protocol_changed`.

A successful token lifetime must exceed the five-minute refresh window so
login and refresh can return `healthy`. A shorter lifetime fails closed as
`contract.protocol_changed` rather than creating a refresh loop.

Protected commands never start login and never receive a private terminal.
Missing credentials return `auth.required`. Stored credentials that are
expired without a usable refresh token return `auth.expired`. A refresh rejected by the reviewed remote mapping also returns `auth.expired`.
Unavailable refresh transport returns `remote.unavailable`; an unrecognized
released refresh response returns `contract.protocol_changed`.

## Public commands

### Events

#### `partiful events list`

Inputs:

- exactly one of `--when upcoming` or `--when past`;
- collection pagination flags.

Each `data.items` entry is an `EventSummary`:

```json
{
  "eventId": "evt_example",
  "title": "Example event",
  "start": "2026-09-12T19:00:00-07:00",
  "end": null,
  "timezone": "America/Los_Angeles",
  "state": "active",
  "userRole": "host",
  "myRsvp": null
}
```

For S3 reads, `eventId` is the only non-null event-read field. It comes from
the reviewed list item `id`. All other documented event-read fields are
nullable because the reviewed remote representation does not establish their
universal presence. A missing optional property produces JSON `null`; it does
not make an otherwise reviewed response protocol drift. A present property
that violates its reviewed type or closed enum is
`contract.protocol_changed`.

The conditional scalar projection is:

| Product field | Reviewed remote property | Null meaning |
| --- | --- | --- |
| `title` | string `title` | The property is unavailable. |
| `start` | string `startDate` | The property is unavailable. |
| `end` | string or null `endDate` | The property is absent or explicitly null. |
| `timezone` | string `timezone` | The property is unavailable. |
| `state` | `status` mapping below | The property is unavailable. |

Event state is an exact closed mapping:

- `PUBLISHED` → `active`;
- `CANCELED` → `cancelled`.

`UNSAVED` exists in the current first-party client vocabulary but has no
reviewed S3 product mapping. If it appears in a released event-read response,
the command returns `contract.protocol_changed`.

Role projection uses private current-user identity only for comparison. It
never emits an owner ID:

- `ownerIds` contains the current user → `host`;
- `ownerIds` is present, does not contain the current user, and `guest` is present → `attendee`;
- `ownerIds` is present, does not contain the current user, and `guest` is absent → `none`;
- `ownerIds` is absent → `null`.

Owner membership takes precedence if a representation also has `guest`.
`cohost` is reserved in the product enum, but S3 does not emit it. Current
first-party assets do not distinguish a primary host from a cohost, so every
owner membership maps to `host` until a distinct reviewed field exists.

`myRsvp` uses the read-only `EventReadRsvp` enum. It is a lossless,
one-to-one normalization and is separate from S5's narrower writable RSVP
intent:

| Remote guest status | `EventReadRsvp` |
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

Null `myRsvp` means that no current guest object or status is available. An
unknown present guest status is `contract.protocol_changed`, not null.

#### `partiful events get <event-id>`

Returns an `Event`:

```json
{
  "eventId": "evt_example",
  "title": null,
  "start": null,
  "end": null,
  "timezone": null,
  "state": null,
  "userRole": null,
  "myRsvp": null,
  "description": null,
  "location": null,
  "address": null,
  "visibility": null,
  "guestLimit": null,
  "poster": null,
  "links": null
}
```

The positional input supplies `eventId`. Reviewed conditional `title`,
`startDate`, `endDate`, `timezone`, and `status` properties use the same
nullable scalar and state mappings as `EventSummary`. The current
`getEventInfo` contract does not map owner or guest properties, so
`userRole` and `myRsvp` are null for this command.

The product boundary requires `result.data.event` to be an object before any
nullable field projection occurs. A null, scalar, or array event value returns
`contract.protocol_changed`; it is not converted into an all-null successful
event. This is a CLI output invariant and does not claim that the remote
operation accepts only one top-level variant.

`description`, `location`, `address`, `visibility`, `guestLimit`, `poster`,
and `links` are unavailable-not-claimed in S3. They are null even if an
unreviewed remote property has a similar name. In particular, `links` is
`null`, not an empty array; this does not mean that the remote event has no
links. A later contract revision can add a field only after its transport
shape and product projection are reviewed.

`getEventInfo` has this read failure boundary:

- `200` uses the nullable projection above;
- `404 NOT_FOUND` → `resource.not_found` / `EVENT_NOT_FOUND`;
- no response → `remote.unavailable`;
- every other unrecognized received status or malformed reviewed property →
  `contract.protocol_changed`.

An unobserved `403` is not `permission.denied`. The permission mapping is deferred
until an inaccessible-event response is reviewed. S3 does not call `firestoreGetEvent`;
`.5` returned the same `403 PERMISSION_DENIED` for a
selected readable event and a synthetic ID, so that operation cannot
distinguish permission or existence. S3 also does not call
`getCurrentGuest` or `firestoreGetGuest`; the list's conditional inline
`guest.status` is the only reviewed guest projection used here.

The two list operations accept only their reviewed `200` bodies. No response
is `remote.unavailable`; every unrecognized received status is
`contract.protocol_changed`. Their remote permission and failure mappings
remain unclaimed.

#### `partiful events create`

Required input is non-empty `title`, RFC 3339 `start`, and an IANA
`timezone`. Optional input is `end`, `description`, `location`, `visibility`,
`guestLimit`, `links`, and `posterId`. `end` must not be before `start`.
`visibility` is `private` by default or `public`. `guestLimit` is a positive
integer. `links` is an array of
`{"label":"Tickets","url":"https://example.test/"}` objects with an HTTP or
HTTPS URL. `posterId` is only an exact built-in ID returned by `posters list`
or `posters search`; a path, upload, or arbitrary URL is invalid. The
equivalent flags are `--title`, `--start`, `--end`, `--timezone`,
`--description`, `--location`, `--visibility`, `--guest-limit`, repeated
`--link <label=url>`, and `--poster-id`.
For create, null for any optional structured-input property has the same
meaning as omission: no end, description, location, limit, or links; private
visibility; and the built-in fallback poster. No optional product null is sent
inside `event`.

The narrow current-client projection is exact:

| Product input | `createEvent.params.event` |
| --- | --- |
| `title` | `title` |
| `start`, `end` | `startDate`, optional `endDate`, normalized to UTC ISO strings |
| `timezone` | `timezone` |
| `description` | `description`; omission and null both omit it on create |
| `location` | `locationInfo: {"type":"freeform","value":<location>}` |
| `visibility` | `public` sends `isPublic: true`; `private` omits `isPublic`; separate first-party `visibility` is fixed to `"public"` |
| `guestLimit` | `maxCapacity` and `enableWaitlist: false` |
| `links` | `customFields` entries with `icon: "link"`, `value: <label>`, and `url` |
| `posterId` | `image` built from the one exact catalog entry |

An omitted `posterId` selects the exact current first-party fallback ID
`Let's Party`. Each invocation resolves the ID against one bounded catalog
representation. Zero matches return `resource.not_found`; more than one exact
ID match returns `contract.protocol_changed`. The image request is
`{"source":"partiful_posters","poster":<record>,...}` with the record's
`url`, `blurHash`, `contentType`, `name`, `height`, and `width` copied to the
outer object. Upload remains unregistered.

The caller cannot set `cohostIds`, `displaySettings`, status, guest counters,
or display/RSVP policy. The request sets `cohostIds: []`,
`status: "UNSAVED"`, all 16 current guest-status counts to zero,
`displaySettings:
{"theme":"cloudflow","effect":"fireflies","titleFont":"display"}`, and these
booleans to `true`: `showHostList`, `showGuestCount`, `showGuestList`,
`showActivityTimestamps`, `displayInviteButton`, `allowGuestPhotoUpload`,
`enableGuestReminders`, `rsvpsEnabled`, and
`allowGuestsToInviteMutuals`. It also sets
`rsvpButtonGlyphType: "emojis"`. These are request defaults, not public input.
Absent optional event properties stay absent; the product never constructs a
nested `undefined` value that the callable encoder would turn into null.

Create has no existing-event read or precondition. `--dry-run` resolves the
poster and returns the normalized public input and exact request without
authenticating or calling `createEvent`. Normal execution authenticates,
resolves the poster, and makes exactly one `createEvent` attempt.

The client requires only generic callable completion data and uses that data
as an event ID. It does not validate or read a complete Event. The product
therefore performs no post-write read and returns only:

```json
{"submitted":true}
```

It does not claim an event ID, persisted Event, or persisted state.

#### `partiful events update <event-id>`

Accepts one or more of `title`, `description`, `start`, `end`, `timezone`,
`guestLimit`, `links`, and `posterId`, through structured input or the
matching create flags. This is a closed allowlist. `location`, `visibility`,
and display settings are rejected because the current client routes them
through `setEventLocation`, `makeEventPublic` or `unpublishEvent`, and
`updateDisplaySettings`; no general callable `updateEvent` was found.

Omission means unchanged. Null is allowed only for `description`, `end`,
`guestLimit`, `links`, and `posterId`, and means delete the mapped field.
Non-null mapping is:

| Product input | Firestore event field |
| --- | --- |
| `title` | `title.stringValue` |
| `description` | `description.stringValue` |
| `start` | `startDate.stringValue`, normalized to a UTC ISO string |
| `end` | `endDate.stringValue`, normalized to a UTC ISO string |
| `timezone` | `timezone.stringValue` |
| `guestLimit` | `maxCapacity.integerValue` and `enableWaitlist.booleanValue: false` |
| `links` | `customFields.arrayValue` of map values with `icon`, `value`, and `url` string values |
| `posterId` | `image.mapValue` using the exact built-in poster representation |

Every update also writes private `updatedBy.referenceValue` for the current
account. It is never printed. The exact PATCH is bearer-authorized at
`/v1/projects/getpartiful/databases/(default)/documents/events/{eventId}`.
It sends sorted, percent-encoded, repeated `updateMask.fieldPaths`, includes
every mapped path and `updatedBy`, and sends
`currentDocument.exists=true`. A delete path remains in the update mask and
is absent from `fields`.

Each invocation calls `getEventInfo` once. `ownerIds` must be present and contain the
private current account ID; otherwise the command returns
`permission.denied` / `HOST_PERMISSION_REQUIRED`. No general first-party
update status guard was found. For a date or timezone change, the command
checks current `startDate`, `endDate`, `hasGuests`, and `ticketing`. Date
changes are rejected when ticketing is present, or when `hasGuests` is exactly
true and the current event is past its end plus two hours (start plus eight
hours when end is absent).

The command merges proposed `start` and `end` values with the current
values. When both resulting values are present, `end` must be at or after
`start`. An inverted merged range returns `input.invalid`.

`--dry-run` returns the normalized input and redacted Firestore request after
these checks without sending a PATCH. Normal execution makes exactly one
`firestorePatchEvent` attempt. HTTP `200` with a Firestore Document is protocol
completion, not a complete product Event or proven Partiful business state.
There is no post-write read. Success is:

```json
{"eventId":"evt_example","fields":["start","title"],"submitted":true}
```

`fields` is the sorted product-field list, not raw Firestore field paths.

#### `partiful events cancel <event-id>`

Accepts optional `message` and `notifyGuests`; the matching flags are
`--message <text>` and `--notify-guests <boolean>`. Defaults are `message: ""`
and `notifyGuests: true`. The exact callable parameters are `eventId`,
`cancellationMessage`, and `shouldSkipNotifyGuests: !notifyGuests`.

Each invocation calls `getEventInfo` once. `ownerIds` must be present and contain the
current private account ID, `status` must be exactly `PUBLISHED`, the observed
guest count must be a positive integer, and the current `startDate` must be in
the future. Missing, null, or differently typed precondition facts return
`contract.protocol_changed`; a known non-matching fact returns
`state.conflict` / `EVENT_PRECONDITION_FAILED` except ownership, which returns
`permission.denied`. Inaccessible-event permission behavior was not observed
and is not inferred. The positive-guest gate deliberately preserves the
reviewed current-client exposure as a conservative product limitation; it is
not a claim about endpoint authorization.

`--dry-run` returns the exact event ID, normalized input, request, effects, and
public precondition markers without prompting or calling `cancelEvent`.
Normal execution prompts on a TTY after the checks above and makes one
`cancelEvent` attempt after confirmation. `--force` skips only the prompt.
`--no-input`, `--non-interactive`, or non-TTY execution without `--force`
returns `safety.confirmation_required`.

The current client inspects no endpoint business field and performs no
post-write read. Generic callable completion returns only:

```json
{"eventId":"evt_example","notifyGuests":true,"submitted":true}
```

It does not claim cancellation, notification delivery, or persisted state.
Any unrecognized status, endpoint error/body, missing generic completion
envelope, or malformed completion body is `contract.protocol_changed`. A
no-response transport failure is `remote.unavailable`; because the write may
have reached the remote, the caller must inspect remote state before another
attempt.

### Guests

#### `partiful guests list <event-id>`

This host-only command accepts collection pagination flags. Each item is:

```json
{
  "displayName": "Example Guest",
  "rsvpStatus": "going",
  "partySize": 1,
  "cohost": false
}
```

The command never returns phone numbers, email addresses, guest IDs, or
Partiful user IDs. Attendee access returns `permission.denied`, not an empty
list.

#### `partiful guests invite <event-id> --contact <name>`

This host-only consequential action invites one contact. The CLI resolves the
name through the contact capability. It never asks the caller for a Partiful
user ID.

No match returns `resource.not_found`. More than one safe match returns
`match.ambiguous` and performs no action. If display information cannot safely
disambiguate the contacts, the CLI refuses the invitation.

Each invocation resolves the contact against current contact data before any
write. `--dry-run` returns the selected contact's display name and a redacted
request preview without sending an invitation. It never exposes a Partiful
user ID, guest ID, phone number, email address, account fingerprint, or private
contact identity. Normal execution dispatches once without prompting.

Success returns submitted-only state:

```json
{
  "eventId": "evt_example",
  "submitted": true
}
```

### RSVP

#### `partiful rsvp get <event-id>`

For the reviewed object variant, returns an `RsvpRead`:

```json
{
  "eventId": "evt_example",
  "status": "going"
}
```

`status` uses the full lossless `EventReadRsvp` enum documented for event
reads. It is not restricted to writable intents. No public RSVP read returns
the current guest's private ID or account ID.

The accepted object must have a nonempty string `id` and a `status` from the
reviewed `GuestStatus` vocabulary. The dated read evidence also establishes
that `currentGuest: null` is the authoritative no-current-guest marker. It
returns:

```json
{"eventId":"evt_example","status":null}
```

The `currentGuest` property is required in the callable envelope.
A missing `currentGuest` property is protocol drift. A scalar, array, object
without a valid ID and status, or any other variant returns
`contract.protocol_changed`. Party size, plus ones, message, timezone,
questionnaire response, name, and user ID remain unavailable in the product
read and are not guessed or returned.

#### `partiful rsvp set <event-id>`

The writable `RsvpIntent` is separate from `RsvpRead`. It has three product
values: `going`, `not-going`, and `interested`.

`going` and `not-going` accept structured input or equivalent flags:

```json
{
  "status": "going",
  "displayName": "Example Attendee",
  "partySize": 1,
  "plusOnes": [],
  "message": null,
  "timezone": "America/Los_Angeles",
  "questionnaireResponse": null
}
```

`displayName` is required for `going` and `not-going`. It is trimmed, must
then contain 1–50 characters, and maps exactly to `addGuest.rsvp.name`. The
CLI does not use a private profile lookup or a current-guest name.

`partySize` is a positive integer. `plusOnes` contains only display-name
strings; each is trimmed and must remain nonempty. It never contains a private
user or guest ID. The local input rule is
`partySize = 1 + plusOnes.length`. `timezone` is required, must identify an
IANA timezone, and is submitted unchanged after validation. `message` is a
string or null. A non-null message is trimmed, must then contain at most 400
characters, and normalizes to null when empty. Null maps to request omission.

For `going`, `questionnaireResponse` is null or
`{"questionnaireVersion","answers"}`, with a nonnegative version and string
answers keyed by question ID. The event pre-read decides which form is valid.
`not-going` accepts only null and omits `questionnaireResponse` from the
remote request, matching the first-party `DECLINED` flow.

`interested` accepts only:

```json
{
  "status": "interested"
}
```

`displayName`, party, plus-one, message, timezone, and questionnaire fields
are invalid with `interested`. Interest removal is an established remote
boolean request but is not a fourth writable product intent in this revision.

The exact product-to-remote mappings are:

- `going` → `addGuest.rsvp.status = "GOING"`;
- `not-going` → `addGuest.rsvp.status = "DECLINED"`; and
- `interested` → `markEventInterest.interested = true`.

`source` is omitted for a direct event-page-equivalent request. The CLI does
not invent an analytics source. The `addGuest` mapping uses
`count = partySize`, maps each plus-one string to `{"name": string}`, uses the
input timezone, sends `shouldFollowOrgs: false`, and omits a null message. It
omits phone number, channel preference, captcha token, image,
`emailInvitationId`, `_discoverSource`, and password.

The current-guest selection is:

- explicit `currentGuest: null` selects create and omits `guestId`; and
- an object with a nonempty string ID, reviewed status, and nonnegative integer
  count selects update and includes the private `guestId`.

Every other current-guest variant returns `contract.protocol_changed` and
does not produce a dry-run preview or dispatch a mutation.

##### Compatible-event safeguards

A dry-run acquires the authenticated session without persisting a refreshed
credential, then calls `getEventInfo` once and `getCurrentGuest` once. It
performs no mutation.

The compatible event class is deliberately narrow:

- `rsvpsEnabled` must be present, boolean, and `true`;
- `atCapacity` and `plusOneNamesRequired` must be present booleans;
- absent `guestAction` and `guestAction: "RSVP"` are supported;
  `guestAction: "APPLY"` is unsupported;
- `ticketing` must be absent or null; an object is unsupported;
- `password` and `passwordProtected` must be absent; explicit null is an
  unobserved raw variant, while any non-null value is an unsupported protected
  event;
- `questionnaireEnabled` can be absent, `false`, or `true`; explicit null is
  unobserved and is protocol drift;
- `questionnaireVersions` must be present and either null or an array;
- `maxCountPerGuest` can be absent or a positive integer;
- `maxCapacity` can be absent, null, or an integer;
- `remainingCapacity` can be absent or an integer, including a negative
  integer; and
- `enableWaitlist` can be absent, null, or boolean.

Any missing required field, wrong type, unsupported enum, or other unobserved
raw variant returns `contract.protocol_changed`. The snapshot preserves
missing, explicit null, and present values as distinct states. It does not
collapse an absent property to JSON null.

The product then applies these local rules:

- `atCapacity: true` is unsupported for `going`, even when
  `enableWaitlist: true`; this CLI does not enter a waitlist;
- for going or not-going, a present maximum requires
  `partySize <= maxCountPerGuest`;
- a numeric `maxCapacity` without numeric `remainingCapacity` is unsupported
  because this slice cannot calculate a safe capacity delta;
- for going, create uses current capacity count zero. Update subtracts
  `currentGuest.count` only when the current status is `GOING` or `APPROVED`,
  the two statuses the current client counts as attended; every other current
  status uses capacity count zero. It computes
  `additionalCount = max(0, partySize - currentCapacityCount)` and, when
  `remainingCapacity` is present, requires
  `additionalCount <= remainingCapacity`;
- every supplied plus one has a nonempty normalized name, and
  `partySize = 1 + plusOnes.length`; this also satisfies
  `plusOneNamesRequired: true`;
- when `questionnaireEnabled: true`, going requires a non-null
  questionnaire response and a nonempty `questionnaireVersions` array; the
  required version is `questionnaireVersions.length - 1`;
- when `questionnaireEnabled` is absent or false, going requires a null
  questionnaire response and omits it; and
- `not-going` omits `questionnaireResponse` for every event.

Input violations return `input.invalid`. An RSVP-disabled,
application, ticketed, password-protected, at-capacity, or insufficient
capacity event returns `state.conflict` without a mutation. These are product
compatibility decisions, not server error mappings. Any received server
rejection remains `contract.protocol_changed` under the current evidence.

##### Dry-run and execution

Each invocation reads `getEventInfo` and `getCurrentGuest` once, then validates
the normalized event safeguards and current guest before any write.
`--dry-run` returns the normalized public input and request. It can show
caller-supplied `displayName`, plus-one names, message, and questionnaire
answers for review. It never exposes an account fingerprint, guest ID, account
ID, or user ID. A public RSVP preview example is:

```json
{
  "operation": "addGuest",
  "mode": "update",
  "input": {
    "status": "going",
    "displayName": "Example Attendee",
    "partySize": 1,
    "plusOnes": [],
    "message": null,
    "timezone": "America/Los_Angeles",
    "questionnaireResponse": null
  },
  "request": {
    "eventId": "evt_example",
    "rsvp": {
      "name": "Example Attendee",
      "count": 1,
      "plusOnes": [],
      "status": "GOING",
      "guestId": "<redacted>",
      "timezone": "America/Los_Angeles",
      "shouldFollowOrgs": false
    }
  },
  "preconditions": {
    "currentGuest": "present",
    "eventSafeguards": "bound"
  }
}
```

Without `--dry-run`, the same invocation dispatches the normalized request
exactly once after the checks. It never retries automatically. A timeout,
connection loss, or other uncertain outcome requires inspection of remote
state before another attempt.

##### Submitted-request result

For `going` and `not-going`, an `addGuest` HTTP `200` with the reviewed
Firebase callable `result` envelope returns only:

```json
{"eventId":"evt_example","intent":"going","submitted":true}
```

For `interested`, the same minimal result uses intent `interested`, but
`submitted: true` is returned only when `result.data.success` is truthy and
`result.data.interested` strictly equals the submitted boolean. A failed
predicate, unrecognized status, or unsupported envelope returns
`contract.protocol_changed`.

The CLI does not perform a post-write read. It does not echo `displayName`,
message, questionnaire answers, plus-one names, or private IDs in the
result. `submitted: true` means only that the exact submitted request met the
reviewed callable and client completion condition. It does not prove
persisted RSVP state, delivery, notification, or another business side
effect.

This `2026-08-12.7` contract is the approved event-write product contract and
the current shipped Go contract revision.

### Contacts

#### `partiful contacts list`

Accepts optional `--query <name>` and collection pagination flags. Each item
is:

```json
{
  "displayName": "Example Contact",
  "sharedEventCount": 2
}
```

No command returns contact phone numbers, email addresses, or Partiful user
IDs.

The CLI traverses contact pages sequentially, keeps the first occurrence of
each private contact identity, and then applies the name filter locally.
Private identity remains internal.

Contact traversal permits at most three nonempty pages and 3,000 transport
items, then requires the empty terminal sentinel. This client execution bound
uses the reviewed 1,000-item request size and observed traversal. It does not
claim future remote completeness. A repeated remote cursor, data beyond the
bound, or a missing empty sentinel fails with `contract.protocol_changed`; the
CLI does not return a truncated collection as complete.

### Cohosts

The public commands are:

```text
partiful cohosts invite <event-id> --contact <name>
partiful cohosts revoke-invite <event-id> --contact <name>
partiful cohosts remove <event-id> --contact <name>
partiful cohosts link create <event-id>
partiful cohosts link revoke <event-id>
```

All are host-only consequential actions. Contact commands use the same
privacy-safe match rules as guest invitations. The machine-readable schema
paths for the subresource commands are `cohosts.link.create` and
`cohosts.link.revoke`.

Each invocation reads the required event, contact, request-state, or link-state
facts once. `--dry-run` returns a redacted preview after those checks and makes
no mutation call. Normal execution dispatches once and never retries.
`cohosts invite` and `cohosts link create` do not prompt.
`cohosts revoke-invite`, `cohosts remove`, and `cohosts link revoke` prompt on
a TTY after the checks unless `--force` is set.

The contact commands use the same privacy-safe resolution rules as guest
invitations:

- a unique exact display-name match wins;
- otherwise a unique partial display-name match wins;
- no safe match returns `resource.not_found`; and
- multiple safe matches return `match.ambiguous`.

The resolved private contact identity is used only within the current
invocation and is never emitted as a user ID.

The observable preconditions are:

- `invite`: the event `ownerIds` must contain the current account and the
  selected contact must have no current request or a current `DECLINED`
  request;
- `revoke-invite`: the selected contact must have current request status
  `PENDING` or `DECLINED`;
- `remove`: the selected contact must have current request status
  `ACCEPTED`;
- `link create`: the current `cohostSecret` document must be absent; and
- `link revoke`: the current `cohostSecret` document must be present.

A non-matching role, contact, cohost request state, or link state fails before
a mutation request.

The three contact actions return:

```json
{
  "eventId": "evt_example",
  "cohost": {
    "displayName": "Example Contact",
    "status": "invited"
  }
}
```

`status` is exactly `invited`, `revoked`, or `removed`, according to the
command.

Link creation returns:

```json
{
  "eventId": "evt_example",
  "link": {
    "url": "https://partiful.com/e/evt_example?accept-cohost=token",
    "state": "active"
  }
}
```

Only reviewed `generateEventCohostLink` success may emit the URL. The CLI
does not echo a pre-existing active URL from a read-only precondition check.

Link revocation returns:

```json
{
  "eventId": "evt_example",
  "link": {
    "url": null,
    "state": "revoked"
  }
}
```

### Blasts

#### `partiful blasts send <event-id>`

This host-only consequential action accepts:

```text
--audience all-guests
--message-file <file|->
--show-on-event-page
```

The message is not accepted as a command-line value because shell history is
not a private input channel. `--message-file -` reads plain UTF-8 text from
stdin. The public dry-run preview contains only a message digest and length,
not the message text. The text never appears in stdout, stderr, or diagnostics.
Images are not supported in v1.

Each invocation reads `getEventInfo` once, the reviewed Firestore guest
collection once, and the reviewed Firestore host-message collection once.
`ownerIds` must
contain the current private account ID. The event must not be older than
`endDate + 67 days`, or `startDate + 6 hours + 67 days` when `endDate` is
absent. The current text-blast count must be at most `10`, and the derived
recipient set must be non-empty.

`all-guests` is not a sentinel or an expanded identity list. The reviewed
request sends the exact current group array in `message.to`, in this order:

1. `invited` when the invited count is positive and not excluded;
2. `checkedIn` when at least one guest has non-null `checkIn`; and
3. the current reviewed status set for the event mode.

The status set is:

- `APPROVED`, `PENDING_APPROVAL`, `WAITLISTED_FOR_APPROVAL`, `WITHDRAWN`, and
  `REJECTED` when `guestAction` is `APPLY`;
- `RESPONDED_TO_FIND_A_TIME` for an active find-a-time event; or
- `GOING`, `MAYBE`, `DECLINED`, and conditional `WAITLIST` for an ordinary
  RSVP event.

The current first-party client excludes the `invited` group from its
all-guests selection when more than `100` invited guests are present. Its one
private owner-ID exception is not promoted here. The CLI therefore fails
closed or conservatively excludes `invited` when it cannot verify an
exemption.

The public dry-run preview states the exact event ID, audience,
`showOnEventPage`, exact derived `message.to`, and only the message digest and
length. It makes no `createTextBlast` call. Normal execution dispatches once
without prompting and never retries automatically.

Success returns:

```json
{
  "eventId": "evt_example",
  "submitted": true,
  "audience": "all-guests",
  "showOnEventPage": false,
  "recipientStatus": "not-reported"
}
```

`submitted` means the reviewed remote success condition occurred. It does not
claim delivery. Recipient delivery status remains `not-reported` unless a
future reviewed remote contract establishes it.

### Posters

#### `partiful posters list`

Accepts collection pagination flags.

#### `partiful posters search --query <text>`

Requires a non-empty query and accepts collection pagination flags.

Both commands return `Poster` items:

```json
{
  "posterId": "poster_example",
  "name": "Example poster",
  "url": "https://example.invalid/poster.jpg",
  "contentType": "image/jpeg",
  "width": 1200,
  "height": 630,
  "tags": [],
  "categories": []
}
```

Poster output uses the observed catalog mapping in remote API contract revision
`2026-08-11.1`.

The CLI applies a local 8 MiB poster-catalog response ceiling, leaving
substantial headroom above the observed response. A larger HTTP `200` response
fails closed as `contract.protocol_changed`; this ceiling is not a claimed
remote limit.

### Local discovery and diagnostics

#### `partiful schema [command.path]`

Without a path, returns every public command path. With a path such as
`events.create`, returns:

```json
{
  "command": "events.create",
  "positionals": [],
  "flags": [],
  "inputSchema": {},
  "successSchema": {},
  "failureTypes": ["usage.invalid", "input.invalid"],
  "safety": {
    "kind": "standard-mutation",
    "destructive": false
  }
}
```

The example shows the default invocation failures. Real arrays add each
command-specific failure, and the schemas contain the complete contract. This
command is generated from the same command definitions used by execution; it
is not a second handwritten description.

#### `partiful doctor`

Returns:

```json
{
  "healthy": true,
  "checks": [
    {
      "name": "credentials",
      "status": "pass",
      "message": "Authentication credentials are available.",
      "remediation": null
    }
  ]
}
```

Check status is `pass`, `warn`, or `fail`. Diagnostics are redacted. Doctor
does not print credentials, phone numbers, email addresses, local secret-file
contents, or Partiful user IDs.

## Remote operations that stay private

| Remote capability | Product treatment |
| --- | --- |
| Callable event, contact, guest, RSVP, cohost, and blast operations | Used only behind the matching domain commands. No callable endpoint command exists. |
| Firestore event read and patch | Used only for reviewed event reads and field-specific updates. Generic patching is prohibited. |
| Firestore guest read and document listing | Used only behind reviewed domain collections. Generic collection access is prohibited. |
| Token refresh, custom-token sign-in, and account lookup | Internal authentication steps. |
| Authentication-code operations | Used only by human-attended `auth login`. |
| Event-photo upload | Not public in v1. Its behavior is inferred and historical upload behavior is unreliable. |
| Poster catalog | Used only by `posters`; its response mapping must pass its implementation gate. |

There is no public raw HTTP, callable, Firebase, Firestore, or upload escape
hatch.

## Implementation gates and remote changes

The owner-reviewed remote API contract records observed response statuses and
bodies for some operations. Unobserved mappings remain unknown. Before a
public command ships, its implementation slice must:

1. obtain privacy-safe evidence for every remote response and failure mapping
   it needs;
2. update the remote API contract and evidence ledger through owner review;
3. prove that it can populate every required product output field;
4. prove that it can distinguish the documented failure types without
   guessing;
5. fail its slice if any requirement remains unsupported.

After a command ships, an unrecognized response status, body, or required field
is `contract.protocol_changed`. The CLI stops, returns exit `9`, and tells the
user to update the CLI. It does not continue with partial data or infer a new
mapping.

The separate supervised contract-refresh process can inspect the changed API,
collect approved browser evidence, propose a new remote API contract revision,
and open a pull request. It cannot approve or merge that revision.

## Deliberately excluded features

| Feature | Decision |
| --- | --- |
| Templates | Excluded. They add a local persistence model unrelated to reviewed remote capabilities. |
| Export | Excluded. Stable JSON and shell redirection already compose. |
| Watch | Excluded. Polling, identity stability, and delivery behavior are not established. |
| Bulk mutation | Excluded. Multi-item consequential safety needs a separate product contract. |
| Clone | Excluded. It is unreviewed orchestration rather than a remote capability. |
| Skill installation | Excluded. The CLI does not mutate agent configuration. Skills can be distributed separately. |
| Image upload | Excluded from v1 until the remote behavior is established. |
| Schema | Included for machine-readable agent discovery. |
| Doctor | Included for redacted authentication and environment diagnostics. |

Adding an excluded feature requires a new reviewed CLI product contract
revision. An operation name in the remote API contract is never sufficient
reason to make it public.
