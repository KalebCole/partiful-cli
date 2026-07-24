# T4 — Port src/commands/ + src/helpers/

**Type:** task (AFK) · **Blocks on:** T3 · **Status:** OPEN

## Question

Port the command + helper layer to TS. Mostly mechanical once T3 exists — these files consume the
lib-layer types rather than defining new API shapes.

## Files

commands/: auth, blasts, bulk, cohosts, contacts, doctor, events, guests, posters, rsvp, setup,
templates, (schema → handled in T5, but its .js→.ts shell can happen here)
helpers/: clone, export, share, watch
plus src/cli.js

## Notes

- Type Commander actions, options objects (plain interfaces), and wire them to lib-layer types.
- No new API types here — if you find yourself authoring an endpoint shape, it belonged in T3; go
  back and add it there.
- events.js is the big one (524 LOC) — expect the most work.

## Done when

All commands/ + helpers/ + cli.js → `.ts`, strict-clean, all tests green, `allowJs` can be turned
off (no JS left except tests if those stay JS).

## Answer

<!-- record on close -->
