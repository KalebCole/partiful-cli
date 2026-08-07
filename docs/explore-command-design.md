# `explore` command — design note & stubbed surface

Status: PROPOSED (COMMAND-SHAPE / ticket 04). React to this, then IMPLEMENT (05).

Discovery = browsing Partiful's public trending/discover feed and (optionally)
RSVP'ing or expressing interest in events you were never invited to. All reads
and writes reuse the existing Firebase-callable auth in `src/lib/http.js`.

Wire callables are hidden — the CLI never says `addGuest`.

| User verb | Hidden callable |
|---|---|
| `explore list` / `explore trending` / `explore regions` | `getDiscoverFeed`, `getDiscoverSections` |
| `explore rsvp set` | `addGuest` |
| `explore interested` | `markEventInterest` |

---

## Command surface

```
partiful explore <subcommand> [options]

Browse and RSVP to public Partiful events you weren't invited to.

Subcommands:
  list                 Browse the discovery feed (default region: nyc)
  trending             Trending carousels grouped by region
  regions              List available regions + their tags
  rsvp get <eventId>   Read your current RSVP and questionnaire answers
  rsvp set <eventId>   RSVP yourself to a public event (going/maybe/declined)
  interested <eventId> Mark yourself interested (softer than RSVP)
```

### `explore list`
```
partiful explore list [options]

  --region <slug>    Region: nyc la sf bos dc chi lon mia atx  (default: nyc)
  --tag <tagId>      Filter by tag (see `explore regions` for valid tags)
  --limit <n>        Max events to return                       (default: 20)
  --cursor <cursor>  Pagination cursor from a prior page
  --format <fmt>     json | table                               (default: json)
```

### `explore trending`
```
partiful explore trending [options]

  --region <slug>    Restrict to one region (default: all regions)
  --format <fmt>     json | table   (default: json)
```

### `explore regions`
```
partiful explore regions [--format json|table]
  Lists region slugs + the tag list (id + friendly name) for --tag filtering.
```

### `explore rsvp set <eventId>`
```
partiful explore rsvp set <eventId> [options]

  --status <s>       going | maybe | declined                   (default: going)
  --plus-one <name>  Add a named plus-one (repeatable)
  --count <n>        Total headcount incl. yourself + plus-ones  (default: 1)
  --message <msg>    Public comment posted on the event page
  -y, --yes          Skip the confirmation prompt (agent flows)
  --dry-run          Print the payload, don't write

  Refuses (exit 4, type unsupported_event) on ticketed / password-gated /
  questionnaire events — use the Partiful app for those.
```

### `explore interested <eventId>`
```
partiful explore interested <eventId> [options]

  --remove           Remove your interested mark
  -y, --yes          Skip confirmation
  --dry-run          Print payload, don't write
```

---

## Output contracts (JSON is default; table is a view)

### `explore list`
```json
{
  "region": "nyc",
  "tag": null,
  "events": [
    {
      "id": "JKQD5kibarjDeBw4LN6W",
      "title": "shimmer ✨ at elsewhere",
      "startDate": "2026-07-25T22:00:00-04:00",
      "neighborhood": "Bushwick",
      "venue": "Elsewhere",
      "host": "nico’s",
      "ticketed": true,
      "url": "https://partiful.com/e/JKQD5kibarjDeBw4LN6W"
    }
  ],
  "paging": { "nextCursor": "CmYKEg... " },
  "total": 20
}
```

`--format table` columns: `title · start · neighborhood · host · 🎟 · id`
(`🎟` marks ticketed; `id` last so long titles don't push it off-screen).

### `explore regions`
```json
{
  "regions": [
    { "slug": "nyc", "name": "New York City" },
    { "slug": "la",  "name": "Los Angeles" }
  ],
  "tags": [
    { "id": "DISCOVER_HOME",  "name": "For You" },
    { "id": "DISCOVER_MUSIC", "name": "Music" },
    { "id": "DISCOVER_FOOD",  "name": "Food & Drink" }
  ]
}
```

### `explore rsvp set` (success)
```json
{
  "eventId": "JKQD5kibarjDeBw4LN6W",
  "status": "GOING",
  "count": 1,
  "plusOnes": [],
  "guestId": "Z8pBamPchdOGNpGQcVoQ",
  "url": "https://partiful.com/e/JKQD5kibarjDeBw4LN6W"
}
```

### `explore rsvp set` (refused — ticketed)
```json
{
  "status": "error",
  "error": {
    "code": 4,
    "type": "unsupported_event",
    "message": "This event requires tickets/payment. RSVP in the Partiful app."
  }
}
```

---

## Behavior notes for IMPLEMENT

1. **Statelessness → read-before-write.** CLI won't have a `guestId` on a repeat
   run. `explore rsvp set` should call `getCurrentGuest {eventId}` first: if a record
   exists, pass its `guestId` back to `addGuest` (update); else send
   `guestId:null` (create). Same for `--status declined` (revert path).
2. **`addGuest` payload** (built by `wrapPayload`, hidden from user):
   `{eventId, rsvp:{name, count, plusOnes[], message, emailInvitationId:null,
   status, guestId, timezone, password:null}}`. `name` = `config.displayName`;
   `timezone` = config tz (default America/Los_Angeles).
3. **Status mapping:** CLI `going|maybe|declined` → wire `GOING|MAYBE|DECLINED`.
4. **Confirmation gate** (AGENTS.md destructive policy): `rsvp set` + `interested`
   write to a real host's guest list → prompt unless `-y`. `--dry-run` prints
   payload + target endpoint, no write.
5. **Ticketed/password/questionnaire guard:** detect via `getEventInfo` /
   `getEventRestrictions` before writing; refuse rather than create a broken
   record. (These branches are UNTESTED against `addGuest` — see ticket 02/07.)
6. **File layout:** new `src/commands/explore.js`, `registerExploreCommands`,
   one command group, structured `{status, error:{code,type,message}}` errors,
   `jsonOutput`/`jsonError` like the rest.

## Open for Kaleb
- Default region `nyc`, or infer from something? (No location signal in auth;
  nyc is the biggest feed. Leaning: default nyc, document it.)
- Expose region **slugs** (`--region nyc`) — friendly names shown in
  `explore regions` output. Agree?
