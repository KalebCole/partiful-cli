<!-- wayfinder:research -->
# BUILD-ID: how to resolve the rotating Next.js build id

Labels: wayfinder:research
Blocked by: (none — frontier)
Assignee: hermes
Status: closed

## Question

The discovery data endpoints are `/_next/data/{BUILD}/explore.json` and
`/_next/data/{BUILD}/explore/{region}.json?region={slug}`, where `{BUILD}` is the
Next.js `buildId` that rotates on every Partiful deploy (observed:
`A1rxlYfFYHBWL3Uop4ELL`). A hardcoded build id will 404 after the next deploy.

Determine the resolution strategy:
- Can the build id be scraped reliably from the `/explore` HTML
  (`__NEXT_DATA__.buildId` or `/_next/static/{buildId}/_buildManifest.js`)?
- Is there a stable `api.partiful.com` endpoint that returns the same trending
  data WITHOUT a build id (recon saw `getDiscoverEventItemDecorators`; is there a
  `getDiscoverFeed` / `getTrendingEvents` sibling)? Probe common names.
- Fallback ordering and failure behavior.

Output: markdown summary in this ticket's answer — chosen strategy + the exact
request(s), with status codes. Feeds the caching decision (Not yet specified).

---

## Resolution (closed)

**Strategy chosen: hit the stable `api.partiful.com` endpoints directly. NO build id needed.**

The `/_next/data/{BUILD}/...` path works but requires the rotating buildId. The web app's own data layer calls two stable Firebase-callable endpoints that the CLI can use directly through the existing `src/lib/http.js` authed client. The build id is fully avoidable.

### Endpoints (POST, Bearer auth required, Firebase-callable `{data:{...}}` envelope)

**`POST https://api.partiful.com/getDiscoverFeed`** — the paginated event feed.
```json
{"data":{"params":{"region":"NYC","tagId":"DISCOVER_HOME","allowedFeedPresentationStyles":["rows"]},"paging":{"maxResults":100}}}
```
Response: `result.data.items[]` (each `{id,type,event:{...}}`), `result.paging.nextCursor`.

**`POST https://api.partiful.com/getDiscoverSections`** — trending carousels + tag list.
```json
{"data":{"params":{"region":"NYC","tagId":"DISCOVER_HOME","allowedSectionPresentationStyles":["carousel-small","rows"],"locale":"en"},"paging":{"maxResults":100}}}
```
Response: `result.data.sections[]` (trending carousels), `result.data.tags[]` (category list).

Also exists: `getDiscoverSection` (singular, `{params}` only) for one section.

### Key facts
- **Envelope is `{"data":{...}}`** (Firebase callable convention). A bare `{params,paging}` returns 400; `{data:...}` returns 401/200. This is why raw recon 400'd.
- **Auth IS required** — 401 without a valid Bearer. Reuse `src/lib/http.js` (it already sends the token + refreshes). The 400s during recon were an unauthed/expired token, not a bad endpoint.
- **Token refresh:** CLI auto-refreshes on any authed call; a stale `auth.json` token 401s until refreshed.
- **Region values:** `NYC, LA, SF, BOS, DC, CHI, LON, MIA, ATX` (uppercase in API params; lowercase slugs `nyc/la/...` only for the `/_next/data` web-page path).
- **Pagination = cursor.** `result.paging.nextCursor` → pass back as `paging.cursor` (or `paging.afterCursor`; confirm exact key in IMPLEMENT). `pageResultCount` also returned.

### Bonus — resolves TAG-FILTER ticket
`tagId` filters **server-side**: NYC DISCOVER_HOME=20, MUSIC=15, FOOD=4 items. `--tag` maps directly to the `tagId` param. No client-side filtering needed. TAG-FILTER can be closed as answered-by-BUILD-ID.

### Fallback
If these endpoints ever change, the `/_next/data/{buildId}/explore/{slug}.json?region={slug}` path still works; scrape `buildId` from `__NEXT_DATA__` on the `/explore` HTML (verified present).
