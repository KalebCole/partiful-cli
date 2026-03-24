/**
 * Contacts commands: list/search
 */

import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

export function registerContactsCommands(program) {
  const contacts = program.command('contacts').description('Manage contacts');

  contacts
    .command('list')
    .description('List or search contacts')
    .argument('[query]', 'Optional name search filter')
    .option('--limit <n>', 'Max results to return', parseInt, 20)
    .action(async (query, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
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

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, endpoint: '/getContacts', payload });
          return;
        }

        const result = await apiRequest('POST', '/getContacts', token, payload, globalOpts.verbose);
        let contactList = result.result?.data || [];

        if (query) {
          const q = query.toLowerCase();
          contactList = contactList.filter(c => (c.name || '').toLowerCase().includes(q));
        }

        contactList = contactList.slice(0, opts.limit);

        jsonOutput(contactList, {
          count: contactList.length,
          query: query || null,
        });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });
}
