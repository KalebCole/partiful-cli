/**
 * Integration tests for the RSVP / interest command surface.
 *
 * Covers `events rsvp` / `explore rsvp` and `events interested` /
 * `explore interested` via --dry-run (no network). The alias (`explore *`) must
 * forward to the SAME handler and produce an identical payload to `events *`.
 */

import { describe, it, expect } from 'vitest';
import { run, runRaw } from './helpers.js';

describe('events rsvp / explore rsvp', () => {
  it('events rsvp --dry-run builds an addGuest payload (GOING default)', () => {
    const out = run(['events', 'rsvp', 'EV123', '--name', 'Kaleb Cole', '--dry-run', '--yes']);
    expect(out.status).toBe('success');
    expect(out.data.dryRun).toBe(true);
    expect(out.data.endpoint).toBe('/addGuest');
    const p = out.data.payload.data.params;
    expect(p.eventId).toBe('EV123');
    expect(p.rsvp.status).toBe('GOING');
    expect(p.rsvp.guestId).toBeNull();
    expect(p.rsvp.count).toBe(1);
    expect(p.rsvp.name).toBe('Kaleb Cole');
  });

  it('--status maybe maps to the MAYBE wire enum', () => {
    const out = run(['events', 'rsvp', 'EV123', '--name', 'Kaleb', '--status', 'maybe', '--dry-run', '--yes']);
    expect(out.data.payload.data.params.rsvp.status).toBe('MAYBE');
  });

  it('--plus-one is repeatable and bumps the count', () => {
    const out = run([
      'events', 'rsvp', 'EV123', '--name', 'Kaleb',
      '--plus-one', 'Maddie', '--plus-one', 'Justin',
      '--dry-run', '--yes',
    ]);
    const rsvp = out.data.payload.data.params.rsvp;
    expect(rsvp.plusOnes).toEqual(['Maddie', 'Justin']);
    expect(rsvp.count).toBe(3);
  });

  it('rejects an invalid --status with exit code 3', () => {
    const { stdout, exitCode } = runRaw(['events', 'rsvp', 'EV123', '--name', 'Kaleb', '--status', 'interested', '--dry-run', '--yes']);
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('error');
    expect(out.error.code).toBe(3);
    expect(exitCode).toBe(3);
  });

  it('explore rsvp forwards to the SAME handler with an identical payload', () => {
    const viaEvents = run(['events', 'rsvp', 'EV123', '--name', 'Kaleb', '--status', 'going', '--dry-run', '--yes']);
    const viaExplore = run(['explore', 'rsvp', 'EV123', '--name', 'Kaleb', '--status', 'going', '--dry-run', '--yes']);
    expect(viaExplore.data.endpoint).toBe('/addGuest');
    expect(viaExplore.data.payload.data.params).toEqual(viaEvents.data.payload.data.params);
  });

  it('requires a name when none can be resolved (no auth profile in test)', () => {
    const { stdout } = runRaw(['events', 'rsvp', 'EV123', '--dry-run', '--yes']);
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('error');
    expect(out.error.code).toBe(3);
    expect(out.error.message).toMatch(/name/i);
  });
});

describe('events interested / explore interested', () => {
  it('events interested --dry-run builds a markEventInterest payload (interested true)', () => {
    const out = run(['events', 'interested', 'EV123', '--dry-run', '--yes']);
    expect(out.status).toBe('success');
    expect(out.data.dryRun).toBe(true);
    expect(out.data.endpoint).toBe('/markEventInterest');
    const p = out.data.payload.data.params;
    expect(p.eventId).toBe('EV123');
    expect(p.interested).toBe(true);
    expect(p.source).toBe('DISCOVER');
  });

  it('--remove flips interested to false', () => {
    const out = run(['events', 'interested', 'EV123', '--remove', '--dry-run', '--yes']);
    expect(out.data.payload.data.params.interested).toBe(false);
  });

  it('explore interested forwards to the SAME handler', () => {
    const viaEvents = run(['events', 'interested', 'EV123', '--dry-run', '--yes']);
    const viaExplore = run(['explore', 'interested', 'EV123', '--dry-run', '--yes']);
    expect(viaExplore.data.endpoint).toBe('/markEventInterest');
    expect(viaExplore.data.payload.data.params).toEqual(viaEvents.data.payload.data.params);
  });
});
