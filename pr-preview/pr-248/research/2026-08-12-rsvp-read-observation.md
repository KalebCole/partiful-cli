# RSVP read observation

## Scope and provenance

On August 12, 2026, the repository owner authorized authenticated, read-only
RSVP evidence capture. The supplied sanitized artifact is
`spec/research/rsvp-read-evidence-redacted-20260812.json`, captured at
`2026-08-12T13:49:55.431Z`. The agent did not use credentials, make a live
request, or handle raw response values. The raw private capture was deleted
before this contract work.

The artifact contains only counts, field names, value types, bounded numeric
ranges, safe enum values, and HTTP statuses. It contains no event, guest, user,
or account identifiers; names; messages; questionnaire answers; credentials;
tokens; phone numbers; or email addresses. This observation proposes contract
revision `2026-08-12.3`; it does not approve that revision.

## Event coverage

The read found 36 upcoming and 294 past list events, with 330 unique event
details. All 330 `getEventInfo` calls returned HTTP `200`. The detail artifact
records 90 field names and their presence counts, but it retains values only
for the allowlisted RSVP safeguard fields.

Forty-one list events had no inline guest and 289 had an inline guest. One
candidate from each class was used for the current-guest variant checks. Those
two probes do not establish the frequency of either callable variant.

## Current-guest variants

One `getCurrentGuest` call returned HTTP `200` with explicit null at
`result.data.currentGuest`. This is authoritative evidence for the
no-current-guest marker. The property itself was present.

One other call returned HTTP `200` with an object. The sanitized object had
string `id`, `name`, `status`, and `userId`; number `count` and
`plusOneCount`; null `anchorGuestId`, `plusOnes`, and `rsvpDate`; object
`invitedBy`; and array `rsvpHistory`. Only ID, status, and count are needed by
the S5 plan. Their values were not retained.

The explicit null and selected object are the only observed callable variants.
A missing `currentGuest` property, scalar, array, object without a valid ID or
status, non-number count, non-null plus-one variant, unsupported status, and
failure response remain unknown.

Current public asset module `52105`, cited in
`docs/research/2026-08-12-rsvp-mapping-public-assets.md#addguest-request`,
adds `guestId` for an existing guest and omits it for no guest. Combined with
the dated null and object observations, the narrow selection is exact:

- explicit `currentGuest: null` selects create and omits `guestId`;
- an object with valid ID and status selects update and includes its private
  ID; the mutation-compatible subset also requires nonnegative integer count;
  and
- every other variant fails closed.

## Event safeguard observations

The table keeps raw property presence separate from the artifact's normalized
null buckets.

| Field | Raw presence | Observed retained variants |
| --- | ---: | --- |
| `rsvpsEnabled` | 330 | 325 true, 5 false |
| `atCapacity` | 330 | 315 false, 15 true |
| `plusOneNamesRequired` | 330 | 317 false, 13 true |
| `questionnaireEnabled` | 93 | 47 true, 46 false, 237 absent |
| `questionnaireVersions` | 330 | 89 arrays and 241 null; array lengths 1 (44), 2 (39), or 3 (6) |
| `ticketing` | 101 | 91 objects, 10 explicit null, 229 absent |
| `guestAction` | 22 | 18 `APPLY`, 4 `RSVP`, 308 absent |
| `maxCountPerGuest` | 36 | 36 numbers from 1 through 10, 294 absent |
| `maxCapacity` | 68 | 52 numbers from 8 through 300, 16 explicit null, 262 absent |
| `remainingCapacity` | 52 | 52 numbers from -9 through 116, 278 absent |
| `enableWaitlist` | 78 | 33 true, 31 false, 14 explicit null, 252 absent |
| `password` | 0 | 330 absent |
| `passwordProtected` | 0 | 330 absent |

The normalized artifact uses null for an absent optional value in several
aggregate groups. The raw presence list is authoritative when absence and
explicit null differ. A mutation safeguard snapshot must retain that
distinction instead of converting every absence to JSON null.

## Narrow compatibility policy

The dated evidence establishes field presence and variants, not server
enforcement. The following rules are conservative product exclusions:

- require `rsvpsEnabled: true`;
- reject `guestAction: "APPLY"` because the approved request mapping has no
  application flow;
- require absent or null `ticketing`; require absent `password` and
  `passwordProtected` because no present password variant was observed and the
  approved request mapping has no ticket purchase or password input;
- reject `atCapacity: true` for going because no reviewed waitlist request
  mapping exists, including when `enableWaitlist` is true;
- enforce a numeric `maxCountPerGuest` against party size when present;
- reject a numeric `maxCapacity` without numeric `remainingCapacity`;
- enforce numeric `remainingCapacity` against only the additional party count,
  using the bound current-guest count for an update; and
- continue to require named plus ones and exact party-size consistency.

These rules define the product's supported event class. They do not claim that
the server returns a particular error or stores a successful write.

Current public asset module `82565`, cited in
`docs/research/2026-08-12-rsvp-mapping-public-assets.md#addguest-request`,
sets questionnaire version to `questionnaireVersions.length - 1` for going
and skips the questionnaire for `DECLINED`. Module `7073` supports the named
plus-one request shape. The product therefore requires the latest version when
`questionnaireEnabled` is true, submits no questionnaire response otherwise,
and always omits it for not-going.

## Privacy boundary

Contract tests require the artifact's exact top-level shape, exact
current-guest type inventory, exact safeguard aggregates, and an allowlist of
all 90 retained event field names. Mutation tests reject unknown keys,
unallowlisted field names, and arbitrary values. Separate patterns reject
identity and credential keys, JWT-like strings, phone numbers, email
addresses, messages, display names, and questionnaire answers.

The public plan can show caller-supplied RSVP input. Its private stored record
binds the exact guest ID, account fingerprint, and request, but public plan
JSON must redact the guest ID and never contain account or user IDs.

## Remaining unknowns

No RSVP mutation response was observed. Server rejection status and body
mappings, stored RSVP state, delivery, notification behavior, waitlist,
ticketing, application, protected-event submission, inaccessible-event
responses, unobserved current-guest variants, and post-write reads remain
unknown. They are not inferred from the read-only evidence.
