# Authentication endpoint observation

## Scope and provenance

On 2026-08-11 at `02:30:19Z`, the repository owner completed an attended
authentication session against Partiful's live endpoints. The agent did not
enter credentials, send codes, use browser login, or access the owner's
tokens. A sanitized evidence artifact retaining only HTTP metadata and
JSON paths/types is committed at
`spec/research/auth-evidence-redacted-20260811.json`. An independent scan
confirmed no phone numbers, verification codes, tokens, API keys, or user IDs
are present in the artifact.

Additional privacy-safe negative probes were performed by the agent at
approximately `02:33Z` using clearly fake tokens, empty/invalid phone numbers,
and structurally malformed requests that could not contact a person, deliver a
message, or authenticate. These probes observed only error responses.

## Direct observations

### sendAuthCodeTrusted

Owner-attended success: `POST /sendAuthCodeTrusted` returned HTTP `200`,
`Content-Type: application/json; charset=utf-8`, 22-byte body. No structured
fields were present in the sanitized shape — the response is a minimal
acknowledgment.

Agent negative probe (invalid phone `"not-a-phone"`): HTTP `500`,
`Content-Type: application/json; charset=utf-8`, body
`{"error":{"message":"INTERNAL","status":"INTERNAL"}}`. This confirms the
callable error envelope shape but does not establish a contract-level failure
mapping since the request was deliberately malformed.

### getLoginToken

Owner-attended success: `POST /getLoginToken` returned HTTP `200`,
`Content-Type: application/json; charset=utf-8`, 849-byte body with shape:
- `result.data.token` (string)

The `result.data` wrapper is consistent with the March 24, 2026 browser
interception. Additional fields may exist (849 bytes vs. one recorded path);
they are not claimed.

Owner-attended wrong-code failure: HTTP `403`,
`Content-Type: application/json; charset=utf-8`, 127-byte body with shape:
- `error.details.authErrorCode` (string)
- `error.message` (string)
- `error.status` (string)

Agent negative probe (empty phone, code `"000000"`): HTTP `400`,
`Content-Type: application/json; charset=utf-8`, body with shape:
- `error.details.authErrorCode` (string)
- `error.message` (string)
- `error.status` (string)

The error envelope is consistent across both failure modes.

### signInWithCustomToken

Owner-attended success: `POST /v1/accounts:signInWithCustomToken` returned
HTTP `200`, `Content-Type: application/json; charset=UTF-8`, 1453-byte body
with shape:
- `expiresIn` (string)
- `idToken` (string)
- `kind` (string)
- `refreshToken` (string)

This is consistent with the March 24, 2026 browser interception and Firebase
Identity Toolkit documentation.

Agent negative probe (fake token, with `Referer: https://partiful.com/`):
HTTP `400`, `Content-Type: application/json; charset=UTF-8`, body with shape:
- `error.code` (number)
- `error.errors[].domain` (string)
- `error.errors[].message` (string)
- `error.errors[].reason` (string)
- `error.message` (string)

Message value: `"INVALID_CUSTOM_TOKEN : Invalid assertion format. 3 dot
separated segments required."` (no credential content).

Agent negative probe (fake token, no `Referer` header): HTTP `403`,
`Content-Type: application/json; charset=UTF-8`, body with shape:
- `error.code` (number)
- `error.details[].@type` (string)
- `error.errors[].domain` (string)
- `error.errors[].message` (string)
- `error.errors[].reason` (string)
- `error.message` (string)
- `error.status` (string)

The Firebase API key is configured with HTTP referrer restrictions. Requests
without an allowed `Referer` header receive `403
API_KEY_HTTP_REFERRER_BLOCKED`. This is a Firebase project configuration fact,
not a Partiful callable behavior.

### refreshToken

Owner-attended success: `POST /v1/token` returned HTTP `200`,
`Content-Type: application/json; charset=UTF-8`, 2632-byte body with shape:
- `access_token` (string)
- `expires_in` (string)
- `id_token` (string)
- `project_id` (string)
- `refresh_token` (string)
- `token_type` (string)
- `user_id` (string)

Owner-attended invalid-token failure: HTTP `400`,
`Content-Type: application/json; charset=UTF-8`, 111-byte body with shape:
- `error.code` (number)
- `error.message` (string)
- `error.status` (string)

