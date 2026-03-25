/**
 * Bulk commands — create/update multiple events from JSON/CSV or repeat pattern.
 */

import fs from 'fs';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { parseDateTime } from '../lib/dates.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { apiRequest, firestoreRequest } from '../lib/http.js';

const DEFAULT_DELAY = 1000; // ms between API calls

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Parse a CSV string into an array of objects (first row = headers).
 */
function parseCsv(text) {
  const lines = text.split('\n').filter(l => l.trim());
  if (lines.length < 2) return [];
  const headers = lines[0].split(',').map(h => h.trim());
  return lines.slice(1).map(line => {
    const values = [];
    let current = '';
    let inQuotes = false;
    for (const ch of line) {
      if (ch === '"') { inQuotes = !inQuotes; continue; }
      if (ch === ',' && !inQuotes) { values.push(current.trim()); current = ''; continue; }
      current += ch;
    }
    values.push(current.trim());
    const obj = {};
    headers.forEach((h, i) => { if (values[i] !== undefined && values[i] !== '') obj[h] = values[i]; });
    return obj;
  });
}

/**
 * Normalize a row from JSON/CSV into the shape events create expects.
 */
function normalizeRow(row) {
  return {
    title: row.title,
    date: row.date || row.startDate,
    endDate: row.endDate || row.end_date || row['end-date'],
    location: row.location,
    address: row.address,
    description: row.description,
    capacity: row.capacity ? parseInt(row.capacity) : undefined,
    private: row.private === true || row.private === 'true',
    timezone: row.timezone || 'America/Los_Angeles',
    theme: row.theme || 'oxblood',
    effect: row.effect || 'sunbeams',
    poster: row.poster,
    posterSearch: row.posterSearch || row['poster-search'],
  };
}

/**
 * Build event payload (shared with events create — duplicated here to avoid circular imports).
 */
function buildEventPayload(opts) {
  const startDate = parseDateTime(opts.date, opts.timezone);
  const endDate = opts.endDate ? parseDateTime(opts.endDate, opts.timezone) : null;

  const event = {
    title: opts.title,
    startDate: startDate.toISOString(),
    timezone: opts.timezone || 'America/Los_Angeles',
    displaySettings: {
      theme: opts.theme || 'oxblood',
      effect: opts.effect || 'sunbeams',
      titleFont: 'display',
    },
    showHostList: true,
    showGuestCount: true,
    showGuestList: true,
    showActivityTimestamps: true,
    displayInviteButton: true,
    visibility: opts.private ? 'private' : 'public',
    allowGuestPhotoUpload: true,
    enableGuestReminders: true,
    rsvpsEnabled: true,
    allowGuestsToInviteMutuals: true,
    rsvpButtonGlyphType: 'emojis',
    status: 'UNSAVED',
    guestStatusCounts: {
      READY_TO_SEND: 0, SENDING: 0, SENT: 0, SEND_ERROR: 0,
      DELIVERY_ERROR: 0, INTERESTED: 0, MAYBE: 0, GOING: 0,
      DECLINED: 0, WAITLIST: 0, PENDING_APPROVAL: 0, APPROVED: 0,
      WITHDRAWN: 0, RESPONDED_TO_FIND_A_TIME: 0,
      WAITLISTED_FOR_APPROVAL: 0, REJECTED: 0,
    },
  };

  if (endDate) event.endDate = endDate.toISOString();
  if (opts.location) event.location = opts.location;
  if (opts.address) event.address = opts.address;
  if (opts.description) event.description = opts.description;
  if (opts.capacity) { event.guestLimit = opts.capacity; event.enableWaitlist = true; }

  return event;
}

/**
 * Generate a series of dates: weekly, biweekly, monthly from a start date.
 */
function generateSeries(startDateStr, repeat, count, timezone) {
  const dates = [];
  const start = parseDateTime(startDateStr, timezone);
  for (let i = 0; i < count; i++) {
    const d = new Date(start);
    switch (repeat) {
      case 'daily': d.setDate(d.getDate() + i); break;
      case 'weekly': d.setDate(d.getDate() + (i * 7)); break;
      case 'biweekly': d.setDate(d.getDate() + (i * 14)); break;
      case 'monthly': d.setMonth(d.getMonth() + i); break;
      default: throw new Error(`Unknown repeat interval: ${repeat}. Use: daily, weekly, biweekly, monthly`);
    }
    dates.push(d.toISOString());
  }
  return dates;
}

