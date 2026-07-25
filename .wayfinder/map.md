<!-- wayfinder:map -->
# Map: Partiful `explore` — trending discovery + RSVP

## Destination

A shipped `partiful explore` command surfacing Partiful's public trending/discovery
events, AND the ability to **RSVP** (going) or mark **interested** on a discovered
event you were never invited to. "Shipped" = command shape, output contract,
build-id handling, and the discover write-path (RSVP/interested) all decided and
implemented in `~/repos/partiful-cli`, with skill/README docs updated.

This is an **execution-carrying** map: it ends in merged code, not just a spec.

## Notes

- Domain: reverse-engineered Partiful internal API. No official docs.
- Repo: `~/repos/partiful-cli`. Commander.js, plain JS, no build step. One file per
  command group in `src/commands/`; ALL API access goes through `src/lib/`.
- Install trap: global `partiful` is a COPY, not a symlink. Source edits don't take
  effect until `npm install -g .` reruns. (per partiful skill + AGENTS.md)
- Tests: `npm test` (vitest). Integration tests hit real API + need auth.
- Skills every session should consult: `partiful` (skill), `cli-api-recon`,
  `cli-architect`. Repo `AGENTS.md`.
- Auth is ASSUMED (see Decisions). Reuse `src/lib/http.js` token flow; no anon mode.
- No em dashes in any user-facing event copy this CLI writes (Kaleb hard rule).

## Decisions so far

- [Auth model: always logged in](#) — `explore` reuses the existing Firebase-token
  flow via `src/lib/http.js`. No anonymous/unauthed mode. The CLI's contract is that
  the user is logged in; discovery read endpoints happen to be public, but we don't
  build a separate anon path.
- [Discovery is not read-only](#) — RSVP (going) and "interested" on a discovered
  event are in scope for v1, not deferred.
- [BUILD-ID: use stable api.partiful.com, no build id](.wayfinder/tickets/01-build-id-recon.md)
  — `POST /getDiscoverFeed` (paginated feed) + `POST /getDiscoverSections` (trending
  carousels + tags), Bearer auth, Firebase `{"data":{params,paging}}` envelope. Cursor
  pagination via `result.paging.nextCursor`. No rotating build id needed; reuse
  `src/lib/http.js`. `/_next/data/{buildId}` is the fallback (scrape buildId from
  `__NEXT_DATA__`).
- [TAG-FILTER: server-side via tagId](.wayfinder/tickets/03-tag-filter-recon.md) —
  `--tag` maps directly to the `tagId` param; verified filtering (NYC: HOME=20,
  MUSIC=15, FOOD=4). Valid tags from `getDiscoverSections` `.tags[]`.
- [RSVP-ENDPOINT: two verbs, not one flag](.wayfinder/tickets/02-rsvp-endpoint-recon.md)
  — INTERESTED and GOING use different endpoints, so `explore interested` and
  `explore rsvp` are separate verbs. **INTERESTED fully solved**: `markEventInterest`
  `{eventId, interested:bool, source?}` — true creates an INTERESTED guest record,
  false removes it (verified live + reverted). `getCurrentGuest {eventId}` reads
  state. **GOING SOLVED (2026-07-17)**: the self-RSVP mutation is `addGuest`
  `{eventId, rsvp:{name,count,plusOnes[],message,status,guestId,timezone,password,...}}`.
  First RSVP `guestId:null` (creates); edits pass returned `guestId`. GOING / MAYBE /
  DECLINED all via the `status` field. Captured live from OpenClaw's logged-in Chrome
  (CDP :9222); verified GOING then reverted DECLINED. Ticket 07 CLOSED.

## Not yet specified

- Caching strategy is now MOOT for the build id (stable API used). Any caching is a
  minor perf choice deferred to IMPLEMENT (e.g. cache the tag list per region).
- Output columns for the human `--format table` view (which event fields matter).
  Graduates once COMMAND-SHAPE is decided.

## Out of scope

- Event detail enrichment via `getDiscoverEventItemDecorators` (guest-count badges).
  Nice-to-have overlay, not required to browse or RSVP. Revisit as a later effort.
- Non-US regions beyond what the region-slug endpoint already returns for free.
