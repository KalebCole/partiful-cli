/**
 * Mock-based orchestration tests for rsvpAction — covers the branches that can't
 * be reached through --dry-run (which skips the network): edit path, ticketed /
 * questionnaire guards, the confirmation-abort path, and the fail-CLOSED
 * behaviour of the read-before-write helpers on API error.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';

// --- module mocks -----------------------------------------------------------
const apiRequest = vi.fn();
const jsonOutput = vi.fn();
const jsonError = vi.fn();
const confirm = vi.fn();

vi.mock('../src/lib/http.js', () => ({ apiRequest: (...a) => apiRequest(...a) }));
vi.mock('../src/lib/output.js', () => ({
  jsonOutput: (...a) => jsonOutput(...a),
  jsonError: (...a) => jsonError(...a),
  EXIT: { SUCCESS: 0, API_ERROR: 1, AUTH_ERROR: 2, VALIDATION_ERROR: 3, NOT_FOUND: 4, INTERNAL_ERROR: 5 },
}));
vi.mock('../src/lib/auth.js', () => ({
  loadConfig: () => ({ name: 'Config Name', userId: 'U1' }),
  getValidToken: async () => 'tok',
  wrapPayload: (_c, body) => body,
  decodeJwtPayload: () => ({ name: 'Token Name' }),
}));
vi.mock('../src/lib/events.js', () => ({ confirm: (...a) => confirm(...a) }));

const { rsvpAction } = await import('../src/commands/rsvp.js');

// Commander-like cmd stub whose optsWithGlobals returns the given globals.
const mkCmd = (globals = {}) => ({ optsWithGlobals: () => globals });

// Route apiRequest by endpoint so each test can script the reads/writes.
function routeApi({ event = null, currentGuest = null, addGuest = { id: 'NEW' }, throwOn = null } = {}) {
  apiRequest.mockImplementation(async (_m, endpoint) => {
    if (throwOn && endpoint === throwOn) throw new Error(`boom ${endpoint}`);
    if (endpoint === '/getEventInfo') return { result: { data: { event } } };
    if (endpoint === '/getCurrentGuest') return { result: { data: { currentGuest } } };
    if (endpoint === '/addGuest') return { result: { data: { guest: addGuest } } };
    return { result: { data: {} } };
  });
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('rsvpAction edit path (existing guest record)', () => {
  it('sends the existing guestId and reports updated:true', async () => {
    routeApi({ currentGuest: { id: 'G7', name: 'Server Name', status: 'MAYBE' }, addGuest: { id: 'G7' } });
    await rsvpAction('EV1', { status: 'going' }, mkCmd({ yes: true }));

    const addCall = apiRequest.mock.calls.find(c => c[1] === '/addGuest');
    expect(addCall).toBeDefined();
    expect(addCall[3].data.params.rsvp.guestId).toBe('G7');
    // name resolves from the existing guest record over config/token
    expect(addCall[3].data.params.rsvp.name).toBe('Server Name');

    const out = jsonOutput.mock.calls.at(-1)[0];
    expect(out.updated).toBe(true);
    expect(out.guestId).toBe('G7');
  });

  it('defaults non-self guest statuses to GOING when status is omitted', async () => {
    routeApi({ currentGuest: { id: 'G7', name: 'Server Name', status: 'SENT' }, addGuest: { id: 'G7' } });
    await rsvpAction('EV1', { message: 'I can make it' }, mkCmd({ yes: true }));

    const addCall = apiRequest.mock.calls.find(c => c[1] === '/addGuest');
    expect(addCall[3].data.params.rsvp.status).toBe('GOING');
  });

  it('keeps a plain dry-run offline', async () => {
    await rsvpAction('EV1', { name: 'Kaleb' }, mkCmd({ dryRun: true }));

    expect(apiRequest).not.toHaveBeenCalled();
    expect(jsonOutput).toHaveBeenCalled();
  });
});

describe('rsvpAction ticketed guard', () => {
  it('refuses a ticketed event and never calls addGuest', async () => {
    routeApi({ event: { ticketing: { enabled: true } } });
    await rsvpAction('EV1', { name: 'Kaleb' }, mkCmd({ yes: true }));

    expect(jsonError).toHaveBeenCalledWith(expect.stringMatching(/ticketed|paid/i), 3, 'validation_error');
    expect(apiRequest.mock.calls.some(c => c[1] === '/addGuest')).toBe(false);
  });

  it('applies the ticket guard to answer-aware dry-runs', async () => {
    routeApi({ event: { ticketing: { enabled: true } } });
    await rsvpAction('EV1', { answer: ['q1=value'] }, mkCmd({ yes: true, dryRun: true }));

    expect(jsonError).toHaveBeenCalledWith(expect.stringMatching(/ticketed|paid/i), 3, 'validation_error');
    expect(jsonOutput).not.toHaveBeenCalled();
    expect(apiRequest.mock.calls.some(c => c[1] === '/addGuest')).toBe(false);
  });
});

describe('rsvpAction questionnaire guard', () => {
  const questionnaireQuestions = [
    { id: 'q1', text: 'Required answer?', required: true },
    { id: 'q2', text: 'Optional answer?', required: false },
  ];
  const questionnaireEvent = {
    questionnaireEnabled: true,
    questionnaireVersions: [{ questions: questionnaireQuestions }],
    questionnaire: { questions: questionnaireQuestions },
  };

  it('preserves existing answers during a status-only update', async () => {
    const existingResponse = {
      questionnaireVersion: 0,
      answers: { q1: 'Saved', q2: 'Also saved' },
    };
    routeApi({
      event: questionnaireEvent,
      currentGuest: { id: 'G7', name: 'Kaleb', questionnaireResponse: existingResponse },
      addGuest: { id: 'G7' },
    });

    await rsvpAction('EV1', { status: 'maybe' }, mkCmd({ yes: true }));

    const addCall = apiRequest.mock.calls.find(c => c[1] === '/addGuest');
    expect(addCall[3].data.params.rsvp.questionnaireResponse).toEqual(existingResponse);
  });

  it('preserves the existing response after the host removes the questionnaire', async () => {
    const existingResponse = {
      questionnaireVersion: 0,
      answers: { q1: 'Saved answer' },
    };
    routeApi({
      event: { questionnaireEnabled: false, questionnaire: { questions: [] } },
      currentGuest: { id: 'G7', name: 'Kaleb', questionnaireResponse: existingResponse },
      addGuest: { id: 'G7' },
    });

    await rsvpAction('EV1', { status: 'maybe' }, mkCmd({ yes: true }));

    const addCall = apiRequest.mock.calls.find(c => c[1] === '/addGuest');
    expect(addCall[3].data.params.rsvp.questionnaireResponse).toEqual(existingResponse);
  });

  it('overrides one answer without dropping other existing answers', async () => {
    routeApi({
      event: questionnaireEvent,
      currentGuest: {
        id: 'G7',
        name: 'Kaleb',
        status: 'MAYBE',
        count: 2,
        plusOnes: ['Friend'],
        rsvpMessage: 'Saved message',
        questionnaireResponse: {
          questionnaireVersion: 0,
          answers: { q1: 'Old', q2: 'Keep me' },
        },
      },
      addGuest: { id: 'G7' },
    });

    await rsvpAction('EV1', { answer: ['q1=New'] }, mkCmd({ yes: true }));

    const addCall = apiRequest.mock.calls.find(c => c[1] === '/addGuest');
    expect(addCall[3].data.params.rsvp).toMatchObject({
      status: 'MAYBE',
      count: 2,
      plusOnes: ['Friend'],
      message: 'Saved message',
      questionnaireResponse: {
        questionnaireVersion: 0,
        answers: { q1: 'New', q2: 'Keep me' },
      },
    });
  });

  it('uses the live read path for answer dry-runs and preserves existing state', async () => {
    routeApi({
      event: questionnaireEvent,
      currentGuest: {
        id: 'G7',
        name: 'Kaleb',
        status: 'MAYBE',
        questionnaireResponse: {
          questionnaireVersion: 0,
          answers: { q1: 'Old', q2: 'Keep me' },
        },
      },
    });

    await rsvpAction('EV1', { answer: ['q1=New'] }, mkCmd({ yes: true, dryRun: true }));

    expect(apiRequest.mock.calls.some(c => c[1] === '/getCurrentGuest')).toBe(true);
    const output = jsonOutput.mock.calls.at(-1)[0];
    expect(output.payload.data.params.rsvp).toMatchObject({
      guestId: 'G7',
      status: 'MAYBE',
      questionnaireResponse: {
        questionnaireVersion: 0,
        answers: { q1: 'New', q2: 'Keep me' },
      },
    });
    expect(apiRequest.mock.calls.some(c => c[1] === '/addGuest')).toBe(false);
  });

  it('refuses a questionnaire with an unanswered required question and never calls addGuest', async () => {
    routeApi({ event: questionnaireEvent });
    await rsvpAction('EV1', { name: 'Kaleb' }, mkCmd({ yes: true }));

    expect(jsonError).toHaveBeenCalledWith(expect.stringMatching(/required question/i), 3, 'validation_error', null);
    expect(apiRequest.mock.calls.some(c => c[1] === '/addGuest')).toBe(false);
  });

  it('allows an optional questionnaire with no answers', async () => {
    const questions = [{ id: 'q1', text: 'Optional answer?', required: false }];
    routeApi({
      event: {
        questionnaireEnabled: true,
        questionnaireVersions: [{ questions }],
        questionnaire: { questions },
      },
    });
    await rsvpAction('EV1', { name: 'Kaleb' }, mkCmd({ yes: true }));

    const addCall = apiRequest.mock.calls.find(c => c[1] === '/addGuest');
    expect(addCall[3].data.params.rsvp.questionnaireResponse).toEqual({
      questionnaireVersion: 0,
      answers: {},
    });
  });

  it('submits supplied answers inside questionnaireResponse', async () => {
    const questions = [{ id: 'q1', text: 'Dietary restrictions?', required: true }];
    routeApi({
      event: {
        questionnaireEnabled: true,
        questionnaireVersions: [{ questions }],
        questionnaire: { questions },
      },
    });
    await rsvpAction('EV1', { name: 'Kaleb', answer: ['q1=Vegan'] }, mkCmd({ yes: true }));

    const addCall = apiRequest.mock.calls.find(c => c[1] === '/addGuest');
    expect(addCall).toBeDefined();
    expect(addCall[3].data.params.rsvp.questionnaireResponse).toEqual({
      questionnaireVersion: 0,
      answers: { q1: 'Vegan' },
    });
  });

  it('refuses incomplete supplied answers and never calls addGuest', async () => {
    const questions = [{ id: 'q1', text: 'Dietary restrictions?', required: true }];
    routeApi({
      event: {
        questionnaireEnabled: true,
        questionnaireVersions: [{ questions }],
        questionnaire: { questions },
      },
    });
    await rsvpAction('EV1', { name: 'Kaleb', answer: ['other=value'] }, mkCmd({ yes: true }));

    expect(jsonError).toHaveBeenCalledWith(expect.stringMatching(/unknown questionnaire answer key/i), 3, 'validation_error', null);
    expect(apiRequest.mock.calls.some(c => c[1] === '/addGuest')).toBe(false);
  });

  it('refuses --answer for an event without a questionnaire', async () => {
    routeApi({ event: { title: 'Plain party' }, currentGuest: { id: 'G7', name: 'Kaleb' } });
    await rsvpAction('EV1', { answer: ['q1=Vegan'] }, mkCmd({ yes: true }));

    expect(jsonError).toHaveBeenCalledWith(
      expect.stringMatching(/does not expose a host questionnaire/i), 3, 'validation_error', null,
    );
    expect(apiRequest.mock.calls.some(c => c[1] === '/addGuest')).toBe(false);
  });
});

describe('rsvpAction confirmation gate', () => {
  it('aborts when the user declines and never calls addGuest', async () => {
    routeApi({});
    confirm.mockResolvedValue(false);
    await rsvpAction('EV1', { name: 'Kaleb' }, mkCmd({})); // no --yes

    expect(confirm).toHaveBeenCalled();
    const out = jsonOutput.mock.calls.at(-1)[0];
    expect(out.rsvp).toBe(false);
    expect(apiRequest.mock.calls.some(c => c[1] === '/addGuest')).toBe(false);
  });

  it('proceeds when the user confirms', async () => {
    routeApi({});
    confirm.mockResolvedValue(true);
    await rsvpAction('EV1', { name: 'Kaleb' }, mkCmd({}));
    expect(apiRequest.mock.calls.some(c => c[1] === '/addGuest')).toBe(true);
  });
});

describe('rsvpAction fail-CLOSED on read error', () => {
  it('aborts (no addGuest) when getEventInfo throws', async () => {
    routeApi({ throwOn: '/getEventInfo' });
    await rsvpAction('EV1', { name: 'Kaleb' }, mkCmd({ yes: true }));
    // Error surfaced; the write must NOT happen with guards disabled.
    expect(apiRequest.mock.calls.some(c => c[1] === '/addGuest')).toBe(false);
    expect(jsonError).toHaveBeenCalled();
  });

  it('aborts (no addGuest) when getCurrentGuest throws — avoids duplicate record', async () => {
    routeApi({ throwOn: '/getCurrentGuest' });
    await rsvpAction('EV1', { name: 'Kaleb' }, mkCmd({ yes: true }));
    expect(apiRequest.mock.calls.some(c => c[1] === '/addGuest')).toBe(false);
    expect(jsonError).toHaveBeenCalled();
  });
});
