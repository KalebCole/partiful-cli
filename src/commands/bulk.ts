/**
 * Bulk commands — create/update multiple events from JSON/CSV or repeat pattern.
 */

import fs from 'fs';
import { Command } from 'commander';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import type { PartifulConfig } from '../lib/auth.js';
import { parseDateTime } from '../lib/dates.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { apiRequest, firestoreRequest } from '../lib/http.js';
import { PartifulError } from '../lib/errors.js';
import { buildBaseEvent } from '../lib/events.js';
import type { EventOptions } from '../lib/events.js';

void parseDateTime; // imported for side-effects/type availability

function makePayload(config: PartifulConfig, params: unknown) {
  return {
    data: wrapPayload(config, {
      params,
      amplitudeSessionId: Date.now(),
      userId: config.userId,
    }),
  };
}

function handleError(e: unknown) {
  if (e instanceof PartifulError) {
    jsonError(e.message, e.exitCode, e.type, e.details);
  } else {
    jsonError(e instanceof Error ? e.message : String(e));
  }
}

const DEFAULT_DELAY = 1000; // ms between API calls

function sleep(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms));
}

/**
 * Parse a CSV string into an array of objects (first row = headers).
 */
function parseCsv(text: string): Array<Record<string, string>> {
  const lines = text.split('\n').filter((l) => l.trim());
  if (lines.length < 2) return [];
  const headers = lines[0]!.split(',').map((h) => h.trim());
  return lines.slice(1).map((line) => {
    const values: string[] = [];
    let current = '';
    let inQuotes = false;
    for (const ch of line) {
      if (ch === '"') { inQuotes = !inQuotes; continue; }
      if (ch === ',' && !inQuotes) { values.push(current.trim()); current = ''; continue; }
      current += ch;
    }
    values.push(current.trim());
    const obj: Record<string, string> = {};
    headers.forEach((h, i) => {
      const v = values[i];
      if (v !== undefined && v !== '') obj[h] = v;
    });
    return obj;
  });
}

/**
 * Normalize a row from JSON/CSV into the shape buildBaseEvent expects.
 */
function normalizeRow(row: Record<string, unknown>): EventOptions {
  return {
    title: (row['title'] as string | undefined) ?? '',
    date: ((row['date'] ?? row['startDate']) as string | undefined) ?? '',
    endDate: (row['endDate'] ?? row['end_date'] ?? row['end-date']) as string | undefined,
    location: row['location'] as string | undefined,
    address: row['address'] as string | undefined,
    description: row['description'] as string | undefined,
    capacity: row['capacity'] ? parseInt(row['capacity'] as string) : undefined,
    private: row['private'] === true || row['private'] === 'true',
    timezone: (row['timezone'] as string | undefined) ?? 'America/Los_Angeles',
    theme: (row['theme'] as string | undefined) ?? 'oxblood',
    effect: (row['effect'] as string | undefined) ?? 'sunbeams',
    poster: row['poster'] as string | undefined,
    posterSearch: (row['posterSearch'] ?? row['poster-search']) as string | undefined,
  };
}

