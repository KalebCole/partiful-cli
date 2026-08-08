/**
 * Events commands: list, get, create, update, cancel
 */

import type { Command } from 'commander';
import type { EventOptions } from '../lib/events.js';
import type { Template } from '../lib/templates.js';
import { loadConfig, getValidToken, wrapPayload, getUserIdFromToken } from '../lib/auth.js';
import { resolveCohostNames, getCohostRequests, getCohostIds, mergeCohostState, setCohostIds, inviteCohostBatch } from '../lib/cohosts.js';
import { fetchCatalog, searchPosters, buildPosterImage } from '../lib/posters.js';
import { apiRequest, firestoreRequest } from '../lib/http.js';
import { parseDateTime, stripMarkdown } from '../lib/dates.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';
import {
  confirm, buildBaseEvent, buildLinks, toFirestoreMap,
  validateImageOptions, resolvePosterImage, resolveUploadImage,
  isUrl, ALLOWED_IMAGE_EXTENSIONS, mapEventSummary,
} from '../lib/events.js';

/**
 * Build the standard API payload wrapper.
 */
function makePayload(config: ReturnType<typeof loadConfig>, params: Record<string, unknown>): Record<string, unknown> {
  return {
    data: wrapPayload(config, {
      params,
      amplitudeSessionId: Date.now(),
      userId: config.userId,
    }),
  };
}

function makeCohostCall(
  token: string,
  config: ReturnType<typeof loadConfig>,
  verbose?: boolean,
): (endpoint: string, params: Record<string, string>) => Promise<unknown> {
  return (endpoint, params) => apiRequest('POST', endpoint, token, makePayload(config, params), verbose);
}

/**
 * Standard error handler for action callbacks.
 */
function handleError(e: unknown): void {
  if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
  else jsonError((e as Error).message);
}

function zonedParts(date: Date, timezone: string): Record<string, number> {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(date);

  return Object.fromEntries(
    parts
      .filter((part) => part.type !== 'literal')
      .map((part) => [part.type, Number(part.value)]),
  );
}

function addCalendarDays(date: Date, days: number, timezone: string): Date {
  const source = zonedParts(date, timezone);
  const targetWallTime = Date.UTC(
    source['year']!,
    source['month']! - 1,
    source['day']! + days,
    source['hour']!,
    source['minute']!,
    source['second']!,
    date.getUTCMilliseconds(),
  );
  let target = new Date(targetWallTime);

  // Convert the target wall-clock time back to its UTC instant. Recalculate
  // because the timezone offset may change across daylight saving boundaries.
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const rendered = zonedParts(target, timezone);
    const renderedWallTime = Date.UTC(
      rendered['year']!,
      rendered['month']! - 1,
      rendered['day']!,
      rendered['hour']!,
      rendered['minute']!,
      rendered['second']!,
      target.getUTCMilliseconds(),
    );
    const correction = targetWallTime - renderedWallTime;
    if (correction === 0) break;
    target = new Date(target.getTime() + correction);
  }

  return target;
}

export function resolveCloneStartDate(
  opts: { date?: unknown; shift?: unknown },
  sourceStartDate: unknown,
  timezone: string,
): Date {
  if (typeof opts.date === 'string' && opts.date) {
    return parseDateTime(opts.date, timezone);
  }

  if (typeof sourceStartDate !== 'string' || !sourceStartDate) {
    throw new Error('Source event has no start date and --date was not provided');
  }

  const shift = opts.shift === undefined ? 7 : Number(opts.shift);
  if (!Number.isInteger(shift)) {
    throw new Error('--shift must be an integer number of days');
  }

  const sourceDate = new Date(sourceStartDate);
  if (Number.isNaN(sourceDate.getTime())) {
    throw new Error('Source event has an invalid start date');
  }
  return addCalendarDays(sourceDate, shift, timezone);
}

