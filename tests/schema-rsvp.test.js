/**
 * Schema introspection coverage for the new RSVP / interest verbs.
 */
import { describe, it, expect } from 'vitest';
import { run } from './helpers.js';

describe('schema covers rsvp / interested verbs', () => {
  it('lists RSVP read, write, and interest commands', () => {
    const out = run(['schema']);
    expect(out.data.commands).toContain('events.rsvp.get');
    expect(out.data.commands).toContain('events.rsvp.set');
    expect(out.data.commands).toContain('events.interested');
    expect(out.data.commands).toContain('explore.rsvp.get');
    expect(out.data.commands).toContain('explore.rsvp.set');
    expect(out.data.commands).toContain('explore.interested');
    expect(out.data.commands).not.toContain('events.rsvp');
    expect(out.data.commands).not.toContain('events.my-rsvp');
  });

  it('events.rsvp.get exposes its event ID', () => {
    const out = run(['schema', 'events.rsvp.get']);
    expect(out.data.command).toBe('events rsvp get <eventId>');
    expect(out.data.parameters.eventId).toEqual(expect.objectContaining({
      type: 'string',
      required: true,
      positional: true,
    }));
  });

  it('events.rsvp.set schema exposes status, plus-one, and questionnaire answer params', () => {
    const out = run(['schema', 'events.rsvp.set']);
    expect(out.data.command).toBe('events rsvp set <eventId>');
    expect(out.data.parameters['--status']).toEqual(expect.objectContaining({
      type: 'string',
      description: expect.stringMatching(/existing status.*new RSVP/i),
    }));
    expect(out.data.parameters['--status']).not.toHaveProperty('default');
    expect(out.data.parameters['--plus-one']).toBeDefined();
    expect(out.data.parameters['--answer']).toEqual(expect.objectContaining({ type: 'string[]' }));
    expect(out.data.parameters.eventId.required).toBe(true);
  });

  it('events.interested schema exposes --remove', () => {
    const out = run(['schema', 'events.interested']);
    expect(out.data.parameters['--remove']).toBeDefined();
  });
});
