# Current mobile callable surface and Firestore migration research

**Issue:** [#206](https://github.com/KalebCole/partiful-cli/issues/206)

**Research date:** 2026-09-05

**Repository baseline:** `edbdd4de7c1c6e768394bed1a989f8091364231d`

**Status:** Research evidence. This document does not change the approved remote or CLI contracts.

## Question

Does the current Partiful mobile client expose enough Firebase callable operations to move a future CLI revision away from direct Firestore access?

## Answer

A callable-first migration is practical and worth pursuing. A callable-only migration is not supported by the evidence.

Partiful Android 3.8.28 contains 275 unique first-party callable names. Only 19 match the current OpenAPI contract and production Go callable references. The mobile surface includes current Explore V2 operations, personal RSVP aggregation, comments, displayed host messages, pending cohost state, guest management, event settings, and many other operations that the current contract does not cover.

Direct Firestore remains current first-party behavior. Generic event edits and the Auto-Reminders toggle used Firestore Write streaming during controlled browser tests. The current CLI also has five direct Firestore route shapes. Several callable reads overlap those routes, but none yet reproduces every field or precondition required by the CLI.

The migration target should therefore be:

1. prefer a proven callable for each command or scoped setting;
2. retain Firestore where the callable loses required state or realtime behavior;
3. replace a Firestore path only after a controlled positive fixture proves equivalent request, response, authorization, and failure behavior; and
4. keep the CLI product model independent from transport names.

## Artifact identity

The mobile inventory came from Partiful Android 3.8.28, package `com.partiful.partiful`.

| Artifact | SHA-256 |
| --- | --- |
| Base APK | Prefix `c0bf54463471` |
| Hermes bytecode bundle | Prefix `53b67260d157` |
| Sanitized callable classification | See `spec/research/mobile-callable-classification-20260905.json` |

The full artifact hashes and acquisition notes are recorded in the linked Explore prototype evidence.

The Hermes bundle uses bytecode version 98. Static extraction recovered uppercase `*_FUNCTION_NAME` constants, paired operation strings, wrapper context, and generic callable-adapter behavior. Names alone do not prove request schemas or business semantics.

The shared mobile adapter uses Firebase `httpsCallable`, defaults to `https://api.partiful.com`, adds account/device metadata when defined, and returns date-normalized callable result data.

## Inventory counts

| Classification | Count |
| --- | ---: |
| Unique mobile callable names | 275 |
| Exact OpenAPI matches | 19 |
| Exact evidence-ledger matches | 19 |
| Exact production Go callable references | 19 |
| No current CLI command mapping | 256 |
| High research priority | 40 |
| Medium research priority | 116 |
| Low research priority | 119 |

The counts are mechanical. The 275 names are non-empty and unique, priorities sum to 275, and duplicate count is zero.

## Current transport map

Partiful currently has several active transport surfaces:

- The public Next.js `/_next/data` projection serves current public-web pages. It is not legacy, but it exposes less discovery and social context than the mobile callable surface.
- Firebase callable operations on `api.partiful.com` provide command-style writes and many query-style reads.
- Firebase Authentication supplies account sessions and callable bearer tokens.
- Firestore REST and Firestore Write streaming provide document, collection, and editor behavior.

The mobile and web applications do not divide those responsibilities consistently. The CLI should hide that inconsistency rather than reproduce it in public commands.

## Verified callable reads

### Explore

The Explore prototype and follow-up probes verified:

- `getDiscoverFeedV2` with dynamic Seattle area, cursor paging, and `tagId`;
- `getDiscoverSectionsV2` with generated Trending, Friends, Open Invite, and Followed Orgs sections;
- `getDiscoverEventItemDecorators` with authenticated mutual-guest previews;
- `getDiscoverMapPins` with viewport bounds, returning 17 pins for a wide Seattle viewport and 6 for a tighter viewport; and
- `getDiscoverEventsByDateRange` with `{area,fromDate,toDate,tagId}` plus cursor paging, returning `daysByDate`.

Feed, sections, tags, map pins, and decorators returned HTTP 200 without authorization for the tested public Seattle requests. Unauthenticated decorators returned an empty map; authenticated requests returned social context. Account-scoped home lists and `getCurrentGuest` still required authentication.

`getDiscoverTagsV2` returned an empty tag array both authenticated and signed out. The current Android bundle contains the registration but no recoverable caller. This operation should remain research-only until a current first-party interaction produces a non-empty request and response.

### Event and account reads

Current mobile and live evidence supports:

- `getEventInfo` for event detail and settings readback;
- `getMyUpcomingEventsForHomePage` and `getMyPastEventsForHomePage` for account event lists;
- `getCurrentGuest` for caller-specific RSVP presence, ID, status, and count; and
- `getMyRsvps`, which returned `result.data.events` in a bounded authenticated probe.

The `getMyRsvps` response was approximately 830 KB. It may be useful for personal event aggregation, but it needs an explicit output bound and a reviewed projection before becoming a CLI command.

## Controlled write evidence

All browser captures used the authenticated durable Hermes profile and the Printing Press capture rules. Request evidence was sanitized before retention. No raw token, cookie, private ID, phone number, email, or response body is in this branch.

### RSVP

A user-authorized attendee test changed one RSVP from `sent` to `declined` through `addGuest`. Independent CLI readback confirmed the final `declined` state. The captured request used the established nested RSVP object and `status:"DECLINED"`.

### Event creation and deletion

Two self-owned throwaway events were created for controlled tests with the UI set to Private, then deleted afterward. Independent `events get` calls returned `EVENT_NOT_FOUND` for both deleted IDs.

The create UI displayed Private while `createEvent` sent or persisted public/published state. That mismatch is unresolved. Do not rely on the UI label or the callable field until a dedicated positive/negative visibility test verifies both persisted state and public accessibility.

### Scoped setting callables

Controlled write plus independent `getEventInfo` readback verified:

| Setting | Transport | Result |
| --- | --- | --- |
| Free-form location | `setEventLocation` callable | Persisted and read back |
| Theme and display settings | `updateDisplaySettings` callable | Final `forest` theme persisted and read back |
| Event deletion | `deleteEvent` callable | Event became not found |

### Firestore writes still used by the current editor

| Setting | Observed transport | Result |
| --- | --- | --- |
| Generic title edit | Firestore Write streaming | Persisted and read back in the first throwaway test |
| Auto-Reminders toggle | Firestore Write streaming | `enableGuestReminders:false` persisted and read back |

Static mobile evidence also contains scoped callable names such as `makeEventPublic`, `publishEvent`, `unpublishEvent`, and `updateEventGuestReminders`. The tested web UI did not use a callable for guest reminders. Publish/unpublish was not tested because the create visibility mismatch made safe reversal unclear.

## Firestore production inventory

The current Go CLI has five distinct production Firestore route shapes. None is a realtime listener in the Go process.

| Firestore route | CLI consumer | Current reason | Replacement state |
| --- | --- | --- | --- |
| `events/{eventId}/guests` collection GET | `blasts.send` | Reads guest status and non-null `checkIn` for audience and bound preconditions | `getGuests` does not prove `checkIn`; retain Firestore |
| `events/{eventId}/hostMessages` collection GET | `blasts.send` | Counts absent/null/`TEXT_BLAST` message types | `getEventDisplayedHostMessages` is display-scoped and element/count equivalence is unproved |
| `events/{eventId}/cohostRequests` collection GET | cohost contact actions | Enumerates target and status for all requests | `getPendingCohostRequestForEvent` is caller-specific and not a collection replacement |
| `events/{eventId}/private/cohostSecret` document GET | cohost link create/revoke | Distinguishes absent/present link state and reads path | No replacement proved |
| `events/{eventId}` document PATCH | `events.update` | Writes the generic allowlisted event-field projection | Mobile editor still uses Firestore; no general callable replacement found |

The machine-readable version is `spec/research/firestore-callsite-classification-20260905.json`.

## Promising operations that are not replacements yet

### Displayed host messages

`getEventDisplayedHostMessages` is a live callable with `{eventId}` and returns `hostMessages`. The positive element schema, pagination, and display filtering are unknown. The blast guard counts all applicable Firestore host-message documents using a specific type rule. The callable cannot replace that guard yet.

### Pending cohost request

`getPendingCohostRequestForEvent` is a live authenticated callable with `{eventId}` and returns one `pendingCohostRequest` value. Current host commands enumerate all cohost requests and need target/status state. This is useful for caller UX, not a collection replacement.

### Comments

`getEventComments` is a live callable with `{eventId}` and returns `comments`. It is a good candidate for a future comments command. A one-shot callable does not replace Android's update-token/live refresh behavior, and no populated controlled fixture established comment shape or ordering.

### Guest and RSVP operations

The mobile inventory contains current wrappers for:

- `getGuestsForHomepage`
- `getMyRsvps`
- `getRsvpUpdateForEvent`
- `getMutualGuestsForEvents`
- `updateGuestStatus`
- `updateGuestStatuses`
- `deleteGuest`
- `removeGuest`
- `removeGuests`

Only `getMyRsvps` received a successful positive read with an operation-specific response field. Host-management mutations remain static candidates until controlled host fixtures verify authorization, persistence, error behavior, and independent readback.

The current `getGuests` decoder also showed protocol drift during a host probe. That issue must be investigated before using `getGuests` as replacement evidence.

No callable candidate proved complete `checkIn`, `questionnaireResponse`, or ordinary non-null `plusOnes` state. Those fields still require targeted Firestore evidence.

## Errors and authentication

Observed read behavior includes:

- public Explore reads returning HTTP 200 for the tested request shapes;
- account lists and current-guest reads returning HTTP 401 with Firebase `UNAUTHENTICATED` errors when signed out;
- malformed read requests commonly returning HTTP 500 with Firebase `INTERNAL`, not HTTP 400; and
- a selected event detail returning HTTP 200 both authenticated and signed out, without establishing universal public readability.

These observations are endpoint samples, not operation-wide guarantees.

## Migration verdict

The mobile callable surface is the best current discovery source for future CLI work. It is large enough to justify a systematic contract refresh and can replace or add several user-facing operations.

It does not justify deleting Firestore support. Generic event updates remain Firestore-backed in the current mobile client, and several callable reads return narrower or caller-specific data than the CLI's current Firestore preconditions require.

A safe migration should proceed in small slices:

1. Promote the verified Explore V2 feed, sections, decorators, map pins, and date-range reads.
2. Fix and re-evidence `getGuests` protocol drift.
3. Add reviewed contracts for `setEventLocation`, `updateDisplaySettings`, and `deleteEvent` using the controlled write evidence.
4. Investigate the create visibility mismatch before exposing visibility controls.
5. Build positive fixtures for displayed host messages, comments, cohost requests, check-in state, questionnaires, and plus ones.
6. Keep the generic event PATCH until a current first-party callable provides equivalent behavior.
7. Re-run the mobile inventory on each app release and diff operation names mechanically.

## Recommended follow-up tickets

1. Promote Explore V2 feed, sections, decorators, map pins, and date-range evidence into a proposed remote contract revision.
2. Diagnose current `getGuests` response drift with a sanitized owned-event fixture.
3. Propose scoped host setting contracts for `setEventLocation`, `updateDisplaySettings`, and `deleteEvent`.
4. Investigate `createEvent` visibility versus UI and public accessibility.
5. Capture populated comments and displayed-host-message fixtures.
6. Capture populated cohost-request and cohost-link state fixtures.
7. Verify host guest-management callables on a private owned event with controlled synthetic guests.
8. Add a repeatable mobile callable inventory/diff tool that produces sanitized output only.

## Evidence boundaries

- The canonical OpenAPI and evidence ledger remain revision `2026-08-12.7` and remain authoritative.
- Static operation names are bundle-derived evidence only.
- Read probes establish only the tested request and account/resource case.
- Write acceptance is separate from persisted-state readback.
- The research branch contains no APK, Hermes bytecode, decompiled source, raw capture, credential, private identifier, or real name.
