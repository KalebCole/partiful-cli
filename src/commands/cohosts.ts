/**
 * Cohosts commands: list, add, remove
 */

import { Command } from 'commander';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { resolveCohostNames, getCohostIds, setCohostIds } from '../lib/cohosts.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

export function registerCohostsCommands(program: Command): void {
  const cohosts = program.command('cohosts').description('Manage event co-hosts');

  cohosts
    .command('list')
    .description('List co-hosts for an event')
    .argument('<eventId>', 'Event ID')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const ids = await getCohostIds(eventId, token, globalOpts['verbose'] as boolean | undefined);
        if (ids.length === 0) {
          jsonOutput([], { eventId, count: 0 });
          return;
        }

        // Cross-reference with contacts for names
        const contactsPayload = { data: wrapPayload(config, { params: {}, amplitudeSessionId: Date.now(), userId: config.userId }) };
        const contactsRaw = await apiRequest('POST', '/getContacts', token, contactsPayload, globalOpts['verbose'] as boolean | undefined);
        const contactsResult = contactsRaw as Record<string, unknown>;
        const resultData = contactsResult['result'] as Record<string, unknown> | undefined;
        const allContacts = (resultData?.['data'] ?? []) as Array<Record<string, unknown>>;

        const result = ids.map((id: string) => {
          const contact = allContacts.find((c) => c['userId'] === id);
          return { userId: id, name: (contact?.['name'] as string | undefined) ?? null };
        });

        jsonOutput(result, { eventId, count: result.length });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e instanceof Error ? e.message : String(e));
      }
    });

  cohosts
    .command('add')
    .description('Add co-hosts to an event')
    .argument('<eventId>', 'Event ID')
    .option('--name <names...>', 'Co-host names (resolved from contacts)')
    .option('--user-id <userIds...>', 'Co-host user IDs (direct)')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        if (!opts['name'] && !opts['userId']) {
          jsonError('Provide --name or --user-id to specify co-hosts', 3, 'validation_error');
          return;
        }

        const config = loadConfig();
        const token = await getValidToken(config);

        const currentIds = await getCohostIds(eventId, token, globalOpts['verbose'] as boolean | undefined);
        const newIds = [...currentIds];

        // Resolve names
        const resolved = await resolveCohostNames((opts['name'] as string[] | undefined) ?? [], token, config, globalOpts['verbose'] as boolean | undefined);
        for (const id of resolved) {
          if (!newIds.includes(id)) newIds.push(id);
        }

        // Add direct user IDs
        for (const id of ((opts['userId'] as string[] | undefined) ?? [])) {
          if (!newIds.includes(id)) newIds.push(id);
        }

        const added = newIds.filter((id) => !currentIds.includes(id));
        if (added.length === 0) {
          jsonOutput({ eventId, added: [], total: currentIds.length, message: 'No new co-hosts to add' });
          return;
        }

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, eventId, currentCohosts: currentIds, newCohosts: newIds });
          return;
        }

        await setCohostIds(eventId, newIds, token, globalOpts['verbose'] as boolean | undefined);

        jsonOutput({ eventId, added, total: newIds.length, url: `https://partiful.com/e/${eventId}` });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e instanceof Error ? e.message : String(e));
      }
    });

  cohosts
    .command('remove')
    .description('Remove a co-host from an event')
    .argument('<eventId>', 'Event ID')
    .requiredOption('--user-id <userId>', 'User ID of the co-host to remove')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const currentIds = await getCohostIds(eventId, token, globalOpts['verbose'] as boolean | undefined);
        const userId = opts['userId'] as string;

        if (!currentIds.includes(userId)) {
          jsonError(`User ${userId} is not a co-host of this event`, 4, 'not_found');
          return;
        }

        const newIds = currentIds.filter((id) => id !== userId);

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, eventId, removing: userId, remaining: newIds });
          return;
        }

        await setCohostIds(eventId, newIds, token, globalOpts['verbose'] as boolean | undefined);

        jsonOutput({ eventId, removed: userId, remaining: newIds.length, url: `https://partiful.com/e/${eventId}` });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e instanceof Error ? e.message : String(e));
      }
    });
}
