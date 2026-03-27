/**
 * Events commands: list, get, create, update, cancel
 */

import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { resolveCohostNames } from '../lib/cohosts.js';
import { fetchCatalog, searchPosters, buildPosterImage } from '../lib/posters.js';
import { apiRequest, firestoreRequest } from '../lib/http.js';
import { parseDateTime, stripMarkdown } from '../lib/dates.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';
import {
  confirm, buildBaseEvent, buildLinks, toFirestoreMap,
  validateImageOptions, resolvePosterImage, resolveUploadImage,
  isUrl, ALLOWED_IMAGE_EXTENSIONS,
} from '../lib/events.js';

/**
 * Build the standard API payload wrapper.
 */
function makePayload(config, params) {
  return {
    data: wrapPayload(config, {
      params,
      amplitudeSessionId: Date.now(),
      userId: config.userId,
    }),
  };
}

/**
 * Standard error handler for action callbacks.
 */
function handleError(e) {
  if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
  else jsonError(e.message);
}

export function registerEventsCommands(program) {
  const events = program.command('events').description('Manage events');

  events
    .command('list')
    .description('List upcoming (or past) events')
    .option('--past', 'Show past events instead of upcoming')
    .option('--include-cancelled', 'Include cancelled events')
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const endpoint = opts.past
          ? '/getMyPastEventsForHomePage'
          : '/getMyUpcomingEventsForHomePage';

        const payload = makePayload(config, {});

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, endpoint, payload });
          return;
        }

        const result = await apiRequest('POST', endpoint, token, payload, globalOpts.verbose);

        let eventList = opts.past
          ? result.result?.data?.pastEvents
          : result.result?.data?.upcomingEvents;

        if (!opts.includeCancelled && eventList) {
          eventList = eventList.filter(e => e.status !== 'CANCELED');
        }

        const mapped = (eventList || []).map(e => ({
          id: e.id,
          title: e.title,
          startDate: e.startDate,
          endDate: e.endDate || null,
          location: e.location || null,
          status: e.status,
          isHost: e.ownerIds?.includes(config.userId) || false,
          going: e.guestStatusCounts?.GOING || 0,
          maybe: e.guestStatusCounts?.MAYBE || 0,
          url: `https://partiful.com/e/${e.id}`,
        }));

        jsonOutput(mapped, { count: mapped.length, type: opts.past ? 'past' : 'upcoming' });
      } catch (e) {
        handleError(e);
      }
    });

  events
    .command('get')
    .description('Get event details')
    .argument('<eventId>', 'Event ID')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const payload = makePayload(config, { eventId });

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, endpoint: '/getEvent', payload });
          return;
        }

        const result = await apiRequest('POST', '/getEvent', token, payload, globalOpts.verbose);
        const event = result.result?.data?.event;

        if (!event) {
          jsonError('Event not found or no data returned', 4, 'not_found');
          return;
        }

        jsonOutput({
          id: eventId,
          title: event.title,
          startDate: event.startDate,
          endDate: event.endDate || null,
          location: event.location || null,
          address: event.address || null,
          description: event.description || null,
          status: event.status,
          timezone: event.timezone || null,
          visibility: event.visibility || null,
          guestStatusCounts: event.guestStatusCounts || {},
          displaySettings: event.displaySettings || {},
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
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        // Template merging
        if (opts.template) {
          const { loadTemplates, applyVariables, mergeTemplateOpts } = await import('../lib/templates.js');
          const templates = loadTemplates();
          if (!templates[opts.template]) {
            jsonError(`Template "${opts.template}" not found. Use "partiful template list" to see available templates.`, 4, 'not_found');
            return;
          }
          let tpl = templates[opts.template];
          if (opts.var) {
            const vars = {};
            for (const v of opts.var) {
              const eq = v.indexOf('=');
              if (eq > 0) vars[v.slice(0, eq)] = v.slice(eq + 1);
            }
            tpl = applyVariables(tpl, vars);
          }
          const merged = mergeTemplateOpts(tpl, opts);
          Object.assign(opts, merged);
        }

        if (!opts.title) {
          jsonError('--title is required (provide directly or via --template).', 3, 'validation_error');
          return;
        }
        if (!opts.date) {
          jsonError('--date is required (provide directly or via --template).', 3, 'validation_error');
          return;
        }

        const config = loadConfig();
        const token = await getValidToken(config);

        validateImageOptions(opts.poster, opts.posterSearch, opts.image);

        // Validate image extension early (before dry-run check) — skip for URLs
        if (opts.image && !isUrl(opts.image)) {
          const { extname } = await import('path');
          const ext = extname(opts.image).toLowerCase();
          if (!ALLOWED_IMAGE_EXTENSIONS.includes(ext)) {
            jsonError(`Unsupported image type "${ext}". Allowed types: ${ALLOWED_IMAGE_EXTENSIONS.join(', ')}`, 3, 'validation_error');
            return;
          }
        }

        const { event, startDate } = buildBaseEvent(opts);

        // Links
        const links = buildLinks(opts.link, opts.linkText);
        if (links) event.links = links;

        // Poster/image handling
        const posterImage = await resolvePosterImage(opts, fetchCatalog, searchPosters, buildPosterImage);
        if (posterImage) {
          event.image = posterImage;
        } else if (opts.image) {
          event.image = await resolveUploadImage(opts.image, token, config, globalOpts.verbose, globalOpts.dryRun);
        }

        const cohostIds = await resolveCohostNames(opts.cohost, token, config, globalOpts.verbose);

        const payload = makePayload(config, { event, cohostIds });

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, endpoint: '/createEvent', payload, cohostsResolved: cohostIds.length, ...(opts.repeat ? { series: { repeat: opts.repeat, count: opts.count } } : {}) });
          return;
        }

        // Series creation: --repeat weekly --count 4
        if (opts.repeat && opts.count && opts.count > 1) {
          const results = [];
          const intervals = { daily: 1, weekly: 7, biweekly: 14 };
          for (let i = 0; i < opts.count; i++) {
            const d = new Date(startDate);
            if (opts.repeat === 'monthly') {
              d.setMonth(d.getMonth() + i);
            } else {
              const days = intervals[opts.repeat];
              if (!days) { jsonError(`Unknown repeat: ${opts.repeat}. Use: daily, weekly, biweekly, monthly`, 3, 'validation_error'); return; }
              d.setDate(d.getDate() + (i * days));
            }
            const seriesEvent = { ...event, startDate: d.toISOString() };
            const seriesPayload = makePayload(config, { event: seriesEvent, cohostIds });
            try {
              const res = await apiRequest('POST', '/createEvent', token, seriesPayload, globalOpts.verbose);
              const id = res.result?.data || res.result?.eventId;
              results.push({ index: i + 1, status: 'created', title: opts.title, date: d.toISOString(), id, url: `https://partiful.com/e/${id}` });
              process.stderr.write(`[${i + 1}/${opts.count}] Created: ${opts.title} (${d.toLocaleDateString()})\n`);
            } catch (err) {
              results.push({ index: i + 1, status: 'error', title: opts.title, date: d.toISOString(), error: err.message });
            }
            if (i < opts.count - 1) await new Promise(r => setTimeout(r, 1000));
          }
          jsonOutput(results, { total: results.length, repeat: opts.repeat });
          return;
        }

        const result = await apiRequest('POST', '/createEvent', token, payload, globalOpts.verbose);
        const newEventId = result.result?.data || result.result?.eventId;

        jsonOutput({
          id: newEventId,
          title: opts.title,
          startDate: startDate.toISOString(),
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
    .option('--poster <posterId>', 'Set poster by ID')
    .option('--poster-search <query>', 'Search and set best matching poster')
    .option('--image <path>', 'Upload and set custom image')
    .option('--link <url...>', 'Link URL (repeatable)')
    .option('--link-text <text...>', 'Display text for link (paired with --link by position)')
    .option('--cohost <names...>', 'Co-host names (resolved from contacts)')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const fields = {};
        const updateFields = [];

        if (opts.title) { fields.title = { stringValue: opts.title }; updateFields.push('title'); }
        if (opts.location) { fields.location = { stringValue: opts.location }; updateFields.push('location'); }
        if (opts.description) { fields.description = { stringValue: stripMarkdown(opts.description) }; updateFields.push('description'); }
        if (opts.date) { fields.startDate = { timestampValue: parseDateTime(opts.date).toISOString() }; updateFields.push('startDate'); }
        if (opts.endDate) { fields.endDate = { timestampValue: parseDateTime(opts.endDate).toISOString() }; updateFields.push('endDate'); }
        if (opts.capacity) { fields.guestLimit = { integerValue: String(opts.capacity) }; updateFields.push('guestLimit'); }

        // Links
        const links = buildLinks(opts.link, opts.linkText);
        if (links) {
          fields.links = {
            arrayValue: {
              values: links.map(l => ({
                mapValue: { fields: toFirestoreMap(l) }
              }))
            }
          };
          updateFields.push('links');
        }

        // Image options (mutually exclusive)
        validateImageOptions(opts.poster, opts.posterSearch, opts.image);

        if (opts.poster || opts.posterSearch) {
          const posterImage = await resolvePosterImage(opts, fetchCatalog, searchPosters, buildPosterImage);
          fields.image = { mapValue: { fields: toFirestoreMap(posterImage) } };
          updateFields.push('image');
        }

        if (opts.image) {
          const imageObj = await resolveUploadImage(opts.image, token, config, globalOpts.verbose, globalOpts.dryRun);
          fields.image = { mapValue: { fields: toFirestoreMap(imageObj) } };
          updateFields.push('image');
        }

        if (opts.cohost && opts.cohost.length > 0) {
          const resolvedIds = await resolveCohostNames(opts.cohost, token, config, globalOpts.verbose);
          if (resolvedIds.length > 0) {
            fields.cohostIds = {
              arrayValue: { values: resolvedIds.map(id => ({ stringValue: id })) }
            };
            updateFields.push('cohostIds');
          }
        }

        if (updateFields.length === 0) {
          jsonError('No fields to update. Use --title, --location, --description, --date, --end-date, --capacity, --link, --poster, --poster-search, --image, or --cohost', 3, 'validation_error');
          return;
        }

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, eventId, fields: updateFields, body: { fields } });
          return;
        }

        await firestoreRequest('PATCH', eventId, { fields }, token, updateFields, globalOpts.verbose);

        jsonOutput({
          id: eventId,
          updated: updateFields,
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
    .requiredOption('--date <date>', 'New event date (required)')
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
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // 1. Fetch source event
        let sourceEvent;
        try {
          const result = await apiRequest('POST', '/getEvent', token, makePayload(config, { eventId }), globalOpts.verbose);
          sourceEvent = result.result?.data?.event;
        } catch (e) {
          if (!globalOpts.dryRun) throw e;
          sourceEvent = null;
        }

        if (!sourceEvent && !globalOpts.dryRun) {
          jsonError('Source event not found', 4, 'not_found');
          return;
        }

        const src = sourceEvent || {};

        // 2. Parse new date and preserve duration
        const tz = opts.timezone || src.timezone || 'America/Los_Angeles';
        const newStart = parseDateTime(opts.date, tz);
        let newEnd = null;

        if (opts.endDate) {
          newEnd = parseDateTime(opts.endDate, tz);
        } else if (src.startDate && src.endDate) {
          const durationMs = new Date(src.endDate).getTime() - new Date(src.startDate).getTime();
          if (durationMs > 0) newEnd = new Date(newStart.getTime() + durationMs);
        }

        // 3. Build cloned event — merge source with overrides
        const cloneOpts = {
          title: opts.title || src.title || 'Untitled Event',
          date: opts.date,
          timezone: tz,
          theme: opts.theme || src.displaySettings?.theme || 'oxblood',
          effect: opts.effect || src.displaySettings?.effect || 'sunbeams',
          titleFont: src.displaySettings?.titleFont || 'display',
          private: opts.private ? true : (src.visibility === 'private'),
          location: opts.location !== undefined ? opts.location : src.location,
          address: opts.address !== undefined ? opts.address : src.address,
          description: opts.description !== undefined ? opts.description : src.description,
          capacity: opts.capacity !== undefined ? opts.capacity : src.guestLimit,
        };

        const { event } = buildBaseEvent(cloneOpts);

        // Preserve source boolean settings
        for (const key of ['showHostList', 'showGuestCount', 'showGuestList', 'showActivityTimestamps',
          'displayInviteButton', 'allowGuestPhotoUpload', 'enableGuestReminders', 'rsvpsEnabled',
          'allowGuestsToInviteMutuals', 'rsvpButtonGlyphType']) {
          if (src[key] !== undefined) event[key] = src[key];
        }

        if (newEnd) event.endDate = newEnd.toISOString();

        // Links
        const links = buildLinks(opts.link, opts.linkText);
        if (links) event.links = links;
        else if (src.links) event.links = src.links;

        // Image handling
        validateImageOptions(opts.poster, opts.posterSearch, opts.image);

        const posterImage = await resolvePosterImage(opts, fetchCatalog, searchPosters, buildPosterImage);
        if (posterImage) {
          event.image = posterImage;
        } else if (opts.image) {
          event.image = await resolveUploadImage(opts.image, token, config, globalOpts.verbose, globalOpts.dryRun);
        } else if (src.image) {
          event.image = src.image;
        }

        const cohostIds = await resolveCohostNames(opts.cohost, token, config, globalOpts.verbose);

        const payload = makePayload(config, { event, cohostIds });

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, endpoint: '/createEvent', clonedFrom: eventId, payload });
          return;
        }

        const result = await apiRequest('POST', '/createEvent', token, payload, globalOpts.verbose);
        const newEventId = result.result?.data || result.result?.eventId;

        jsonOutput({
          id: newEventId,
          clonedFrom: eventId,
          title: event.title,
          startDate: newStart.toISOString(),
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
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // Confirm unless --yes or --force
        if (!globalOpts.yes && !globalOpts.force) {
          const eventResult = await apiRequest('POST', '/getEvent', token, makePayload(config, { eventId }), globalOpts.verbose);
          const event = eventResult.result?.data?.event;
          if (event) {
            const going = event.guestStatusCounts?.GOING || 0;
            const maybe = event.guestStatusCounts?.MAYBE || 0;
            console.error(`About to cancel: "${event.title}" (${going} going, ${maybe} maybe)`);
          }

          const confirmed = await confirm('Are you sure? This cannot be undone.');
          if (!confirmed) {
            jsonOutput({ cancelled: false, message: 'Aborted by user' });
            return;
          }
        }

        const payload = makePayload(config, { eventId });

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, endpoint: '/cancelEvent', payload });
          return;
        }

        await apiRequest('POST', '/cancelEvent', token, payload, globalOpts.verbose);
        jsonOutput({ id: eventId, cancelled: true });
      } catch (e) {
        handleError(e);
      }
    });
}
