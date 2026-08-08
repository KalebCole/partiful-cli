/**
 * Date parsing and formatting utilities for Partiful CLI.
 */

/** Parsed hour/minute pair from a time string. */
export interface ParsedTime {
  hours: number;
  minutes: number;
}

interface WallClockParts {
  year: number;
  month: number;
  day: number;
  hours: number;
  minutes: number;
  seconds: number;
  milliseconds: number;
}

const zonedFormatterCache = new Map<string, Intl.DateTimeFormat>();

function zonedParts(date: Date, timezone: string): WallClockParts {
  let formatter = zonedFormatterCache.get(timezone);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat('en-US', {
      timeZone: timezone,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
    zonedFormatterCache.set(timezone, formatter);
  }
  const values = Object.fromEntries(
    formatter.formatToParts(date)
      .filter((part) => part.type !== 'literal')
      .map((part) => [part.type, Number(part.value)]),
  );
  return {
    year: values['year']!,
    month: values['month']!,
    day: values['day']!,
    hours: values['hour']!,
    minutes: values['minute']!,
    seconds: values['second']!,
    milliseconds: date.getUTCMilliseconds(),
  };
}

function wallClockToInstant(parts: WallClockParts, timezone: string): Date {
  const target = Date.UTC(
    parts.year,
    parts.month - 1,
    parts.day,
    parts.hours,
    parts.minutes,
    parts.seconds,
    parts.milliseconds,
  );
  let guess = target;
  for (let i = 0; i < 3; i++) {
    const actual = zonedParts(new Date(guess), timezone);
    const actualAsUtc = Date.UTC(
      actual.year,
      actual.month - 1,
      actual.day,
      actual.hours,
      actual.minutes,
      actual.seconds,
      actual.milliseconds,
    );
    const correction = target - actualAsUtc;
    if (correction === 0) return new Date(guess);
    guess += correction;
  }
  return new Date(guess);
}

function localParts(date: Date): WallClockParts {
  return {
    year: date.getFullYear(),
    month: date.getMonth() + 1,
    day: date.getDate(),
    hours: date.getHours(),
    minutes: date.getMinutes(),
    seconds: date.getSeconds(),
    milliseconds: date.getMilliseconds(),
  };
}

function hasExplicitOffset(dateStr: string): boolean {
  return /(?:z|[+-]\d{2}:?\d{2})\s*$/i.test(dateStr.trim());
}

export function parseDateTime(dateStr: string, timezone = 'America/Los_Angeles'): Date {
  const lower = dateStr.trim().toLowerCase();
  const now = new Date();

  if (lower === 'tomorrow') {
    const today = zonedParts(now, timezone);
    const wallDate = new Date(Date.UTC(today.year, today.month - 1, today.day + 1));
    return wallClockToInstant({
      year: wallDate.getUTCFullYear(),
      month: wallDate.getUTCMonth() + 1,
      day: wallDate.getUTCDate(),
      hours: 19,
      minutes: 0,
      seconds: 0,
      milliseconds: 0,
    }, timezone);
  }

  const nextDayMatch = lower.match(/^next\s+(sunday|monday|tuesday|wednesday|thursday|friday|saturday)(?:\s+(.+))?$/i);
  if (nextDayMatch) {
    const dayNames = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];
    const targetDay = dayNames.indexOf(nextDayMatch[1]!.toLowerCase());
    const today = zonedParts(now, timezone);
    const wallDate = new Date(Date.UTC(today.year, today.month - 1, today.day));
    let daysAhead = targetDay - wallDate.getUTCDay();
    if (daysAhead <= 0) daysAhead += 7;
    wallDate.setUTCDate(wallDate.getUTCDate() + daysAhead);
    const parsedTime = nextDayMatch[2] ? parseTimeString(nextDayMatch[2].trim()) : null;
    return wallClockToInstant({
      year: wallDate.getUTCFullYear(),
      month: wallDate.getUTCMonth() + 1,
      day: wallDate.getUTCDate(),
      hours: parsedTime?.hours ?? 19,
      minutes: parsedTime?.minutes ?? 0,
      seconds: 0,
      milliseconds: 0,
    }, timezone);
  }

  // Preserve explicit UTC offsets exactly as supplied.
  if (hasExplicitOffset(dateStr)) {
    const explicitDate = new Date(dateStr);
    if (isNaN(explicitDate.getTime())) throw new Error(`Could not parse date: ${dateStr}`);
    return explicitDate;
  }

  // Parse human-friendly dates like "2026-04-01 7pm", "April 1, 2026 7:00 PM", "Mar 15 8am".
  const cleanStr = dateStr.replace(/(\d{1,2})(am|pm)/i, '$1:00 $2');
  let parsed = new Date(cleanStr);

  if (isNaN(parsed.getTime()) || needsYearFix(dateStr, parsed)) {
    const withYear = tryAddYear(dateStr, now);
    if (withYear) {
      const cleanWithYear = withYear.replace(/(\d{1,2})(am|pm)/i, '$1:00 $2');
      parsed = new Date(cleanWithYear);
    }
  }

  if (isNaN(parsed.getTime())) throw new Error(`Could not parse date: ${dateStr}`);

  const parts = localParts(parsed);
  let date = wallClockToInstant(parts, timezone);
  if (!hasExplicitYear(dateStr) && date < now) {
    parts.year += 1;
    date = wallClockToInstant(parts, timezone);
  }
  return date;
}

export function parseTimeString(str: string): ParsedTime | null {
  const match = str.match(/^(\d{1,2})(?::(\d{2}))?\s*(am|pm)?$/i);
  if (!match) return null;
  let hours = parseInt(match[1]!);
  const minutes = parseInt(match[2] || '0');
  const ampm = match[3]?.toLowerCase();
  if (ampm === 'pm' && hours < 12) hours += 12;
  if (ampm === 'am' && hours === 12) hours = 0;
  return { hours, minutes };
}

export function hasExplicitYear(dateStr: string): boolean {
  return /\b20\d{2}\b/.test(dateStr);
}

export function needsYearFix(dateStr: string, date: Date): boolean {
  if (hasExplicitYear(dateStr)) return false;
  const currentYear = new Date().getFullYear();
  return date.getFullYear() < currentYear || date.getFullYear() > currentYear + 1;
}

export function tryAddYear(dateStr: string, now: Date): string {
  const year = now.getFullYear();
  const timeMatch = dateStr.match(/^(.+?)(\d{1,2}(?::\d{2})?\s*(?:am|pm).*)$/i);
  if (timeMatch) {
    return `${timeMatch[1]!.trim()} ${year} ${timeMatch[2]!.trim()}`;
  }
  return `${dateStr} ${year}`;
}

export function stripMarkdown(text: string): string {
  if (!text) return text;
  return text
    .replace(/\*\*(.*?)\*\*/g, '$1')
    .replace(/\*(.*?)\*/g, '$1')
    .replace(/\[(.*?)\]\(.*?\)/g, '$1')
    .replace(/#{1,6}\s+/g, '')
    .replace(/`(.*?)`/g, '$1')
    .replace(/~~(.*?)~~/g, '$1')
    .replace(/>\s+/g, '');
}

export function formatDate(isoStr: string): string {
  const d = new Date(isoStr);
  return d.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}
