# GOAL: Ship `events rsvp` (+ `explore rsvp` alias) with questionnaire support in partiful-cli

## Context
Repo: `~/repos/partiful-cli` (Commander.js, plain JS, no build step, no TypeScript). One file per command group in `src/commands/`; ALL API access goes through `src/lib/`. Tests: `npm test` (vitest). Install trap: the global `partiful` binary is a COPY, not a symlink, so source edits do NOT take effect until you rerun `npm install -g .`.

Read these BEFORE writing code:
- `AGENTS.md` (repo root) for conventions and boundaries.
- `docs/explore-command-design.md` (design note).
- `.wayfinder/map.md` and all `.wayfinder/tickets/*.md` (especially 02, 04, 05, 07).
- Load the `partiful` skill, `hermes-shared-chrome-cdp` skill, and `cli-api-recon` skill.

## What is already decided (do NOT relitigate)
- **Self-RSVP endpoint = `addGuest`** (confirmed, captured live). Params: `{eventId, rsvp:{name, count, plusOnes[], message, emailInvitationId, status, guestId, timezone, password}}`. Statuses `GOING|MAYBE|DECLINED` all go through the ONE call via the `status` field. First RSVP sends `guestId:null` (server creates the record); edits send the returned `guestId` back to update.
- **Interested = `markEventInterest`** `{eventId, interested:bool, source?}`.
- **`updateGuestStatus` is HOST-ONLY** (403 on your own record). Do NOT use it for self-RSVP.
- **Auth:** reuse `src/lib/http.js` (Firebase Bearer, auto-refresh). A raw unauthed fetch 401s. Do NOT hand-roll auth.
- **Same endpoint for all events** (invited, discovered, self-owned). Ownership only gates host-management calls.
- **Command naming (DECIDED by Kaleb 2026-07-24):**
  - CANONICAL: `events rsvp <id> [--status going|maybe|declined] [--plus-one NAME] [--count N] [--message TXT] [-y] [--dry-run]` -> `addGuest`
  - ALIAS: `explore rsvp <id>` -> thin forward to the SAME handler.
  - Same alias pattern for `events interested` / `explore interested <id> [--remove]` -> `markEventInterest`.
  - Single shared handler; `explore *` verbs just forward to the `events *` implementation.

## The ONE open unknown to resolve first (recon)
How custom questionnaire answers ride inside the `addGuest` payload. Some hosts require Q&A before an RSVP submits (`getLastQuestionnaireAnswers` is the tell). This affects regular invited events too, not just discovery. Do NOT ship without handling it.

### Recon method (ticket 07 rig)
1. Create a THROWAWAY self-owned Partiful event WITH a required custom question, via the web app (the CLI has no questionnaire flag). Keep it private/unlisted. (Ask Kaleb to create it and hand you the `/e/{id}` link if you cannot create questions programmatically.)
2. Attach to Kaleb's logged-in local Chrome via CDP on port 9222 (`hermes-shared-chrome-cdp` skill). The remote/cloud browser is NOT logged into Partiful; the local one is.
3. In the CDP tab, hook `window.fetch` + `XMLHttpRequest` to log all `api.partiful.com` calls, THEN navigate to the event `/e/{id}`.
4. Click RSVP, answer the question, choose Going, Continue. Capture the `addGuest` request body.
5. Diff against the known clean payload to find where answers live (likely a new field inside `rsvp`, e.g. `answers[]` / `questionnaireAnswers`). Note the shape (question id vs text, answer format).
6. DELETE the test event. Zero residue.

## Build (after recon)
1. Implement a single shared RSVP handler in `src/commands/` reusing `src/lib/http.js`.
2. Read-before-write: the CLI is stateless, so the handler must call `getCurrentGuest {eventId}` first to decide create (`guestId:null`) vs update (pass existing `guestId`).
3. Wire `events rsvp` (canonical) and `explore rsvp` (alias -> same handler). Same for `interested`.
4. Bake in questionnaire support per the recon findings. If an event requires a questionnaire and the user did not supply answers, fail clearly (do not silently submit).
5. Flags: `--status` (default `going`), `--plus-one` (repeatable), `--count`, `--message`, `--password` (field already in payload), `-y/--yes`, `--dry-run`.
6. Confirmation gate on writes by default (writing to a guest list); `-y` skips for agent flows. `--dry-run` previews the payload without sending.
7. Refuse ticketed/paid events cleanly (Stripe wall) and point the user to the app.

## Hard rules
- **NO em dashes anywhere** in user-facing output or event copy the CLI writes (Kaleb hard rule). Use colons/commas.
- Never expose phone numbers or Partiful user IDs in user-facing output.
- Ask before sending text blasts, cancelling events, or bulk ops.
- Do not hardcode auth tokens in source.

## Definition of done
- `events rsvp` and `explore rsvp` both work end-to-end against a real event (verify with `--dry-run` first, then a live RSVP on a self-owned event, then revert/delete).
- Questionnaire event RSVP works (verified on the recon test event before deletion).
- `npm test` passes; add unit tests for the handler (mock `src/lib/http.js`).
- `npm install -g .` rerun so the global binary reflects changes.
- Update the `partiful` skill + repo README with the new commands.
- Update the Todoist task `id:6h6Pvxgw2mJf466G` to reflect shipped state (or leave a comment with the final command surface).