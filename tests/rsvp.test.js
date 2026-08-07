/**
 * Unit tests for the RSVP / interest library (src/lib/rsvp.js).
 *
 * These cover the PURE, side-effect-free builders and guards that back the
 * `events rsvp set` / `explore rsvp set` and `events interested` / `explore interested`
 * commands. All network/orchestration is tested separately via CLI dry-run
 * integration tests; here we pin the wire-payload shapes and the refusal guards.
 */

import { describe, it, expect } from 'vitest';
import {
  RSVP_STATUSES,
  normalizeStatus,
  buildRsvpParams,
  buildInterestParams,
  isTicketedEvent,
  eventRequiresQuestionnaire,
  buildQuestionnaireResponse,
  parseQuestionnaireAnswers,
  resolveDisplayName,
} from '../src/lib/rsvp.js';

describe('normalizeStatus', () => {
  it('defaults to GOING when no status given', () => {
    expect(normalizeStatus()).toBe('GOING');
    expect(normalizeStatus(null)).toBe('GOING');
    expect(normalizeStatus(undefined)).toBe('GOING');
  });

  it('maps the three canonical statuses case-insensitively', () => {
    expect(normalizeStatus('going')).toBe('GOING');
    expect(normalizeStatus('Maybe')).toBe('MAYBE');
    expect(normalizeStatus('DECLINED')).toBe('DECLINED');
  });

  it('accepts human synonyms', () => {
    expect(normalizeStatus('yes')).toBe('GOING');
    expect(normalizeStatus('no')).toBe('DECLINED');
    expect(normalizeStatus('decline')).toBe('DECLINED');
  });

  it('trims surrounding whitespace', () => {
    expect(normalizeStatus('  going  ')).toBe('GOING');
  });

  it('rejects unknown statuses with a helpful message', () => {
    expect(() => normalizeStatus('interested')).toThrow(/status/i);
    expect(() => normalizeStatus('bogus')).toThrow(/going/i);
  });

  it('exports exactly the three self-RSVP statuses', () => {
    expect(RSVP_STATUSES).toEqual(['going', 'maybe', 'declined']);
  });
});

describe('buildRsvpParams', () => {
  it('builds the canonical first-RSVP payload with guestId null', () => {
    const params = buildRsvpParams({ eventId: 'EV1', name: 'Kaleb Cole' });
    expect(params).toEqual({
      eventId: 'EV1',
      rsvp: {
        name: 'Kaleb Cole',
        count: 1,
        plusOnes: [],
        message: null,
        emailInvitationId: null,
        status: 'GOING',
        guestId: null,
        timezone: 'America/Los_Angeles',
        password: null,
      },
    });
  });

  it('passes an existing guestId back for edits', () => {
    const params = buildRsvpParams({ eventId: 'EV1', name: 'Kaleb', guestId: 'G9', status: 'declined' });
    expect(params.rsvp.guestId).toBe('G9');
    expect(params.rsvp.status).toBe('DECLINED');
  });

  it('derives count from plus-ones when --count is omitted', () => {
    const params = buildRsvpParams({ eventId: 'EV1', name: 'Kaleb', plusOnes: ['Maddie', 'Justin'] });
    expect(params.rsvp.count).toBe(3); // self + 2
    expect(params.rsvp.plusOnes).toEqual(['Maddie', 'Justin']);
  });

  it('honours an explicit --count over the derived value', () => {
    const params = buildRsvpParams({ eventId: 'EV1', name: 'Kaleb', plusOnes: ['Maddie'], count: 5 });
    expect(params.rsvp.count).toBe(5);
  });

  it('rejects a count lower than self plus named plus-ones', () => {
    expect(() => buildRsvpParams({
      eventId: 'EV1', name: 'Kaleb', plusOnes: ['Maddie', 'Justin'], count: 2,
    })).toThrow(/count.*plus-one/i);
  });

  it('carries message, password and timezone through', () => {
    const params = buildRsvpParams({
      eventId: 'EV1', name: 'Kaleb', message: 'stoked', password: 'sesame', timezone: 'America/New_York',
    });
    expect(params.rsvp.message).toBe('stoked');
    expect(params.rsvp.password).toBe('sesame');
    expect(params.rsvp.timezone).toBe('America/New_York');
  });

  it('requires an eventId', () => {
    expect(() => buildRsvpParams({ name: 'Kaleb' })).toThrow(/eventId/i);
  });

  it('requires a name (server rejects an empty guest name)', () => {
    expect(() => buildRsvpParams({ eventId: 'EV1', name: '' })).toThrow(/name/i);
  });

  it('never emits an em dash in a passed-through message unchanged is caller concern, but count is always an integer', () => {
    const params = buildRsvpParams({ eventId: 'EV1', name: 'Kaleb', count: 2 });
    expect(Number.isInteger(params.rsvp.count)).toBe(true);
  });
});

