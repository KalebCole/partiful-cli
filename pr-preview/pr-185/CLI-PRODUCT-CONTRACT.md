# CLI product contract

**Status:** Proposed pending delegated review

**Product contract revision:** `2026-08-12.3`

**Remote API contract revision:** `2026-08-12.3`

**Owner-reviewed baseline:** product and remote `2026-08-12.2`

**Currently shipped Go revisions:** product and remote `2026-08-12.1`

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
4. Remote mutations produce a plan before they can execute.
5. Actions that contact people, remove access, expose access, or cancel an
   event require confirmation of the exact plan.
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
| `--version` | Return `{"version","productContractRevision","remoteContractRevision"}` in the success envelope with command `version`. |

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
    "cliVersion": "1.0.0",
    "productContractRevision": "2026-08-12.3",
    "remoteContractRevision": "2026-08-12.3",
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
    "cliVersion": "1.0.0",
    "productContractRevision": "2026-08-12.3",
    "remoteContractRevision": "2026-08-12.3"
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
| `0` | None | Success. A mutation plan is also a success because no execution was attempted. |
| `2` | `usage.invalid`, `input.invalid`, `match.ambiguous` | The invocation must change. |
| `3` | `auth.required`, `auth.expired`, `auth.human_required` | A human must establish or repair authentication. |
| `4` | `permission.denied` | The signed-in account lacks the required role. |
| `5` | `resource.not_found` | The selected domain resource does not exist. |
| `6` | `state.conflict` | Remote state changed or violates a command precondition. |
| `7` | `safety.confirmation_required`, `safety.plan_stale` | The caller must plan again or confirm the exact live plan. |
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
    "cliVersion": "1.0.0",
    "productContractRevision": "2026-08-12.3",
    "remoteContractRevision": "2026-08-12.3",
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

### Plan

Running a remote mutation without `--apply` returns a plan and performs no
remote mutation:

```json
{
  "ok": true,
  "data": {
    "kind": "mutation-plan",
    "operation": "events.update",
    "summary": "Update the title and start time of the event.",
    "effects": [
      "Changes remote event data"
    ],
    "executes": false,
    "applyRequired": true,
    "planToken": "plan_example",
    "confirmationRequired": false,
    "expiresAt": "2026-08-10T23:30:00Z"
  },
  "meta": {
    "command": "events.update",
    "cliVersion": "1.0.0",
    "productContractRevision": "2026-08-12.3",
    "remoteContractRevision": "2026-08-12.3",
    "warnings": []
  }
}
```

The plan contains normalized inputs, current preconditions, an effect summary,
and an opaque short-lived `planToken`. The mutation authority binds it to an
opaque, private stable account fingerprint supplied by the authentication
seam. The fingerprint is stable across token refresh for one account and
changes when the signed-in account changes. The raw account identifier and
fingerprint do not appear in the token, plan, output, diagnostics, or an
error.

The plan token is also bound to the command, normalized input, exact remote
request projection, and a digest of every pre-read fact used by the plan.
Secrets, account identifiers, and the account fingerprint are redacted. A
command-specific contract can permit user-supplied personal data in the exact
input and request shown for review; that data is not copied to diagnostics or
errors. There is no separate `--dry-run` flag: omitting `--apply` is the only
dry-run behavior.

### Standard mutation

Event creation, event update, and RSVP changes are standard mutations. The
caller first reviews the plan, then repeats the same command input with:

```text
--apply --plan <plan-token>
```

The command cannot execute without a live token from the matching plan.
Standard-mutation plan tokens are single-use and expire after five minutes.
Apply reacquires the private account fingerprint and every bound remote
precondition before it consumes the token. Changed input, account, or
precondition, expiry, or reuse returns `safety.plan_stale` before a mutation
request. After successful precondition checks, apply atomically consumes the
token immediately before dispatch and makes exactly one transport attempt.
The consumed token cannot be reused after a response, timeout, connection
loss, or ambiguous completion. The CLI does not automatically retry; an
uncertain transport outcome requires a new plan.

