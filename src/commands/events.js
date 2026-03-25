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
        if (typeof v === 'number') {
          if (Number.isInteger(v)) return { integerValue: String(v) };
          return { doubleValue: v };
        }
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
    .option('--link <url...>', 'Link URL (repeatable)')
    .option('--link-text <text...>', 'Display text for link (paired with --link by position)')
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

        // Validate image extension early (before dry-run check) — skip for URLs
        const isImageUrl = opts.image && (opts.image.startsWith('http://') || opts.image.startsWith('https://'));
        if (opts.image && !isImageUrl) {
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

        if (opts.link && opts.link.length > 0) {
          event.links = opts.link.map((url, i) => ({
            url,
            text: opts.linkText?.[i] || url,
          }));
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
            if (isImageUrl) {
              event.image = { source: 'upload', url: opts.image, note: 'URL will be downloaded and uploaded on real run' };
            } else {
              event.image = { source: 'upload', file: opts.image, note: 'File will be uploaded on real run' };
            }
          } else if (isImageUrl) {
            const { downloadToTemp, uploadEventImage, buildUploadImage } = await import('../lib/upload.js');
            const { basename } = await import('path');
            const { tempPath, cleanup } = await downloadToTemp(opts.image);
            try {
              const uploadData = await uploadEventImage(tempPath, token, config, globalOpts.verbose);
              event.image = buildUploadImage(uploadData, basename(tempPath));
            } finally {
              cleanup();
            }
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
    .option('--link <url...>', 'Link URL (repeatable)')
    .option('--link-text <text...>', 'Display text for link (paired with --link by position)')
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

        if (opts.link && opts.link.length > 0) {
          const links = opts.link.map((url, i) => ({
            url,
            text: opts.linkText?.[i] || url,
          }));
          fields.links = {
            arrayValue: {
              values: links.map(l => ({
                mapValue: { fields: toFirestoreMap(l) }
              }))
            }
          };
          updateFields.push('links');
        }

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
          const isImageUrl = opts.image.startsWith('http://') || opts.image.startsWith('https://');
          if (!isImageUrl) {
            const ext = pathExtname(opts.image).toLowerCase();
            const allowed = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.avif'];
            if (!allowed.includes(ext)) {
              jsonError(`Unsupported image type: "${ext}". Allowed: ${allowed.join(', ')}`, 3, 'validation_error');
              return;
            }
          }

          if (globalOpts.dryRun) {
            if (isImageUrl) {
              fields.image = { mapValue: { fields: toFirestoreMap({ source: 'upload', url: opts.image, note: 'URL will be downloaded and uploaded on real run' }) } };
            } else {
              fields.image = { mapValue: { fields: {} } };
            }
            updateFields.push('image');
          } else if (isImageUrl) {
            const { downloadToTemp, uploadEventImage, buildUploadImage } = await import('../lib/upload.js');
            const { tempPath, cleanup } = await downloadToTemp(opts.image);
            try {
              const uploadData = await uploadEventImage(tempPath, token, config, globalOpts.verbose);
              const imageObj = buildUploadImage(uploadData, pathBasename(tempPath));
              fields.image = { mapValue: { fields: toFirestoreMap(imageObj) } };
              updateFields.push('image');
            } finally {
              cleanup();
            }
          } else {
            const { uploadEventImage, buildUploadImage } = await import('../lib/upload.js');
            const uploadData = await uploadEventImage(opts.image, token, config, globalOpts.verbose);
            const imageObj = buildUploadImage(uploadData, pathBasename(opts.image));
            fields.image = { mapValue: { fields: toFirestoreMap(imageObj) } };
            updateFields.push('image');
          }
        }

        if (updateFields.length === 0) {
          jsonError('No fields to update. Use --title, --location, --description, --date, --end-date, --capacity, --link, --poster, --poster-search, or --image', 3, 'validation_error');
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
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // 1. Fetch source event
        const getPayload = {
          data: wrapPayload(config, {
            params: { eventId },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        let sourceEvent;
        if (globalOpts.dryRun) {
          // In dry-run, still try to fetch the event for field extraction
          try {
            const result = await apiRequest('POST', '/getEvent', token, getPayload, globalOpts.verbose);
            sourceEvent = result.result?.data?.event;
          } catch {
            // If fetch fails in dry-run, use a placeholder
            sourceEvent = null;
          }
        } else {
          const result = await apiRequest('POST', '/getEvent', token, getPayload, globalOpts.verbose);
          sourceEvent = result.result?.data?.event;
        }

        if (!sourceEvent && !globalOpts.dryRun) {
          jsonError('Source event not found', 4, 'not_found');
          return;
        }

        // Use empty object if source not available in dry-run
        const src = sourceEvent || {};

        // 2. Parse new date and preserve duration
        const tz = opts.timezone || src.timezone || 'America/Los_Angeles';
        const newStart = parseDateTime(opts.date, tz);
        let newEnd = null;

        if (opts.endDate) {
          newEnd = parseDateTime(opts.endDate, tz);
        } else if (src.startDate && src.endDate) {
          const srcStart = new Date(src.startDate);
          const srcEnd = new Date(src.endDate);
          const durationMs = srcEnd.getTime() - srcStart.getTime();
          if (durationMs > 0) {
            newEnd = new Date(newStart.getTime() + durationMs);
          }
        }

        // 3. Build cloned event payload
        const event = {
          title: opts.title || src.title || 'Untitled Event',
          startDate: newStart.toISOString(),
          timezone: tz,
          displaySettings: {
            theme: opts.theme || src.displaySettings?.theme || 'oxblood',
            effect: opts.effect || src.displaySettings?.effect || 'sunbeams',
            titleFont: src.displaySettings?.titleFont || 'display',
          },
          showHostList: true,
          showGuestCount: true,
          showGuestList: true,
          showActivityTimestamps: true,
          displayInviteButton: true,
          visibility: opts.private ? 'private' : (src.visibility || 'public'),
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

        if (newEnd) event.endDate = newEnd.toISOString();

        // Copy location fields
        const loc = opts.location !== undefined ? opts.location : src.location;
        if (loc) event.location = loc;
        const addr = opts.address !== undefined ? opts.address : src.address;
        if (addr) event.address = addr;

        // Copy description
        const desc = opts.description !== undefined ? stripMarkdown(opts.description) : src.description;
        if (desc) event.description = desc;

        // Copy capacity
        const cap = opts.capacity !== undefined ? opts.capacity : src.guestLimit;
        if (cap) {
          event.guestLimit = cap;
          event.enableWaitlist = true;
        }

        // Copy links
        if (opts.link && opts.link.length > 0) {
          event.links = opts.link.map((url, i) => ({
            url,
            text: opts.linkText?.[i] || url,
          }));
        } else if (src.links) {
          event.links = src.links;
        }

        // Copy image from source (unless overridden)
        const imageOptCount = [opts.poster, opts.posterSearch, opts.image].filter(Boolean).length;
        if (imageOptCount > 1) {
          jsonError('Use only one of --poster, --poster-search, or --image.', 3, 'validation_error');
          return;
        }

        if (opts.poster) {
          const catalog = await fetchCatalog();
          const poster = catalog.find(p => p.id === opts.poster);
          if (!poster) {
            jsonError(`Poster not found: "${opts.poster}".`, 4, 'not_found');
            return;
          }
          event.image = buildPosterImage(poster);
        } else if (opts.posterSearch) {
          const catalog = await fetchCatalog();
          const results = searchPosters(catalog, opts.posterSearch);
          if (results.length === 0) {
            jsonError(`No posters found matching "${opts.posterSearch}".`, 4, 'not_found');
            return;
          }
          event.image = buildPosterImage(results[0]);
        } else if (opts.image) {
          const isImageUrl = opts.image.startsWith('http://') || opts.image.startsWith('https://');
          if (!isImageUrl) {
            const ext = pathExtname(opts.image).toLowerCase();
            const allowed = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.avif'];
            if (!allowed.includes(ext)) {
              jsonError(`Unsupported image type "${ext}".`, 3, 'validation_error');
              return;
            }
          }
          if (globalOpts.dryRun) {
            event.image = isImageUrl
              ? { source: 'upload', url: opts.image, note: 'URL will be downloaded and uploaded on real run' }
              : { source: 'upload', file: opts.image, note: 'File will be uploaded on real run' };
          } else if (isImageUrl) {
            const { downloadToTemp, uploadEventImage, buildUploadImage } = await import('../lib/upload.js');
            const { tempPath, cleanup } = await downloadToTemp(opts.image);
            try {
              const uploadData = await uploadEventImage(tempPath, token, config, globalOpts.verbose);
              event.image = buildUploadImage(uploadData, pathBasename(tempPath));
            } finally {
              cleanup();
            }
          } else {
            const { uploadEventImage, buildUploadImage } = await import('../lib/upload.js');
            const uploadData = await uploadEventImage(opts.image, token, config, globalOpts.verbose);
            event.image = buildUploadImage(uploadData, pathBasename(opts.image));
          }
        } else if (src.image) {
          event.image = src.image;
        }

        // 4. Build API payload
        const payload = {
          data: wrapPayload(config, {
            params: { event, cohostIds: [] },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, endpoint: '/createEvent', clonedFrom: eventId, payload });
          return;
        }

        // 5. Create the event
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
