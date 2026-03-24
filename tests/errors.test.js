import { describe, it, expect } from 'vitest';
import { PartifulError, ApiError, AuthError, ValidationError, NotFoundError } from '../src/lib/errors.js';
import { EXIT } from '../src/lib/output.js';

describe('error classes', () => {
  it('ApiError has code 1', () => {
    const err = new ApiError('fail');
    expect(err.exitCode).toBe(EXIT.API_ERROR);
    expect(err.type).toBe('api_error');
  });
  it('AuthError has code 2', () => {
    expect(new AuthError('expired').exitCode).toBe(EXIT.AUTH_ERROR);
  });
  it('ValidationError has code 3', () => {
    expect(new ValidationError('bad input').exitCode).toBe(EXIT.VALIDATION_ERROR);
  });
  it('NotFoundError has code 4', () => {
    expect(new NotFoundError('missing').exitCode).toBe(EXIT.NOT_FOUND);
  });
  it('toJSON returns error shape', () => {
    const err = new ApiError('fail', { statusCode: 500 });
    const json = err.toJSON();
    expect(json.code).toBe(1);
    expect(json.type).toBe('api_error');
    expect(json.details.statusCode).toBe(500);
  });
});
