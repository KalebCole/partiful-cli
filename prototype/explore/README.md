# Partiful Explore prototype

> **THROWAWAY PROTOTYPE.** This directory is evidence for a future product design. It is not part of the public CLI, command catalog, OpenAPI contract, or release binary.

## Question

Can the current authenticated Partiful Explore API support Seattle discovery, pagination, filtering, and mutual-friend context with a sane agent-facing data model?

## Run

The live probe reuses the installed Partiful CLI credentials and performs read-only requests:

```bash
go run ./prototype/explore \
  --city Seattle --state Washington --country US \
  --max-results 20 --pages 2
```

Server-generated discovery modes:

```bash
go run ./prototype/explore --section trending
go run ./prototype/explore --section friends
go run ./prototype/explore --section open-invite
go run ./prototype/explore --section followed
```

Client-side date facets recovered from the mobile app:

```bash
go run ./prototype/explore --date-filter today
# anytime | today | tomorrow | this-week | this-weekend
```

The standalone logic demo is `demo.html`. Open it directly in a browser. It contains sanitized in-memory examples only and makes no network requests.

## Verdict

**Yes.** The current authenticated API can support a useful Seattle Explore command.

Verified on 2026-09-05:

- Seattle is represented as `{type:"DYNAMIC",countryCode:"US",state:"Washington",city:"Seattle"}`.
- `POST /getDiscoverFeedV2` returns Seattle events and cursor paging.
- A two-page probe returned 10 unique events and a non-empty continuation cursor.
- Reading the complete current feed returned 17 events across two pages.
- `POST /getDiscoverEventItemDecorators` returns `mutualGuests[]` with display names and RSVP statuses for the authenticated user.
- The two-page sample contained six mutual previews across four events.
- `POST /getDiscoverSectionsV2` returns four populated Seattle discovery modes:
  - `area-dynamic-trending-events`: Trending in Seattle
  - `area-mutual-events`: Find your Friends
  - `area-open-invite`: Open Invite
  - `area-followed-events`: From Orgs you Follow
- Direct prototype counts were 8 trending events, 12 friends events, 9 open-invite events, and 4 followed-org events.
- The friends section contained 21 mutual previews across 9 events.

## Filtering findings

There are three distinct concepts. They should not be collapsed into one flag:

1. **Discovery mode** is server-backed through generated sections. The prototype exposes `--section`.
2. **Date facet** is client-side. The mobile app fetches events, converts start/end times into each event's timezone, and keeps events whose local calendar-day range overlaps the selected range.
3. **Content category** uses `tagId` in the feed request. Current mobile constants include `ARTS`, `COMMUNITY`, `FITNESS`, `FILM`, `FOOD`, and `MUSIC`, but Seattle currently returns no tag catalog and all 17 sampled feed items had `tags: []`. Every tested category returned zero Seattle events. Category filtering exists in the client/backend model, but useful Seattle category data is not established.

Date facets match the mobile behavior:

- `today`: today through today;
- `tomorrow`: tomorrow through tomorrow;
- `this-week`: later of today or Monday through Sunday;
- `this-weekend`: later of today or Saturday through Sunday;
- `anytime`: no date filter.

On 2026-09-05, the earliest Seattle Explore event started on 2026-09-09 UTC. Therefore, the complete 17-event sample correctly returned zero for today, tomorrow, this week, and this weekend.

## Current request contracts

### Feed

```json
{
  "data": {
    "params": {
      "area": {
        "type": "DYNAMIC",
        "countryCode": "US",
        "state": "Washington",
        "city": "Seattle"
      },
      "tagId": "DISCOVER_HOME",
      "allowedFeedPresentationStyles": ["rows"]
    },
    "paging": {
      "maxResults": 20,
      "cursor": "<optional opaque cursor>"
    }
  }
}
```

### Sections

```json
{
  "data": {
    "params": {
      "area": {
        "type": "DYNAMIC",
        "countryCode": "US",
        "state": "Washington",
        "city": "Seattle"
      },
      "tagId": "DISCOVER_HOME",
      "allowedSectionPresentationStyles": [
        "carousel-small",
        "carousel-medium",
        "carousel-large"
      ],
      "locale": "en"
    },
    "paging": {"maxResults": 10, "cursor": "1"},
    "userId": "<private, never output>"
  }
}
```

### Mutual decorators

```json
{
  "data": {
    "params": {"eventIds": ["<private request values>"]},
    "userId": "<private, never output>"
  }
}
```

The normalized prototype emits event IDs and public event URLs, but strips owner IDs, guest IDs, user IDs, and other private identifiers. Mutual display names and statuses are included because they are the authenticated feature being prototyped. No live response or name fixture is committed.

## Evidence and limitations

- Authenticated web capture proved the current decorator request and response shape.
- The web Explore route is still a limited public landing page and does not expose Seattle feed traffic.
- Mobile evidence came from Partiful Android 3.8.28, downloaded through APKMirror's browser-bound verified flow.
- Downloaded APKMirror bundle SHA-256: `4465742057684c97997352e33c6abecd285b1c870b4f80594fcc58e7f4892ba4`.
- Extracted base APK SHA-256: `c0bf544634719dff8ee870e886dceabd1260e5f9163f8f00596eb7fc3532a9ea`.
- `hermes-dec` 0.1.7 produced pseudocode for bytecode version 98 with many unknown-register warnings. Endpoint names, request object keys, and straightforward date-filter control flow were corroborated through successful live read-only calls. The decompiled output itself is not committed or treated as authoritative executable source.
- The APK, bytecode, decompiler output, credentials, raw API bodies, real names, and real identifiers remain outside the repository.
- The API does not establish whether `mutualGuests[]` is complete. The prototype reports completeness as unknown.
- This prototype does not change `spec/partiful.openapi.json`, `spec/partiful.api-evidence.json`, `docs/CLI-PRODUCT-CONTRACT.md`, or the production Go command catalog.