describe('buildInterestParams', () => {
  it('marks interested true with the DISCOVER source by default', () => {
    expect(buildInterestParams({ eventId: 'EV1', interested: true })).toEqual({
      eventId: 'EV1', interested: true, source: 'DISCOVER',
    });
  });

  it('marks interested false for --remove', () => {
    expect(buildInterestParams({ eventId: 'EV1', interested: false })).toEqual({
      eventId: 'EV1', interested: false, source: 'DISCOVER',
    });
  });

  it('requires an eventId', () => {
    expect(() => buildInterestParams({ interested: true })).toThrow(/eventId/i);
  });
});

describe('isTicketedEvent', () => {
  it('flags events with a ticketing block', () => {
    expect(isTicketedEvent({ ticketing: { enabled: true } })).toBe(true);
  });

  it('flags events with a paid/Stripe indicator', () => {
    expect(isTicketedEvent({ ticketInfo: { priceCents: 500 } })).toBe(true);
    expect(isTicketedEvent({ requiresPayment: true })).toBe(true);
  });

  it('does not flag a plain free event', () => {
    expect(isTicketedEvent({ title: 'Free Party' })).toBe(false);
    expect(isTicketedEvent({})).toBe(false);
    expect(isTicketedEvent(null)).toBe(false);
  });
});

describe('eventRequiresQuestionnaire', () => {
  it('detects a non-empty questions array', () => {
    expect(eventRequiresQuestionnaire({ questions: [{ id: 'q1', required: true }] })).toBe(true);
  });

  it('detects alternate field names Partiful may use', () => {
    expect(eventRequiresQuestionnaire({ questionnaire: { questions: [{ id: 'q1' }] } })).toBe(true);
    expect(eventRequiresQuestionnaire({ rsvpQuestions: [{ id: 'q1' }] })).toBe(true);
    expect(eventRequiresQuestionnaire({ customQuestions: [{ id: 'q1' }] })).toBe(true);
  });

  it('returns false for an event with no questions', () => {
    expect(eventRequiresQuestionnaire({ title: 'No questions' })).toBe(false);
    expect(eventRequiresQuestionnaire({ questions: [] })).toBe(false);
    expect(eventRequiresQuestionnaire({})).toBe(false);
    expect(eventRequiresQuestionnaire(null)).toBe(false);
  });
});

describe('resolveDisplayName', () => {
  it('prefers an existing guest record name', () => {
    const name = resolveDisplayName({ currentGuest: { name: 'From Guest' }, config: { name: 'From Config' }, tokenName: 'From Token' });
    expect(name).toBe('From Guest');
  });

  it('falls back to config name, then token name', () => {
    expect(resolveDisplayName({ config: { name: 'From Config' }, tokenName: 'From Token' })).toBe('From Config');
    expect(resolveDisplayName({ tokenName: 'From Token' })).toBe('From Token');
  });

  it('honours an explicit override above everything', () => {
    const name = resolveDisplayName({ override: 'Override', currentGuest: { name: 'Guest' }, config: { name: 'Config' } });
    expect(name).toBe('Override');
  });

  it('returns null when nothing is resolvable (caller must error)', () => {
    expect(resolveDisplayName({})).toBeNull();
  });
});

