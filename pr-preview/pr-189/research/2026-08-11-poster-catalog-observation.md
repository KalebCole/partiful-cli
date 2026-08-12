# Poster catalog observation

## Scope and provenance

On 2026-08-11 at `01:08:30Z`, an unauthenticated, read-only request was made
to the contract's public endpoint:

`GET https://assets.getpartiful.com/posters.json`

No credentials, mutations, uploads, personal data, or third-party services
were used. The response body was inspected locally and then deleted. A raw
fixture is intentionally not committed: aggregate shape evidence is sufficient
and avoids retaining a 1.1 MB copy of 2,114 public creative records.

## Direct observations

The unconditional request returned HTTP `200`, `Content-Type:
application/json`, no `Content-Range`, and a 1,125,932-byte JSON array with
2,114 entries. Its SHA-256 was
`35e22005b19dd5795cecf582dee4c4fe4ddc5349e3142f0aae8014f4e471cc6e`.
The response included:

- `ETag: W/"9dbafb9aedef91b93a1b94e8969ae8b5"`
- `x-goog-generation: 1786410042765282`
- `Last-Modified: Tue, 11 Aug 2026 01:00:42 GMT`
- `Cache-Control: public, max-age=600, s-maxage=600,
  stale-while-revalidate=86400, stale-if-error=86400`

Every entry had `id`, `name`, `url`, `contentType`, `width`, `height`, `tags`,
and `categories`. IDs and names were non-empty strings; URLs were HTTPS
strings; tags and categories were arrays containing only strings. A repeated
privacy-safe GET at `01:42:58Z` returned the same byte length and SHA-256;
every `contentType` was a string and the complete observed value set was
`image/avif`, `image/gif`, `image/jpeg`, and `image/png`. Width and height were
integers except that one entry used `null` for both. One ID occurred twice at
non-adjacent positions; the two entries differed in tags and categories. No
uniqueness or deduplication claim is supported.

An `If-None-Match` request using the observed ETag returned `304` with no body.
A deliberately nonmatching `If-Match` request still returned `200` with the
full body, so safe resumption must not depend on that precondition being
enforced. A read-only request with the unsatisfiable range
`Range: bytes=999999999-` returned `416`, `Content-Range: bytes */1125932`,
and a zero-byte body.

## Reviewed contract conclusions

The catalog's only accepted success is HTTP `200` with the documented complete
array representation. The schema-free default response remains an explicit
unknown. The observed `416` resulted only from a Range request that the
implementation will not send, so it does not establish ordinary failure
mapping. A received non-`200` status is unrecognized by this contract revision
and must fail closed as `contract.protocol_changed`. A no-response or network
failure may map to `remote.unavailable` as a local transport fact. No
`remote.rate_limited` mapping is claimed. A `200` body that violates the
released schema is also `contract.protocol_changed`.

The product mapping is direct and total for its documented output:
`id` becomes `posterId`; `name`, `url`, `contentType`, `width`, `height`,
`tags`, and `categories` retain their wire names. The observation supplies
every source field on every entry, including the one entry whose dimensions
are explicitly `null`; no alternate identifier or fallback value is inferred.

The observed response has no remote page envelope or cursor. Bounded local
pagination is safe by fully materializing the array, preserving order and
duplicates, and binding an opaque cursor to the payload SHA-256, normalized
filters, and next array offset. A later payload digest mismatch must reject
resumption rather than skip, duplicate, or guess items. This is an
implementation requirement inferred from the complete observed representation,
not a claim that the server provides pagination.

## Explicit unknowns

- Whether this endpoint contains every poster usable anywhere in Partiful.
- How often catalog membership or order changes.
- The semantics of the duplicate ID and nullable dimensions.
- Status codes and bodies for failures of an ordinary unconditional request.
- Whether cache validators or storage-generation headers retain their current
  behavior.

These unknowns do not require a runtime fallback. The implementation remains
gated until this evidence and contract revision receive owner approval and are
merged. Under the fail-closed boundary above, S1 can close without guessing:
the implementation accepts only the observed `200` representation, maps only
local no-response/network failures to `remote.unavailable`, and treats every
received unrecognized status or malformed success body as a protocol change.
