/**
 * Clone helper: +clone <eventId> — clone an event with shifted date
 */

import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { parseDateTime, stripMarkdown } from '../lib/dates.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

export function registerCloneHelper(program) {
  program
    .command('+clone')
    .description('Clone an event with a new date')
    .argument('<eventId>', 'Source event ID')
    .option('--title <title>', 'Override event title')
    .option('--date <date>', 'New date/time for cloned event')
    .option('--shift <days>', 'Shift date forward N days (default 7)', '7')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // Fetch source event
        const getPayload = {
          data: wrapPayload(config, {
            params: { eventId },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        const getResult = await apiRequest('POST', '/getEvent', token, getPayload, globalOpts.verbose);
        const source = getResult.result?.data?.event;

        if (!source) {
          jsonError('Source event not found', 4, 'not_found');
          return;
        }

        // Determine new start date
        let startDate;
        if (opts.date) {
          startDate = parseDateTime(opts.date, source.timezone || 'America/Los_Angeles');
        } else if (source.startDate) {
          startDate = new Date(source.startDate);
          startDate.setDate(startDate.getDate() + parseInt(opts.shift));
        } else {
          jsonError('Source event has no start date and --date not provided', 3, 'validation_error');
          return;
        }

        // Preserve duration for end date
        let endDate = null;
        if (source.endDate && source.startDate) {
          const durationMs = new Date(source.endDate) - new Date(source.startDate);
          if (durationMs > 0) {
            endDate = new Date(startDate.getTime() + durationMs);
          }
        }

        const title = opts.title || source.title;
        const event = {
          title,
          startDate: startDate.toISOString(),
          timezone: source.timezone || 'America/Los_Angeles',
          displaySettings: source.displaySettings || {},
          showHostList: true,
          showGuestCount: true,
          showGuestList: true,
          showActivityTimestamps: true,
          displayInviteButton: true,
          visibility: source.visibility || 'public',
          allowGuestPhotoUpload: true,
          enableGuestReminders: true,
          rsvpsEnabled: true,
          allowGuestsToInviteMutuals: true,
          rsvpButtonGlyphType: 'emojis',
          status: 'UNSAVED',
          guestStatusCounts: {},
        };

        if (endDate) event.endDate = endDate.toISOString();
        if (source.location) event.location = source.location;
        if (source.address) event.address = source.address;
        if (source.description) event.description = stripMarkdown(source.description);
        if (source.guestLimit) {
          event.guestLimit = source.guestLimit;
          event.enableWaitlist = true;
        }

        const createPayload = {
          data: wrapPayload(config, {
            params: { event, cohostIds: [] },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, source: eventId, event });
          return;
        }

        const result = await apiRequest('POST', '/createEvent', token, createPayload, globalOpts.verbose);
        const newEventId = result.result?.data || result.result?.eventId;

        jsonOutput({
          id: newEventId,
          title,
          startDate: startDate.toISOString(),
          clonedFrom: eventId,
          url: `https://partiful.com/e/${newEventId}`,
        });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });
}