### Consequential action

These actions require a short-lived confirmation token:

- cancel an event;
- invite a guest or cohost;
- revoke a cohost invitation;
- remove a cohost;
- create or revoke a cohost access link;
- send a text blast.

The no-apply invocation returns a plan token bound to the normalized input and
observed preconditions. Execution repeats the same input with:

```text
--apply --confirm <token>
```

Consequential tokens have the same five-minute, single-use, account, input,
request, and precondition binding. A changed binding, expired token, or reused
token returns `safety.plan_stale`. `--apply` without `--confirm` on a
consequential action returns `safety.confirmation_required`. There is no
`--yes` bypass, automatic mutation retry, or interactive mutation prompt.

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

Accepts structured input or equivalent flags:

| Field | Required | Meaning |
| --- | --- | --- |
| `title` | Yes | Event title. |
| `start` | Yes | RFC 3339 start time. |
| `end` | No | RFC 3339 end time. |
| `timezone` | Yes | IANA timezone. |
| `description` | No | Event description. |
| `location` | No | Human location label. |
| `address` | No | Address text. |
| `visibility` | No | `public` or `private`; default `private`. |
| `guestLimit` | No | Positive integer. |
| `posterId` | No | Poster selected from `posters`. |
| `links` | No | Array of `{ "label", "url" }`. |

The plan identifies the new event. Applied success returns the complete
`Event`.

#### `partiful events update <event-id>`

Accepts one or more mutable `events create` fields through structured input or
equivalent flags. It does not expose raw Firestore fields or update masks.
Applied success returns the complete updated `Event`.

#### `partiful events cancel <event-id>`

This is a consequential action. Applied success returns:

```json
{
  "eventId": "evt_example",
  "state": "cancelled"
}
```

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

The mutation plan binds the resolved private contact identity, not only the
name supplied through `--contact`. Applying the plan cannot resolve the name
again or select a different person. The bound identity does not appear in
output.

Applied success returns:

```json
{
  "eventId": "evt_example",
  "invited": {
    "displayName": "Example Contact",
    "status": "invited"
  }
}
```

### RSVP

#### `partiful rsvp get <event-id>`

For the reviewed current-guest object variant, returns an `RsvpRead`:

```json
{
  "eventId": "evt_example",
  "status": "going"
}
```

`status` uses the full lossless `EventReadRsvp` enum documented for event
reads. It is not restricted to writable intents. No public RSVP read returns
the current guest's private ID or account ID.

The accepted object variant is an object with a nonempty string `id` and a
`status` from the reviewed `GuestStatus` vocabulary. An explicit null or
missing `currentGuest` uses the current first-party client's null-safe
no-guest behavior and returns the documented no-RSVP shape:

```json
{"eventId":"evt_example","status":null}
```

This null-safe selection is current first-party product behavior. It is not a
claim that a dated remote observation returned null or omitted the property.
A scalar, array, object without a valid ID and status, or any other response
variant returns `contract.protocol_changed`. Revision `2026-08-12.2`
separated this read from the writable shape; revision `2026-08-12.3` permits
only the exact reviewed object and null-safe no-guest product variants.

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

`displayName` is required for `going` and `not-going`. After trimming leading
and trailing whitespace, it must contain 1–50 characters. The normalized
value maps exactly to `addGuest.rsvp.name`; the CLI does not use an
unavailable private profile lookup or reuse a current-guest name.

`partySize` is a positive integer. `plusOnes` contains nonempty normalized
display-name strings; it never contains a private user or guest ID.
Each plus-one value is trimmed and must remain nonempty. `partySize` must equal
one plus the number of named plus ones. `timezone` is required, identifies the
attendee's IANA timezone, and is submitted unchanged after validation.
`message` is a string or null. A non-null message is trimmed, must then contain
at most 400 characters, and normalizes to null when empty. Null maps to
omission from the remote request.

