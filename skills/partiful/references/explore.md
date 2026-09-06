# Explore

## Discover events

```bash
partiful doctor
partiful explore list --city Seattle --state WA --country US
partiful explore list --city Seattle --state WA --country US --section trending --date-filter this-weekend
```

`explore list` is authenticated and read-only. Run `partiful doctor` first to
check local authentication state; use `partiful auth login` when credentials
are unavailable. The command requires an explicit `--city`, `--state`, and
`--country`; it does not infer an area or region code.

Optional `--section` values are `trending`, `friends`, `open-invite`, and
`followed`. Optional `--date-filter` values are `anytime` (the default),
`today`, `tomorrow`, `this-week`, and `this-weekend`. Date facets filter by the
event's local timezone calendar overlap.

## Pagination and completeness

```bash
partiful explore list --city Seattle --state WA --country US --limit 25
partiful explore list --city Seattle --state WA --country US --cursor <cursor-from-prior-result>
partiful explore list --city Seattle --state WA --country US --all --max-items 100
```

`--limit` is 1 through 100. `--all` requires `--max-items` (1 through 1000)
and cannot be combined with `--limit`. A cursor is opaque and valid only for
the same city, state, country, section, and date filter; pass it unchanged.
The command deduplicates event IDs, fails closed if remote pagination cannot
progress, and returns bounded results.

Inspect `meta.page.filterComplete` when a date filter is used. It is `false`
when unread remote feed pages could contain further matching events, and `true`
for `anytime`, a terminal feed traversal, or a section result after filtering.

## Result handling and follow-ons

Every result item includes `eventId`, `url`, `title`, `start`, `end`,
`timezone`, `location`, `mapsUrl`, `goingCount`, `maybeCount`,
`interestedCount`, `mutuals`, and `mutualsPreviewComplete`. `eventId` and `url`
are non-null; `location` and `mutuals` are always arrays (possibly empty);
`title`, `start`, `end`, `timezone`, `mapsUrl`, `goingCount`, `maybeCount`, and
`interestedCount` may be `null`. `mutualsPreviewComplete` is always `null`
because remote completeness is unproved.

Location can preserve hidden or approximate display lines. Mutuals expose only
a display name and normalized RSVP status. Do not expose access tokens, phone
numbers, email addresses, Partiful user IDs, guest IDs, owner IDs, or other
private identifiers in user-facing summaries.

Use canonical commands after discovery:

```bash
partiful events get <event-id>
partiful rsvp get <event-id>
partiful rsvp set <event-id> --status interested
```

Explore does not provide mutations, arbitrary text search, map pins, date-range
callables, custom RSVP/interest aliases, or raw transport escape hatches.
