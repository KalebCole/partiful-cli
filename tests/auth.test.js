import { describe, it, expect, vi, afterEach } from 'vitest';
import { resolveCredentialsPath, loadConfig, generateAmplitudeDeviceId } from '../src/lib/auth.js';
import path from 'path';

describe('resolveCredentialsPath', () => {
  afterEach(() => { delete process.env.PARTIFUL_CREDENTIALS_FILE; });

  it('uses env var when set', () => {
    process.env.PARTIFUL_CREDENTIALS_FILE = '/custom/path/auth.json';
    expect(resolveCredentialsPath()).toBe('/custom/path/auth.json');
  });

  it('falls back to default', () => {
    delete process.env.PARTIFUL_CREDENTIALS_FILE;
    const expected = path.join(process.env.HOME, '.config/partiful/auth.json');
    expect(resolveCredentialsPath()).toBe(expected);
  });
});

describe('loadConfig', () => {
  afterEach(() => { delete process.env.PARTIFUL_TOKEN; });

  it('uses PARTIFUL_TOKEN env var', () => {
    process.env.PARTIFUL_TOKEN = 'test-token-123';
    const config = loadConfig();
    expect(config.accessToken).toBe('test-token-123');
  });
});

describe('generateAmplitudeDeviceId', () => {
  it('returns a string without +/= chars', () => {
    const id = generateAmplitudeDeviceId();
    expect(typeof id).toBe('string');
    expect(id.length).toBeGreaterThan(0);
    expect(id).not.toMatch(/[+/=]/);
  });
});
