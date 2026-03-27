/**
 * Blasts commands: send, list
 *
 * Endpoint: POST https://api.partiful.com/createTextBlast
 * Discovered via browser interception 2026-03-24.
 * See docs/research/2026-03-24-text-blast-endpoint.md
 */

import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError, ValidationError } from '../lib/errors.js';
import { confirm } from '../lib/events.js';

const VALID_TO_VALUES = ['GOING', 'MAYBE', 'DECLINED', 'SENT', 'INTERESTED', 'WAITLIST', 'APPROVED', 'RESPONDED_TO_FIND_A_TIME'];
const MAX_MESSAGE_LENGTH = 480;

export function registerBlastsCommands(program) {
  const blasts = program.command('blasts').description('Text blasts to event guests');

  blasts
    .command('send')
    .description('Send a text blast to event guests')
    .argument('<eventId>', 'Event ID')
    .requiredOption('--message <msg>', 'Message to send (max 480 chars)')
    .option('--to <statuses>', 'Comma-separated guest statuses to send to (default: GOING)', 'GOING')
    .option('--show-on-event-page', 'Show blast in event activity feed (default: true)')
    .option('--no-show-on-event-page', 'Hide blast from event activity feed')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();

      try {
        // Validate message length
        if (opts.message.length > MAX_MESSAGE_LENGTH) {
          throw new ValidationError(`Message exceeds ${MAX_MESSAGE_LENGTH} char limit (got ${opts.message.length})`);
        }

        // Parse and validate 'to' statuses
        const toStatuses = opts.to.split(',').map(s => s.trim().toUpperCase());
        for (const status of toStatuses) {
          if (!VALID_TO_VALUES.includes(status)) {
            throw new ValidationError(
              `Invalid status "${status}". Valid: ${VALID_TO_VALUES.join(', ')}`
            );
          }
        }

        // Default showOnEventPage to true unless explicitly disabled
        const showOnEventPage = opts.showOnEventPage !== false;

        const config = loadConfig();
        const token = await getValidToken(config);

        const payload = {
          data: wrapPayload(config, {
            params: {
              eventId,
              message: {
                text: opts.message,
                to: toStatuses,
                showOnEventPage,
              },
            },
            amplitudeSessionId: Date.now(),
            userId: config.userId || null,
          }),
        };

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, endpoint: '/createTextBlast', payload }, {}, globalOpts);
          return;
        }

        // Safety confirmation unless --yes
        if (!globalOpts.yes) {
          console.error(`\nText Blast Preview:`);
          console.error(`  Event: ${eventId}`);
          console.error(`  To: ${toStatuses.join(', ')}`);
          console.error(`  Show on event page: ${showOnEventPage}`);
          console.error(`  Message: "${opts.message}"`);
          console.error('');
          const ok = await confirm('Send this text blast? This will SMS real people');
          if (!ok) {
            jsonOutput({ cancelled: true, message: 'Blast not sent' }, {}, globalOpts);
            return;
          }
        }

        const result = await apiRequest('POST', '/createTextBlast', token, payload, globalOpts.verbose);

        jsonOutput({
          sent: true,
          eventId,
          to: toStatuses,
          messageLength: opts.message.length,
          showOnEventPage,
          response: result?.result?.data || result?.result || result,
        }, {}, globalOpts);
      } catch (err) {
        if (err instanceof PartifulError) {
          jsonError(err.message, err.exitCode, err.type, err.details);
        } else {
          jsonError(err.message, 1, 'blast_error');
        }
      }
    });
}
