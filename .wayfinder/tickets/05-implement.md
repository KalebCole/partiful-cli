<!-- wayfinder:task -->
# IMPLEMENT: build the explore command + discover lib

Labels: wayfinder:task
Blocked by: COMMAND-SHAPE
Assignee: (unclaimed)
Status: open

## Question

Implement the decided command surface. Not a decision ticket — this is the DO
step the map carries to (execution-carrying map).

Work:
- New `src/lib/explore.js` (or extend `src/lib/events.js`): build-id resolution
  per BUILD-ID, region feed fetch, trending fetch, RSVP + interested writes per
  RSVP-ENDPOINT. All HTTP through `src/lib/http.js`.
- New `src/commands/explore.js` wired into the CLI entry, matching the
  COMMAND-SHAPE stub. Register in whatever the main command index is.
- `partiful schema explore.*` introspection support if the CLI auto-derives it;
  otherwise add.
- Structured errors `{status, error:{code,type,message}}`; exit codes 0-5.
- Vitest unit tests (mock HTTP) for feed parse + RSVP body build. Integration
  test behind auth for one live region fetch.
- Reinstall to activate: `npm install -g .` (COPY-not-symlink trap).
- Verify end to end: `partiful explore --region nyc --format table` returns
  events; a dry-run RSVP builds the right body.

Output: working command, tests green, linked commit/branch.
