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

/** User-facing status verbs for `--status` on the rsvp command. */
export const RSVP_STATUSES = ['going', 'maybe', 'declined'];

const DEFAULT_TIMEZONE = 'America/Los_Angeles';

// Unverified legacy field-name fallbacks for the questionnaire question list.
// The verified field is `questionnaire.questions[]` (see eventRequiresQuestionnaire).
const LEGACY_QUESTIONNAIRE_FIELDS = ['questions', 'rsvpQuestions', 'customQuestions'];

// Human input -> wire enum. Only GOING/MAYBE/DECLINED are valid self-RSVP
// statuses. INTERESTED is a DIFFERENT endpoint (markEventInterest), so it is
// deliberately NOT accepted here.
const STATUS_ALIASES = {
  going: 'GOING',
  yes: 'GOING',
  maybe: 'MAYBE',
  declined: 'DECLINED',
  decline: 'DECLINED',
  no: 'DECLINED',
};

/**
 * Normalize a user-supplied status into its wire enum value.
 * Defaults to GOING. Throws a validation PartifulError on unknown input.
 */
export function normalizeStatus(status) {
  if (status === null || status === undefined || status === '') return 'GOING';
  const key = String(status).trim().toLowerCase();
  const wire = STATUS_ALIASES[key];
  if (!wire) {
    throw new PartifulError(
      `Invalid status "${status}". Use one of: going, maybe, declined.`,
      3,
      'validation_error'
    );
  }
  return wire;
}

/**
 * Build the `params` object for POST /addGuest.
 *
 * @param {object} o
 * @param {string} o.eventId          required
 * @param {string} o.name             required (server rejects an empty name)
 * @param {string} [o.status]         going|maybe|declined (default going)
 * @param {string[]} [o.plusOnes]     plus-one names
 * @param {number} [o.count]          headcount incl. plus-ones (derived if omitted)
 * @param {string} [o.message]        optional public comment
 * @param {string} [o.password]       event password if gated
 * @param {string} [o.guestId]        existing guest record id (edit); null on first RSVP
 * @param {string} [o.timezone]       IANA tz (default America/Los_Angeles)
 * @param {object} [o.questionnaireResponse]  verified shape:
 *                 { questionnaireVersion:int, answers:{ "<questionId>": "<answer>" } }
 *                 Build via buildQuestionnaireResponse(); omit when the event has none.
 * @returns {{eventId:string, rsvp:object}}
 */
export function buildRsvpParams(o = {}) {
  const { eventId, name } = o;
  if (!eventId) {
    throw new PartifulError('eventId is required to RSVP.', 3, 'validation_error');
  }
  if (!name || !String(name).trim()) {
    throw new PartifulError('A guest name is required to RSVP (the server rejects an empty name).', 3, 'validation_error');
  }

  const plusOnes = Array.isArray(o.plusOnes) ? o.plusOnes.filter(Boolean) : [];
  const derivedCount = 1 + plusOnes.length;
  const count = Number.isFinite(o.count) ? Math.trunc(o.count) : derivedCount;

  const rsvp = {
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
 *
 * @param {object} o
 * @param {string} o.eventId       required
 * @param {boolean} o.interested   true to mark, false to remove
 * @param {string} [o.source]      analytics source (default DISCOVER)
 */
export function buildInterestParams(o = {}) {
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
export function isTicketedEvent(event) {
  if (!event || typeof event !== 'object') return false;
  if (event.ticketing && (event.ticketing.enabled ?? true)) return true;
  if (event.ticketInfo && typeof event.ticketInfo === 'object') return true;
  if (event.requiresPayment === true) return true;
  if (event.isTicketed === true) return true;
  return false;
}

/**
 * Detect an event that requires questionnaire answers before an RSVP submits.
 *
 * Verified live (2026-07-24, host-side recon on a throwaway event, see
 * .wayfinder/tickets/07 follow-up). The authoritative shape in the event
 * object is:
 *   questionnaireEnabled: true
 *   questionnaire: {
 *     createdBy, createdAt,
 *     questions: [ { id, type: 'short_answer', text, required } ]
 *   }
 *   questionnaireVersions: [ { ...same shape... } ]   // history
 * When no questionnaire exists these keys are simply ABSENT from the event.
 *
 * Detection rule: questionnaireEnabled truthy AND at least one question present.
 * (Legacy field names kept as a defensive fallback in case older/host payloads
 * expose the list under a different key.)
 */
export function eventRequiresQuestionnaire(event) {
  if (!event || typeof event !== 'object') return false;

  const q = event.questionnaire;
  const hasQuestions =
    q && typeof q === 'object' && Array.isArray(q.questions) && q.questions.length > 0;

  // Primary, verified signal.
  if (event.questionnaireEnabled && hasQuestions) return true;

  // Defensive fallback: a populated questionnaire.questions[] even if the
  // enabled flag is absent (older payloads / host-scoped views).
  if (hasQuestions) return true;

  // Legacy field-name fallbacks (unverified, kept for resilience).
  for (const f of LEGACY_QUESTIONNAIRE_FIELDS) {
    if (Array.isArray(event[f]) && event[f].length > 0) return true;
  }
  return false;
}

/**
 * Build the questionnaireResponse object for an RSVP, given the event's
 * questions and a map of { questionId | questionText -> answer } supplied by
 * the caller. Returns null when the event has no questionnaire.
 *
 * Verified wire shape (guest.questionnaireResponse, 2026-07-24):
 *   { questionnaireVersion: <int>, answers: { "<questionId>": "<answer>" } }
 * The answers map is keyed by question id, NOT by text or index.
 *
 * Throws a validation error if a REQUIRED question has no supplied answer, so
 * the CLI refuses rather than submitting an incomplete RSVP.
 */
export function buildQuestionnaireResponse(event, answersByKey = {}) {
  if (!eventRequiresQuestionnaire(event)) return null;
  const questions = event.questionnaire.questions;
  const answers = {};
  const missing = [];
  for (const question of questions) {
    // Accept an answer supplied under the question id OR its exact text.
    const val =
      answersByKey[question.id] ??
      answersByKey[question.text] ??
      undefined;
    if (val === undefined || val === null || String(val).trim() === '') {
      if (question.required) missing.push(question.text);
      continue;
    }
    answers[question.id] = String(val);
  }
  if (missing.length > 0) {
    throw new PartifulError(
      `Missing answer(s) for required question(s): ${missing.join('; ')}`,
      3,
      'validation_error'
    );
  }
  return {
    // Version index into questionnaireVersions; current is the last entry.
    questionnaireVersion: Array.isArray(event.questionnaireVersions)
      ? Math.max(0, event.questionnaireVersions.length - 1)
      : 0,
    answers,
  };
}

/**
 * Resolve the display name to submit with an RSVP, in priority order:
 * explicit override > existing guest record > config profile > token-derived.
 * Returns null when nothing is resolvable (caller must surface an error).
 */
export function resolveDisplayName({ override, currentGuest, config, tokenName } = {}) {
  if (override && String(override).trim()) return String(override);
  if (currentGuest && currentGuest.name) return currentGuest.name;
  if (config && config.name) return config.name;
  if (tokenName) return tokenName;
  return null;
}
