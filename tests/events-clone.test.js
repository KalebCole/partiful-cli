import { describe, expect, it } from 'vitest';
import { resolveCloneStartDate } from '../src/commands/events.js';

describe('resolveCloneStartDate', () => {
  it('uses an explicit date instead of shifting the source date', () => {
    const result = resolveCloneStartDate(
      { date: '2026-09-01T19:00:00Z', shift: '30' },
      '2026-08-01T19:00:00Z',
      'America/Los_Angeles',
    );

    expect(result.toISOString()).toBe('2026-09-01T19:00:00.000Z');
  });

  it('shifts the source start date by seven days by default', () => {
    const result = resolveCloneStartDate({}, '2026-08-01T19:00:00Z', 'America/Los_Angeles');

    expect(result.toISOString()).toBe('2026-08-08T19:00:00.000Z');
  });

  it('accepts an explicit negative or positive integer shift', () => {
    expect(resolveCloneStartDate({ shift: '-2' }, '2026-08-10T19:00:00Z', 'America/Los_Angeles').toISOString())
      .toBe('2026-08-08T19:00:00.000Z');
    expect(resolveCloneStartDate({ shift: '3' }, '2026-08-10T19:00:00Z', 'America/Los_Angeles').toISOString())
      .toBe('2026-08-13T19:00:00.000Z');
  });

  it('preserves event-local wall time when host and event offsets diverge', () => {
    const result = resolveCloneStartDate(
      { shift: '7' },
      '2026-03-01T17:00:00.000Z',
      'America/Phoenix',
    );

    expect(result.toISOString()).toBe('2026-03-08T17:00:00.000Z');
  });

  it.each([
    ['spring forward', '2026-03-01T17:00:00.000Z', '2026-03-08T16:00:00.000Z'],
    ['fall back', '2026-10-25T16:00:00.000Z', '2026-11-01T17:00:00.000Z'],
  ])('preserves event-local wall time across %s', (_label, source, expected) => {
    expect(resolveCloneStartDate({ shift: '7' }, source, 'America/Los_Angeles').toISOString())
      .toBe(expected);
  });

  it('rejects invalid shifts and missing source dates', () => {
    expect(() => resolveCloneStartDate({ shift: '1.5' }, '2026-08-01T19:00:00Z', 'America/Los_Angeles'))
      .toThrow('--shift must be an integer number of days');
    expect(() => resolveCloneStartDate({}, undefined, 'America/Los_Angeles'))
      .toThrow('Source event has no start date and --date was not provided');
  });
});
