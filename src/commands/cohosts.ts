/** Cohost commands backed by Partiful's canonical request/link lifecycle. */

import { Command } from 'commander';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import type { PartifulConfig } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import {
  resolveCohostNames,
  getContacts,
  getCohostState,
  getCohostRequests,
  getCohostIds,
  mergeCohostState,
  setCohostIds,
  inviteCohost,
  removeCohostCanonical,
  planCohostInvite,
  planCohostRemoval,
  getCohostLink,
  planCohostLinkAction,
  cohostPathToUrl,
} from '../lib/cohosts.js';
import type { CanonicalCohostCall, CohostLinkRequest } from '../lib/cohosts.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { ApiError, PartifulError, ValidationError } from '../lib/errors.js';

function callable(
  token: string,
  config: PartifulConfig,
  verbose = false,
): CanonicalCohostCall {
  return async (endpoint, params) => apiRequest('POST', endpoint, token, {
    data: wrapPayload(config, {
      params,
      amplitudeSessionId: Date.now(),
      userId: config.userId,
    }),
  }, verbose);
}

function reportError(error: unknown): void {
  if (error instanceof PartifulError) jsonError(error.message, error.exitCode, error.type, error.details);
  else jsonError(error instanceof Error ? error.message : String(error));
}

export function registerCohostsCommands(program: Command): void {
  const cohosts = program.command('cohosts').description('Manage event co-hosts');

  cohosts
    .command('list')
    .description('List co-host invitations and accepted co-hosts')
    .argument('<eventId>', 'Event ID')
    .action(async (eventId: string, _opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);
        const verbose = globalOpts['verbose'] as boolean | undefined;
        const [states, contacts] = await Promise.all([
          getCohostState(eventId, token, verbose),
          getContacts(token, config, verbose),
        ]);
        const result = states.map((state) => ({
          ...state,
          name: contacts.find((contact) => contact.userId === state.userId)?.name ?? null,
        }));
        jsonOutput(result, { eventId, count: result.length });
      } catch (error) {
        reportError(error);
      }
    });

  cohosts
    .command('add')
    .description('Invite co-hosts through Partiful’s canonical request lifecycle')
    .argument('<eventId>', 'Event ID')
    .option('--name <names...>', 'Co-host names (must resolve uniquely from contacts)')
    .option('--user-id <userIds...>', 'Partiful user IDs')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        if (!opts['name'] && !opts['userId']) {
          throw new ValidationError('Provide --name or --user-id to specify co-hosts');
        }
        const config = loadConfig();
        const token = await getValidToken(config);
        const verbose = globalOpts['verbose'] as boolean | undefined;
        const [requests, resolved, existingIds] = await Promise.all([
          getCohostRequests(eventId, token, verbose),
          resolveCohostNames((opts['name'] as string[] | undefined) ?? [], token, config, verbose),
          getCohostIds(eventId, token, verbose),
        ]);
        const states = mergeCohostState(requests, existingIds);
        let currentIds = existingIds;
        const ids = [...new Set([...resolved, ...((opts['userId'] as string[] | undefined) ?? [])].filter(Boolean))];
        const plans = ids.map((userId) => {
          const state = states.find((item) => item.userId === userId);
          const action = planCohostInvite(state);
          return {
            userId,
            action,
            endpoints: action === 'noop'
              ? []
              : action === 'repair'
                ? ['Firestore PATCH cohostIds (remove stale ID)', '/createCohostRequest']
                : ['/createCohostRequest'],
            currentStatus: state?.status ?? null,
          };
        });

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, eventId, plans });
          return;
        }

        const call = callable(token, config, verbose);
        const succeeded: Array<{ userId: string; outcome: string }> = [];
        const failed: Array<{ userId: string; error: string }> = [];
        for (const userId of ids) {
          const state = states.find((item) => item.userId === userId);
          const repairStale = state?.status === 'stale'
            ? async () => {
                currentIds = currentIds.filter((id) => id !== userId);
                await setCohostIds(eventId, currentIds, token, verbose);
              }
            : undefined;
          try {
            succeeded.push(await inviteCohost(eventId, userId, state, call, repairStale));
          } catch (error) {
            failed.push({ userId, error: String(error) });
          }
        }
        if (failed.length > 0) {
          throw new ApiError('One or more co-host invitations failed', { eventId, succeeded, failed });
        }
        jsonOutput({ eventId, results: succeeded, url: `https://partiful.com/e/${eventId}` });
      } catch (error) {
        reportError(error);
      }
    });

  cohosts
    .command('remove')
    .description('Remove a request or accepted co-host through the canonical lifecycle')
    .argument('<eventId>', 'Event ID')
    .requiredOption('--user-id <userId>', 'Partiful user ID')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);
        const verbose = globalOpts['verbose'] as boolean | undefined;
        const userId = opts['userId'] as string;
        const [states, currentIds] = await Promise.all([
          getCohostState(eventId, token, verbose),
          getCohostIds(eventId, token, verbose),
        ]);
        const state = states.find((item) => item.userId === userId);
        const action = planCohostRemoval(state);
        const endpoint = state?.status === 'stale'
          ? 'Firestore PATCH cohostIds (remove stale ID)'
          : action === 'delete_request' ? '/deleteCohostRequest' : '/removeCohost';
        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, eventId, userId, currentStatus: state?.status ?? 'unknown', action, endpoint });
          return;
        }
        const removeStale = state?.status === 'stale'
          ? () => setCohostIds(eventId, currentIds.filter((id) => id !== userId), token, verbose)
          : undefined;
        const result = await removeCohostCanonical(
          eventId,
          state,
          callable(token, config, verbose),
          removeStale,
        );
        jsonOutput({ eventId, ...result, url: `https://partiful.com/e/${eventId}` });
      } catch (error) {
        reportError(error);
      }
    });

  cohosts
    .command('link')
    .description('Inspect, enable, or disable the co-host invite link')
    .argument('<eventId>', 'Event ID')
    .option('--enable', 'Enable/create the co-host invite link')
    .option('--disable', 'Disable/revoke the co-host invite link')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        if (opts['enable'] && opts['disable']) {
          throw new ValidationError('--enable and --disable are mutually exclusive');
        }
        const requested: CohostLinkRequest = opts['enable'] ? 'enable' : opts['disable'] ? 'disable' : 'inspect';
        const config = loadConfig();
        const token = await getValidToken(config);
        const verbose = globalOpts['verbose'] as boolean | undefined;
        const current = await getCohostLink(eventId, token, verbose);
        const action = planCohostLinkAction(requested, current.enabled);

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, eventId, requested, action, ...current });
          return;
        }
        if (action === 'inspect' || action === 'noop') {
          jsonOutput({ eventId, action, ...current });
          return;
        }

        const call = callable(token, config, verbose);
        if (action === 'revoke') {
          await call('/revokeEventCohostLink', { eventId });
          jsonOutput({ eventId, action: 'revoked', enabled: false, url: null });
          return;
        }

        const raw = await call('/generateEventCohostLink', { eventId }) as {
          result?: { data?: { path?: string } };
        };
        const path = raw.result?.data?.path;
        if (!path) throw new ApiError('Partiful did not return a co-host invite-link path');
        jsonOutput({ eventId, action: 'generated', enabled: true, url: cohostPathToUrl(path) });
      } catch (error) {
        reportError(error);
      }
    });
}
