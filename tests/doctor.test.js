import { describe, it, expect } from 'vitest';
import { execFileSync } from 'child_process';
import { resolve } from 'path';

const CLI = resolve('bin/partiful');

function runCli(args, env = {}) {
  try {
    const stdout = execFileSync('node', [CLI, ...args], {
      encoding: 'utf8',
      env: {
        ...process.env,
        PARTIFUL_CREDENTIALS_FILE: '/tmp/__nonexistent_partiful_auth__.json',
        ...env,
      },
      timeout: 10000,
    });
    return { stdout, exitCode: 0 };
  } catch (e) {
    return { stdout: e.stdout || '', stderr: e.stderr || '', exitCode: e.status };
  }
}

describe('doctor command', () => {
  it('--dry-run returns list of checks without executing', () => {
    const { stdout } = runCli(['--dry-run', 'doctor']);
    const parsed = JSON.parse(stdout.trim());
    expect(parsed.status).toBe('success');
    expect(parsed.data.checks).toBeInstanceOf(Array);
    expect(parsed.data.checks.length).toBeGreaterThanOrEqual(5);
    expect(parsed.data.checks.map(c => c.name)).toEqual(
      expect.arrayContaining(['config_file', 'token_refresh', 'api_connectivity', 'environment', 'platform'])
    );
    expect(parsed.data.note).toMatch(/dry run/i);
  });

  it('missing config file produces failed config_file check', () => {
    const { stdout, exitCode } = runCli(['doctor']);
    expect(exitCode).not.toBe(0);
    const parsed = JSON.parse(stdout.trim());
    expect(parsed.status).toBe('success');
    expect(parsed.data.checks).toBeInstanceOf(Array);
    const configCheck = parsed.data.checks.find(c => c.name === 'config_file');
    expect(configCheck).toBeDefined();
    expect(configCheck.passed).toBe(false);
    expect(parsed.data.allPassed).toBe(false);
  });

  it('output shape matches expected envelope', () => {
    const { stdout } = runCli(['doctor']);
    const parsed = JSON.parse(stdout.trim());
    expect(parsed).toHaveProperty('status');
    expect(parsed).toHaveProperty('data');
    expect(parsed.data).toHaveProperty('checks');
    expect(parsed.data).toHaveProperty('allPassed');
    for (const check of parsed.data.checks) {
      expect(check).toHaveProperty('name');
      expect(check).toHaveProperty('passed');
      expect(check).toHaveProperty('detail');
    }
  });
});
