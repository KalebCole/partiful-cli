/**
 * Export helper: +export <eventId> — export event + guests to file
 */

import fs from 'fs';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { fetchGuests } from '../commands/guests.js';
import { jsonOutput, jsonError, formatCsv } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

export function registerExportHelper(program) {
  program
    .command('+export')
    .description('Export event details and guest list')
    .argument('<eventId>', 'Event ID to export')
    .option('--format <format>', 'Output format: json or csv', 'json')
    .option('--output <path>', 'Write to file instead of stdout')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
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

        const result = await apiRequest('POST', '/getEvent', token, payload, globalOpts.verbose);
        const event = result.result?.data?.event;

        if (!event) {
          jsonError('Event not found', 4, 'not_found');
          return;
        }

        // Fetch guests
        const guests = await fetchGuests(eventId, token, config, globalOpts.verbose);

        const exportData = {
          event: {
            id: eventId,
            title: event.title,
            startDate: event.startDate,
            endDate: event.endDate || null,
            location: event.location || null,
            address: event.address || null,
            description: event.description || null,
            status: event.status,
            timezone: event.timezone || null,
            url: `https://partiful.com/e/${eventId}`,
          },
          guests: guests.map(g => ({
            name: g.name,
            status: g.status,
            count: g.count,
            createdAt: g.createdAt,
            channel: g.channel,
          })),
          exportedAt: new Date().toISOString(),
          totalGuests: guests.length,
        };

        if (opts.format === 'csv') {
          const csvHeader = `Event: ${event.title} (${eventId})\nExported: ${exportData.exportedAt}\n\n`;
          const csvBody = formatCsv(exportData.guests, ['name', 'status', 'count', 'createdAt', 'channel']);
          const output = csvHeader + csvBody;
          if (opts.output) {
            fs.writeFileSync(opts.output, output + '\n');
            process.stderr.write(`Exported to ${opts.output}\n`);
          } else {
            process.stdout.write(output + '\n');
          }
        } else {
          if (opts.output) {
            fs.writeFileSync(opts.output, JSON.stringify(exportData, null, 2) + '\n');
            process.stderr.write(`Exported to ${opts.output}\n`);
          } else {
            jsonOutput(exportData);
          }
        }
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });
}
