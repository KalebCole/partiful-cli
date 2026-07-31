/**
 * RSVP / interest library for the Partiful CLI.
 *
 * Pure builders + guards backing the `events rsvp` / `explore rsvp` and
 * `events interested` / `explore interested` commands. All network access lives
 * in the command layer (via src/lib/http.js); this module stays side-effect free
 * and unit-testable.
 *
 * Wire facts (captured live, see .wayfinder/tickets/07):
 *   - Self-RSVP mutation  = POST /addGuest
 *       params: { eventId, rsvp: { name, count, plusOnes[], message,
 *                 emailInvitationId, status, guestId, timezone, password } }
 *       First RSVP sends guestId:null (server creates); edits send it back.
 *       GOING | MAYBE | DECLINED all go through this ONE call via `status`.
 *   - Interest mutation   = POST /markEventInterest
 *       params: { eventId, interested: bool, source? }
 *   - Read-before-write   = POST /getCurrentGuest { eventId }
 *       -> result.data.currentGuest.{ id, status, name } (or null)
 */

import { PartifulError } from './errors.js';
import type { AddGuestParams, MarkEventInterestParams, RsvpDraft } from './api/endpoints.js';

/** User-facing status verbs for `--status` on the rsvp command. */
export const RSVP_STATUSES = ['going', 'maybe', 'declined'] as const;

const DEFAULT_TIMEZONE = 'America/Los_Angeles';

// Unverified legacy field-name fallbacks for the questionnaire question list.
// The verified field is `questionnaire.questions[]` (see eventRequiresQuestionnaire).
const LEGACY_QUESTIONNAIRE_FIELDS = ['questions', 'rsvpQuestions', 'customQuestions'];

// Human input -> wire enum. Only GOING/MAYBE/DECLINED are valid self-RSVP
// statuses. INTERESTED is a DIFFERENT endpoint (markEventInterest), so it is
// deliberately NOT accepted here.
const STATUS_ALIASES: Record<string, string> = {
  going: 'GOING',
  yes: 'GOING',
  maybe: 'MAYBE',
  declined: 'DECLINED',
  decline: 'DECLINED',
  no: 'DECLINED',
};

/** A questionnaire question as it appears in the event object. */
export interface QuestionnaireQuestion {
  id: string;
  text: string;
  type?: string;
  required?: boolean;
}

/** The questionnaireResponse object attached to an RSVP. */
export interface QuestionnaireResponse {
  questionnaireVersion: number;
  answers: Record<string, string>;
}

/** Parse repeatable `--answer key=value` options into a lookup map. */
export function parseQuestionnaireAnswers(pairs: string[] = []): Record<string, string> {
  const answers: Record<string, string> = {};
  for (const pair of pairs) {
    const separator = pair.indexOf('=');
    if (separator < 0) {
      throw new PartifulError(`Invalid --answer "${pair}". Use key=value.`, 3, 'validation_error');
    }
    const key = pair.slice(0, separator).trim();
    const value = pair.slice(separator + 1).trim();
    if (!key) {
      throw new PartifulError('Invalid --answer: question key cannot be empty.', 3, 'validation_error');
    }
    if (!value) {
      throw new PartifulError(`Invalid --answer "${pair}": value cannot be empty.`, 3, 'validation_error');
    }
    if (Object.hasOwn(answers, key)) {
      throw new PartifulError(`Duplicate questionnaire answer key: ${key}`, 3, 'validation_error');
    }
    answers[key] = value;
  }
  return answers;
}

/** Loose event shape read by the RSVP guards (broad — hosts see more fields). */
export interface RsvpEvent {
  ticketing?: { enabled?: boolean };
  ticketInfo?: unknown;
  requiresPayment?: boolean;
  isTicketed?: boolean;
  questionnaireEnabled?: boolean;
  questionnaire?: { questions?: QuestionnaireQuestion[] };
  questionnaireVersions?: unknown[];
  [key: string]: unknown;
}

/** Options accepted by buildRsvpParams(). */
export interface BuildRsvpOptions {
  eventId?: string;
  name?: string;
  status?: string | null;
  plusOnes?: string[];
  count?: number | null;
  message?: string | null;
  emailInvitationId?: string | null;
  password?: string | null;
  guestId?: string | null;
  timezone?: string;
  questionnaireResponse?: QuestionnaireResponse | null;
}

/** Options accepted by buildInterestParams(). */
export interface BuildInterestOptions {
  eventId?: string;
  interested?: boolean;
  source?: string;
}

/**
 * Normalize a user-supplied status into its wire enum value.
 * Defaults to GOING. Throws a validation PartifulError on unknown input.
 */
