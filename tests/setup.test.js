import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import fs from 'fs';
import path from 'path';
import os from 'os';
import { run, runRaw } from './helpers.js';

describe('setup openclaw', () => {
  let tmpDir;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'partiful-setup-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('--dry-run lists what would be linked', () => {
    const result = run(['setup', 'openclaw', '--workspace', tmpDir, '--dry-run']);
    expect(result.status).toBe('success');
    expect(result.data.dryRun).toBe(true);
    expect(result.data.action).toBe('install');
    expect(result.data.linked.length).toBeGreaterThan(0);
    // No actual symlinks created
    const skillsDir = path.join(tmpDir, 'skills');
    expect(fs.existsSync(skillsDir)).toBe(false);
  });

  it('creates symlinks in workspace', () => {
    const result = run(['setup', 'openclaw', '--workspace', tmpDir]);
    expect(result.status).toBe('success');
    expect(result.data.action).toBe('install');
    expect(result.data.linked.length).toBeGreaterThan(0);

    // Verify symlinks exist
    for (const item of result.data.linked) {
      const linkPath = path.join(tmpDir, 'skills', item.skill);
      expect(fs.existsSync(linkPath)).toBe(true);
      expect(fs.lstatSync(linkPath).isSymbolicLink()).toBe(true);
    }
  });

  it('is idempotent — second run skips already-linked skills', () => {
    run(['setup', 'openclaw', '--workspace', tmpDir]);
    const result2 = run(['setup', 'openclaw', '--workspace', tmpDir]);
    expect(result2.status).toBe('success');
    expect(result2.data.linked.length).toBe(0);
    expect(result2.data.skipped.length).toBeGreaterThan(0);
    expect(result2.data.skipped[0].reason).toBe('already linked');
  });

  it('--uninstall removes symlinks', () => {
    run(['setup', 'openclaw', '--workspace', tmpDir]);
    const result = run(['setup', 'openclaw', '--workspace', tmpDir, '--uninstall']);
    expect(result.status).toBe('success');
    expect(result.data.action).toBe('uninstall');
    expect(result.data.removed.length).toBeGreaterThan(0);

    // Verify symlinks are gone
    for (const item of result.data.removed) {
      expect(fs.existsSync(item.path)).toBe(false);
    }
  });

  it('errors when workspace path does not exist and no env set', () => {
    const { stdout, exitCode } = runRaw(['setup', 'openclaw'], {
      env: { OPENCLAW_WORKSPACE: '', HOME: '/tmp/nonexistent-home-' + Date.now() },
    });
    expect(exitCode).not.toBe(0);
    const parsed = JSON.parse(stdout.trim());
    expect(parsed.status).toBe('error');
    expect(parsed.error.message).toMatch(/Could not find OpenClaw workspace/);
  });
});
