/**
 * Real-API smoke tests (T6) — THE SPEC VERIFIER.
 *
 * These hit the LIVE Partiful API. When Partiful changes a response shape, one
 * of these fails BEFORE users do, and drift detection (src/lib/drift.ts) names
 * the offending fields. They are the runtime counterpart to the types-as-spec
 * in src/lib/api/endpoints.ts.
 *
 * GATED + SKIPPED BY DEFAULT. They require valid auth and network, so they are
 * OFF unless you explicitly opt in. Secrets never live in CI here — run these
 * manually (or in a secrets-provisioned job).
 *
 * HOW TO RUN:
 *   PARTIFUL_SMOKE=1 PARTIFUL_TOKEN=<real-jwt> npx vitest run tests/smoke-real-api.test.js
 *   # or with a refresh token the CLI can exchange:
 *   PARTIFUL_SMOKE=1 PARTIFUL_REFRESH_TOKEN=<...> npx vitest run tests/smoke-real-api.test.js
 *   # optional: exercise event-scoped reads against an event you can access
 *   PARTIFUL_SMOKE=1 PARTIFUL_TOKEN=<jwt> PARTIFUL_SMOKE_EVENT_ID=<id> npx vitest run ...
 *   # optional: surface unknown vendor fields while running
 *   PARTIFUL_DRIFT_LOG=1 PARTIFUL_SMOKE=1 PARTIFUL_TOKEN=<jwt> npx vitest run ...
 *
 * WHAT THEY VERIFY: read-only endpoints only (no event creation / mutation), so
 * they are safe to run repeatedly against a real account.
 */
import { describe, it, expect } from 'vitest';
import { run, runRaw } from './helpers.js';

const SMOKE = process.env.PARTIFUL_SMOKE === '1' || process.env.PARTIFUL_SMOKE === 'true';
const HAS_AUTH = Boolean(process.env.PARTIFUL_TOKEN || process.env.PARTIFUL_REFRESH_TOKEN);
const EVENT_ID = process.env.PARTIFUL_SMOKE_EVENT_ID;

// Real auth is passed straight through; do NOT override PARTIFUL_TOKEN with the
// fake one the unit-test helper injects.
const realEnv = {
  PARTIFUL_TOKEN: process.env.PARTIFUL_TOKEN,
  PARTIFUL_REFRESH_TOKEN: process.env.PARTIFUL_REFRESH_TOKEN,
  PARTIFUL_DRIFT_LOG: process.env.PARTIFUL_DRIFT_LOG,
};

describe.skipIf(!SMOKE)('real-API smoke (live Partiful)', () => {
  it('has auth configured', () => {
    expect(HAS_AUTH, 'set PARTIFUL_TOKEN or PARTIFUL_REFRESH_TOKEN to run smoke tests').toBe(true);
  });

  it('events list — getMyUpcomingEventsForHomePage returns a JSON envelope', () => {
    const out = run(['events', 'list'], { env: realEnv });
    expect(out.status).toBe('success');
    expect(Array.isArray(out.data) || Array.isArray(out.data?.events)).toBe(true);
  });

  it('events list --past — getMyPastEventsForHomePage returns a JSON envelope', () => {
    const out = run(['events', 'list', '--past'], { env: realEnv });
    expect(out.status).toBe('success');
  });

  it('contacts list — getContacts returns a JSON envelope', () => {
    const out = run(['contacts', 'list'], { env: realEnv });
    expect(out.status).toBe('success');
  });

  it('schema api.createEvent still matches the live createEvent contract surface', () => {
    // Pure-local guard that rides along with the smoke run: if the spec's
    // request params were edited away from the real contract, catch it here.
    const out = run(['schema', 'api.createEvent']);
    expect(out.data.requestParams).toContain('event');
    expect(out.data.path).toBe('/createEvent');
  });

  it.skipIf(!EVENT_ID)('events get <id> — getEventInfo returns the event', () => {
    const { stdout, exitCode } = runRaw(['events', 'get', EVENT_ID], { env: realEnv });
    expect(exitCode, `stdout: ${stdout}`).toBe(0);
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('success');
    expect(out.data).toBeDefined();
  });
});
