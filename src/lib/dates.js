/**
 * Date parsing and formatting utilities for Partiful CLI.
 *
 * KNOWN LIMITATION: parseDateTime() accepts a timezone param but Date construction
 * uses the machine's local timezone. This works correctly when the machine timezone
 * matches the target timezone (the common case). For cross-timezone event creation,
 * the ISO string may be offset. Partiful stores timezone separately so the event
 * displays correctly on their end, but the UTC instant may differ slightly.
 */

export function parseDateTime(dateStr, timezone = 'America/Los_Angeles') {
  const lower = dateStr.trim().toLowerCase();
  const now = new Date();

  if (lower === 'tomorrow') {
    const d = new Date(now);
    d.setDate(d.getDate() + 1);
    d.setHours(19, 0, 0, 0);
    return d;
  }

  const nextDayMatch = lower.match(/^next\s+(sunday|monday|tuesday|wednesday|thursday|friday|saturday)(?:\s+(.+))?$/i);
  if (nextDayMatch) {
    const dayNames = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];
    const targetDay = dayNames.indexOf(nextDayMatch[1].toLowerCase());
    const d = new Date(now);
    let daysAhead = targetDay - d.getDay();
    if (daysAhead <= 0) daysAhead += 7;
    d.setDate(d.getDate() + daysAhead);

    if (nextDayMatch[2]) {
      const timeParsed = parseTimeString(nextDayMatch[2].trim());
      if (timeParsed) {
        d.setHours(timeParsed.hours, timeParsed.minutes, 0, 0);
      } else {
        d.setHours(19, 0, 0, 0);
      }
    } else {
      d.setHours(19, 0, 0, 0);
    }
    return d;
  }

  // Parse human-friendly dates like "2026-04-01 7pm", "April 1, 2026 7:00 PM", "Mar 15 8am"
  const cleanStr = dateStr.replace(/(\d{1,2})(am|pm)/i, '$1:00 $2');
  let date = new Date(cleanStr);

  if (isNaN(date.getTime()) || needsYearFix(dateStr, date)) {
    const withYear = tryAddYear(dateStr, now);
    if (withYear) {
      const cleanWithYear = withYear.replace(/(\d{1,2})(am|pm)/i, '$1:00 $2');
      date = new Date(cleanWithYear);
    }
  }

  if (isNaN(date.getTime())) {
    throw new Error(`Could not parse date: ${dateStr}`);
  }

  if (!hasExplicitYear(dateStr) && date < now) {
    date.setFullYear(date.getFullYear() + 1);
  }

  return date;
}

export function parseTimeString(str) {
  const match = str.match(/^(\d{1,2})(?::(\d{2}))?\s*(am|pm)?$/i);
  if (!match) return null;
  let hours = parseInt(match[1]);
  const minutes = parseInt(match[2] || '0');
  const ampm = match[3]?.toLowerCase();
  if (ampm === 'pm' && hours < 12) hours += 12;
  if (ampm === 'am' && hours === 12) hours = 0;
  return { hours, minutes };
}

export function hasExplicitYear(dateStr) {
  return /\b20\d{2}\b/.test(dateStr);
}

export function needsYearFix(dateStr, date) {
  if (hasExplicitYear(dateStr)) return false;
  const currentYear = new Date().getFullYear();
  return date.getFullYear() < currentYear || date.getFullYear() > currentYear + 1;
}

export function tryAddYear(dateStr, now) {
  const year = now.getFullYear();
  const timeMatch = dateStr.match(/^(.+?)(\d{1,2}(?::\d{2})?\s*(?:am|pm).*)$/i);
  if (timeMatch) {
    return `${timeMatch[1].trim()} ${year} ${timeMatch[2].trim()}`;
  }
  return `${dateStr} ${year}`;
}

export function stripMarkdown(text) {
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

export function formatDate(isoStr) {
  const d = new Date(isoStr);
  return d.toLocaleDateString('en-US', {
    weekday: 'short', month: 'short', day: 'numeric',
    hour: 'numeric', minute: '2-digit'
  });
}
