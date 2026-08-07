/**
 * RSVP / interest commands: a single shared implementation wired under both the
 * canonical `events *` verbs and the `explore *` aliases.
 *
 *   events rsvp set <id>    (alias: explore rsvp set <id>)    -> POST /addGuest
 *   events rsvp get <id>    (alias: explore rsvp get <id>)    -> read current guest
 *   events interested <id>  (alias: explore interested <id>)  -> POST /markEventInterest
 *
 * The `explore *` verbs are thin forwards to the SAME handler; there is no
 * duplicate logic. Wire names (addGuest / markEventInterest) never leak into the
 * user-facing verb surface.
 */

import type { Command } from 'commander';
import { loadConfig, getValidToken, wrapPayload, decodeJwtPayload } from '../lib/auth.js';
import { apiRequest, firestoreGetDocument } from '../lib/http.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';
import { confirm } from '../lib/events.js';
import {
  RSVP_STATUSES,
  buildRsvpParams,
  buildInterestParams,
  isTicketedEvent,
  eventRequiresQuestionnaire,
  buildQuestionnaireResponse,
  parseQuestionnaireAnswers,
  resolveDisplayName,
  type QuestionnaireResponse,
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

interface FirestoreValue {
  stringValue?: string;
  integerValue?: string;
  mapValue?: { fields?: Record<string, FirestoreValue> };
}

interface FirestoreGuestDocument {
  fields?: Record<string, FirestoreValue>;
}

function questionnaireResponseFromDocument(document: FirestoreGuestDocument): QuestionnaireResponse | null {
  const responseFields = document.fields?.['questionnaireResponse']?.mapValue?.fields;
  const versionValue = responseFields?.['questionnaireVersion']?.integerValue;
  const answerFields = responseFields?.['answers']?.mapValue?.fields;
  if (versionValue === undefined || answerFields === undefined) return null;

  const questionnaireVersion = Number.parseInt(versionValue, 10);
  if (!Number.isFinite(questionnaireVersion)) return null;

  const answers: Record<string, string> = {};
  for (const [questionId, value] of Object.entries(answerFields)) {
    if (value.stringValue !== undefined) answers[questionId] = value.stringValue;
  }
  return { questionnaireVersion, answers };
}

function questionnaireResponsesEqual(
  actual: QuestionnaireResponse | null,
  expected: QuestionnaireResponse,
): boolean {
  if (actual === null || actual.questionnaireVersion !== expected.questionnaireVersion) return false;

  const actualKeys = Object.keys(actual.answers);
  const expectedKeys = Object.keys(expected.answers);
  return actualKeys.length === expectedKeys.length
    && expectedKeys.every((key) => Object.hasOwn(actual.answers, key)
      && actual.answers[key] === expected.answers[key]);
}

async function fetchGuestDocument(
  token: string,
  eventId: string,
  guestId: string,
  verbose: boolean | undefined,
): Promise<FirestoreGuestDocument> {
  return await firestoreGetDocument(
    `events/${eventId}/guests/${guestId}`,
    token,
    verbose ?? false,
  ) as FirestoreGuestDocument;
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
  const currentGuest = res.result?.data?.currentGuest ?? null;
  const guestId = currentGuest?.['id'];
  if (!currentGuest || typeof guestId !== 'string') return currentGuest;

  const document = await fetchGuestDocument(token, eventId, guestId, verbose);
  const questionnaireResponse = questionnaireResponseFromDocument(document);
  return questionnaireResponse
    ? { ...currentGuest, questionnaireResponse }
    : currentGuest;
}

/** Read the caller's RSVP, including questionnaire answers, without mutating it. */
export async function currentGuestAction(eventId: string, _opts: Record<string, unknown>, cmd: Command): Promise<void> {
  const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
  try {
    const config = loadConfig();
    const token = await getValidToken(config);
    const guest = await fetchCurrentGuest(
      config,
      token,
      eventId,
      globalOpts['verbose'] as boolean | undefined,
    );

    jsonOutput({
      eventId,
      guest,
      url: `https://partiful.com/e/${eventId}`,
    });
  } catch (e) {
    handleError(e);
  }
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
 * Shared RSVP handler. Backs `events rsvp set` and `explore rsvp set`.
 * Exported for unit testing of the orchestration branches.
 */
export async function rsvpAction(eventId: string, opts: Record<string, unknown>, cmd: Command): Promise<void> {
  const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
  try {
    const config = loadConfig();
    const token = await getValidToken(config);

    const answerPairs = (opts['answer'] as string[] | undefined) ?? [];
    const suppliedAnswers = parseQuestionnaireAnswers(answerPairs);

    // Live writes and answer-aware previews use the same read-before-write state.
    // Plain dry-runs remain offline for backward compatibility.
    let currentGuest: Record<string, unknown> | null = null;
    let event: RsvpEvent | null = null;
    const needsRemoteState = !globalOpts['dryRun'] || answerPairs.length > 0;
    if (needsRemoteState) {
      [currentGuest, event] = await Promise.all([
        fetchCurrentGuest(config, token, eventId, globalOpts['verbose'] as boolean | undefined),
        fetchEvent(config, token, eventId, globalOpts['verbose'] as boolean | undefined),
      ]);

      // Ticketed events cannot be represented by /addGuest, including previews.
      if (isTicketedEvent(event)) {
        jsonError(
          'This is a ticketed or paid event. Self-RSVP is not supported here; use the Partiful app to purchase a ticket.',
          3,
          'validation_error'
        );
        return;
      }
    }

    const existingResponse = currentGuest?.['questionnaireResponse'] as QuestionnaireResponse | undefined;
    let questionnaireResponse: QuestionnaireResponse | null = existingResponse ?? null;
    if (eventRequiresQuestionnaire(event)) {
      questionnaireResponse = buildQuestionnaireResponse(event!, suppliedAnswers, existingResponse ?? null);
    } else if (answerPairs.length > 0) {
      throw new PartifulError(
        'This event does not expose a host questionnaire, so --answer cannot be used.',
        3,
        'validation_error'
      );
    }

    const name = resolveDisplayName({
      override: opts['name'] as string | undefined,
      currentGuest,
      config,
      tokenName: nameFromToken(token),
    });

    const suppliedPlusOnes = opts['plusOne'] as string[] | undefined;
    const existingPlusOnes = currentGuest?.['plusOnes'];
    const plusOnes = suppliedPlusOnes
      ?? (Array.isArray(existingPlusOnes) ? existingPlusOnes as string[] : undefined);
    const count = opts['count'] !== undefined
      ? opts['count'] as number
      : suppliedPlusOnes !== undefined
        ? undefined
        : currentGuest?.['count'] as number | undefined;
    const message = opts['message'] !== undefined
      ? opts['message'] as string
      : currentGuest?.['rsvpMessage'] as string | null | undefined;
    const currentStatus = currentGuest?.['status'];
    const reusableCurrentStatus = typeof currentStatus === 'string'
      && ['GOING', 'MAYBE', 'DECLINED'].includes(currentStatus.trim().toUpperCase())
      ? currentStatus
      : undefined;

    const params = buildRsvpParams({
      eventId,
      name: name ?? undefined,
      status: (opts['status'] as string | undefined) ?? reusableCurrentStatus,
      plusOnes,
      count,
      message,
      password: opts['password'] as string | undefined,
      timezone: opts['timezone'] as string | undefined,
      guestId: (currentGuest?.['id'] as string | undefined) ?? null,
      questionnaireResponse,
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
    const guestId = (guest as Record<string, unknown>)['id'] ?? currentGuest?.['id'] ?? null;

    let persistedStatus: string | null = null;
    let persistedQuestionnaireResponse: QuestionnaireResponse | null = null;
    let verificationError: string | null = null;
    if (typeof guestId === 'string') {
      try {
        const document = await fetchGuestDocument(
          token,
          eventId,
          guestId,
          globalOpts['verbose'] as boolean | undefined,
        );
        persistedStatus = document.fields?.['status']?.stringValue ?? null;
        persistedQuestionnaireResponse = questionnaireResponseFromDocument(document);
      } catch (error) {
        verificationError = (error as Error).message;
      }
    }

    const expectedQuestionnaireResponse = params.rsvp.questionnaireResponse ?? null;
    const verified = {
      status: persistedStatus === params.rsvp.status,
      questionnaireResponse: expectedQuestionnaireResponse === null
        ? null
        : questionnaireResponsesEqual(persistedQuestionnaireResponse, expectedQuestionnaireResponse),
    };

    jsonOutput({
      eventId,
      status: params.rsvp.status,
      guestId,
      count: params.rsvp.count,
      updated: Boolean(currentGuest),
      questionnaireResponse: persistedQuestionnaireResponse,
      verified,
      ...(verificationError ? { verificationError } : {}),
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

/** Attach RSVP/interest subcommands to a parent command (events or explore). */
function attachRsvpVerbs(parent: Command): void {
  const rsvp = parent
    .command('rsvp')
    .description('Read or change your RSVP');

  rsvp
    .command('get')
    .description('Read your RSVP status and questionnaire answers')
    .argument('<eventId>', 'Event ID')
    .action(currentGuestAction);

  rsvp
    .command('set')
    .description('RSVP to an event (going, maybe, or declined)')
    .argument('<eventId>', 'Event ID')
    .option('--status <status>', `RSVP status: ${RSVP_STATUSES.join(', ')}`)
    .option('--name <name>', 'Display name to RSVP with (defaults to your profile name)')
    .option('--plus-one <name...>', 'Plus-one name (repeatable)')
    .option('--count <n>', 'Total headcount including plus-ones', (v: string) => parseInt(v, 10))
    .option('--message <text>', 'Optional public comment on the event')
    .option('--password <password>', 'Event password (if the event is password-gated)')
    .option('--timezone <tz>', 'IANA timezone for the RSVP')
    .option('--answer <key=value...>', 'Host-questionnaire answers (question id or exact text)')
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
