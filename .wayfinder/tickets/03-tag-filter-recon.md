<!-- wayfinder:research -->
# TAG-FILTER: is category/tag filtering server-side or client-side?

Labels: wayfinder:research
Blocked by: (none — frontier)
Assignee: (unclaimed)
Status: closed

## Question

The region feed returns a `tags` array (DISCOVER_HOME, MUSIC, COMMUNITY, ARTS,
FITNESS, FOOD, + neighborhood tags). In recon, `?tag=MUSIC` and `?tagId=MUSIC`
did NOT change `selectedTagId` (stayed DISCOVER_HOME) or the feed — filtering
appears client-side or uses an unknown param.

Determine:
- Grep the `explore/[region].js` Next chunk for how a tag click changes the feed
  (does it re-fetch with a param, or filter `feedItems` in-memory?).
- If server-side: the exact param name + value format.
- If client-side: confirm the CLI must filter `feedItems` locally by each item's
  `tags` field.

Output: answer states server-side (with param) vs client-side (filter locally).
Resolves the `--tag` behavior fog item. If this proves expensive, v1 may ship
region+trending only and defer `--tag` — flag that in the answer.

---

## Resolution (closed — answered by BUILD-ID recon)

**Server-side.** The stable `getDiscoverFeed` / `getDiscoverSections` endpoints take a
`tagId` param that filters server-side. Verified on region=NYC:
DISCOVER_HOME=20 items, MUSIC=15, FOOD=4.

CLI `--tag` maps directly to `tagId`. Valid values come from the `tags[]` array in
`getDiscoverSections` (DISCOVER_HOME, MUSIC, COMMUNITY, ARTS, FITNESS, FOOD, plus
region-specific neighborhood tags like NYC_BROOKLYN). No client-side filtering needed.

See ticket 01 (BUILD-ID) resolution for full endpoint contract.
