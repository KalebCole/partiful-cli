# Explore V1 contract evidence

**Date:** 2026-09-05
**Contract revision:** `2026-09-05.1`
**Status:** Owner-reviewed promotion for issue #102

## Scope and privacy

This promotion consumes the sanitized issue #206 mobile-callable research and
throwaway Explore prototype. It promotes only `getDiscoverFeedV2`,
`getDiscoverSectionsV2`, and `getDiscoverEventItemDecorators`. The underlying
read-only Seattle checks retained request/response paths, types, bounds, and
aggregate results only. No raw response body, credential, private user/owner/
guest identifier, or real display name is committed.

## getDiscoverFeedV2

The reviewed request is `data.params.area` with exact `type: "DYNAMIC"` and
explicit `countryCode`, `state`, and `city`; `data.params.tagId:
"DISCOVER_HOME"`; `allowedFeedPresentationStyles:["rows"]`; and
`data.paging.maxResults` with optional opaque `cursor`. The observed success
is HTTP `200` JSON at `result.data.items` and `result.paging.nextCursor`.

A complete observed Seattle feed had 17 unique events across two pages. Feed
items establish a non-empty event ID and reviewed optional event title, dates,
timezone, location presentation, Maps URL, nonnegative counts, and tags.

## getDiscoverSectionsV2

The reviewed request uses the same dynamic area and `DISCOVER_HOME`,
`allowedSectionPresentationStyles:["carousel-small","carousel-medium",
"carousel-large"]`, `locale:"en"`, paging `{maxResults:10,cursor:"1"}`, and
the authenticated private user ID required by the observed shape. The observed
HTTP `200` JSON success is `result.data.sections`.

The reviewed generated IDs are `area-dynamic-trending-events`,
`area-mutual-events`, `area-open-invite`, and `area-followed-events`; public
product values remain Trending, Friends, Open Invite, and Followed. Missing or
empty selected sections are successful empty values.

## getDiscoverEventItemDecorators

The reviewed request is `data.params.eventIds` (a bounded event-ID list) plus
the authenticated private user ID. The observed HTTP `200` JSON success is
`result.data.decoratorsByEventId`. Decorators establish `mutualGuests` preview
entries with a display-name source and a closed RSVP status. The preview is not
proven complete; product output must report `mutualsPreviewComplete: null`.
No-mutual and empty decorator maps are successful values.

## Boundaries

Only HTTP `200` JSON success envelopes are promoted. Any other received status,
non-JSON body, malformed reviewed envelope/field, invalid closed status, or
item/response bound breach is protocol drift. No-response behavior,
authorization variation, error bodies, cursor lifetime, ordering, unknown event
fields, and mutual-preview completeness remain explicit unknowns. Map pins,
date-range reads, tag catalog, search, Next.js fallbacks, discovery writes, and
Explore RSVP/interest aliases are excluded.
