# Architecture

Partiful has no public API. This CLI uses the private calls made by its web
client; those calls can change without notice.

[`spec/partiful.openapi.json`](../spec/partiful.openapi.json) is the single
source of truth for every remote call: servers, paths, methods, parameters,
request bodies, responses, and authentication. Firestore REST calls are normal
paths tagged `firestore`. Use standard OpenAPI fields for transport facts;
`x-evidence` carries provenance that OpenAPI cannot express.

Each operation has a small `x-evidence` object with `status`, `date`, and a
`source` note. `observed` means supporting evidence was inspected, including
public assets or official protocols; the note distinguishes that from live
traffic. `explicit-unknown` marks unsupported behavior. Existing schema and
`default` response boundaries retain finer uncertainty. Research under
[`docs/research/`](research/) and `spec/research/` supplies historical context,
not another contract to update.

The native Go program lives in `cmd/partiful` and `internal/`. `internal/app`
handles commands and product safety; `internal/remote` handles wire requests;
`internal/auth` handles credential state and attended login. The command
definitions and Go tests define user-facing behavior.

## Checks against the spec

`go test ./...` checks the published remote revision against the spec, validates
operation evidence and local references, and captures outbound Go client calls
to match their methods, hosts, paths, and query/header requirements to OpenAPI.
The black-box tests at `app.Execute` check request payloads, response decoding,
product projections, failure handling, and mutation safeguards using fake HTTP.
These are focused contract checks, not a generated client or a full OpenAPI
schema validator. Unknown statuses or malformed required data fail closed as
`contract.protocol_changed`; mutations dispatch once without automatic retry.

`go vet ./...` and `./scripts/verify-native-release.sh` check static correctness
and native packaging. Release verification reruns the spec checks and smokes
only `--version`, `schema`, and `doctor`. Tests use no live credentials or events.

## Authentication

Human-attended `auth login` sends a code with `sendAuthCodeTrusted`, exchanges
it through `getLoginToken`, then uses Firebase `signInWithCustomToken` to obtain
an ID token and refresh token. Firebase Secure Token refreshes expiring tokens.
Firebase calls include the public web API key and required Referer from the
spec. Account lookup is documented but optional. Other authenticated calls use
a bearer ID token. Credential storage and redaction belong to `internal/auth`;
`doctor` checks local authentication state safely.

## Adding or correcting a call

Edit the spec once: add or update the normal operation, its wire schemas, and
its dated `x-evidence` note. Cite privacy-safe research and keep unsupported
responses explicit. Add a focused Go test for the call or correction, implement
the adapter as needed, and run the checks above. No separate evidence ledger,
proposal document, or owner sign-off ceremony is required. User-facing changes
also need to satisfy the product tests and reviewed schema snapshot.

## Why these product rules exist

Task-oriented commands and complete JSON results let people and agents compose
workflows without knowing private transport details. Stable errors let callers
choose a safe next step. Redacted previews and explicit destructive confirmation
make consequential changes reviewable; dispatching once avoids duplicate effects
when completion is uncertain. Failing closed on unreviewed responses protects
users from guesses about a changing private API. The old TypeScript CLI remains
research evidence, rather than a compatibility target.

`partiful schema [command.path]` describes the executable public surface.
`internal/app/product_rules_test.go` applies shared envelope, privacy, and failure
checks to command fixtures; `internal/app/testdata/schema.golden.json` records the
catalog and every command definition. `productContractRevision` remains a public
compatibility field identifying this executable product baseline and its goldens;
`remoteContractRevision` independently identifies the OpenAPI revision.

| Deliberately excluded feature | Rationale |
| --- | --- |
| Templates, clone | Add persistence or orchestration beyond reviewed remote behavior. |
| Export | JSON and shell redirection already compose. |
| Watch | Polling, identity stability, and delivery guarantees are unestablished. |
| Bulk mutation | Needs separately designed safety for multiple consequential effects. |
| Skill installation | Agent configuration belongs outside this CLI. |
| Image upload; raw HTTP, callable, Firebase, Firestore access | Unreviewed upload behavior and generic escape hatches bypass domain safeguards. |

New features need reviewed product tests and remote evidence; the existence of a
remote endpoint alone is insufficient. Tests can check evidence structure and
known fixtures, but people must assess evidence quality and product scope.