For `going`, `questionnaireResponse` is null when the event has no
questionnaire. Otherwise it is
`{"questionnaireVersion","answers"}`, with a nonnegative version and string
answers keyed by question ID. This is caller-supplied normalized input; the
answer strings are submitted unchanged, and the CLI does not read the event
to validate the version. `not-going` requires `questionnaireResponse: null`,
matching the first-party flow, which skips the questionnaire for `DECLINED`.

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
`emailInvitationId`, `_discoverSource`, and password. It attaches the private
existing `guestId` only when the reviewed current-guest pre-read returned an
object with a nonempty string ID and reviewed status. A null or missing
`currentGuest` marks no current guest and omits `guestId`. All going and
not-going requests use the input `displayName`, whether a current guest is
present or absent.

The plan-only invocation performs these steps and no mutation:

1. acquire an authenticated session and private stable account fingerprint;
2. call `getCurrentGuest` once, which is the only remote precondition read;
   and
3. bind an explicit no-current-guest marker or the current-guest identity/status snapshot.

The present snapshot contains only the private guest ID and reviewed status.
The absent marker represents null or missing `currentGuest`. Non-object,
non-null values and objects without a valid ID and status return
`contract.protocol_changed`; they do not produce a plan. The CLI does not read
event party limits, questionnaire versions, password state, ticketing state,
or other event fields as S5 preconditions. If the product input supplies
plus-one, message, timezone, or questionnaire values, the plan binds the exact
normalized values and the request submits them exactly. Any server rejection
status remains protocol drift under the reviewed remote evidence.

The five-minute, single-use plan binds the operation, exact normalized product
input, exact callable request projection, private account fingerprint, and
current-guest snapshot or absent marker. The input `displayName` can appear in
the exact plan input and request so the caller can review it. It never appears
in diagnostics, errors, or the applied result, and the private account
fingerprint is never output.

Apply reacquires the account fingerprint and calls `getCurrentGuest` once. A
change between present and absent, guest ID, or status returns
`safety.plan_stale` before token consumption. No other remote fact is read.
After the check, apply consumes the token immediately before dispatch and
makes exactly one transport attempt. It never retries automatically. A
timeout, connection loss, or other uncertain transport outcome consumes the
token and requires a new plan.

For `going` and `not-going`, an `addGuest` HTTP `200` with the reviewed
Firebase callable `result` envelope returns:

```json
{"eventId":"evt_example","intent":"going","submitted":true}
```

For `interested`, the same minimal shape uses intent `interested`, but
`submitted: true` is returned only when `result.data.success` is truthy and
`result.data.interested` strictly equals the submitted boolean. Truthy has the
current JavaScript-client meaning: it excludes a missing value, null, false,
numeric zero, and the empty string. A failed predicate, any unrecognized
status, or any unsupported envelope returns `contract.protocol_changed` and
never returns `submitted: true`.

The CLI does not perform a post-write read. It never echoes `displayName` or
the other normalized RSVP input in the applied result. `submitted: true`
means only that one exact request met the reviewed callable and client
completion condition. It does not prove persisted RSVP state, message or
notification delivery, or another business side effect.

This `2026-08-12.3` contract is proposed pending delegated review. After
approval, S5 can implement these exact read, set, and plan behaviors without
additional event preconditions. Unobserved remote nulls, mutation failures,
server-side validation, and persisted business state remain unknown.

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
privacy-safe match rules and resolved-identity plan binding as guest
invitations.
The machine-readable schema paths for the subresource commands are
`cohosts.link.create` and `cohosts.link.revoke`.

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

`status` is `invited`, `revoked`, or `removed`.

Link creation returns:

```json
{
  "eventId": "evt_example",
  "link": {
    "url": "https://partiful.com/example-cohost-link",
    "state": "active"
  }
}
```

Link revocation returns the same shape with `url: null` and `state:
"revoked"`.

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
stdin. Images are not supported in v1.

Applied success returns:

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
    "planRequired": true,
    "confirmationRequired": false
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
