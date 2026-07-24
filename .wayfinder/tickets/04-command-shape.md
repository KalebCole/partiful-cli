<!-- wayfinder:prototype -->
# COMMAND-SHAPE: explore command surface + output contract

Labels: wayfinder:prototype
Blocked by: (none — BUILD-ID, RSVP-ENDPOINT, TAG-FILTER all resolved)
Assignee: hermes
Status: in-progress

## DECIDED (2026-07-17)

### Naming — user-facing verbs never leak wire names
Partiful's internal callables (`addGuest`, `markEventInterest`) stay buried in
`src/lib/http.js`. `addGuest` is a terrible CLI word — it's the server's
guest-list-writer primitive, not what the user is doing. User types intent:

```
partiful explore rsvp <eventId>                       # you're going (default)
partiful explore rsvp <eventId> --status maybe|declined
partiful explore rsvp <eventId> --plus-one "Name"     # +1 rides in same addGuest call
partiful explore rsvp <eventId> --count 2             # headcount incl. plus-ones
partiful explore interested <eventId> [--remove]      # softer signal (markEventInterest)
```

- `rsvp` verb chosen over `join`/`going` — matches Partiful's own button
  ("RSVP → Going"), reads like a human, and cleanly holds going/maybe/declined
  as a `--status` since they're ONE wire call (status field on `addGuest`).
- Plus-ones surface as `--plus-one` / `--count` flags ON the rsvp verb, NOT a
  separate command — because on the wire they're sub-fields of your own guest
  record, not distinct guests. This is honest naming (`--plus-one` describes the
  real thing) without exposing `addGuest`.
- `interested` stays a SEPARATE verb (different endpoint `markEventInterest`),
  not a `--status interested` on rsvp. Don't merge two endpoints under one flag.

### Still open (the actual prototype work)
- Discovery surface: `explore` with flags (`--region`, `--tag`, `--trending`,
  `--limit`) vs a group (`explore list|regions|trending`)? Lean: single
  `explore` + flags for browse, sub-verbs `rsvp`/`interested` for writes.
- Default region resolution: require `--region` or infer/default to one?
- `--format table` columns (id, title, startDate, neighborhood, url).
- Region slug map (nyc/la/sf/bos/dc/chi/lon/mia/atx) — slugs or friendly names?
- **Confirmation gate on `rsvp`** (per AGENTS.md destructive-command policy):
  `rsvp`/`interested` write to a real host's guest list → default confirm, `-y`
  to skip in agent flows. Ticketed/password/questionnaire events → refuse with a
  "use the app" guard rather than a half-broken guest record (see ticket 02/07
  untested branches).

Output: a `docs/` design note + stubbed `--help` text linked from this ticket.

## Prototype delivered → docs/explore-command-design.md
Full stubbed `--help` for all 5 subcommands + example JSON output contracts
(list, regions, rsvp success, rsvp refused) + IMPLEMENT behavior notes
(read-before-write for stateless guestId, confirmation gate, ticketed guard).
Two open Qs for Kaleb at the bottom (default region, slugs vs names).

## Original question (retained)

Decide the command surface and JSON/table output contract for discovery, given
what the three recon tickets resolve. Produce a rough stub (help text + example
JSON output) to react to — not the implementation.

Open sub-questions:
- Single command `explore` with flags (`--region`, `--tag`, `--trending`,
  `--limit`) vs a command group (`explore list|regions|trending`)?
- Where do RSVP/interested live? Sub-verbs `explore rsvp <eventId>` /
  `explore interested <eventId>`, OR one `explore rsvp <eventId> --status going|
  interested`, OR fold into existing top-level (`events rsvp`)? (depends on
  RSVP-ENDPOINT status enum)
- Default region resolution: require `--region`, or infer/ default to one?
- Output columns for `--format table` (id, title, startDate, neighborhood, url).
- Region slug map (nyc/la/sf/bos/dc/chi/lon/mia/atx) — expose slugs or friendly
  names?

Output: a `docs/` design note + stubbed `--help` text linked from this ticket.
This is the last decision before implementation; graduates the IMPLEMENT + DOCS
tickets out of fog.
