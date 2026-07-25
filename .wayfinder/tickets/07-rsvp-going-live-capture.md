<!-- wayfinder:research -->
# RSVP-GOING-LIVE-CAPTURE: capture the self-RSVP (Going) mutation from a logged-in browser

Labels: wayfinder:research
Blocked by: (none)
Assignee: hermes
Status: CLOSED — resolved 2026-07-17

## Why this exists

BUILD-ID and RSVP-ENDPOINT recon proved the discovery feed and the "interested"
write (`markEventInterest`). The **"Going" self-RSVP** mutation for a public event
could NOT be found by static analysis: `updateGuestStatus` is host-only, and the
real self-RSVP call is lazy-loaded in a dynamic chunk absent from the initial
`/e/[event]` bundle. 12 guessed endpoint names all 404'd.

## RESOLUTION — the mutation is `addGuest`

Captured live via `hermes-shared-chrome-cdp` (OpenClaw's persistent local Chrome
on port 9222, already logged in as Kaleb). Opened a dedicated CDP tab, hooked
`window.fetch` + `XMLHttpRequest`, navigated to public discover event
`JKQD5kibarjDeBw4LN6W` ("shimmer ✨ at elsewhere"), clicked RSVP → Going → Continue.

### Callable: `POST https://api.partiful.com/addGuest`

Request body (Firebase-callable wrapper):
```json
{
  "data": {
    "params": {
      "eventId": "JKQD5kibarjDeBw4LN6W",
      "rsvp": {
        "name": "Kaleb Cole",
        "count": 1,
        "plusOnes": [],
        "message": null,
        "emailInvitationId": null,
        "status": "GOING",
        "guestId": null,
        "timezone": "America/Los_Angeles",
        "password": null
      }
    },
    "amplitudeDeviceId": "<amp device id>",
    "amplitudeSessionId": <amp session id>,
    "userId": "<firebase uid>"
  }
}
```

### Key facts

- **First RSVP:** `guestId: null`. Server creates the guest record.
- **Edit/revert:** `guestId` becomes populated (e.g. `Z8pBamPchdOGNpGQcVoQ`); pass it back on subsequent `addGuest` calls to update the same record.
- **Status values (all through the SAME `addGuest` call):**
  - Going → `"GOING"`
  - Maybe → `"MAYBE"` (inferred from 3-button UI; not individually captured)
  - Can't Go → `"DECLINED"` ✅ (captured on revert)
- **count** = attendee count incl. plus-ones; **plusOnes** = array of names.
- **timezone** = IANA tz string.
- **password** = event password if the event is password-gated (else null).
- **message** = optional public comment posted on the event page.

### Auth caveat (IMPORTANT for implementation)

A raw `fetch` to `addGuest` WITHOUT the Firebase ID token returns
`401 UNAUTHENTICATED`. The web app injects a Firebase auth header the fetch hook
did not expose. The CLI already handles this via `src/lib/http.js` (Bearer token,
auto-refresh) — reuse it. Do NOT hand-roll the auth.

### Companion reads fired alongside RSVP (context, not required for the write)

`getLastQuestionnaireAnswers`, `recordMetrics` (analytics), then post-write
refresh: `getEventInfo`, `getEventRestrictions`, `getGuests`, `getUsers`.

## Cleanup performed

RSVP was created as GOING on a stranger's public event during capture, then
reverted to DECLINED via the UI (native CDP `Input.dispatchMouseEvent` — React
ignored synthetic JS clicks on the status buttons; a real dispatched mouse click
was required). Verified: event page button returned to "😢Can't Go", Kaleb no
longer shows as Going. NOTE: DECLINED still leaves a guest record on the event;
there was no "remove me entirely" affordance in the web flow. Acceptable residue.

## Deliverable — DONE

- ✅ Callable name: `addGuest`
- ✅ params shape + Going status (`GOING`)
- ✅ MAYBE / DECLINED go through the same call (status field)
- ✅ Same Bearer auth — reuse `src/lib/http.js`

Unblocks the GOING half of IMPLEMENT (05) and COMMAND-SHAPE (04).

## Follow-up: questionnaire shape (VERIFIED live 2026-07-24)

The original capture left the questionnaire field shape unverified — code
guessed field names and refused questionnaire events as a safe default. Closed
that gap with host-side recon on a throwaway private event (created via CLI,
added one required short-answer question, RSVP'd, then cancelled the event).

Verified facts (source: __NEXT_DATA__.props.pageProps on the logged-in event page):

- Event object (present ONLY when a questionnaire exists; keys absent otherwise):
    questionnaireEnabled: true
    questionnaire: {
      createdBy: { id, path }, createdAt,
      questions: [ { id: "<epoch-ms string>", type: "short_answer", text, required } ]
    }
    questionnaireVersions: [ { ...same shape... } ]   // append-only history

- Answer storage (guest object, pageProps.guest.questionnaireResponse):
    { questionnaireVersion: <int, index into versions>, answers: { "<questionId>": "<answer string>" } }
  The answers map is keyed by QUESTION ID (not text, not index).

- On write, the answers ride inside the /addGuest `rsvp` object as
  `questionnaireResponse` (same shape).

Implemented in src/lib/rsvp.js:
- eventRequiresQuestionnaire() now keys off questionnaireEnabled + questionnaire.questions[]
  (legacy field-name guesses kept as defensive fallback).
- buildQuestionnaireResponse(event, answersByKey) builds the verified response,
  keyed by id, accepts answers by id OR text, throws on unanswered REQUIRED
  questions (fail-closed).
- buildRsvpParams() attaches questionnaireResponse into the rsvp payload when supplied.
- 10 new unit tests (tests/rsvp.test.js). Full suite: 187/187 passing.

Cleanup: test event ztL5bpOhID4UfSaKOxXR cancelled (was private, never invited anyone).
