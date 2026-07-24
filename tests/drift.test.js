/**
 * Drift-detection unit tests (T6).
 *
 * Verifies that responses carrying fields the spec does not declare are
 * flagged, that conforming responses are silent, and that array responses
 * (getContacts, homepage events) are diffed against their element schema.
 */
import { describe, it, expect, afterEach } from 'vitest';
import { detectDrift, reportDrift } from '../src/lib/drift.ts';

describe('drift detection', () => {
  it('flags unknown top-level fields against an object schema', () => {
    const drift = detectDrift('createEvent', {
      id: 'evt_1',
      title: 'Party',
      status: 'published',
      startDate: '2026-08-01',
      brandNewVendorField: 42,
      anotherOne: true,
    });
    expect(drift).toContain('brandNewVendorField');
    expect(drift).toContain('anotherOne');
    expect(drift).not.toContain('id');
    expect(drift).not.toContain('title');
  });

  it('is silent when a response conforms to the spec', () => {
    const drift = detectDrift('createEvent', { id: 'evt_1', title: 'Party' });
    expect(drift).toEqual([]);
  });

  it('diffs array responses against the element schema (getContacts)', () => {
    const drift = detectDrift('getContacts', [
      { id: 'u1', name: 'Alice' },
      { id: 'u2', name: 'Bob', mysteryField: 'x' },
    ]);
    expect(drift).toContain('mysteryField');
  });

  it('returns [] for a method whose schema declares no fields (passthrough-only)', () => {
    // cancelEvent response schema is z.object({}).passthrough() — no declared
    // keys, so there is no field surface to diff against; never flags drift.
    const drift = detectDrift('cancelEvent', { anything: 1, goes: 2 });
    expect(drift).toEqual([]);
  });

  it('reportDrift returns a record with force=true, null when conforming', () => {
    const rec = reportDrift('createEvent', { id: 'e', surprise: 1 }, true);
    expect(rec).not.toBeNull();
    expect(rec.method).toBe('createEvent');
    expect(rec.unknownFields).toContain('surprise');
    expect(typeof rec.observedAt).toBe('string');

    const none = reportDrift('createEvent', { id: 'e' }, true);
    expect(none).toBeNull();
  });

  it('reportDrift never throws on malformed input', () => {
    expect(() => reportDrift('createEvent', null)).not.toThrow();
    expect(() => reportDrift('createEvent', 'not-an-object')).not.toThrow();
    expect(reportDrift('createEvent', null)).toBeNull();
  });
});