export function normalizeStatus(status?: string | null): string {
  if (status === null || status === undefined || status === '') return 'GOING';
  const key = String(status).trim().toLowerCase();
  const wire = STATUS_ALIASES[key];
  if (!wire) {
    throw new PartifulError(
      `Invalid status "${status}". Use one of: going, maybe, declined.`,
      3,
      'validation_error',
    );
  }
  return wire;
}

/**
 * Build the `params` object for POST /addGuest.
 */
export function buildRsvpParams(o: BuildRsvpOptions = {}): AddGuestParams {
  const { eventId, name } = o;
  if (!eventId) {
    throw new PartifulError('eventId is required to RSVP.', 3, 'validation_error');
  }
  if (!name || !String(name).trim()) {
    throw new PartifulError('A guest name is required to RSVP (the server rejects an empty name).', 3, 'validation_error');
  }

  const plusOnes = Array.isArray(o.plusOnes) ? o.plusOnes.filter(Boolean) : [];
  const derivedCount = 1 + plusOnes.length;
  let count: number;
  if (o.count == null) {
    count = derivedCount;
  } else if (!Number.isFinite(o.count) || o.count <= 0) {
    throw new PartifulError(
      `Invalid --count "${o.count}". Must be a positive whole number.`,
      3,
      'validation_error',
    );
  } else {
    count = Math.trunc(o.count);
  }

  const rsvp: RsvpDraft = {
    name: String(name),
    count,
    plusOnes,
    message: o.message ?? null,
    emailInvitationId: o.emailInvitationId ?? null,
    status: normalizeStatus(o.status),
    guestId: o.guestId ?? null,
    timezone: o.timezone || DEFAULT_TIMEZONE,
    password: o.password ?? null,
  };

  // Questionnaire answers ride inside the rsvp object as questionnaireResponse.
  // Only attach when present so non-questionnaire events keep the lean payload.
  if (o.questionnaireResponse) {
    rsvp.questionnaireResponse = o.questionnaireResponse;
  }

  return { eventId, rsvp };
}

/**
 * Build the `params` object for POST /markEventInterest.
 */
export function buildInterestParams(o: BuildInterestOptions = {}): MarkEventInterestParams {
  const { eventId } = o;
  if (!eventId) {
    throw new PartifulError('eventId is required to mark interest.', 3, 'validation_error');
  }
  return {
    eventId,
    interested: Boolean(o.interested),
    source: o.source || 'DISCOVER',
  };
}

/**
 * Detect a ticketed / paid event we should refuse to RSVP for (Stripe wall).
 * Conservative: any ticketing/payment signal counts.
 */
export function isTicketedEvent(event: RsvpEvent | null | undefined): boolean {
  if (!event || typeof event !== 'object') return false;
  if (event.ticketing && (event.ticketing.enabled ?? true)) return true;
  if (event.ticketInfo && typeof event.ticketInfo === 'object') return true;
  if (event.requiresPayment === true) return true;
  if (event.isTicketed === true) return true;
  return false;
}

/**
 * Detect an event that requires questionnaire answers before an RSVP submits.
 */
export function eventRequiresQuestionnaire(event: RsvpEvent | null | undefined): boolean {
  if (!event || typeof event !== 'object') return false;

  const q = event.questionnaire;
  const hasQuestions =
    !!q && typeof q === 'object' && Array.isArray(q.questions) && q.questions.length > 0;

  // Primary, verified signal.
  if (event.questionnaireEnabled && hasQuestions) return true;

  // Defensive fallback: a populated questionnaire.questions[] even if the
  // enabled flag is absent (older payloads / host-scoped views).
  if (hasQuestions) return true;

  // Legacy field-name fallbacks (unverified, kept for resilience).
  for (const f of LEGACY_QUESTIONNAIRE_FIELDS) {
    const legacy = event[f];
    if (Array.isArray(legacy) && legacy.length > 0) return true;
  }
  return false;
}

/**
 * Build the questionnaireResponse object for an RSVP.
 * Returns null when the event has no questionnaire.
 */
