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

function runRaw(args, opts = {}) {
  try {
    const stdout = execFileSync('node', [CLI, ...args], {
      env,
      encoding: 'utf8',
      timeout: 10000,
      ...opts,
    });
    return { stdout, exitCode: 0 };
  } catch (e) {
    return { stdout: e.stdout || '', stderr: e.stderr || '', exitCode: e.status };
  }
}

describe('events integration', () => {
  describe('--dry-run returns payload without API calls', () => {
    it('events list --dry-run', () => {
      const out = run(['events', 'list', '--dry-run']);
      expect(out.status).toBe('success');
      expect(out.data.dryRun).toBe(true);
      expect(out.data.endpoint).toContain('UpcomingEvents');
      expect(out.data.payload).toBeDefined();
    });

    it('events list --past --dry-run', () => {
      const out = run(['events', 'list', '--past', '--dry-run']);
      expect(out.data.endpoint).toContain('PastEvents');
    });

    it('events get --dry-run', () => {
      const out = run(['events', 'get', 'test-event-123', '--dry-run']);
      expect(out.status).toBe('success');
      expect(out.data.dryRun).toBe(true);
      expect(out.data.endpoint).toBe('/getEvent');
    });

    it('events create --dry-run', () => {
      const out = run([
        'events', 'create',
        '--title', 'Test Party',
        '--date', '2026-06-01 7pm',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      expect(out.data.dryRun).toBe(true);
      expect(out.data.endpoint).toBe('/createEvent');
      expect(out.data.payload.data).toBeDefined();
    });

    it('events cancel --dry-run --yes', () => {
      const out = run(['events', 'cancel', 'test-id', '--dry-run', '--yes']);
      expect(out.status).toBe('success');
      expect(out.data.dryRun).toBe(true);
      expect(out.data.endpoint).toBe('/cancelEvent');
    });
  });

  describe('JSON envelope shape', () => {
    it('dry-run output has status, data, metadata', () => {
      const out = run(['events', 'list', '--dry-run']);
      expect(out).toHaveProperty('status', 'success');
      expect(out).toHaveProperty('data');
      expect(out).toHaveProperty('metadata');
    });
  });

  describe('schema command', () => {
    it('schema with no args lists all commands', () => {
      const out = run(['schema']);
      expect(out.status).toBe('success');
      expect(out.data.commands).toBeInstanceOf(Array);
      expect(out.data.commands).toContain('events.list');
      expect(out.data.commands).toContain('guests.invite');
      expect(out.metadata.count).toBeGreaterThan(0);
    });

    it('schema events.create returns parameters', () => {
      const out = run(['schema', 'events.create']);
      expect(out.status).toBe('success');
      expect(out.data.command).toBe('events create');
      expect(out.data.parameters['--title'].required).toBe(true);
      expect(out.data.parameters['--date'].required).toBe(true);
    });

    it('schema unknown path returns error', () => {
      const { stdout, exitCode } = runRaw(['schema', 'nonexistent']);
      expect(exitCode).not.toBe(0);
      const out = JSON.parse(stdout.trim());
      expect(out.status).toBe('error');
      expect(out.error.type).toBe('not_found');
    });
  });
});

describe('version command', () => {
  it('returns version info', () => {
    const { stdout } = runRaw(['version']);
    expect(stdout).toBeTruthy();
  });
});
