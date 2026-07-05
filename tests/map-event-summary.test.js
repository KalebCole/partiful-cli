import { describe, it, expect } from 'vitest';
import { mapEventSummary, buildEventUrl } from '../src/lib/events.js';

const ME = 'eBhI7Kx0hDTVW56uZHO519Ifm452';

// Minimal raw event as returned by getMyUpcomingEventsForHomePage.
function rawEvent(overrides = {}) {
  return {
    id: 'evt1',
    title: 'Test Event',
    startDate: '2026-07-08T01:30:00.000Z',
    status: 'PUBLISHED',
    guestStatusCounts: { GOING: 12, MAYBE: 14 },
    ownerIds: ['someOtherHost'],
    ...overrides,
  };
}

describe('buildEventUrl', () => {
  it('builds the canonical Partiful event URL', () => {
    expect(buildEventUrl('abc123')).toBe('https://partiful.com/e/abc123');
  });

  it('is the single source used by mapEventSummary', () => {
    const summary = mapEventSummary({ id: 'zzz', guestStatusCounts: {} }, null);
    expect(summary.url).toBe(buildEventUrl('zzz'));
  });
});

describe('mapEventSummary — myRsvp', () => {
  it.each(['GOING', 'MAYBE', 'DECLINED', 'SENT'])(
    'surfaces my own RSVP status "%s" from the guest record',
    (status) => {
      const e = rawEvent({ guest: { userId: ME, status } });
      expect(mapEventSummary(e, ME).myRsvp).toBe(status);
    }
  );

  it('is null when there is no guest record (e.g. events I host)', () => {
    const e = rawEvent({ ownerIds: [ME] }); // no guest field
    expect(mapEventSummary(e, ME).myRsvp).toBeNull();
  });

  it('is null when guest exists but has no status', () => {
    const e = rawEvent({ guest: { userId: ME } });
    expect(mapEventSummary(e, ME).myRsvp).toBeNull();
  });
});

describe('mapEventSummary — isHost', () => {
  it('is true when my id is in ownerIds', () => {
    const e = rawEvent({ ownerIds: ['x', ME, 'y'] });
    expect(mapEventSummary(e, ME).isHost).toBe(true);
  });

  it('is false when I am only a guest', () => {
    const e = rawEvent({ ownerIds: ['someOtherHost'], guest: { userId: ME, status: 'GOING' } });
    expect(mapEventSummary(e, ME).isHost).toBe(false);
  });

  it('is false (not a crash) when me is null — the pre-fix broken state', () => {
    const e = rawEvent({ ownerIds: [ME] });
    expect(mapEventSummary(e, null).isHost).toBe(false);
  });

  it('is false when me is undefined (userId never resolved)', () => {
    const e = rawEvent({ ownerIds: [ME] });
    expect(mapEventSummary(e, undefined).isHost).toBe(false);
  });

  it('is false when ownerIds is missing entirely', () => {
    const e = rawEvent({ ownerIds: undefined });
    expect(mapEventSummary(e, ME).isHost).toBe(false);
  });
});

describe('mapEventSummary — backward compatibility', () => {
  it('preserves all pre-existing fields and values', () => {
    const e = rawEvent({
      endDate: '2026-07-08T03:30:00.000Z',
      location: 'The Park',
      guest: { userId: ME, status: 'MAYBE' },
    });
    const out = mapEventSummary(e, ME);
    expect(out).toEqual({
      id: 'evt1',
      title: 'Test Event',
      startDate: '2026-07-08T01:30:00.000Z',
      endDate: '2026-07-08T03:30:00.000Z',
      location: 'The Park',
      status: 'PUBLISHED',
      isHost: false,
      myRsvp: 'MAYBE',
      going: 12,
      maybe: 14,
      url: 'https://partiful.com/e/evt1',
    });
  });

  it('defaults counts, endDate, and location the same way as before', () => {
    const out = mapEventSummary(rawEvent({ guestStatusCounts: undefined }), ME);
    expect(out.going).toBe(0);
    expect(out.maybe).toBe(0);
    expect(out.endDate).toBeNull();
    expect(out.location).toBeNull();
  });
});