Agent negative probe (fake token, with `Referer`): HTTP `400`,
`Content-Type: application/json; charset=UTF-8`, body with same shape:
- `error.code` (number)
- `error.message` (string)
- `error.status` (string)

Consistent error envelope across owner and agent probes.

### lookupFirebaseUser

Owner-attended success: `POST /v1/accounts:lookup` returned HTTP `200`,
`Content-Type: application/json; charset=UTF-8`, 765-byte body with shape:
- `kind` (string)
- `users[].createdAt` (string)
- `users[].customAuth` (boolean)
- `users[].displayName` (string)
- `users[].lastLoginAt` (string)
- `users[].lastRefreshAt` (string)
- `users[].localId` (string)
- `users[].phoneNumber` (string)
- `users[].photoUrl` (string)
- `users[].providerUserInfo[].phoneNumber` (string)
- `users[].providerUserInfo[].providerId` (string)
- `users[].providerUserInfo[].rawId` (string)
- `users[].validSince` (string)

Owner-attended invalid-token failure: HTTP `400`,
`Content-Type: application/json; charset=UTF-8`, 206-byte body with shape:
- `error.code` (number)
- `error.errors[].domain` (string)
- `error.errors[].message` (string)
- `error.errors[].reason` (string)
- `error.message` (string)

Agent negative probe (fake token, with `Referer`): HTTP `400`,
`Content-Type: application/json; charset=UTF-8`, body with identical shape.

## Firebase API key referrer restriction

The Firebase Identity Toolkit and Secure Token endpoints require an HTTP
`Referer` header matching an allowed pattern (observed: `https://partiful.com/`).
Requests without a matching referrer receive `403
PERMISSION_DENIED` with reason `API_KEY_HTTP_REFERRER_BLOCKED`. This is a
Firebase project configuration fact observed during agent negative probes and
does not affect Partiful callable endpoints (`sendAuthCodeTrusted`,
`getLoginToken`), which do not use API key authentication.

## Reviewed contract conclusions

Each operation's only accepted success is HTTP `200` with the documented
shape. The schema-free default response remains an explicit unknown. Error
responses were observed but are not promoted to contract-level status codes
because:
1. The error shapes differ across Firebase standard (`error.code` number) and
   Partiful callable (`error.message`/`error.status` strings) endpoints.
2. Failure status codes (400, 403, 500) are request-condition-dependent and
   the full failure space is not characterized.
3. The referrer restriction is a Firebase project configuration that may change.

A received non-`200` status is unrecognized by this contract revision and must
fail closed as `contract.protocol_changed`. A no-response or network failure
may map to `remote.unavailable` as a local transport fact. No
`remote.rate_limited` mapping is claimed. A `200` body that violates the
released schema is also `contract.protocol_changed`.

## lookupFirebaseUser S2 requirement

The `lookupFirebaseUser` operation provides the only observed way to confirm
account existence and retrieve display metadata after sign-in. However, the
CLI product contract's `auth login` output (`authenticated`, `tokenState`,
`expiresAt`) does not require account metadata — `expiresAt` can be derived
from `signInWithCustomToken`'s `expiresIn`. The userId is available from the
Firebase JWT's `localId`/`user_id` claim without a lookup call.

`lookupFirebaseUser` is included in this evidence revision because it was
observed and completes the authentication surface. Whether the S2
implementation requires it at login time is an implementation decision, not a
contract gate.

## Explicit unknowns

- Whether `sendAuthCodeTrusted` differs from `sendAuthCode` in behavior beyond
  endpoint name.
- The full set of `authErrorCode` values and their semantics.
- Whether the 22-byte `sendAuthCodeTrusted` success body ever contains fields.
- Rate-limiting behavior on any authentication endpoint.
- Token lifetimes, refresh token rotation policy, and session duration.
- Whether the Firebase referrer restriction applies to all allowed origins or
  only specific patterns.
- Additional fields in `getLoginToken` success (849 bytes, only `result.data.token`
  recorded in sanitized shape).
- Whether `lookupFirebaseUser` returns additional user fields for accounts with
  email or other providers.
