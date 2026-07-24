/**
 * Watch helper: +watch <eventId> — poll for guest RSVP changes
 */

import { Command } from 'commander';
import { loadConfig, getValidToken } from '../lib/auth.js';
import { fetchGuests } from '../commands/guests.js';
import { jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

type GuestLike = Awaited<ReturnType<typeof fetchGuests>>[number];

export function registerWatchHelper(program: Command): void {
  program
    .command('+watch')
    .description('Poll for guest RSVP changes (NDJSON output)')
    .argument('<eventId>', 'Event ID to watch')
    .option('--interval <seconds>', 'Poll interval in seconds', '30')
    .option('--duration <minutes>', 'Total watch duration in minutes', '60')
    .action(async (eventId: string, opts: Record<string, unknown>) => {
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const intervalMs = parseInt(opts['interval'] as string) * 1000;
        const durationMs = parseInt(opts['duration'] as string) * 60 * 1000;
        const endTime = Date.now() + durationMs;

        let previousSnapshot: Record<string, string | undefined> = {};
        let totalChanges = 0;
        let polls = 0;

        // Initial fetch
        const initialGuests: GuestLike[] = await fetchGuests(eventId, token, config);
        for (const g of initialGuests) {
          previousSnapshot[g.name] = g.status;
        }
        process.stderr.write(`Watching ${eventId} — ${initialGuests.length} guests, polling every ${opts['interval']}s for ${opts['duration']}m\n`);

        const poll = async (): Promise<boolean> => {
          if (Date.now() >= endTime) return false;
          polls++;

          const freshToken = await getValidToken(config);
          const guests: GuestLike[] = await fetchGuests(eventId, freshToken, config);
          const currentSnapshot: Record<string, string | undefined> = {};

          for (const g of guests) {
            currentSnapshot[g.name] = g.status;

            if (previousSnapshot[g.name] !== undefined && previousSnapshot[g.name] !== g.status) {
              totalChanges++;
              const change = {
                type: 'rsvp_change',
                guest: { name: g.name, count: g.count },
                from: previousSnapshot[g.name],
                to: g.status,
                timestamp: new Date().toISOString(),
              };
              process.stdout.write(JSON.stringify(change) + '\n');
            } else if (previousSnapshot[g.name] === undefined) {
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
          await new Promise<void>(resolve => setTimeout(resolve, intervalMs));
          const shouldContinue = await poll();
          if (!shouldContinue) break;
        }

        // Summary
        process.stderr.write(`\nWatch complete: ${polls} polls, ${totalChanges} change(s) detected\n`);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError((e as Error).message);
      }
    });
}
