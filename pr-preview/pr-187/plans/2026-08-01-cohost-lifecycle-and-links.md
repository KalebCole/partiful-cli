# Cohost Lifecycle and Invite Links Implementation Plan

> **For Hermes:** Use strict red-green-refactor for every behavior-changing task.

**Goal:** Replace false-success Firestore writes with Partiful's canonical cohost request lifecycle, add invite-link lifecycle commands, and expose accurate state through CLI/schema/skill docs.

**Architecture:** Put canonical cohost operations and state normalization in `src/lib/cohosts.ts`; keep Commander handlers in `src/commands/cohosts.ts` thin. Direct add calls Firebase callable `createCohostRequest`; remove routes to `deleteCohostRequest` or `removeCohost` based on request state. Link inspection reads `events/{eventId}/private/cohostSecret`, while enable/disable call `generateEventCohostLink` and `revokeEventCohostLink`. Never PATCH `cohostIds` for normal lifecycle transitions. Legacy `cohostIds`-only corruption is the narrow exception: clear the stale ID first because both canonical add/remove endpoints return `INTERNAL`, then create the canonical request.

**Tech Stack:** TypeScript, Commander, Firebase callable HTTP, Firestore REST, Zod, Vitest.

---

## Canonical API discovery

Production Next.js bundle and live reversible probes establish:

| Operation | Canonical endpoint/state | Params/result |
|---|---|---|
| Invite direct contact | `POST /createCohostRequest` | `params: { eventId, targetUserId }` |
| Accept/decline invite | `POST /updateCohostRequestStatus` | `params: { eventId, status }`, status `ACCEPTED` or `DECLINED` |
| Delete pending request | `POST /deleteCohostRequest` | `params: { eventId, targetUserId }` |
| Remove accepted cohost | `POST /removeCohost` | `params: { eventId, targetUserId }` |
| List lifecycle state | Firestore `events/{eventId}/cohostRequests` | docs keyed by cohost user ID; `status` is `PENDING`, `ACCEPTED`, or `DECLINED` |
| Inspect invite link | Firestore `events/{eventId}/private/cohostSecret` | fields: `path`, `createdAt`, `createdBy`; absent means disabled |
| Enable/rotate link | `POST /generateEventCohostLink` | `params: { eventId }`; result contains `path` |
| Disable link | `POST /revokeEventCohostLink` | `params: { eventId }` |
| Accept link | Event URL query | server-generated `/e/{eventId}?accept-cohost={uuid}`; UI validates secret and calls acceptance lifecycle |

Live probe generated a real link, verified the private Firestore document and field shape, then revoked it. Post-revoke state returned to disabled.

## Behavioral contract

1. `cohosts add` resolves every requested name. Exact matching wins; duplicate exact or ambiguous partial matches fail closed with candidates; misses fail the command.
2. Direct add always calls `createCohostRequest`. For legacy `cohostIds`-only state, repair first removes the corrupt raw ID, then calls the canonical endpoint. Live verification showed calling either canonical endpoint before cleanup returns `INTERNAL`.
3. Existing `PENDING` or `ACCEPTED` request is a no-op. `DECLINED` is re-invited through `createCohostRequest`.
4. `cohosts list` merges request docs with legacy `cohostIds`. Request docs report lowercase `pending`, `accepted`, or `declined`; IDs only in `cohostIds` report `stale`.
5. `cohosts remove` uses `deleteCohostRequest` for pending/declined and `removeCohost` for accepted membership. Stale membership is removed through the explicit legacy-repair Firestore path because the canonical endpoint cannot process it.
6. `cohosts link <eventId>` inspects without mutation. `--enable` returns an existing URL as a no-op or generates one. `--disable` revokes an existing link or no-ops when absent. `--enable` and `--disable` are mutually exclusive.
7. Global `--dry-run` may perform authenticated reads/resolution but no writes. Output states intended endpoint/action.
8. Link output returns the URL only; delivery stays with approved messaging tools.

### Task 1: Add typed API specs

**Files:**
- Modify: `src/lib/api/endpoints.ts`
- Modify: `tests/api-spec.test.js`

**RED:** Add assertions that endpoint registry exposes the five lifecycle callables with exact request params.

**Verify RED:** `npm test -- --run tests/api-spec.test.js` fails because methods are absent.

**GREEN:** Add request interfaces, permissive response schemas, endpoint metadata, and response-schema registry entries for:
- `createCohostRequest(eventId, targetUserId)`
- `deleteCohostRequest(eventId, targetUserId)`
- `removeCohost(eventId, targetUserId)`
- `generateEventCohostLink(eventId)`
- `revokeEventCohostLink(eventId)`

**Verify GREEN:** targeted test and `npm run typecheck` pass.

### Task 2: Make contact resolution fail closed

**Files:**
- Create: `tests/cohosts.test.js`
- Modify: `src/lib/cohosts.ts`

**RED:** Test exact match, unique partial match, ambiguous partial candidates, duplicate exact names, unresolved names, and deduped input.

**Verify RED:** targeted tests fail against current first-substring/warn-and-skip behavior.

**GREEN:** Introduce typed `CohostResolutionError` or `PartifulError` details with query/candidates. Return resolved IDs only when all requested names resolve uniquely. Keep contact fetching injectable or split pure `resolveContactNames(names, contacts)` from transport.

**Verify GREEN:** targeted tests and typecheck pass.

### Task 3: Normalize request and stale state

**Files:**
- Modify: `tests/cohosts.test.js`
- Modify: `src/lib/cohosts.ts`
- Modify: `src/lib/http.ts`

**RED:** Test Firestore typed-value parsing for request docs, accepted/pending/declined normalization, merge with `cohostIds`, and `stale` classification.

