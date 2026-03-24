import { describe, it, expect } from 'vitest';
import { execFileSync } from 'child_process';
import path from 'path';

const CLI = path.resolve('bin/partiful');
const env = { ...process.env, PARTIFUL_TOKEN: 'fake-token', NODE_NO_WARNINGS: '1' };

function run(args) {
  const result = execFileSync('node', [CLI, ...args], {
    env,
    encoding: 'utf8',
    timeout: 10000,
  });
  return JSON.parse(result.trim());
}

function runRaw(args) {
  try {
    const stdout = execFileSync('node', [CLI, ...args], {
      env,
      encoding: 'utf8',
      timeout: 10000,
    });
    return { stdout, exitCode: 0 };
  } catch (e) {
    return { stdout: e.stdout || '', stderr: e.stderr || '', exitCode: e.status };
  }
}

describe('guests integration', () => {
  describe('--dry-run', () => {
    it('guests list --dry-run returns collection path', () => {
      const out = run(['guests', 'list', 'evt-123', '--dry-run']);
      expect(out.status).toBe('success');
      expect(out.data.dryRun).toBe(true);
      expect(out.data.eventId).toBe('evt-123');
      expect(out.data.collection).toContain('evt-123');
    });

    it('guests invite --dry-run with phone', () => {
      const out = run([
        'guests', 'invite', 'evt-123',
        '--phone', '+12065551234',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      expect(out.data.dryRun).toBe(true);
      expect(out.data.endpoint).toBe('/addInvitedGuestsAsHost');
    });

    it('guests invite --dry-run with user-id', () => {
      const out = run([
        'guests', 'invite', 'evt-123',
        '--user-id', 'user-abc',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      expect(out.data.dryRun).toBe(true);
    });
  });

  describe('JSON envelope shape', () => {
    it('dry-run output has status, data, metadata', () => {
      const out = run(['guests', 'list', 'evt-123', '--dry-run']);
      expect(out).toHaveProperty('status', 'success');
      expect(out).toHaveProperty('data');
      expect(out).toHaveProperty('metadata');
    });
  });

  describe('invite validation', () => {
    it('invite without --phone or --user-id returns validation error', () => {
      const { stdout, exitCode } = runRaw(['guests', 'invite', 'evt-123']);
      expect(exitCode).not.toBe(0);
      const out = JSON.parse(stdout.trim());
      expect(out.status).toBe('error');
      expect(out.error.type).toBe('validation_error');
      expect(out.error.message).toMatch(/phone|user-id/i);
    });
  });
});
