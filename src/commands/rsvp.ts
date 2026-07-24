/**
 * RSVP / interest commands: a single shared implementation wired under both the
 * canonical `events *` verbs and the `explore *` aliases.
 *
 *   events rsvp <id>        (alias: explore rsvp <id>)        -> POST /addGuest
 *   events interested <id>  (alias: explore interested <id>)  -> POST /markEventInterest
 *
 * The `explore *` verbs are thin forwards to the SAME handler; there is no
 * duplicate logic. Wire names (addGuest / markEventInterest) never leak into the
 * user-facing verb surface.
 */

import type { Command } from 'commander';
import { loadConfig, getValidToken, wrapPayload, decodeJwtPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';
import { confirm } from '../lib/events.js';
import {
  RSVP_STATUSES,
  buildRsvpParams,
  buildInterestParams,
  isTicketedEvent,
  eventRequiresQuestionnaire,
  resolveDisplayName,
  type RsvpEvent,
} from '../lib/rsvp.js';

/** Wrap params in the Firebase-callable envelope the API expects. */
function makePayload(config: ReturnType<typeof loadConfig>, params: Record<string, unknown>): Record<string, unknown> {
  return {
    data: wrapPayload(config, {
      params,
      amplitudeSessionId: Date.now(),
      userId: config.userId,
    }),
  };
}

function handleError(e: unknown): void {
  if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
  else jsonError((e as Error).message);
}

/**
 * Read the caller's own guest record (read-before-write).
 *
 * Fail-CLOSED: a real API/network error propagates to the caller's catch and
 * aborts the RSVP. We must NOT swallow errors here: treating a failed read as
 * "no existing record" would send guestId:null and create a DUPLICATE guest
 * record instead of updating the existing one. A genuine "no record yet" is a
 * successful response with currentGuest absent -> null, which is correct.
 */
async function fetchCurrentGuest(config: ReturnType<typeof loadConfig>, token: string, eventId: string, verbose: boolean | undefined): Promise<Record<string, unknown> | null> {
  const res = await apiRequest('POST', '/getCurrentGuest', token, makePayload(config, { eventId }), verbose) as { result?: { data?: { currentGuest?: Record<string, unknown> } } };
  return res.result?.data?.currentGuest ?? null;
}

/**
 * Read the event (for ticketed / questionnaire guards).
 *
 * Fail-CLOSED: errors propagate. Swallowing them would return null, which both
 * guards treat as "not ticketed / no questionnaire", silently disabling the
 * safety rails on any transient error and letting an incomplete RSVP through.
 */
async function fetchEvent(config: ReturnType<typeof loadConfig>, token: string, eventId: string, verbose: boolean | undefined): Promise<RsvpEvent | null> {
  const res = await apiRequest('POST', '/getEventInfo', token, makePayload(config, { eventId }), verbose) as { result?: { data?: { event?: RsvpEvent } } };
  return res.result?.data?.event ?? null;
}

/** Derive the caller's display name from the Firebase token, if present. */
function nameFromToken(token: string): string | null {
  const payload = decodeJwtPayload(token) as { name?: string; displayName?: string } | null;
  return payload?.name ?? payload?.displayName ?? null;
}

/**
 * Shared RSVP handler. Backs `events rsvp` and `explore rsvp`.
 * Exported for unit testing of the orchestration branches.
 */
export async function rsvpAction(eventId: string, opts: Record<string, unknown>, cmd: Command): Promise<void> {
  const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
  try {
    const config = loadConfig();
    const token = await getValidToken(config);

    // Read-before-write: decide create (guestId:null) vs update. Skipped on
    // dry-run so the preview is fully offline.
    let currentGuest: Record<string, unknown> | null = null;
    let event: RsvpEvent | null = null;
    if (!globalOpts['dryRun']) {
      [currentGuest, event] = await Promise.all([
        fetchCurrentGuest(config, token, eventId, globalOpts['verbose'] as boolean | undefined),
        fetchEvent(config, token, eventId, globalOpts['verbose'] as boolean | undefined),
      ]);

      // Refuse ticketed/paid events cleanly (Stripe wall).
      if (isTicketedEvent(event)) {
        jsonError(
          'This is a ticketed or paid event. Self-RSVP is not supported here; use the Partiful app to purchase a ticket.',
          3,
          'validation_error'
        );
        return;
      }

      // Refuse questionnaire-gated events. The CLI cannot yet capture or submit
      // questionnaire answers (live recon of the addGuest answer shape is still
      // pending, see .wayfinder/tickets/07), so we refuse rather than silently
      // submit an incomplete RSVP. When --answer support lands, gate this on it.
      // NOTE: eventRequiresQuestionnaire field names are UNVERIFIED against a
      // real /getEventInfo payload; confirm via CDP recon before trusting it as
      // a positive-detection guarantee.
      if (eventRequiresQuestionnaire(event)) {
        jsonError(
          'This event requires answering a host questionnaire before you can RSVP. The CLI cannot submit questionnaire answers yet; please RSVP in the Partiful app.',
          3,
          'validation_error'
        );
        return;
      }
    }

    const name = resolveDisplayName({
      override: opts['name'] as string | undefined,
      currentGuest,
      config,
      tokenName: nameFromToken(token),
    });

    const params = buildRsvpParams({
      eventId,
      name: name ?? undefined,
      status: opts['status'] as string | undefined,
      plusOnes: opts['plusOne'] as string[] | undefined,
      count: opts['count'] as number | undefined,
      message: opts['message'] as string | undefined,
      password: opts['password'] as string | undefined,
      timezone: opts['timezone'] as string | undefined,
      guestId: (currentGuest?.['id'] as string | undefined) ?? null,
    });

    const payload = makePayload(config, params as unknown as Record<string, unknown>);

    if (globalOpts['dryRun']) {
      jsonOutput({ dryRun: true, endpoint: '/addGuest', payload });
      return;
    }

    // Confirmation gate: writing to a real host's guest list.
    if (!globalOpts['yes'] && !globalOpts['force']) {
      const verb = currentGuest ? 'update your RSVP on' : 'RSVP to';
      const confirmed = await confirm(`About to ${verb} "${eventId}" as ${params.rsvp.status}. Continue?`);
      if (!confirmed) {
        jsonOutput({ rsvp: false, message: 'Aborted by user' });
        return;
      }
    }

    const result = await apiRequest('POST', '/addGuest', token, payload, globalOpts['verbose'] as boolean | undefined) as { result?: { data?: { guest?: Record<string, unknown> } | Record<string, unknown> } };
    const guest = (result.result?.data as { guest?: Record<string, unknown> })?.guest ?? result.result?.data ?? {};

    jsonOutput({
      eventId,
      status: params.rsvp.status,
      guestId: (guest as Record<string, unknown>)['id'] ?? currentGuest?.['id'] ?? null,
      count: params.rsvp.count,
      updated: Boolean(currentGuest),
      url: `https://partiful.com/e/${eventId}`,
    });
  } catch (e) {
    handleError(e);
  }
}

/**
 * Shared interest handler. Backs `events interested` and `explore interested`.
 */
async function interestedAction(eventId: string, opts: Record<string, unknown>, cmd: Command): Promise<void> {
  const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
  try {
    const config = loadConfig();
    const token = await getValidToken(config);

    const interested = !opts['remove'];
    // No confirmation gate here: marking interest is non-destructive and easily
    // reversible via --remove, unlike an RSVP write to a host's guest list.
    const params = buildInterestParams({ eventId, interested });
    const payload = makePayload(config, params as unknown as Record<string, unknown>);

    if (globalOpts['dryRun']) {
      jsonOutput({ dryRun: true, endpoint: '/markEventInterest', payload });
      return;
    }

    await apiRequest('POST', '/markEventInterest', token, payload, globalOpts['verbose'] as boolean | undefined);

    jsonOutput({
      eventId,
      interested,
      url: `https://partiful.com/e/${eventId}`,
    });
  } catch (e) {
    handleError(e);
  }
}

/** Attach rsvp + interested subcommands to a parent command (events or explore). */
function attachRsvpVerbs(parent: Command): void {
  parent
    .command('rsvp')
    .description('RSVP to an event (going, maybe, or declined)')
    .argument('<eventId>', 'Event ID')
    .option('--status <status>', `RSVP status: ${RSVP_STATUSES.join(', ')}`, 'going')
    .option('--name <name>', 'Display name to RSVP with (defaults to your profile name)')
    .option('--plus-one <name...>', 'Plus-one name (repeatable)')
    .option('--count <n>', 'Total headcount including plus-ones', (v: string) => parseInt(v, 10))
    .option('--message <text>', 'Optional public comment on the event')
    .option('--password <password>', 'Event password (if the event is password-gated)')
    .option('--timezone <tz>', 'IANA timezone for the RSVP')
    .action(rsvpAction);

  parent
    .command('interested')
    .description('Mark (or remove) interest in an event')
    .argument('<eventId>', 'Event ID')
    .option('--remove', 'Remove your interest instead of adding it')
    .action(interestedAction);
}

export function registerRsvpCommands(program: Command, { events, explore }: { events: Command; explore: Command }): void {
  // Canonical verbs live under `events`; `explore` gets the same verbs as
  // thin aliases to the SAME handlers.
  attachRsvpVerbs(events);
  attachRsvpVerbs(explore);
}