export function registerBulkCommands(program: Command): void {
  const bulk = program.command('bulk').description('Bulk create or update events');

  bulk
    .command('create <file>')
    .description('Create multiple events from a JSON or CSV file')
    .option('--delay <ms>', 'Delay between API calls (ms)', parseInt, DEFAULT_DELAY)
    .action(async (file: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        if (!fs.existsSync(file)) {
          jsonError(`File not found: ${file}`, 3, 'validation_error');
          return;
        }

        const raw = fs.readFileSync(file, 'utf8');
        const isCsv = file.endsWith('.csv');
        let rows: Array<Record<string, unknown>>;

        if (isCsv) {
          rows = parseCsv(raw);
        } else {
          rows = JSON.parse(raw) as Array<Record<string, unknown>>;
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

        if (globalOpts['dryRun']) {
          jsonOutput(normalized.map((n) => buildBaseEvent(n).event), {
            total: normalized.length,
            action: 'dry_run',
            hint: 'Remove --dry-run to create these events',
          }, globalOpts);
          return;
        }

        const config = loadConfig();
        const token = await getValidToken(config);
        const results: Array<Record<string, unknown>> = [];

        for (let i = 0; i < normalized.length; i++) {
          const n = normalized[i]!;
          const { event } = buildBaseEvent(n);
          const payload = makePayload(config, { event, cohostIds: [] });

          try {
            const rawResp = await apiRequest('POST', '/createEvent', token, payload, globalOpts['verbose'] as boolean | undefined);
            const resp = rawResp as Record<string, unknown>;
            const respResult = resp['result'] as Record<string, unknown> | undefined;
            results.push({ index: i + 1, status: 'created', title: n.title, eventId: respResult?.['data'] ?? respResult?.['eventId'] });
            process.stderr.write(`[${i + 1}/${normalized.length}] Created: ${n.title}\n`);
          } catch (e) {
            const msg = e instanceof Error ? e.message : String(e);
            results.push({ index: i + 1, status: 'error', title: n.title, error: msg });
            process.stderr.write(`[${i + 1}/${normalized.length}] Failed: ${n.title} — ${msg}\n`);
          }

          if (i < normalized.length - 1) await sleep(opts['delay'] as number);
        }

        jsonOutput(results, {
          total: results.length,
          created: results.filter((r) => r['status'] === 'created').length,
          errors: results.filter((r) => r['status'] === 'error').length,
        }, globalOpts);
      } catch (e) {
        handleError(e);
      }
    });

  // Series creation: --repeat weekly --count 4
  const events = program.commands.find((c) => c.name() === 'events');
  if (events) {
    const create = events.commands.find((c) => c.name() === 'create');
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
    .action(async (opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
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
        const rawListResp = await apiRequest('POST', '/getMyUpcomingEventsForHomePage', token, listPayload, globalOpts['verbose'] as boolean | undefined);
        const listResp = rawListResp as Record<string, unknown>;
        const listResult = listResp['result'] as Record<string, unknown> | undefined;
        const listData = listResult?.['data'] as Record<string, unknown> | undefined;
        const allEvents = ((listData?.['upcomingEvents'] ?? []) as Array<Record<string, unknown>>);

        // Parse filter: "title contains <text>"
        const filterStr = opts['filter'] as string;
        const filterMatch = filterStr.match(/^title\s+contains\s+(.+)$/i);
        if (!filterMatch) {
          jsonError('Filter format: "title contains <text>". More filters coming soon.', 3, 'validation_error');
          return;
        }
        const filterText = filterMatch[1]!.toLowerCase();
        const matched = allEvents.filter((e) => {
          const title = e['title'] as string | undefined;
          return title && title.toLowerCase().includes(filterText);
        });

        if (matched.length === 0) {
          jsonOutput([], { total: 0, filter: opts['filter'], hint: 'No events matched the filter' }, globalOpts);
          return;
        }

        // Build update fields
        const updates: Record<string, unknown> = {};
        if (opts['capacity']) updates['guestLimit'] = opts['capacity'];
        if (opts['location']) updates['location'] = opts['location'];
        if (opts['description']) updates['description'] = opts['description'];

        if (Object.keys(updates).length === 0) {
          jsonError('No update fields provided. Use --capacity, --location, or --description.', 3, 'validation_error');
          return;
        }

        if (globalOpts['dryRun']) {
          jsonOutput(matched.map((e) => ({
            eventId: e['id'],
            title: e['title'],
            updates,
          })), { total: matched.length, action: 'dry_run' }, globalOpts);
          return;
        }

        const results: Array<Record<string, unknown>> = [];
        for (let i = 0; i < matched.length; i++) {
          const e = matched[i]!;
          try {
            const fields: Record<string, unknown> = {};
            const updateFields: string[] = [];
            for (const [key, val] of Object.entries(updates)) {
              if (typeof val === 'number') {
                fields[key] = { integerValue: val };
              } else {
                fields[key] = { stringValue: val };
              }
              updateFields.push(key);
            }

            await firestoreRequest('PATCH', e['id'] as string, { fields }, token, updateFields, globalOpts['verbose'] as boolean | undefined);
            results.push({ eventId: e['id'], title: e['title'], status: 'updated' });
            process.stderr.write(`[${i + 1}/${matched.length}] Updated: ${e['title']}\n`);
          } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            results.push({ eventId: e['id'], title: e['title'], status: 'error', error: msg });
            process.stderr.write(`[${i + 1}/${matched.length}] Failed: ${e['title']} — ${msg}\n`);
          }

          if (i < matched.length - 1) await sleep(opts['delay'] as number);
        }

        jsonOutput(results, {
          total: results.length,
          updated: results.filter((r) => r['status'] === 'updated').length,
          errors: results.filter((r) => r['status'] === 'error').length,
        }, globalOpts);
      } catch (e) {
        handleError(e);
      }
    });
}
