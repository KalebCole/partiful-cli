import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { mapEventSummary } from '../src/lib/events.js';
import { run } from './helpers.js';

const rootFile = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');

const expectedStatuses = {
  GOING: { meaning: 'User accepted the invitation', category: 'Going' },
  MAYBE: { meaning: 'User replied maybe', category: 'Maybe' },
  INTERESTED: { meaning: 'User marked interest', category: 'Interested' },
  DECLINED: { meaning: 'User declined the invitation', category: 'Declined' },
  SENT: {
    meaning: 'Invitation sent to the authenticated user; no RSVP reply yet',
    category: 'Awaiting RSVP',
  },
  null: { meaning: 'No personal guest RSVP record is present', category: 'No RSVP record' },
};

describe('events.list RSVP semantics contract', () => {
  it('publishes every myRsvp enum meaning and category in command schema output', () => {
    const schema = run(['schema', 'events.list']).data;

    expect(schema.output.fields.myRsvp.values).toEqual(expectedStatuses);
  });

  it('states that SENT is inbound and host status takes categorization precedence', () => {
    const schema = run(['schema', 'events.list']).data;

    expect(schema.output.fields.myRsvp.warning).toMatch(
      /does not mean the authenticated user sent/i,
    );
    expect(schema.output.categorization.precedence).toBe('isHost === true');
    expect(schema.output.categorization.hosting).toMatch(/isHost.*true/i);
    expect(schema.output.categorization.awaitingRsvp).toMatch(/myRsvp.*SENT/i);
  });

  it('preserves SENT as the raw myRsvp value without adding a derived runtime field', () => {
    const summary = mapEventSummary(
      {
        id: 'evt-sent',
        guest: { status: 'SENT' },
        ownerIds: ['someone-else'],
        guestStatusCounts: {},
      },
      'me',
    );

    expect(summary.myRsvp).toBe('SENT');
    expect(summary).not.toHaveProperty('myRsvpCategory');
    expect(summary).not.toHaveProperty('myRsvpLabel');
  });

  it.each([
    ['AGENTS.md', /SENT.*invitation sent to (?:the )?authenticated user.*awaiting.*RSVP/is],
    ['README.md', /SENT.*invitation sent to (?:the )?authenticated user.*awaiting.*RSVP/is],
    ['skills/partiful-events/SKILL.md', /SENT.*invitation sent to (?:the )?authenticated user.*awaiting.*RSVP/is],
  ])('%s teaches the canonical SENT meaning', (path, sentPattern) => {
    const contents = rootFile(path);

    expect(contents).toMatch(sentPattern);
    expect(contents).toMatch(/INTERESTED/);
    expect(contents).toMatch(/SENT.*does not mean.*you sent/is);
    expect(contents).toMatch(/isHost.*true.*Hosting/is);
  });
});
