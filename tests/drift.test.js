/**
 * Drift-detection unit tests (T6).
 *
 * Verifies that responses carrying fields the spec does not declare are
 * flagged, that conforming responses are silent, and that array responses
 * (getContacts, homepage events) are diffed against their element schema.
 */
import { describe, it, expect, afterEach } from 'vitest';
import { detectDrift, reportDrift, unwrapPayload } from '../src/lib/drift.ts';

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

  it('unwrapPayload descends into method-specific nested payloads', () => {
    // getEventInfo real shape is result.data.event — diffing at .data would
    // otherwise flag { event } as an unknown field on every single call.
    const event = { id: 'e1', title: 'Party' };
    expect(unwrapPayload('getEventInfo', { event })).toBe(event);
    // homepage lists nest under upcomingEvents/pastEvents arrays.
    const up = [{ id: 'e1' }];
    expect(unwrapPayload('getMyUpcomingEventsForHomePage', { upcomingEvents: up })).toBe(up);
    const past = [{ id: 'e2' }];
    expect(unwrapPayload('getMyPastEventsForHomePage', { pastEvents: past })).toBe(past);
    // methods with no nesting pass data through untouched.
    const data = { id: 'e' };
    expect(unwrapPayload('createEvent', data)).toBe(data);
    // missing sub-key falls back to the original (never throws).
    expect(unwrapPayload('getEventInfo', { notEvent: 1 })).toEqual({ notEvent: 1 });
  });

  it('nested-payload methods do not false-positive after unwrap', () => {
    // A conforming getEventInfo event unwrapped to its inner shape reports no drift.
    const eventData = { event: { id: 'e1', title: 'Party', status: 'published', startDate: '2026-08-01' } };
    const drift = detectDrift('getEventInfo', unwrapPayload('getEventInfo', eventData));
    // The inner event matches GetEventInfoResponseSchema's declared fields → no drift.
    expect(drift).toEqual([]);
    // But the raw un-unwrapped data would look like a single unknown key 'event'.
    const naive = detectDrift('createEvent', eventData); // createEvent declares id/title/...
    expect(naive).toContain('event');
  });
});
