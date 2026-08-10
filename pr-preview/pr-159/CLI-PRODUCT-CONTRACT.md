# CLI product contract

**Status:** Approved product contract  
**Product contract revision:** `2026-08-10.1`  
**Remote API contract revision:** `2026-08-10.1`

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
    "productContractRevision": "2026-08-10.1",
    "remoteContractRevision": "2026-08-10.1",
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
    "productContractRevision": "2026-08-10.1",
    "remoteContractRevision": "2026-08-10.1"
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
    "productContractRevision": "2026-08-10.1",
    "remoteContractRevision": "2026-08-10.1",
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
    "productContractRevision": "2026-08-10.1",
    "remoteContractRevision": "2026-08-10.1",
    "warnings": []
  }
}
```

The plan contains normalized inputs, current preconditions, an effect summary,
and an opaque short-lived `planToken`. Secrets and personal data are redacted.
There is no separate `--dry-run` flag: omitting `--apply` is the only dry-run
behavior.

### Standard mutation

Event creation, event update, and RSVP changes are standard mutations. The
caller first reviews the plan, then repeats the same command input with:

```text
--apply --plan <plan-token>
```

The command cannot execute without a live token from the matching plan.

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

Tokens are single-use and expire after five minutes. A changed input, changed
remote precondition, expired token, or reused token returns
`safety.plan_stale`. `--apply` without `--confirm` on a consequential action
returns `safety.confirmation_required`. There is no `--yes` bypass and no
interactive mutation prompt.

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
does not return identity details or credentials.

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

`state` is `active` or `cancelled`. `userRole` is `host`, `cohost`,
`attendee`, or `none`.

#### `partiful events get <event-id>`

Returns an `Event`:

```json
{
  "eventId": "evt_example",
  "title": "Example event",
  "start": "2026-09-12T19:00:00-07:00",
  "end": null,
  "timezone": "America/Los_Angeles",
  "state": "active",
  "userRole": "host",
  "myRsvp": null,
  "description": null,
  "location": null,
  "address": null,
  "visibility": "private",
  "guestLimit": null,
  "poster": null,
  "links": []
}
```

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

Returns:

```json
{
  "eventId": "evt_example",
  "status": "going",
  "partySize": 1,
  "plusOnes": [],
  "message": null
}
```

#### `partiful rsvp set <event-id>`

Accepts structured input or equivalent flags:

```json
{
  "status": "going",
  "partySize": 1,
  "plusOnes": [],
  "message": null,
  "timezone": "America/Los_Angeles",
  "questionnaire": null
}
```

`status` is `going`, `not-going`, or `interested`. Exact transport mappings
for these product values are an implementation gate. `timezone` is required
and identifies the attendee's IANA timezone. Applied success returns the
complete current RSVP shape.

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
privacy-safe match rules as guest invitations.
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

Poster output depends on the inferred poster catalog and remains behind its
implementation gate until the response is observed and reviewed.

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
  "failureTypes": [],
  "safety": {
    "kind": "standard-mutation",
    "planRequired": true,
    "confirmationRequired": false
  }
}
```

The real arrays and schemas contain the complete contract. This command is
generated from the same command definitions used by execution; it is not a
second handwritten description.

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

The remote API contract currently leaves every operation response status and
body unknown. Before a public command ships, its implementation slice must:

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