// Live-verified questionnaire shape (2026-07-24 host-side recon).
// Event fields: questionnaireEnabled + questionnaire.questions[{id,type,text,required}].
// Answer storage: guest.questionnaireResponse = { questionnaireVersion, answers:{id:val} }.
describe('questionnaire (verified shape)', () => {
  const questions = [
    { id: '111', type: 'short_answer', text: 'Dietary restrictions?', required: true },
    { id: '222', type: 'short_answer', text: 'Song request?', required: false },
  ];
  const qEvent = {
    questionnaireEnabled: true,
    questionnaireVersions: [{ questions }],
    questionnaire: { questions },
  };

  it('detects a questionnaire via questionnaireEnabled + questions[]', () => {
    expect(eventRequiresQuestionnaire(qEvent)).toBe(true);
  });

  it('returns false when the questionnaire keys are absent', () => {
    expect(eventRequiresQuestionnaire({ title: 'plain event' })).toBe(false);
  });

  it('returns false for an empty questions list', () => {
    expect(eventRequiresQuestionnaire({ questionnaireEnabled: true, questionnaire: { questions: [] } })).toBe(false);
  });

  it('builds answers keyed by question id', () => {
    const resp = buildQuestionnaireResponse(qEvent, { '111': 'None', '222': 'Anything' });
    expect(resp).toEqual({
      questionnaireVersion: 0,
      answers: { '111': 'None', '222': 'Anything' },
    });
  });

  it('accepts answers supplied by question text too', () => {
    const resp = buildQuestionnaireResponse(qEvent, { 'Dietary restrictions?': 'Vegan' });
    expect(resp.answers['111']).toBe('Vegan');
  });

  it('merges supplied overrides with an existing response from the same version', () => {
    const existing = {
      questionnaireVersion: 0,
      answers: { '111': 'None', '222': 'Jazz' },
    };
    const resp = buildQuestionnaireResponse(qEvent, { 'Dietary restrictions?': 'Vegan' }, existing);
    expect(resp).toEqual({
      questionnaireVersion: 0,
      answers: { '111': 'Vegan', '222': 'Jazz' },
    });
  });

  it('preserves an existing response unchanged when no overrides are supplied', () => {
    const existing = {
      questionnaireVersion: 0,
      answers: { '111': 'None', '222': 'Jazz' },
    };
    expect(buildQuestionnaireResponse(qEvent, {}, existing)).toBe(existing);
  });

  it('preserves the exact existing version during a status-only update after a host edit', () => {
    const oldQuestions = questions;
    const newQuestions = [{ id: '333', text: 'New required question?', required: true }];
    const changedEvent = {
      questionnaireEnabled: true,
      questionnaireVersions: [
        { questions: oldQuestions },
        { questions: newQuestions },
      ],
      questionnaire: { questions: newQuestions },
    };
    const existing = {
      questionnaireVersion: 0,
      answers: { '111': 'None', '222': 'Jazz' },
    };
    expect(buildQuestionnaireResponse(changedEvent, {}, existing)).toBe(existing);
  });

  it('rejects duplicate question text instead of applying one answer twice', () => {
    const ambiguousQuestions = [
      { id: '111', text: 'Anything else?', required: false },
      { id: '222', text: 'Anything else?', required: false },
    ];
    const ambiguousEvent = {
      ...qEvent,
      questionnaireVersions: [{ questions: ambiguousQuestions }],
      questionnaire: { questions: ambiguousQuestions },
    };
    expect(() => buildQuestionnaireResponse(ambiguousEvent, { 'Anything else?': 'No' }))
      .toThrow(/ambiguous questionnaire answer key/i);
  });

  it('rejects ID and text aliases supplied for the same question', () => {
    expect(() => buildQuestionnaireResponse(qEvent, {
      '111': 'None',
      'Dietary restrictions?': 'Vegan',
    })).toThrow(/multiple answers.*same question/i);
  });

  it('rejects a key that is one question ID and another question text', () => {
    const collidingQuestions = [
      { id: 'shared', text: 'First question', required: false },
      { id: 'second', text: 'shared', required: false },
    ];
    const event = {
      questionnaireEnabled: true,
      questionnaireVersions: [{ questions: collidingQuestions }],
      questionnaire: { questions: collidingQuestions },
    };
    expect(() => buildQuestionnaireResponse(event, { shared: 'Answer' }))
      .toThrow(/ambiguous questionnaire answer key/i);
  });

  it('rejects partial updates against a newer questionnaire version', () => {
    const existing = { questionnaireVersion: 0, answers: { '111': 'Saved' } };
    const changedEvent = {
      ...qEvent,
      questionnaireVersions: [
        { questions: [{ id: 'old', text: 'Old question', required: false }] },
        { questions },
      ],
    };
    expect(() => buildQuestionnaireResponse(changedEvent, { '111': 'New' }, existing))
      .toThrow(/questionnaire changed/i);
  });

  it('omits optional questions left unanswered', () => {
    const resp = buildQuestionnaireResponse(qEvent, { '111': 'None' });
    expect(resp.answers).toEqual({ '111': 'None' });
  });

  it('rejects supplied keys that match no question instead of silently dropping them', () => {
    expect(() => buildQuestionnaireResponse(qEvent, { unknown: 'value', '111': 'None' }))
      .toThrow(/unknown questionnaire answer key/i);
  });

  it('throws when a required question is unanswered', () => {
    expect(() => buildQuestionnaireResponse(qEvent, { '222': 'Song' })).toThrow(/required question/i);
  });

  it('fails closed when verified questionnaire version history is missing', () => {
    expect(() => buildQuestionnaireResponse({
      questionnaireEnabled: true,
      questionnaire: { questions },
    }, { '111': 'None' })).toThrow(/version history/i);
  });

  it('fails closed when the latest version does not match the active questionnaire', () => {
    expect(() => buildQuestionnaireResponse({
      ...qEvent,
      questionnaireVersions: [{ questions: [{ ...questions[0], text: 'Old question' }] }],
    }, { '111': 'None' })).toThrow(/does not match/i);
  });

  it('uses the matching latest questionnaire version index', () => {
    const response = buildQuestionnaireResponse({
      ...qEvent,
      questionnaireVersions: [
        { questions: [{ id: 'old', text: 'Old question', required: false }] },
        { questions },
      ],
    }, { '111': 'None' });
    expect(response.questionnaireVersion).toBe(1);
  });

  it('returns null for a non-questionnaire event', () => {
    expect(buildQuestionnaireResponse({ title: 'plain' }, {})).toBeNull();
  });

  it('attaches questionnaireResponse into the addGuest rsvp payload', () => {
    const qr = buildQuestionnaireResponse(qEvent, { '111': 'None' });
    const { rsvp } = buildRsvpParams({ eventId: 'e1', name: 'Kaleb', questionnaireResponse: qr });
    expect(rsvp.questionnaireResponse).toEqual({ questionnaireVersion: 0, answers: { '111': 'None' } });
  });

  it('leaves questionnaireResponse off the payload when not supplied', () => {
    const { rsvp } = buildRsvpParams({ eventId: 'e1', name: 'Kaleb' });
    expect(rsvp).not.toHaveProperty('questionnaireResponse');
  });
});