export function buildQuestionnaireResponse(
  event: RsvpEvent,
  answersByKey: Record<string, unknown> = {},
  existingResponse: QuestionnaireResponse | null = null,
): QuestionnaireResponse | null {
  if (!eventRequiresQuestionnaire(event)) return null;

  let rawQuestions: Array<QuestionnaireQuestion | string>;
  const primaryQuestions = event.questionnaire?.questions;
  if (Array.isArray(primaryQuestions) && primaryQuestions.length > 0) {
    rawQuestions = primaryQuestions;
  } else {
    const legacyField = LEGACY_QUESTIONNAIRE_FIELDS.find(
      (f) => Array.isArray(event[f]) && (event[f] as unknown[]).length > 0,
    );
    if (!legacyField) {
      throw new PartifulError(
        'Event questionnaire is enabled but no questions were found.',
        3,
        'validation_error',
      );
    }
    rawQuestions = event[legacyField] as Array<QuestionnaireQuestion | string>;
  }

  // For status-only edits, preserve the exact server-accepted response and version.
  // Rebuilding under a newer host schema would drop answers or demand new ones.
  if (existingResponse && Object.keys(answersByKey).length === 0) return existingResponse;

  const normalizeQuestion = (question: QuestionnaireQuestion | string): QuestionnaireQuestion =>
    typeof question === 'string'
      ? { id: question, text: question, required: false }
      : question;
  const questions = rawQuestions.map(normalizeQuestion);

  let questionnaireVersion = 0;
  const versions = event.questionnaireVersions;
  if (Array.isArray(versions) && versions.length > 0) {
    const latestQuestions = (versions.at(-1) as { questions?: Array<QuestionnaireQuestion | string> } | undefined)?.questions;
    const latest = Array.isArray(latestQuestions) ? latestQuestions.map(normalizeQuestion) : [];
    const sameQuestion = (left: QuestionnaireQuestion, right: QuestionnaireQuestion): boolean =>
      left.id === right.id
      && left.text === right.text
      && left.type === right.type
      && Boolean(left.required) === Boolean(right.required);
    if (latest.length !== questions.length || latest.some((question, index) => !sameQuestion(question, questions[index]!))) {
      throw new PartifulError(
        'The active questionnaire does not match its latest version history. Update answers in Partiful.',
        3,
        'validation_error',
      );
    }
    questionnaireVersion = versions.length - 1;
  } else {
    throw new PartifulError(
      'Event questionnaire version history is missing. Update answers in Partiful.',
      3,
      'validation_error',
    );
  }

  if (existingResponse && existingResponse.questionnaireVersion !== questionnaireVersion) {
    throw new PartifulError(
      'The event questionnaire changed after your existing response. Update answers in Partiful.',
      3,
      'validation_error',
    );
  }

  const questionsByAlias = new Map<string, QuestionnaireQuestion[]>();
  for (const question of questions) {
    for (const alias of new Set([question.id, question.text])) {
      const matches = questionsByAlias.get(alias) ?? [];
      matches.push(question);
      questionsByAlias.set(alias, matches);
    }
  }

  const answers: Record<string, string> = { ...existingResponse?.answers };
  const overriddenQuestionIds = new Set<string>();
  for (const [key, rawValue] of Object.entries(answersByKey)) {
    const matches = questionsByAlias.get(key) ?? [];
    if (matches.length > 1) {
      throw new PartifulError(
        `Ambiguous questionnaire answer key "${key}". Update this questionnaire in Partiful.`,
        3,
        'validation_error',
      );
    }
    const question = matches[0];
    if (!question) {
      throw new PartifulError(`Unknown questionnaire answer key: ${key}`, 3, 'validation_error');
    }
    if (overriddenQuestionIds.has(question.id)) {
      throw new PartifulError(
        `Multiple answers target the same question: ${question.text}`,
        3,
        'validation_error',
      );
    }
    overriddenQuestionIds.add(question.id);
    answers[question.id] = String(rawValue);
  }

  const missing = questions
    .filter((question) => question.required && !String(answers[question.id] ?? '').trim())
    .map((question) => question.text);
  if (missing.length > 0) {
    throw new PartifulError(
      `Missing answer(s) for required question(s): ${missing.join('; ')}`,
      3,
      'validation_error',
    );
  }
  return { questionnaireVersion, answers };
}

/** Inputs for resolveDisplayName(). */
export interface ResolveDisplayNameArgs {
  override?: string | null;
  currentGuest?: { name?: string } | null;
  config?: { name?: string } | null;
  tokenName?: string | null;
}

/**
 * Resolve the display name to submit with an RSVP, in priority order:
 * explicit override > existing guest record > config profile > token-derived.
 * Returns null when nothing is resolvable (caller must surface an error).
 */
export function resolveDisplayName({
  override,
  currentGuest,
  config,
  tokenName,
}: ResolveDisplayNameArgs = {}): string | null {
  if (override && String(override).trim()) return String(override);
  if (currentGuest && currentGuest.name) return currentGuest.name;
  if (config && config.name) return config.name;
  if (tokenName) return tokenName;
  return null;
}
