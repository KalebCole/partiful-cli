/**
 * Contacts commands: list/search
 */

import { Command } from 'commander';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

export function registerContactsCommands(program: Command): void {
  const contacts = program.command('contacts').description('Manage contacts');

  contacts
    .command('list')
    .description('List or search contacts')
    .argument('[query]', 'Optional name search filter')
    .option('--limit <n>', 'Max results to return', parseInt, 20)
    .action(async (query: string | undefined, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const payload = {
          data: wrapPayload(config, {
            params: {},
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, endpoint: '/getContacts', payload });
          return;
        }

        const rawResult = await apiRequest('POST', '/getContacts', token, payload, globalOpts['verbose'] as boolean | undefined);
        const result = rawResult as Record<string, unknown>;
        const resultData = result['result'] as Record<string, unknown> | undefined;
        let contactList = ((resultData?.['data'] ?? []) as Array<Record<string, unknown>>);

        if (query) {
          const q = query.toLowerCase();
          contactList = contactList.filter((c) => ((c['name'] as string | undefined) ?? '').toLowerCase().includes(q));
        }

        const limit = opts['limit'] as number;
        contactList = contactList.slice(0, limit);

        jsonOutput(contactList, {
          count: contactList.length,
          query: query ?? null,
        });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e instanceof Error ? e.message : String(e));
      }
    });
}
