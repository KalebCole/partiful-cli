/**
 * Cohosts commands: list, add, remove
 */

import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest, firestoreRequest } from '../lib/http.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

export function registerCohostsCommands(program) {
  const cohosts = program.command('cohosts').description('Manage event co-hosts');

  cohosts
    .command('list')
    .description('List co-hosts for an event')
    .argument('<eventId>', 'Event ID')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // Fetch event via Firestore to get cohostIds
        const eventDoc = await firestoreRequest('GET', eventId, null, token, [], globalOpts.verbose);
        const cohostIdsField = eventDoc.fields?.cohostIds?.arrayValue?.values || [];
        const cohostIds = cohostIdsField.map(v => v.stringValue).filter(Boolean);

        if (cohostIds.length === 0) {
          jsonOutput([], { eventId, count: 0 });
          return;
        }

        // Cross-reference with contacts for names
        const contactsPayload = { data: wrapPayload(config, { params: {}, amplitudeSessionId: Date.now(), userId: config.userId }) };
        const contactsResult = await apiRequest('POST', '/getContacts', token, contactsPayload, globalOpts.verbose);
        const allContacts = contactsResult.result?.data || [];

        const result = cohostIds.map(id => {
          const contact = allContacts.find(c => c.userId === id);
          return { userId: id, name: contact?.name || null };
        });

        jsonOutput(result, { eventId, count: result.length });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });

  cohosts
    .command('add')
    .description('Add co-hosts to an event')
    .argument('<eventId>', 'Event ID')
    .option('--name <names...>', 'Co-host names (resolved from contacts)')
    .option('--user-id <userIds...>', 'Co-host user IDs (direct)')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        if (!opts.name && !opts.userId) {
          jsonError('Provide --name or --user-id to specify co-hosts', 3, 'validation_error');
          return;
        }

        const config = loadConfig();
        const token = await getValidToken(config);

        // Get current cohostIds
        const eventDoc = await firestoreRequest('GET', eventId, null, token, [], globalOpts.verbose);
        const currentField = eventDoc.fields?.cohostIds?.arrayValue?.values || [];
        const currentIds = currentField.map(v => v.stringValue).filter(Boolean);

        const newIds = [...currentIds];

        // Resolve names
        if (opts.name && opts.name.length > 0) {
          const contactsPayload = { data: wrapPayload(config, { params: {}, amplitudeSessionId: Date.now(), userId: config.userId }) };
          const contactsResult = await apiRequest('POST', '/getContacts', token, contactsPayload, globalOpts.verbose);
          const allContacts = contactsResult.result?.data || [];
          for (const name of opts.name) {
            const q = name.toLowerCase();
            const match = allContacts.find(c => (c.name || '').toLowerCase() === q) || allContacts.find(c => (c.name || '').toLowerCase().includes(q));
            if (match && match.userId) {
              if (!newIds.includes(match.userId)) newIds.push(match.userId);
            } else {
              process.stderr.write(`Warning: could not resolve co-host "${name}" from contacts — skipping\n`);
            }
          }
        }

        // Add direct user IDs
        if (opts.userId && opts.userId.length > 0) {
          for (const id of opts.userId) {
            if (!newIds.includes(id)) newIds.push(id);
          }
        }

        const fields = {
          cohostIds: {
            arrayValue: { values: newIds.map(id => ({ stringValue: id })) }
          }
        };

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, eventId, currentCohosts: currentIds, newCohosts: newIds, body: { fields } });
          return;
        }

        await firestoreRequest('PATCH', eventId, { fields }, token, ['cohostIds'], globalOpts.verbose);

        const added = newIds.filter(id => !currentIds.includes(id));
        jsonOutput({
          eventId,
          added,
          total: newIds.length,
          url: `https://partiful.com/e/${eventId}`,
        });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });

  cohosts
    .command('remove')
    .description('Remove a co-host from an event')
    .argument('<eventId>', 'Event ID')
    .requiredOption('--user-id <userId>', 'User ID of the co-host to remove')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // Get current cohostIds
        const eventDoc = await firestoreRequest('GET', eventId, null, token, [], globalOpts.verbose);
        const currentField = eventDoc.fields?.cohostIds?.arrayValue?.values || [];
        const currentIds = currentField.map(v => v.stringValue).filter(Boolean);

        if (!currentIds.includes(opts.userId)) {
          jsonError(`User ${opts.userId} is not a co-host of this event`, 4, 'not_found');
          return;
        }

        const newIds = currentIds.filter(id => id !== opts.userId);
        const fields = {
          cohostIds: {
            arrayValue: { values: newIds.map(id => ({ stringValue: id })) }
          }
        };

        if (globalOpts.dryRun) {
          jsonOutput({ dryRun: true, eventId, removing: opts.userId, remaining: newIds });
          return;
        }

        await firestoreRequest('PATCH', eventId, { fields }, token, ['cohostIds'], globalOpts.verbose);

        jsonOutput({
          eventId,
          removed: opts.userId,
          remaining: newIds.length,
          url: `https://partiful.com/e/${eventId}`,
        });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });
}