export function registerBulkCommands(program) {
  const bulk = program.command('bulk').description('Bulk create or update events');

  bulk
    .command('create <file>')
    .description('Create multiple events from a JSON or CSV file')
    .option('--delay <ms>', 'Delay between API calls (ms)', parseInt, DEFAULT_DELAY)
    .action(async (file, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        if (!fs.existsSync(file)) {
          jsonError(`File not found: ${file}`, 3, 'validation_error');
          return;
        }

        const raw = fs.readFileSync(file, 'utf8');
        const isCsv = file.endsWith('.csv');
        let rows;

        if (isCsv) {
          rows = parseCsv(raw);
        } else {
          rows = JSON.parse(raw);
          if (!Array.isArray(rows)) {
            jsonError('JSON file must contain an array of event objects.', 3, 'validation_error');
            return;
          }
        }

        if (rows.length === 0) {
          jsonError('No events found in file.', 3, 'validation_error');
          return;
        }

        // Validate all rows before starting
        const normalized = rows.map((row, i) => {
          const n = normalizeRow(row);
          if (!n.title) throw new Error(`Row ${i + 1}: missing "title"`);
          if (!n.date) throw new Error(`Row ${i + 1}: missing "date"`);
          return n;
        });

        if (globalOpts.dryRun) {
          jsonOutput(normalized.map(n => buildEventPayload(n)), {
            total: normalized.length,
            action: 'dry_run',
            hint: 'Remove --dry-run to create these events',
          }, globalOpts);
          return;
        }

        const config = loadConfig();
        const token = await getValidToken(config);
        const results = [];

        for (let i = 0; i < normalized.length; i++) {
          const event = buildEventPayload(normalized[i]);
          const payload = { data: wrapPayload(config, { event }) };

          try {
            const resp = await apiRequest('POST', '/createEvent', token, payload, globalOpts.verbose);
            results.push({ index: i + 1, status: 'created', title: normalized[i].title, eventId: resp.result?.data || resp.result?.eventId });
            process.stderr.write(`[${i + 1}/${normalized.length}] Created: ${normalized[i].title}\n`);
          } catch (e) {
            results.push({ index: i + 1, status: 'error', title: normalized[i].title, error: e.message });
            process.stderr.write(`[${i + 1}/${normalized.length}] Failed: ${normalized[i].title} — ${e.message}\n`);
          }

          if (i < normalized.length - 1) await sleep(opts.delay);
        }

        jsonOutput(results, {
          total: results.length,
          created: results.filter(r => r.status === 'created').length,
          errors: results.filter(r => r.status === 'error').length,
        }, globalOpts);
      } catch (e) {
        jsonError(e.message);
      }
    });

  // Series creation: --repeat weekly --count 4
  const events = program.commands.find(c => c.name() === 'events');
  if (events) {
    const create = events.commands.find(c => c.name() === 'create');
    if (create) {
      create
        .option('--repeat <interval>', 'Create a series: daily, weekly, biweekly, monthly')
        .option('--count <n>', 'Number of events in series', parseInt);
    }
  }

  bulk
    .command('update')
    .description('Update multiple events matching a filter')
    .requiredOption('--filter <query>', 'Filter events (format: "title contains <text>")')
    .option('--capacity <n>', 'New guest limit', parseInt)
    .option('--location <location>', 'New location')
    .option('--description <desc>', 'New description')
    .option('--delay <ms>', 'Delay between API calls (ms)', parseInt, DEFAULT_DELAY)
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // Fetch upcoming events
        const listPayload = {
          data: wrapPayload(config, {
            params: {},
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };
        const listResp = await apiRequest('POST', '/getMyUpcomingEventsForHomePage', token, listPayload, globalOpts.verbose);
        const allEvents = listResp.result?.data?.upcomingEvents || [];

        // Parse filter: "title contains <text>"
        const filterMatch = opts.filter.match(/^title\s+contains\s+(.+)$/i);
        if (!filterMatch) {
          jsonError('Filter format: "title contains <text>". More filters coming soon.', 3, 'validation_error');
          return;
        }
        const filterText = filterMatch[1].toLowerCase();
        const matched = allEvents.filter(e => e.title && e.title.toLowerCase().includes(filterText));

        if (matched.length === 0) {
          jsonOutput([], { total: 0, filter: opts.filter, hint: 'No events matched the filter' }, globalOpts);
          return;
        }

        // Build update fields
        const updates = {};
        if (opts.capacity) updates.guestLimit = opts.capacity;
        if (opts.location) updates.location = opts.location;
        if (opts.description) updates.description = opts.description;

        if (Object.keys(updates).length === 0) {
          jsonError('No update fields provided. Use --capacity, --location, or --description.', 3, 'validation_error');
          return;
        }

        if (globalOpts.dryRun) {
          jsonOutput(matched.map(e => ({
            eventId: e.id,
            title: e.title,
            updates,
          })), { total: matched.length, action: 'dry_run' }, globalOpts);
          return;
        }

        const results = [];
        for (let i = 0; i < matched.length; i++) {
          const e = matched[i];
          try {
            // Build Firestore update
            const fields = {};
            const updateFields = [];
            for (const [key, val] of Object.entries(updates)) {
              if (typeof val === 'number') {
                fields[key] = { integerValue: val };
              } else {
                fields[key] = { stringValue: val };
              }
              updateFields.push(key);
            }

            await firestoreRequest('PATCH', e.id, { fields }, token, updateFields, globalOpts.verbose);
            results.push({ eventId: e.id, title: e.title, status: 'updated' });
            process.stderr.write(`[${i + 1}/${matched.length}] Updated: ${e.title}\n`);
          } catch (err) {
            results.push({ eventId: e.id, title: e.title, status: 'error', error: err.message });
            process.stderr.write(`[${i + 1}/${matched.length}] Failed: ${e.title} — ${err.message}\n`);
          }

          if (i < matched.length - 1) await sleep(opts.delay);
        }

        jsonOutput(results, {
          total: results.length,
          updated: results.filter(r => r.status === 'updated').length,
          errors: results.filter(r => r.status === 'error').length,
        }, globalOpts);
      } catch (e) {
        if (e instanceof (await import('../lib/errors.js')).PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });
}