describe('parseQuestionnaireAnswers', () => {
  it('parses repeated key=value answers and preserves equals signs in values', () => {
    expect(parseQuestionnaireAnswers(['111=Vegan', 'Song request?=A=B'])).toEqual({
      '111': 'Vegan',
      'Song request?': 'A=B',
    });
  });

  it('rejects malformed or empty answer pairs', () => {
    expect(() => parseQuestionnaireAnswers(['missing-separator'])).toThrow(/key=value/i);
    expect(() => parseQuestionnaireAnswers(['=value'])).toThrow(/key/i);
    expect(() => parseQuestionnaireAnswers(['111='])).toThrow(/value/i);
  });

  it('rejects a repeated answer key instead of silently taking the final value', () => {
    expect(() => parseQuestionnaireAnswers(['111=None', '111=Vegan']))
      .toThrow(/duplicate questionnaire answer key/i);
  });

  it('retains prototype-like keys so unknown-key validation cannot be bypassed', () => {
    const parsed = parseQuestionnaireAnswers(['__proto__=value']);
    expect(Object.hasOwn(parsed, '__proto__')).toBe(true);
    expect(parsed.__proto__).toBe('value');
  });
});

describe('buildRsvpParams — count validation (Fix 1)', () => {
  it('throws for count 0', () => {
    expect(() => buildRsvpParams({ eventId: 'EV1', name: 'A', count: 0 }))
      .toThrow(/Invalid --count/);
  });

  it('throws for negative count', () => {
    expect(() => buildRsvpParams({ eventId: 'EV1', name: 'A', count: -3 }))
      .toThrow(/Invalid --count/);
  });

  it('throws for NaN count (simulating bad parseInt)', () => {
    expect(() => buildRsvpParams({ eventId: 'EV1', name: 'A', count: NaN }))
      .toThrow(/Invalid --count/);
  });

  it('accepts a positive explicit count', () => {
    const { rsvp } = buildRsvpParams({ eventId: 'EV1', name: 'A', count: 5 });
    expect(rsvp.count).toBe(5);
  });

  it('derives count from plus-ones when count is omitted (null/undefined)', () => {
    const { rsvp } = buildRsvpParams({ eventId: 'EV1', name: 'A', plusOnes: ['B', 'C'] });
    expect(rsvp.count).toBe(3); // 1 + 2
  });
});

