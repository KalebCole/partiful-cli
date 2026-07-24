<!-- wayfinder:research -->
# RSVP-ENDPOINT: how to RSVP / mark interested on a discovered event

Labels: wayfinder:research
Blocked by: (none — frontier)
Assignee: hermes
Status: closed

## Question

RSVP-ing to a DISCOVERED event is different from `guests invite` — the user was
never invited; they're crashing a public event. Find the API call the web
`/explore` and `/e/{id}` pages fire when a logged-in user clicks "Going" or
"Interested" on a public event.

Method: attach to the browser logged in as Kaleb, open a discovered public event,
capture the XHR/fetch on the RSVP + interested buttons (via performance entries /
CDP network). Identify:
- Endpoint URL(s) on `api.partiful.com` (e.g. `setRsvp` / `rsvpToEvent` /
  `setGuestStatus` / `expressInterest`).
- Request body shape (eventId, status enum — GOING / INTERESTED / MAYBE?).
- Whether "interested" is a distinct status value on the same endpoint or a
  separate endpoint.
- Auth header used (should be the same Firebase token `src/lib/http.js` sends).
- Success + error response shapes.

Do NOT actually RSVP to a stranger's event during recon unless unavoidable; if a
live write is needed, use a throwaway/self-owned event and clean up.

Output: markdown answer — endpoint(s), body, status enum, auth. Resolves the
"one --status flag vs two verbs" fog item.

---

## Resolution (closed — partially, with a follow-up)

All endpoints are Firebase-callable: `POST https://api.partiful.com/<name>`, Bearer
auth (reuse `src/lib/http.js`), body `{"data":{"params":{...}}}`, response
`{"result":{"data":{...}}}`.

### INTERESTED — fully solved and verified end-to-end
**`markEventInterest`** — params `{eventId, interested: bool, source?}`.
- `interested:true`  -> `{interested:true, success:true}`; **creates a guest record**
  with `status:"INTERESTED"`.
- `interested:false` -> `{interested:false, previousStatus:"INTERESTED", success:true}`;
  **removes** the guest record (verified: currentGuest -> NONE after).
- `source` is OPTIONAL (200 with it omitted). Web sends an enum value (`DISCOVER`);
  any string or absence works. Recommend sending `"DISCOVER"`.
- Read current state with **`getCurrentGuest`** `{eventId}` ->
  `result.data.currentGuest.{id,status}` (or null).
- Verified live on 2 discovered events (5R73..., UHjP...), then reverted, no residue.

### GOING (RSVP) — SOLVED 2026-07-17: the mutation is `addGuest`
Captured live from OpenClaw's logged-in local Chrome (CDP :9222) — see ticket 07.
**`addGuest`** — params `{eventId, rsvp:{name, count, plusOnes[], message, emailInvitationId,
status, guestId, timezone, password}}`. First RSVP sends `guestId:null` (server creates
record); edits send the returned `guestId` to update. Statuses `GOING` / `MAYBE` / `DECLINED`
ALL go through this one call via the `status` field. Same Bearer auth (`src/lib/http.js`);
a raw unauthed fetch 401s. Verified end-to-end: RSVP'd GOING then reverted to DECLINED, clean.

### (superseded) earlier dead-ends on the GOING path
- **`updateGuestStatus`** `{eventId, guestId, guestStatus, rsvpReason?, newGuestName?}`
  is **HOST-ONLY**. Returns 403 `PERMISSION_DENIED "User is not a host of this event
  and is not an admin"` even for the caller's OWN guest record, even on an event the
  caller is legitimately invited to (tested idempotent MAYBE->MAYBE). So it is the
  host's guest-management tool, NOT the self-RSVP path. Do not use it for `explore rsvp`.
- **`addInvitedGuestsAsGuest`** `{eventId, userIdsToInvite[], phoneContactsToInvite[],
  invitationMessage?}` is for inviting mutuals, and 400'd `FAILED_PRECONDITION
  "Guests are not allowed to invite mutuals to this event"` when self-targeted. Not it.
- Probed 12 plausible names (respondToEvent, rsvpToEvent, setMyRsvp, joinEvent,
  selfRsvp, addSelfAsGuest, ...) -> all 404. The real self-RSVP mutation is
  **lazy-loaded** in a dynamic chunk not present in the initial `/e/[event]` bundle,
  so static grep of the 27 eager chunks did not surface it.

### Status enum (wire values, from public getMyRsvps / getCurrentGuest)
`GOING . MAYBE . DECLINED . INTERESTED . WAITLIST . APPROVED . SENT` (uppercase literals).

### Follow-up ticket created: 07-rsvp-going-live-capture
The GOING mutation must be captured from a **logged-in browser**: open a public
discovered event, click "RSVP -> Going", record the XHR to `api.partiful.com`
(method name + `params` shape). The remote automation browser is NOT logged into
Partiful, so this needs Kaleb's local logged-in browser (CDP) or a manual devtools
capture. Blocks the GOING half of IMPLEMENT.

### Recommendation for COMMAND-SHAPE
Both write paths now proven. Ship two verbs (not a shared `--status` flag, since
interested and RSVP use different endpoints):
- **`explore interested <eventId>` (+ `--remove`)** — `markEventInterest`.
- **`explore rsvp <eventId> [--status going|maybe|declined]`** — `addGuest`.
Both reuse `src/lib/http.js` auth. Ticket 07 fully closed; nothing gating IMPLEMENT.
