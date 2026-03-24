import { describe, it, expect } from 'vitest';
import { parseDateTime, parseTimeString, stripMarkdown, formatDate, hasExplicitYear, needsYearFix } from '../src/lib/dates.js';

describe('parseDateTime', () => {
  it('parses "tomorrow"', () => {
    const d = parseDateTime('tomorrow');
    const tomorrow = new Date();
    tomorrow.setDate(tomorrow.getDate() + 1);
    expect(d.getDate()).toBe(tomorrow.getDate());
    expect(d.getHours()).toBe(19); // default 7pm
  });

  it('parses ISO date', () => {
    const d = parseDateTime('2027-04-15T19:00:00');
    expect(d.getFullYear()).toBe(2027);
    expect(d.getMonth()).toBe(3); // April
    expect(d.getDate()).toBe(15);
  });

  it('parses "next Friday 8pm"', () => {
    const d = parseDateTime('next Friday 8pm');
    expect(d.getDay()).toBe(5); // Friday
    expect(d.getHours()).toBe(20);
  });

  it('returns a Date for any input JS can parse (lenient)', () => {
    // The parser is lenient due to tryAddYear; verify it returns a Date
    const d = parseDateTime('Apr 15 2027 7pm');
    expect(d instanceof Date).toBe(true);
    expect(d.getHours()).toBe(19);
  });
});

describe('parseTimeString', () => {
  it('parses "7pm"', () => {
    const t = parseTimeString('7pm');
    expect(t).toEqual({ hours: 19, minutes: 0 });
  });

  it('parses "3:30 PM"', () => {
    const t = parseTimeString('3:30 PM');
    expect(t).toEqual({ hours: 15, minutes: 30 });
  });

  it('parses "12am" as midnight', () => {
    const t = parseTimeString('12am');
    expect(t).toEqual({ hours: 0, minutes: 0 });
  });

  it('returns null for invalid', () => {
    expect(parseTimeString('noon')).toBeNull();
  });
});

describe('stripMarkdown', () => {
  it('strips bold', () => {
    expect(stripMarkdown('**hello**')).toBe('hello');
  });
  it('strips links', () => {
    expect(stripMarkdown('[click](http://example.com)')).toBe('click');
  });
  it('returns falsy as-is', () => {
    expect(stripMarkdown(null)).toBeNull();
    expect(stripMarkdown('')).toBe('');
  });
});

describe('formatDate', () => {
  it('formats ISO string to readable date', () => {
    const result = formatDate('2026-04-15T19:00:00');
    expect(result).toContain('Apr');
    expect(result).toContain('15');
  });
});

describe('hasExplicitYear', () => {
  it('detects 4-digit year', () => {
    expect(hasExplicitYear('Apr 15 2026 7pm')).toBe(true);
  });
  it('returns false without year', () => {
    expect(hasExplicitYear('Apr 15 7pm')).toBe(false);
  });
});