describe('buildQuestionnaireResponse — legacy questionnaire guard (Fix 2)', () => {
  const legacyEvent = {
    // No event.questionnaire — detected only via the legacy `questions` field
    questions: [
      { id: 'q1', text: 'Dietary needs?', required: true },
      { id: 'q2', text: 'Song request?', required: false },
    ],
  };

  it('does not throw a TypeError when event.questionnaire is absent', () => {
    let caughtError;
    try {
      buildQuestionnaireResponse(legacyEvent, { q1: 'None' });
    } catch (e) {
      caughtError = e;
    }
    // Either succeeds or throws a handled PartifulError — never a raw TypeError
    if (caughtError) {
      expect(caughtError).not.toBeInstanceOf(TypeError);
      expect(caughtError.constructor.name).toBe('PartifulError');
    }
  });

  it('fails closed for a legacy event without version history', () => {
    expect(() => buildQuestionnaireResponse(legacyEvent, { q1: 'Vegan' }))
      .toThrow(/version history/i);
  });

  it('uses validated latest-version history for a legacy question field', () => {
    const latestQuestions = legacyEvent.questions;
    const event = {
      ...legacyEvent,
      questionnaireVersions: [
        { questions: [{ id: 'old', text: 'Old?', required: false }] },
        { questions: latestQuestions },
      ],
    };
    expect(buildQuestionnaireResponse(event, { q1: 'Vegan' })).toEqual({
      questionnaireVersion: 1,
      answers: { q1: 'Vegan' },
    });
  });

  it('still returns correct versioned answers for primary event.questionnaire shape', () => {
    const questions = [
      { id: '111', type: 'short_answer', text: 'Dietary restrictions?', required: true },
    ];
    const primaryEvent = {
      questionnaireEnabled: true,
      questionnaireVersions: [{ questions }],
      questionnaire: { questions },
    };
    const resp = buildQuestionnaireResponse(primaryEvent, { '111': 'None' });
    expect(resp).toEqual({ questionnaireVersion: 0, answers: { '111': 'None' } });
  });
});
