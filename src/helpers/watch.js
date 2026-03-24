/**
 * Watch helper: +watch <eventId> — poll for guest RSVP changes
 */

import { loadConfig, getValidToken } from '../lib/auth.js';
import { fetchGuests } from '../commands/guests.js';
import { jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

export function registerWatchHelper(program) {
  program
    .command('+watch')
    .description('Poll for guest RSVP changes (NDJSON output)')
    .argument('<eventId>', 'Event ID to watch')
    .option('--interval <seconds>', 'Poll interval in seconds', '30')
    .option('--duration <minutes>', 'Total watch duration in minutes', '60')
    .action(async (eventId, opts) => {
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const intervalMs = parseInt(opts.interval) * 1000;
        const durationMs = parseInt(opts.duration) * 60 * 1000;
        const endTime = Date.now() + durationMs;

        let previousSnapshot = {};
        let totalChanges = 0;
        let polls = 0;

        // Initial fetch
        const initialGuests = await fetchGuests(eventId, token, config);
        for (const g of initialGuests) {
          previousSnapshot[g.name] = g.status;
        }
        process.stderr.write(`Watching ${eventId} — ${initialGuests.length} guests, polling every ${opts.interval}s for ${opts.duration}m\n`);

        const poll = async () => {
          if (Date.now() >= endTime) return false;
          polls++;

          const freshToken = await getValidToken(config);
          const guests = await fetchGuests(eventId, freshToken, config);
          const currentSnapshot = {};

          for (const g of guests) {
            currentSnapshot[g.name] = g.status;

            if (previousSnapshot[g.name] && previousSnapshot[g.name] !== g.status) {
              totalChanges++;
              const change = {
                type: 'rsvp_change',
                guest: { name: g.name, count: g.count },
                from: previousSnapshot[g.name],
                to: g.status,
                timestamp: new Date().toISOString(),
              };
              process.stdout.write(JSON.stringify(change) + '\n');
            } else if (!previousSnapshot[g.name]) {
              totalChanges++;
              const change = {
                type: 'new_guest',
                guest: { name: g.name, count: g.count },
                from: null,
                to: g.status,
                timestamp: new Date().toISOString(),
              };
              process.stdout.write(JSON.stringify(change) + '\n');
            }
          }

          previousSnapshot = currentSnapshot;
          return true;
        };

        // Poll loop
        while (Date.now() < endTime) {
          await new Promise(resolve => setTimeout(resolve, intervalMs));
          const shouldContinue = await poll();
          if (!shouldContinue) break;
        }

        // Summary
        process.stderr.write(`\nWatch complete: ${polls} polls, ${totalChanges} change(s) detected\n`);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });
}
