import { describe, it, expect, vi } from 'vitest';
import { run, runRaw } from './helpers.js';
import {
  resolveContactNames,
  mergeCohostState,
  planCohostInvite,
  planCohostRemoval,
  cohostPathToUrl,
  inviteCohost,
  inviteCohostBatch,
  removeCohostCanonical,
  parseCohostLinkDocument,
  planCohostLinkAction,
} from '../src/lib/cohosts.js';

const contacts = [
  { userId: 'u1', name: 'Alex Smith' },
  { userId: 'u2', name: 'Alex Johnson' },
  { userId: 'u3', name: 'Sam Lee' },
];

describe('resolveContactNames', () => {
  it('prefers a unique exact name match', () => {
    expect(resolveContactNames(['Alex Smith'], contacts)).toEqual(['u1']);
  });

  it('allows a unique partial match', () => {
    expect(resolveContactNames(['Sam'], contacts)).toEqual(['u3']);
  });

  it('fails closed for ambiguous partial names and includes candidates', () => {
    expect(() => resolveContactNames(['Alex'], contacts)).toThrowError(/ambiguous/i);
    try {
      resolveContactNames(['Alex'], contacts);
    } catch (error) {
      expect(error.details.candidates).toEqual([
        { userId: 'u1', name: 'Alex Smith' },
        { userId: 'u2', name: 'Alex Johnson' },
      ]);
    }
  });

  it('fails when an exact name is duplicated', () => {
    const duplicate = [...contacts, { userId: 'u4', name: 'Alex Smith' }];
    expect(() => resolveContactNames(['Alex Smith'], duplicate)).toThrowError(/ambiguous/i);
  });

  it('fails on unresolved names instead of warning and continuing', () => {
    expect(() => resolveContactNames(['Nobody Here'], contacts)).toThrowError(/could not resolve/i);
  });

  it('deduplicates resolved user ids', () => {
    expect(resolveContactNames(['Sam Lee', 'sam'], contacts)).toEqual(['u3']);
  });
});

describe('cohost lifecycle state', () => {
  const requests = [
    { userId: 'u1', status: 'PENDING' },
    { userId: 'u2', status: 'ACCEPTED' },
    { userId: 'u3', status: 'DECLINED' },
  ];

  it('normalizes requests and marks ids without a request as stale', () => {
    expect(mergeCohostState(requests, ['u2', 'legacy'])).toEqual([
      { userId: 'u1', status: 'pending' },
      { userId: 'u2', status: 'accepted' },
      { userId: 'u3', status: 'declined' },
      { userId: 'legacy', status: 'stale' },
    ]);
  });

  it.each([
    [undefined, 'invite'],
    ['stale', 'repair'],
    ['declined', 'reinvite'],
    ['pending', 'noop'],
    ['accepted', 'noop'],
  ])('plans invite for %s as %s', (status, action) => {
    const state = status ? { userId: 'u1', status } : undefined;
    expect(planCohostInvite(state)).toBe(action);
  });

  it.each([
    ['pending', 'delete_request'],
    ['declined', 'delete_request'],
    ['accepted', 'remove_cohost'],
    ['stale', 'remove_cohost'],
  ])('plans removal for %s as %s', (status, action) => {
    expect(planCohostRemoval({ userId: 'u1', status })).toBe(action);
  });

  it('fails removal when the user has no lifecycle state', () => {
    expect(() => planCohostRemoval(undefined)).toThrowError(/not a co-host/i);
  });
});

