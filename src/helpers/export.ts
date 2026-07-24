/**
 * Export helper: +export <eventId> — export event + guests to file
 */

import fs from 'fs';
import { Command } from 'commander';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { fetchGuests } from '../commands/guests.js';
import { jsonOutput, jsonError, formatCsv } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

type GuestLike = Awaited<ReturnType<typeof fetchGuests>>[number];

export function registerExportHelper(program: Command): void {
  program
    .command('+export')
    .description('Export event details and guest list')
    .argument('<eventId>', 'Event ID to export')
    .option('--format <format>', 'Output format: json or csv', 'json')
    .option('--output <path>', 'Write to file instead of stdout')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // Fetch event
        const payload = {
          data: wrapPayload(config, {
            params: { eventId },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        const result = await apiRequest('POST', '/getEventInfo', token, payload, globalOpts['verbose'] as boolean | undefined) as Record<string, unknown>;
        const data = result['result'] as Record<string, unknown> | undefined;
        const eventData = data?.['data'] as Record<string, unknown> | undefined;
        const event = eventData?.['event'] as Record<string, unknown> | undefined;

        if (!event) {
          jsonError('Event not found', 4, 'not_found');
          return;
        }

        // Fetch guests
        const guests: GuestLike[] = await fetchGuests(eventId, token, config, globalOpts['verbose'] as boolean | undefined);

        const exportData = {
          event: {
            id: eventId,
            title: event['title'],
            startDate: event['startDate'],
            endDate: (event['endDate'] as unknown) ?? null,
            location: (event['location'] as unknown) ?? null,
            address: (event['address'] as unknown) ?? null,
            description: (event['description'] as unknown) ?? null,
            status: event['status'],
            timezone: (event['timezone'] as unknown) ?? null,
            url: `https://partiful.com/e/${eventId}`,
          },
          guests: guests.map(g => ({
            name: g['name'],
            status: g['status'],
            count: g['count'],
            createdAt: g['createdAt'],
            channel: g['channel'],
          })),
          exportedAt: new Date().toISOString(),
          totalGuests: guests.length,
        };

        if (opts['format'] === 'csv') {
          const csvHeader = `Event: ${event['title']} (${eventId})\nExported: ${exportData.exportedAt}\n\n`;
          const csvBody = formatCsv(exportData.guests as unknown as import('../lib/output.js').TableRow[], ['name', 'status', 'count', 'createdAt', 'channel']);
          const output = csvHeader + csvBody;
          if (opts['output']) {
            fs.writeFileSync(opts['output'] as string, output + '\n');
            process.stderr.write(`Exported to ${opts['output']}\n`);
          } else {
            process.stdout.write(output + '\n');
          }
        } else {
          if (opts['output']) {
            fs.writeFileSync(opts['output'] as string, JSON.stringify(exportData, null, 2) + '\n');
            process.stderr.write(`Exported to ${opts['output']}\n`);
          } else {
            jsonOutput(exportData);
          }
        }
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError((e as Error).message);
      }
    });
}
