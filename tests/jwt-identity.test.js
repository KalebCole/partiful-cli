import { describe, it, expect } from 'vitest';
import { decodeJwtPayload, getUserIdFromToken } from '../src/lib/auth.js';

/**
 * Build an unsigned JWT (header.payload.signature) for testing decode logic.
 * The signature is irrelevant — we never verify it — so a placeholder is fine.
 */
function makeJwt(payload) {
  const b64url = (obj) =>
    Buffer.from(JSON.stringify(obj)).toString('base64')
      .replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return `${b64url({ alg: 'RS256', typ: 'JWT' })}.${b64url(payload)}.sig`;
}

describe('decodeJwtPayload', () => {
  it('decodes a well-formed JWT payload', () => {
    const token = makeJwt({ user_id: 'abc123', sub: 'abc123' });
    expect(decodeJwtPayload(token)).toEqual({ user_id: 'abc123', sub: 'abc123' });
  });

  it('decodes base64url payloads containing - and _ chars', () => {
    // A payload that base64-encodes with + and / (→ - and _ in base64url).
    const payload = { user_id: 'a>b?c>d?', note: 'ok' };
    const token = makeJwt(payload);
    expect(decodeJwtPayload(token)).toEqual(payload);
  });

  it('returns null for a non-string token', () => {
    expect(decodeJwtPayload(null)).toBeNull();
    expect(decodeJwtPayload(undefined)).toBeNull();
    expect(decodeJwtPayload(12345)).toBeNull();
  });

  it('returns null when the token is not three segments', () => {
    expect(decodeJwtPayload('not-a-jwt')).toBeNull();
    expect(decodeJwtPayload('only.two')).toBeNull();
    expect(decodeJwtPayload('a.b.c.d')).toBeNull();
  });

  it('returns null when the payload segment is not valid JSON', () => {
    const garbage = `${Buffer.from('x').toString('base64')}.@@@notbase64@@@.sig`;
    expect(decodeJwtPayload(garbage)).toBeNull();
  });
});

describe('getUserIdFromToken', () => {
  it('prefers the user_id claim', () => {
    const token = makeJwt({ user_id: 'from-user-id', sub: 'from-sub' });
    expect(getUserIdFromToken(token)).toBe('from-user-id');
  });

  it('falls back to the sub claim when user_id is absent', () => {
    const token = makeJwt({ sub: 'from-sub' });
    expect(getUserIdFromToken(token)).toBe('from-sub');
  });

  it('returns null when neither claim is present', () => {
    const token = makeJwt({ email: 'x@example.com' });
    expect(getUserIdFromToken(token)).toBeNull();
  });

  it('returns null for an undecodable token instead of throwing', () => {
    expect(getUserIdFromToken('garbage')).toBeNull();
    expect(getUserIdFromToken(null)).toBeNull();
  });

  it('returns null when the payload decodes to a non-object primitive', () => {
    // Guards against reading .sub off a string (String.prototype.sub is a
    // real legacy function, so a bare typeof check is not enough).
    const b64url = (s) => Buffer.from(s).toString('base64')
      .replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
    const stringPayload = `${b64url('{}')}.${b64url('"hello"')}.sig`;
    const numberPayload = `${b64url('{}')}.${b64url('42')}.sig`;
    expect(getUserIdFromToken(stringPayload)).toBeNull();
    expect(getUserIdFromToken(numberPayload)).toBeNull();
  });
});
