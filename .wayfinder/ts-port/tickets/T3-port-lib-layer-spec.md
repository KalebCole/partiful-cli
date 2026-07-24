# T3 — Port src/lib/ API layer + author endpoint types/Zod (THE SPEC)

**Type:** task (AFK) · **Blocks on:** T2 · **Status:** OPEN

## Question

Port the `src/lib/` layer to TS. This is the keystone ticket: typing these files IS authoring the
API spec. The ~23 untyped `result?.data...` response spreads become endpoint interfaces + Zod
schemas. Everything downstream (T4, T5, T6) consumes what this produces.

## Files (port in this order — API core first)

http.js, auth.js, events.js, rsvp.js, upload.js, cohosts.js, posters.js, dates.js, errors.js,
output.js, templates.js

## What "the spec" means here

For every endpoint the lib layer calls (createEvent, cancelEvent, getEventInfo, getContacts,
createTextBlast, addInvitedGuestsAsHost, getMyUpcomingEventsForHomePage, getMyPastEventsForHomePage,
addGuest, markEventInterest, getCurrentGuest, + Firestore GET/PATCH + auth endpoints):
- typed request interface (complete)
- Zod `.passthrough()` response schema + `z.infer` type (known-fields, non-exhaustive)
- referencing the shared envelope generic
- tagged by transport (firebase-callable / firestore / firebase-auth)

Collect these into a coherent location (e.g. `src/lib/api/` types + schemas) so T5 can surface them.

## FIRST-SLICE ORACLE (replaces the old human-approval gate)

The FIRST file (http.js + one fully-typed endpoint) must satisfy, as written criteria, before the
agent replicates the pattern across the rest:
- `tsc --noEmit` clean under strict
- endpoint has: request interface + Zod `.passthrough()` response + z.infer type + envelope reuse
- existing tests for that path still green
If met, that IS the approved pattern — replicate across all lib files. No human pause.

## Done when

All `src/lib/*.js` → `.ts`, strict-clean, all endpoint types+schemas authored, tests green.

## Answer

<!-- list endpoints spec'd + where types live, on close -->
