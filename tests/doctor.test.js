import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { execSync } from 'child_process';
import path from 'path';
import fs from 'fs';
import os from 'os';

const CLI_PATH = path.resolve('src/cli.js');

function runCli(args, env = {}) {
  try {
    const result = execSync(`node ${CLI_PATH} ${args}`, {
      encoding: 'utf8',
      env: { ...process.env, ...env, PARTIFUL_CREDENTIALS_FILE: '/tmp/__nonexistent_partiful_auth__.json' },
      timeout: 10000,
    });
    return { stdout: result, exitCode: 0 };
  } catch (e) {
    return { stdout: e.stdout || '', stderr: e.stderr || '', exitCode: e.status };
  }
}

describe('doctor command', () => {
  it('--dry-run returns list of checks without executing', () => {
    const { stdout, exitCode } = runCli('doctor --dry-run');
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
    const { stdout, exitCode } = runCli('doctor');
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
    const { stdout } = runCli('doctor');
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
