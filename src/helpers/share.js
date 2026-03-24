/**
 * Share helper: +share <eventId> — generate shareable event link
 */

import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

export function registerShareHelper(program) {
  program
    .command('+share')
    .description('Generate shareable event link')
    .argument('<eventId>', 'Event ID')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // Fetch event for title
        const payload = {
          data: wrapPayload(config, {
            params: { eventId },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        const result = await apiRequest('POST', '/getEvent', token, payload, globalOpts.verbose);
        const event = result.result?.data?.event;

        const title = event?.title || 'Unknown Event';
        const url = `https://partiful.com/e/${eventId}`;

        jsonOutput({ url, eventId, title });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });
}