**Verify RED:** tests fail because request-state readers do not exist.

**GREEN:** Add generic authenticated Firestore document GET helper and cohost request listing. Add pure merge function. Preserve unknown server fields but only emit stable CLI fields.

**Verify GREEN:** targeted tests and typecheck pass.

### Task 4: Implement direct invite orchestration

**Files:**
- Modify: `tests/cohosts.test.js`
- Modify: `src/lib/cohosts.ts`
- Modify: `src/commands/cohosts.ts`

**RED:** With injected transport, test:
- new target calls `/createCohostRequest`;
- stale `cohostIds`-only target still calls endpoint;
- pending and accepted requests no-op;
- declined request re-invites;
- multiple targets produce per-target outcomes;
- dry-run performs no mutation;
- partial failure is surfaced, never reported as blanket success.

**Verify RED:** tests fail because command still PATCHes `cohostIds`.

**GREEN:** Add `inviteCohost`/`inviteCohosts` orchestration and switch command from `setCohostIds` to the callable. Remove `setCohostIds` from command paths. Return `invited`, `pending`, `accepted`, `reinvited`, and `stale_repair` outcomes.

**Verify GREEN:** targeted tests, integration dry-runs, and typecheck pass.

### Task 5: Implement canonical remove routing

**Files:**
- Modify: `tests/cohosts.test.js`
- Modify: `src/lib/cohosts.ts`
- Modify: `src/commands/cohosts.ts`

**RED:** Test pending/declined route to `/deleteCohostRequest`, accepted/stale route to `/removeCohost`, missing target fails not-found, and dry-run is read-only.

**Verify RED:** tests fail because current command PATCHes `cohostIds`.

**GREEN:** Add removal planner/executor and update handler/output.

**Verify GREEN:** targeted tests and full suite pass.

### Task 6: Implement link inspect/enable/disable

**Files:**
- Modify: `tests/cohosts.test.js`
- Modify: `src/lib/cohosts.ts`
- Modify: `src/commands/cohosts.ts`

**RED:** Test missing document = disabled, path conversion to absolute URL, inspect read-only, enable existing = no-op, enable absent calls generation, disable existing calls revoke, disable absent = no-op, conflicting flags fail, and dry-run returns planned action without mutation.

**Verify RED:** tests fail because `cohosts link` does not exist.

**GREEN:** Add `getCohostLink`, `generateCohostLink`, `revokeCohostLink`, path validation, and Commander subcommand. Never synthesize token paths.

**Verify GREEN:** targeted tests, CLI `--help`, schema tests, and typecheck pass.

### Task 7: Route event create/update cohosts canonically

**Files:**
- Modify: `tests/events-integration.test.js`
- Modify: `src/commands/events.ts`
- Modify: `src/lib/cohosts.ts`

**RED:** Test create dry-run reports empty `createEvent.cohostIds` plus planned post-create requests; update dry-run reports request actions rather than a raw Firestore `cohostIds` update.

**Verify RED:** existing output uses raw IDs/writes.

**GREEN:** Create event first without directly assigning cohosts, then issue canonical requests after receiving event ID. Route update `--cohost` through same inviter. If event creation succeeds but any invite fails, return explicit partial-success details including event URL and failed target.

**Verify GREEN:** targeted event tests and full suite pass.

### Task 8: Update schema, help, and skill routing

**Files:**
- Modify: `src/commands/schema.ts`
- Modify: `tests/events-integration.test.js`
- Modify: `skills/partiful/SKILL.md`
- Modify: `skills/partiful/references/guests-invitations-and-cohosts.md`
- Modify: event-management reference(s) discovered in `skills/partiful/references/`

**RED:** Add schema assertions for changed `cohosts.add/remove` and new `cohosts.link`, plus new API methods.

**Verify RED:** tests fail on absent schema surfaces.

**GREEN:** Document direct invitation as approval-required external action; link creation as reversible state change; link sending as a separate approval-gated messaging action. Route cohost asks from both event-management and guest/invitation sections. Update create/update examples.

**Verify GREEN:** schema tests and skill validator pass.

### Task 9: Full automated verification

Run:

```bash
npm test
npm run typecheck
npm run build
npm run lint --if-present
```

Expected: all commands exit 0 with pristine output. Also run CLI smoke checks for `cohosts --help`, `cohosts link --help`, command schemas, and API schemas.

### Task 10: Live end-to-end verification

1. Inspect initial request/link state on a disposable event.
2. Run direct add to the approved second account and verify a `PENDING` request doc plus recipient invitation.
3. Accept from second-account UI; verify request becomes `ACCEPTED`, ID appears in event host state, and host controls are visible.
4. Remove through CLI; verify host controls disappear and lifecycle state is removed.
5. Enable link; verify returned URL matches stored private `path` without exposing it in logs beyond intended CLI output.
6. Open/accept with second account; verify accepted host state.
7. Disable link; verify secret document absent.
8. Restore disposable event/account state.

Do not send links or invitations to a third party without explicit authorization.

### Task 11: Adversarial review and remediation

Dispatch an independent adversarial reviewer against issue #74, this plan, and the branch diff. Require verdict, severity-ranked findings, missing tests, security/privacy concerns, and exact evidence. Reproduce every credible finding, add a failing regression test, fix, and rerun all gates. Repeat review if material changes result.

### Task 12: Commit and open PR

1. Review diff and ensure no captured secrets/tokens/build artifacts.
2. Commit coherent changes with issue reference.
3. Push `fix/cohost-lifecycle-links`.
4. Open PR with problem, canonical lifecycle evidence, test matrix, live verification evidence, and `Fixes #74`.
5. Verify CI checks and review-bot surfaces; fix failures before reporting completion.