describe('canonical orchestration', () => {
  it.each([
    [undefined, 'invited'],
    ['declined', 'reinvited'],
  ])('calls createCohostRequest for %s state', async (status, outcome) => {
    const call = vi.fn().mockResolvedValue({});
    const state = status ? { userId: 'u1', status } : undefined;
    expect(await inviteCohost('event', 'u1', state, call)).toEqual({ userId: 'u1', outcome });
    expect(call).toHaveBeenCalledWith('/createCohostRequest', { eventId: 'event', targetUserId: 'u1' });
  });

  it('repairs stale direct membership before creating the request', async () => {
    const call = vi.fn().mockResolvedValue({});
    const repairStale = vi.fn().mockResolvedValue(undefined);
    expect(await inviteCohost('event', 'u1', { userId: 'u1', status: 'stale' }, call, repairStale))
      .toEqual({ userId: 'u1', outcome: 'stale_repair' });
    expect(repairStale).toHaveBeenCalledOnce();
    expect(call.mock.calls).toEqual([
      ['/createCohostRequest', { eventId: 'event', targetUserId: 'u1' }],
    ]);
  });

  it('fails closed rather than issuing a stale request without a repair hook', async () => {
    const call = vi.fn();
    await expect(inviteCohost('event', 'u1', { userId: 'u1', status: 'stale' }, call))
      .rejects.toThrow('repair hook');
    expect(call).not.toHaveBeenCalled();
  });

  it('reports partial batch failures without hiding successful invitations', async () => {
    const call = vi.fn(async (_endpoint, params) => {
      if (params.targetUserId === 'u2') throw new Error('denied');
      return {};
    });
    await expect(inviteCohostBatch('event', ['u1', 'u2'], [], call)).resolves.toEqual({
      succeeded: [{ userId: 'u1', outcome: 'invited' }],
      failed: [{ userId: 'u2', error: 'denied' }],
    });
  });

  it.each(['pending', 'accepted'])('does not mutate an existing %s request', async (status) => {
    const call = vi.fn();
    expect(await inviteCohost('event', 'u1', { userId: 'u1', status }, call)).toEqual({ userId: 'u1', outcome: status });
    expect(call).not.toHaveBeenCalled();
  });

  it('routes pending request removal to deleteCohostRequest', async () => {
    const call = vi.fn().mockResolvedValue({});
    await removeCohostCanonical('event', { userId: 'u1', status: 'pending' }, call);
    expect(call).toHaveBeenCalledWith('/deleteCohostRequest', { eventId: 'event', targetUserId: 'u1' });
  });

  it('routes accepted removal to removeCohost', async () => {
    const call = vi.fn().mockResolvedValue({});
    await removeCohostCanonical('event', { userId: 'u1', status: 'accepted' }, call);
    expect(call).toHaveBeenCalledWith('/removeCohost', { eventId: 'event', targetUserId: 'u1' });
  });

  it('removes stale membership through the explicit migration hook', async () => {
    const call = vi.fn();
    const removeStale = vi.fn().mockResolvedValue(undefined);
    await removeCohostCanonical('event', { userId: 'u1', status: 'stale' }, call, removeStale);
    expect(removeStale).toHaveBeenCalledOnce();
    expect(call).not.toHaveBeenCalled();
  });
});

describe('cohost command surfaces', () => {
  it('registers link lifecycle help', () => {
    const { stdout, exitCode } = runRaw(['cohosts', 'link', '--help']);
    expect(exitCode).toBe(0);
    expect(stdout).toContain('--enable');
    expect(stdout).toContain('--disable');
  });

  it('exposes cohosts.link schema', () => {
    const out = run(['schema', 'cohosts.link']);
    expect(out.data.command).toBe('cohosts link <eventId>');
    expect(out.data.parameters['--enable']).toBeDefined();
    expect(out.data.parameters['--disable']).toBeDefined();
  });
});

describe('cohost invite links', () => {
  it('converts server path to an absolute Partiful URL', () => {
    expect(cohostPathToUrl('/e/event?accept-cohost=token')).toBe('https://partiful.com/e/event?accept-cohost=token');
  });

  it('rejects non-Partiful and malformed paths', () => {
    expect(() => cohostPathToUrl('https://evil.test/x')).toThrowError(/invalid/i);
    expect(() => cohostPathToUrl('/other/path')).toThrowError(/invalid/i);
  });

  it('parses missing and active secret documents', () => {
    expect(parseCohostLinkDocument(null)).toEqual({ enabled: false, url: null });
    expect(parseCohostLinkDocument({ fields: { path: { stringValue: '/e/event?accept-cohost=token' } } }))
      .toEqual({ enabled: true, url: 'https://partiful.com/e/event?accept-cohost=token' });
  });

  it('fails closed when an existing secret document has no valid path', () => {
    expect(() => parseCohostLinkDocument({ fields: {} })).toThrowError(/invalid/i);
  });

  it.each([
    ['inspect', false, 'inspect'],
    ['inspect', true, 'inspect'],
    ['enable', false, 'generate'],
    ['enable', true, 'noop'],
    ['disable', false, 'noop'],
    ['disable', true, 'revoke'],
  ])('plans %s when enabled=%s as %s', (requested, enabled, expected) => {
    expect(planCohostLinkAction(requested, enabled)).toBe(expected);
  });
});
