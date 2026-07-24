# T0 — RSVP work merged to main, tree clean, port branch cut

**Type:** task (AFK) · **Blocks on:** nothing · **Status:** OPEN (external — RSVP agent in-flight)

## Question

The whole port frontier is locked behind this. Another agent is actively writing the RSVP
command into `main`'s working tree RIGHT NOW (untracked `src/commands/rsvp.js`, `src/lib/rsvp.js`,
4 test files; modified `src/cli.js`, `src/commands/schema.js`). Two agents cannot edit the same
tree. The port cannot start on a moving target, and the new RSVP API-layer files must themselves
be typed (they call addGuest/getCurrentGuest/markEventInterest — spec endpoints).

## Done when

1. RSVP work reaches a natural stopping point and is committed + merged to `main`.
2. `git status --short` on `main` is clean (no untracked/modified port-relevant files).
3. `npm test` green on `main`.
4. `feat/typescript-port` branch cut FROM that clean commit.

## Answer

<!-- record the merge commit SHA + branch-cut commit here on close -->