export function registerEventsCommands(program: Command): void {
  const events = program.command('events').description('Manage events');

  events
    .command('list')
    .description('List upcoming (or past) events')
    .option('--past', 'Show past events instead of upcoming')
    .option('--include-cancelled', 'Include cancelled events')
    .action(async (opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const endpoint = opts['past']
          ? '/getMyPastEventsForHomePage'
          : '/getMyUpcomingEventsForHomePage';

        const payload = makePayload(config, {});

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, endpoint, payload });
          return;
        }

        const result = await apiRequest('POST', endpoint, token, payload, globalOpts['verbose'] as boolean | undefined) as {
          result?: { data?: { pastEvents?: unknown[]; upcomingEvents?: unknown[] } };
        };

        let eventList = opts['past']
          ? result.result?.data?.pastEvents
          : result.result?.data?.upcomingEvents;

        if (!opts['includeCancelled'] && eventList) {
          eventList = eventList.filter((e: unknown) => (e as { status?: string }).status !== 'CANCELED');
        }

        // Identify the authenticated user so we can surface their own RSVP
        // (myRsvp) and host status. config.userId is backfilled on token
        // refresh, but fall back to decoding the token directly for the
        // PARTIFUL_TOKEN env path where config.userId is never set.
        const me = (config.userId ?? getUserIdFromToken(token)) as string | null;

        const mapped = (eventList || []).map((e: unknown) => mapEventSummary(e as Parameters<typeof mapEventSummary>[0], me));

        jsonOutput(mapped, { count: mapped.length, type: opts['past'] ? 'past' : 'upcoming' });
      } catch (e) {
        handleError(e);
      }
    });

  events
    .command('get')
    .description('Get event details')
    .argument('<eventId>', 'Event ID')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const payload = makePayload(config, { eventId });

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, endpoint: '/getEventInfo', payload });
          return;
        }

        const result = await apiRequest('POST', '/getEventInfo', token, payload, globalOpts['verbose'] as boolean | undefined) as {
          result?: { data?: { event?: Record<string, unknown> } };
        };
        const event = result.result?.data?.event;

        if (!event) {
          jsonError('Event not found or no data returned', 4, 'not_found');
          return;
        }

        jsonOutput({
          id: eventId,
          title: event['title'],
          startDate: event['startDate'],
          endDate: event['endDate'] ?? null,
          location: event['location'] ?? null,
          address: event['address'] ?? null,
          description: event['description'] ?? null,
          guestLimit: event['guestLimit'] ?? null,
          rsvpDeadline: event['rsvpDeadline'] ?? null,
          allowResponsesAfterRsvpDeadline: event['allowResponsesAfterRsvpDeadline'] ?? null,
          status: event['status'],
          timezone: event['timezone'] ?? null,
          visibility: event['visibility'] ?? null,
          guestStatusCounts: event['guestStatusCounts'] ?? {},
          displaySettings: event['displaySettings'] ?? {},
          url: `https://partiful.com/e/${eventId}`,
        });
      } catch (e) {
        handleError(e);
      }
    });

  events
    .command('create')
    .description('Create a new event')
    .option('--title <title>', 'Event title (required unless using --template)')
    .option('--date <date>', 'Start date/time (required unless using --template with date)')
    .option('--end-date <endDate>', 'End date/time')
    .option('--location <location>', 'Location name')
    .option('--address <address>', 'Street address')
    .option('--description <desc>', 'Event description')
    .option('--capacity <n>', 'Guest limit', parseInt)
    .option('--rsvp-deadline <date>', 'Close RSVPs at this date/time')
    .option('--private', 'Make event private')
    .option('--timezone <tz>', 'Timezone', 'America/Los_Angeles')
    .option('--theme <theme>', 'Color theme', 'oxblood')
    .option('--effect <effect>', 'Visual effect', 'sunbeams')
    .option('--poster <posterId>', 'Built-in poster ID (use "posters search" to find)')
    .option('--poster-search <query>', 'Search for a poster by keyword')
    .option('--image <path>', 'Custom image file to upload')
    .option('--link <url...>', 'Link URL (repeatable)')
    .option('--link-text <text...>', 'Display text for link (paired with --link by position)')
    .option('--template <name>', 'Create from a saved template')
    .option('--var <vars...>', 'Template variables (key=value)')
    .option('--cohost <names...>', 'Co-host names (resolved from contacts)')
    .action(async (opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        // Template merging
        if (opts['template']) {
          const { loadTemplates, applyVariables, mergeTemplateOpts } = await import('../lib/templates.js');
          const templates = loadTemplates();
          if (!templates[opts['template'] as string]) {
            jsonError(`Template "${opts['template']}" not found. Use "partiful template list" to see available templates.`, 4, 'not_found');
            return;
          }
          let tpl = templates[opts['template'] as string] as Template;
          if (opts['var']) {
            const vars: Record<string, string> = {};
            for (const v of opts['var'] as string[]) {
              const eq = v.indexOf('=');
              if (eq > 0) vars[v.slice(0, eq)] = v.slice(eq + 1);
            }
            tpl = applyVariables(tpl as Template, vars) as Template;
          }
          const merged = mergeTemplateOpts(tpl as Template, opts) as Record<string, unknown>;
          Object.assign(opts, merged);
        }

        if (!opts['title']) {
          jsonError('--title is required (provide directly or via --template).', 3, 'validation_error');
          return;
        }
        if (!opts['date']) {
          jsonError('--date is required (provide directly or via --template).', 3, 'validation_error');
          return;
        }

        const config = loadConfig();
        const token = await getValidToken(config);

        validateImageOptions(opts['poster'], opts['posterSearch'], opts['image']);

        // Validate image extension early (before dry-run check) — skip for URLs
        if (opts['image'] && !isUrl(opts['image'] as string)) {
          const { extname } = await import('path');
          const ext = extname(opts['image'] as string).toLowerCase();
          if (!ALLOWED_IMAGE_EXTENSIONS.includes(ext as typeof ALLOWED_IMAGE_EXTENSIONS[number])) {
            jsonError(`Unsupported image type "${ext}". Allowed types: ${ALLOWED_IMAGE_EXTENSIONS.join(', ')}`, 3, 'validation_error');
            return;
          }
        }

        const { event, startDate } = buildBaseEvent(opts as unknown as EventOptions);

        // Links
        const links = buildLinks(opts['link'] as string[] | undefined, opts['linkText'] as string[] | undefined);
        if (links) event['links'] = links;

        // Poster/image handling
        const posterImage = await resolvePosterImage(opts, fetchCatalog, searchPosters, buildPosterImage);
        if (posterImage) {
          event['image'] = posterImage;
        } else if (opts['image']) {
          event['image'] = await resolveUploadImage(opts['image'] as string, token, config, globalOpts['verbose'] as boolean | undefined, globalOpts['dryRun'] as boolean | undefined);
        }

        const cohostIds = await resolveCohostNames((opts['cohost'] as string[] | undefined) ?? [], token, config, globalOpts['verbose'] as boolean | undefined);
        // Cohosts are invited only after creation through createCohostRequest.
        // Passing IDs directly to createEvent reproduces the stale-membership bug.
        const payload = makePayload(config, { event, cohostIds: [] });
        const cohostInvites = cohostIds.map((cohostId) => ({
          endpoint: '/createCohostRequest',
          params: { targetUserId: cohostId },
        }));

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, endpoint: '/createEvent', payload, cohostsResolved: cohostIds.length, cohostInvites, ...(opts['repeat'] ? { series: { repeat: opts['repeat'], count: opts['count'] } } : {}) });
          return;
        }

        // Series creation: --repeat weekly --count 4
        if (opts['repeat'] && opts['count'] && (opts['count'] as number) > 1) {
          const results: unknown[] = [];
          const intervals: Record<string, number> = { daily: 1, weekly: 7, biweekly: 14 };
          for (let i = 0; i < (opts['count'] as number); i++) {
            const d = new Date(startDate);
            if (opts['repeat'] === 'monthly') {
              d.setMonth(d.getMonth() + i);
            } else {
              const days = intervals[opts['repeat'] as string];
              if (!days) { jsonError(`Unknown repeat: ${opts['repeat']}. Use: daily, weekly, biweekly, monthly`, 3, 'validation_error'); return; }
              d.setDate(d.getDate() + (i * days));
            }
            const seriesEvent = { ...event, startDate: d.toISOString() };
            const seriesPayload = makePayload(config, { event: seriesEvent, cohostIds: [] });
            try {
              const res = await apiRequest('POST', '/createEvent', token, seriesPayload, globalOpts['verbose'] as boolean | undefined) as { result?: { data?: string | { id?: string }; eventId?: string } };
              const data = res.result?.data;
              const id = typeof data === 'string' ? data : data?.id ?? res.result?.eventId;
              if (!id) throw new Error('Partiful did not return an event ID');
              const inviteResults = await inviteCohostBatch(
                id, cohostIds, [], makeCohostCall(token, config, globalOpts['verbose'] as boolean | undefined),
              );
              if (inviteResults.failed.length > 0) process.exitCode = 1;
              results.push({ index: i + 1, status: 'created', title: opts['title'], date: d.toISOString(), id, cohostInvites: inviteResults, url: `https://partiful.com/e/${id}` });
              process.stderr.write(`[${i + 1}/${opts['count']}] Created: ${opts['title']} (${d.toLocaleDateString()})\n`);
            } catch (err) {
              results.push({ index: i + 1, status: 'error', title: opts['title'], date: d.toISOString(), error: (err as Error).message });
            }
            if (i < (opts['count'] as number) - 1) await new Promise(r => setTimeout(r, 1000));
          }
          jsonOutput(results, { total: results.length, repeat: opts['repeat'] });
          return;
        }

        const result = await apiRequest('POST', '/createEvent', token, payload, globalOpts['verbose'] as boolean | undefined) as { result?: { data?: string | { id?: string }; eventId?: string } };
        const data = result.result?.data;
        const newEventId = typeof data === 'string' ? data : data?.id ?? result.result?.eventId;
        if (!newEventId) throw new Error('Partiful did not return an event ID');
        const inviteResults = await inviteCohostBatch(
          newEventId, cohostIds, [], makeCohostCall(token, config, globalOpts['verbose'] as boolean | undefined),
        );

        if (inviteResults.failed.length > 0) process.exitCode = 1;
        jsonOutput({
          id: newEventId,
          title: opts['title'],
          startDate: startDate.toISOString(),
          cohostInvites: inviteResults,
          url: `https://partiful.com/e/${newEventId}`,
        });
      } catch (e) {
        handleError(e);
      }
    });

  events
    .command('update')
    .description('Update an existing event via Firestore')
    .argument('<eventId>', 'Event ID')
    .option('--title <title>', 'New title')
    .option('--date <date>', 'New start date/time')
    .option('--end-date <endDate>', 'New end date/time')
    .option('--location <location>', 'New location')
    .option('--description <desc>', 'New description')
    .option('--capacity <n>', 'New guest limit', parseInt)
    .option('--rsvp-deadline <date>', 'Close RSVPs at this date/time')
    .option('--timezone <tz>', 'Timezone for natural-language dates', 'America/Los_Angeles')
    .option('--poster <posterId>', 'Set poster by ID')
    .option('--poster-search <query>', 'Search and set best matching poster')
    .option('--image <path>', 'Upload and set custom image')
    .option('--link <url...>', 'Link URL (repeatable)')
    .option('--link-text <text...>', 'Display text for link (paired with --link by position)')
    .option('--cohost <names...>', 'Co-host names (resolved from contacts)')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const fields: Record<string, unknown> = {};
        const updateFields: string[] = [];
        let cohostIds: string[] = [];
        let currentCohostIds: string[] = [];
        let cohostStates: ReturnType<typeof mergeCohostState> = [];

        if (opts['title']) { fields['title'] = { stringValue: opts['title'] }; updateFields.push('title'); }
        if (opts['location']) { fields['location'] = { stringValue: opts['location'] }; updateFields.push('location'); }
        if (opts['description']) { fields['description'] = { stringValue: stripMarkdown(opts['description'] as string) }; updateFields.push('description'); }
        if (opts['date']) { fields['startDate'] = { timestampValue: parseDateTime(opts['date'] as string, opts['timezone'] as string).toISOString() }; updateFields.push('startDate'); }
        if (opts['endDate']) { fields['endDate'] = { timestampValue: parseDateTime(opts['endDate'] as string, opts['timezone'] as string).toISOString() }; updateFields.push('endDate'); }
        if (opts['capacity']) { fields['guestLimit'] = { integerValue: String(opts['capacity']) }; updateFields.push('guestLimit'); }
        if (opts['rsvpDeadline']) {
          fields['rsvpDeadline'] = { timestampValue: parseDateTime(opts['rsvpDeadline'] as string, opts['timezone'] as string).toISOString() };
          fields['allowResponsesAfterRsvpDeadline'] = { booleanValue: false };
          updateFields.push('rsvpDeadline', 'allowResponsesAfterRsvpDeadline');
        }

        // Links
        const links = buildLinks(opts['link'] as string[] | undefined, opts['linkText'] as string[] | undefined);
        if (links) {
          fields['links'] = {
            arrayValue: {
              values: links.map((l: unknown) => ({
                mapValue: { fields: toFirestoreMap(l as Record<string, unknown>) }
              }))
            }
          };
          updateFields.push('links');
        }

        // Image options (mutually exclusive)
        validateImageOptions(opts['poster'], opts['posterSearch'], opts['image']);

        if (opts['poster'] || opts['posterSearch']) {
          const posterImage = await resolvePosterImage(opts, fetchCatalog, searchPosters, buildPosterImage);
          fields['image'] = { mapValue: { fields: toFirestoreMap(posterImage as unknown as Record<string, unknown>) } };
          updateFields.push('image');
        }

        if (opts['image']) {
          const imageObj = await resolveUploadImage(opts['image'] as string, token, config, globalOpts['verbose'] as boolean | undefined, globalOpts['dryRun'] as boolean | undefined);
          fields['image'] = { mapValue: { fields: toFirestoreMap(imageObj as Record<string, unknown>) } };
          updateFields.push('image');
        }

        if (opts['cohost'] && (opts['cohost'] as string[]).length > 0) {
          const [resolvedIds, requests, existingIds] = await Promise.all([
            resolveCohostNames(opts['cohost'] as string[], token, config, globalOpts['verbose'] as boolean | undefined),
            getCohostRequests(eventId, token, globalOpts['verbose'] as boolean | undefined),
            getCohostIds(eventId, token, globalOpts['verbose'] as boolean | undefined),
          ]);
          cohostIds = resolvedIds;
          currentCohostIds = existingIds;
          cohostStates = mergeCohostState(requests, existingIds);
        }

        if (updateFields.length === 0 && cohostIds.length === 0) {
          jsonError('No update fields provided. Use --title, --location, --description, --date, --end-date, --capacity, --rsvp-deadline, --link, --poster, --poster-search, --image, or --cohost', 3, 'validation_error');
          return;
        }

        const cohostInvites = cohostIds.map((cohostId) => {
          const state = cohostStates.find((item) => item.userId === cohostId);
          return {
            cohostId,
            currentStatus: state?.status ?? null,
            endpoints: state?.status === 'pending' || state?.status === 'accepted'
              ? []
              : state?.status === 'stale'
                ? ['Firestore PATCH cohostIds (remove stale ID)', '/createCohostRequest']
                : ['/createCohostRequest'],
          };
        });
        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, eventId, fields: updateFields, body: { fields }, cohostInvites });
          return;
        }

        if (updateFields.length > 0) {
          await firestoreRequest('PATCH', eventId, { fields }, token, updateFields, globalOpts['verbose'] as boolean | undefined);
        }
        const call = makeCohostCall(token, config, globalOpts['verbose'] as boolean | undefined);
        const inviteResults = await inviteCohostBatch(
          eventId,
          cohostIds,
          cohostStates,
          call,
          async (cohostId) => {
            currentCohostIds = currentCohostIds.filter((id) => id !== cohostId);
            await setCohostIds(eventId, currentCohostIds, token, globalOpts['verbose'] as boolean | undefined);
          },
        );

        if (inviteResults.failed.length > 0) process.exitCode = 1;
        jsonOutput({
          id: eventId,
          updated: updateFields,
          cohostInvites: inviteResults,
          url: `https://partiful.com/e/${eventId}`,
        });
      } catch (e) {
        handleError(e);
      }
    });

  events
    .command('clone')
    .description('Clone an existing event with a new date')
    .argument('<eventId>', 'Source event ID')
    .option('--date <date>', 'New event date')
    .option('--shift <days>', 'Shift source date by N days (default 7)')
    .option('--end-date <endDate>', 'End date/time (overrides duration preservation)')
    .option('--title <title>', 'Override title')
    .option('--location <location>', 'Override location name')
    .option('--address <address>', 'Override street address')
    .option('--description <desc>', 'Override description')
    .option('--capacity <n>', 'Override guest limit', parseInt)
    .option('--private', 'Make event private')
    .option('--timezone <tz>', 'Override timezone')
    .option('--theme <theme>', 'Override color theme')
    .option('--effect <effect>', 'Override visual effect')
    .option('--poster <posterId>', 'Override with built-in poster ID')
    .option('--poster-search <query>', 'Override with poster search')
    .option('--image <path>', 'Override with custom image')
    .option('--link <url...>', 'Override links (repeatable)')
    .option('--link-text <text...>', 'Display text for links')
    .option('--cohost <names...>', 'Co-host names (resolved from contacts)')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // 1. Fetch source event
        let sourceEvent: Record<string, unknown> | null;
        try {
          const result = await apiRequest('POST', '/getEventInfo', token, makePayload(config, { eventId }), globalOpts['verbose'] as boolean | undefined) as { result?: { data?: { event?: Record<string, unknown> } } };
          sourceEvent = result.result?.data?.event ?? null;
        } catch (e) {
          if (!globalOpts['dryRun']) throw e;
          sourceEvent = null;
        }

        if (!sourceEvent && !globalOpts['dryRun']) {
          jsonError('Source event not found', 4, 'not_found');
          return;
        }

        const src = sourceEvent ?? {};

        // 2. Parse new date and preserve duration
        const tz = (opts['timezone'] ?? src['timezone'] ?? 'America/Los_Angeles') as string;
        const newStart = resolveCloneStartDate(opts, src['startDate'], tz);
        let newEnd: Date | null = null;

        if (opts['endDate']) {
          newEnd = parseDateTime(opts['endDate'] as string, tz);
        } else if (src['startDate'] && src['endDate']) {
          const durationMs = new Date(src['endDate'] as string).getTime() - new Date(src['startDate'] as string).getTime();
          if (durationMs > 0) newEnd = new Date(newStart.getTime() + durationMs);
        }

        // 3. Build cloned event — merge source with overrides
        const srcDisplaySettings = src['displaySettings'] as Record<string, unknown> | undefined;
        const cloneOpts = {
          title: (opts['title'] ?? src['title'] ?? 'Untitled Event') as string,
          date: newStart.toISOString(),
          timezone: tz,
          theme: (opts['theme'] ?? srcDisplaySettings?.['theme'] ?? 'oxblood') as string,
          effect: (opts['effect'] ?? srcDisplaySettings?.['effect'] ?? 'sunbeams') as string,
          titleFont: (srcDisplaySettings?.['titleFont'] ?? 'display') as string,
          private: opts['private'] ? true : (src['visibility'] === 'private'),
          location: opts['location'] !== undefined ? opts['location'] : src['location'],
          address: opts['address'] !== undefined ? opts['address'] : src['address'],
          description: opts['description'] !== undefined ? opts['description'] : src['description'],
          capacity: opts['capacity'] !== undefined ? opts['capacity'] : src['guestLimit'],
        };

        const { event } = buildBaseEvent(cloneOpts as unknown as EventOptions);

        // Preserve source boolean settings
        for (const key of ['showHostList', 'showGuestCount', 'showGuestList', 'showActivityTimestamps',
          'displayInviteButton', 'allowGuestPhotoUpload', 'enableGuestReminders', 'rsvpsEnabled',
          'allowGuestsToInviteMutuals', 'rsvpButtonGlyphType']) {
          if (src[key] !== undefined) event[key] = src[key];
        }

        if (newEnd) event['endDate'] = newEnd.toISOString();

        // Links
        const links = buildLinks(opts['link'] as string[] | undefined, opts['linkText'] as string[] | undefined);
        if (links) event['links'] = links;
        else if (src['links']) event['links'] = src['links'] as import('../lib/api/endpoints.js').EventLink[];

        // Image handling
        validateImageOptions(opts['poster'], opts['posterSearch'], opts['image']);

        const posterImage = await resolvePosterImage(opts, fetchCatalog, searchPosters, buildPosterImage);
        if (posterImage) {
          event['image'] = posterImage;
        } else if (opts['image']) {
          event['image'] = await resolveUploadImage(opts['image'] as string, token, config, globalOpts['verbose'] as boolean | undefined, globalOpts['dryRun'] as boolean | undefined);
        } else if (src['image']) {
          event['image'] = src['image'];
        }

        const cohostIds = await resolveCohostNames((opts['cohost'] as string[] | undefined) ?? [], token, config, globalOpts['verbose'] as boolean | undefined);
        const payload = makePayload(config, { event, cohostIds: [] });
        const cohostInvites = cohostIds.map((cohostId) => ({
          endpoint: '/createCohostRequest',
          params: { targetUserId: cohostId },
        }));

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, endpoint: '/createEvent', clonedFrom: eventId, payload, cohostInvites });
          return;
        }

        const result = await apiRequest('POST', '/createEvent', token, payload, globalOpts['verbose'] as boolean | undefined) as { result?: { data?: string | { id?: string }; eventId?: string } };
        const data = result.result?.data;
        const newEventId = typeof data === 'string' ? data : data?.id ?? result.result?.eventId;
        if (!newEventId) throw new Error('Partiful did not return an event ID');
        const inviteResults = await inviteCohostBatch(
          newEventId, cohostIds, [], makeCohostCall(token, config, globalOpts['verbose'] as boolean | undefined),
        );

        if (inviteResults.failed.length > 0) process.exitCode = 1;
        jsonOutput({
          id: newEventId,
          clonedFrom: eventId,
          title: event['title'],
          startDate: newStart.toISOString(),
          cohostInvites: inviteResults,
          url: `https://partiful.com/e/${newEventId}`,
        });
      } catch (e) {
        handleError(e);
      }
    });

  events
    .command('cancel')
    .description('Cancel an event')
    .argument('<eventId>', 'Event ID')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // Confirm unless --yes or --force
        if (!globalOpts['yes'] && !globalOpts['force']) {
          const eventResult = await apiRequest('POST', '/getEventInfo', token, makePayload(config, { eventId }), globalOpts['verbose'] as boolean | undefined) as { result?: { data?: { event?: Record<string, unknown> } } };
          const event = eventResult.result?.data?.event;
          if (event) {
            const counts = event['guestStatusCounts'] as Record<string, number> | undefined;
            const going = counts?.['GOING'] ?? 0;
            const maybe = counts?.['MAYBE'] ?? 0;
            console.error(`About to cancel: "${event['title']}" (${going} going, ${maybe} maybe)`);
          }

          const confirmed = await confirm('Are you sure? This cannot be undone.');
          if (!confirmed) {
            jsonOutput({ cancelled: false, message: 'Aborted by user' });
            return;
          }
        }

        const payload = makePayload(config, { eventId });

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, endpoint: '/cancelEvent', payload });
          return;
        }

        await apiRequest('POST', '/cancelEvent', token, payload, globalOpts['verbose'] as boolean | undefined);
        jsonOutput({ id: eventId, cancelled: true });
      } catch (e) {
        handleError(e);
      }
    });
}
