/**
 * Share helper: +share <eventId> — generate shareable event link
 */

import { Command } from 'commander';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

export function registerShareHelper(program: Command): void {
  program
    .command('+share')
    .description('Generate shareable event link')
    .argument('<eventId>', 'Event ID')
    .action(async (eventId: string, _opts: Record<string, unknown>, _cmd: Command) => {
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

        const result = await apiRequest('POST', '/getEventInfo', token, payload, false) as Record<string, unknown>;
        const data = (result as Record<string, unknown>)['result'] as Record<string, unknown> | undefined;
        const eventData = data?.['data'] as Record<string, unknown> | undefined;
        const event = eventData?.['event'] as Record<string, unknown> | undefined;

        const title = (event?.['title'] as string | undefined) ?? 'Unknown Event';
        const url = `https://partiful.com/e/${eventId}`;

        jsonOutput({ url, eventId, title });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError((e as Error).message);
      }
    });
}
