/**
 * Schema introspection coverage for the `api.<method>` namespace (T5).
 * The API-endpoint spec (src/lib/api/endpoints.ts) is the source of truth;
 * `schema api` / `schema api.<method>` surfaces it from the CLI.
 */
import { describe, it, expect } from 'vitest';
import { run, runRaw } from './helpers.js';

describe('schema api.<method> namespace', () => {
  it('bare `schema` lists api.* methods alongside CLI commands', () => {
    const out = run(['schema']);
    expect(out.data.commands).toContain('events.create');
    expect(out.data.api).toContain('api.createEvent');
    expect(out.data.api).toContain('api.addGuest');
  });

  it('`schema api` lists every spec\'d endpoint method', () => {
    const out = run(['schema', 'api']);
    expect(Array.isArray(out.data.methods)).toBe(true);
    expect(out.data.methods).toContain('createEvent');
    expect(out.data.methods).toContain('markEventInterest');
    expect(out.data.methods).toContain('refreshToken');
    expect(out.metadata.count).toBe(out.data.methods.length);
  });

  it('`schema api.createEvent` prints endpoint spec derived from T3 types', () => {
    const out = run(['schema', 'api.createEvent']);
    expect(out.data.method).toBe('api.createEvent');
    expect(out.data.httpMethod).toBe('POST');
    expect(out.data.transport).toBe('firebase-callable');
    expect(out.data.path).toBe('/createEvent');
    expect(out.data.requestParams).toContain('event');
    expect(Array.isArray(out.data.responseFields)).toBe(true);
  });

  it('`schema api.firestoreGetEvent` reflects firestore transport + GET', () => {
    const out = run(['schema', 'api.firestoreGetEvent']);
    expect(out.data.transport).toBe('firestore');
    expect(out.data.httpMethod).toBe('GET');
    expect(out.data.requestParams).toContain('eventId');
  });

  it('`schema api.firestoreGetGuest` documents the current RSVP detail read path', () => {
    const out = run(['schema', 'api.firestoreGetGuest']);
    expect(out.data.transport).toBe('firestore');
    expect(out.data.httpMethod).toBe('GET');
    expect(out.data.requestParams).toEqual(expect.arrayContaining(['eventId', 'guestId']));
    expect(out.data.path).toContain('/events/{eventId}/guests/{guestId}');
  });

  it('unknown api method errors with not_found and lists available', () => {
    const { stdout, exitCode } = runRaw(['schema', 'api.nope']);
    expect(exitCode).toBe(4);
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('error');
    expect(out.error.type).toBe('not_found');
    expect(out.error.message).toContain('createEvent');
  });
});
