/**
 * Events commands: list, get, create, update, cancel
 */

import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { fetchCatalog, searchPosters, buildPosterImage } from '../lib/posters.js';
import { apiRequest, firestoreRequest } from '../lib/http.js';
import { parseDateTime, stripMarkdown, formatDate } from '../lib/dates.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError, ValidationError } from '../lib/errors.js';
import { extname as pathExtname, basename as pathBasename } from 'path';
import readline from 'readline';

async function confirm(question) {
  const rl = readline.createInterface({ input: process.stdin, output: process.stderr });
  return new Promise(resolve => {
    rl.question(question + ' [y/N]: ', answer => {
      rl.close();
      resolve(answer.toLowerCase() === 'y' || answer.toLowerCase() === 'yes');
    });
  });
}

function toFirestoreMap(obj) {
  const fields = {};
  for (const [key, value] of Object.entries(obj)) {
    if (value === null || value === undefined) continue;
    if (typeof value === 'string') fields[key] = { stringValue: value };
    else if (typeof value === 'number') {
      if (Number.isInteger(value)) fields[key] = { integerValue: String(value) };
      else fields[key] = { doubleValue: value };
    }
    else if (typeof value === 'boolean') fields[key] = { booleanValue: value };
    else if (Array.isArray(value)) {
      fields[key] = { arrayValue: { values: value.map(v => {
        if (typeof v === 'string') return { stringValue: v };
        if (typeof v === 'number') return { integerValue: String(v) };
        if (typeof v === 'object') return { mapValue: { fields: toFirestoreMap(v) } };
        return { stringValue: String(v) };
      })}};
    }
    else if (typeof value === 'object') {
      fields[key] = { mapValue: { fields: toFirestoreMap(value) } };
    }
  }
  return fields;
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

        const payload = {
          data: wrapPayload(config, {
            params: {},
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

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
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
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

        const payload = {
          data: wrapPayload(config, {
            params: { eventId },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

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
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });

  events
    .command('create')
    .description('Create a new event')
    .requiredOption('--title <title>', 'Event title')
    .requiredOption('--date <date>', 'Start date/time (e.g. "2026-04-01 7pm")')
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
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const imageOptCount = [opts.poster, opts.posterSearch, opts.image].filter(Boolean).length;
        if (imageOptCount > 1) {
          jsonError('Use only one of --poster, --poster-search, or --image.', 3, 'validation_error');
          return;
        }

        // Validate image extension early (before dry-run check)
        if (opts.image) {
          const { extname } = await import('path');
          const ext = extname(opts.image).toLowerCase();
          const allowed = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.avif'];
          if (!allowed.includes(ext)) {
            jsonError(`Unsupported image type "${ext}". Allowed types: ${allowed.join(', ')}`, 3, 'validation_error');
            return;
          }
        }

        const startDate = parseDateTime(opts.date, opts.timezone);
        const endDate = opts.endDate ? parseDateTime(opts.endDate, opts.timezone) : null;

        const event = {
          title: opts.title,
          startDate: startDate.toISOString(),
          timezone: opts.timezone,
          displaySettings: {
            theme: opts.theme,
            effect: opts.effect,
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
        if (opts.description) event.description = stripMarkdown(opts.description);
        if (opts.capacity) {
          event.guestLimit = opts.capacity;
          event.enableWaitlist = true;
        }

        // Poster image handling
        if (opts.poster) {
          const catalog = await fetchCatalog();
          const poster = catalog.find(p => p.id === opts.poster);
          if (!poster) {
            jsonError(`Poster not found: "${opts.poster}". Use "partiful posters search <term>" to find posters.`, 4, 'not_found');
            return;
          }
          event.image = buildPosterImage(poster);
        } else if (opts.posterSearch) {
          const catalog = await fetchCatalog();
          const results = searchPosters(catalog, opts.posterSearch);
          if (results.length === 0) {
            jsonError(`No posters found matching "${opts.posterSearch}". Try "partiful posters search <term>".`, 4, 'not_found');
            return;
          }
          event.image = buildPosterImage(results[0]);
        } else if (opts.image) {
          if (globalOpts.dryRun) {
            event.image = { source: 'upload', file: opts.image, note: 'File will be uploaded on real run' };
          } else {
            const { uploadEventImage, buildUploadImage } = await import('../lib/upload.js');
            const { basename } = await import('path');
            const uploadData = await uploadEventImage(opts.image, token, config, globalOpts.verbose);
            event.image = buildUploadImage(uploadData, basename(opts.image));
          }
        }

        const payload = {
          data: wrapPayload(config, {
            params: { event, cohostIds: [] },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, endpoint: '/createEvent', payload });
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
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
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

        // Handle image options
        const imageOpts = [opts.poster, opts.posterSearch, opts.image].filter(Boolean).length;
        if (imageOpts > 1) {
          jsonError('Use only one of --poster, --poster-search, or --image.', 3, 'validation_error');
          return;
        }

        if (opts.poster || opts.posterSearch) {
          const { fetchCatalog, searchPosters, buildPosterImage } = await import('../lib/posters.js');
          const catalog = await fetchCatalog();
          let poster;

          if (opts.poster) {
            poster = catalog.find(p => p.id === opts.poster);
            if (!poster) {
              jsonError(`Poster not found: "${opts.poster}". Use "partiful posters search <term>" to find posters.`, 4, 'not_found');
              return;
            }
          } else {
            const results = searchPosters(catalog, opts.posterSearch);
            if (results.length === 0) {
              jsonError(`No posters found matching "${opts.posterSearch}".`, 4, 'not_found');
              return;
            }
            poster = results[0].poster;
          }

          const imageObj = buildPosterImage(poster);
          fields.image = { mapValue: { fields: toFirestoreMap(imageObj) } };
          updateFields.push('image');
        }

        if (opts.image) {
          const ext = pathExtname(opts.image).toLowerCase();
          const allowed = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.avif'];
          if (!allowed.includes(ext)) {
            jsonError(`Unsupported image type: "${ext}". Allowed: ${allowed.join(', ')}`, 3, 'validation_error');
            return;
          }

          if (globalOpts.dryRun) {
            fields.image = { mapValue: { fields: {} } };
            updateFields.push('image');
          } else {
            const { uploadEventImage, buildUploadImage } = await import('../lib/upload.js');
            const uploadData = await uploadEventImage(opts.image, token, config, globalOpts.verbose);
            const imageObj = buildUploadImage(uploadData, pathBasename(opts.image));
            fields.image = { mapValue: { fields: toFirestoreMap(imageObj) } };
            updateFields.push('image');
          }
        }

        if (updateFields.length === 0) {
          jsonError('No fields to update. Use --title, --location, --description, --date, --end-date, --capacity, --poster, --poster-search, or --image', 3, 'validation_error');
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
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
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
          // Fetch event info first
          const getPayload = {
            data: wrapPayload(config, {
              params: { eventId },
              amplitudeSessionId: Date.now(),
              userId: config.userId,
            }),
          };
          const eventResult = await apiRequest('POST', '/getEvent', token, getPayload, globalOpts.verbose);
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

        const payload = {
          data: wrapPayload(config, {
            params: { eventId },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, endpoint: '/cancelEvent', payload });
          return;
        }

        await apiRequest('POST', '/cancelEvent', token, payload, globalOpts.verbose);
        jsonOutput({ id: eventId, cancelled: true });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });
}
