import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import fs from 'fs';
import os from 'os';
import path from 'path';
import { run, runRaw } from './helpers.js';

const marker = '.partiful-cli-install.json';

let home;

function env(extra = {}) {
  return { HOME: home, HERMES_HOME: '', OPENCLAW_WORKSPACE: '', ...extra };
}

function destination(agent) {
  const roots = {
    hermes: path.join(home, '.hermes', 'skills'),
    openclaw: path.join(home, '.openclaw', 'skills'),
    copilot: path.join(home, '.copilot', 'skills'),
    claude: path.join(home, '.claude', 'skills'),
  };
  return path.join(roots[agent], 'partiful');
}

describe('skill installer', () => {
  beforeEach(() => {
    home = fs.mkdtempSync(path.join(os.tmpdir(), 'partiful-skill-install-'));
  });

  afterEach(() => {
    fs.rmSync(home, { recursive: true, force: true });
  });

  it.each(['hermes', 'openclaw', 'copilot', 'claude'])('dry-runs installation for %s without touching disk', (agent) => {
    const result = run(['skill', 'install', agent, '--dry-run'], { env: env() });

    expect(result.data).toMatchObject({ action: 'install', agent, dryRun: true, destination: destination(agent) });
    expect(fs.existsSync(destination(agent))).toBe(false);
  });

  it('honors HERMES_HOME for Hermes', () => {
    const hermesHome = path.join(home, 'custom-hermes');
    const result = run(['skill', 'install', 'HERMES', '--dry-run'], { env: env({ HERMES_HOME: hermesHome }) });

    expect(result.data.destination).toBe(path.join(hermesHome, 'skills', 'partiful'));
  });

  it('copies the bundled skill and records provenance', () => {
    const result = run(['skill', 'install', 'hermes'], { env: env() });
    const target = destination('hermes');

    expect(result.data.state).toBe('installed');
    expect(fs.lstatSync(target).isSymbolicLink()).toBe(false);
    expect(fs.readFileSync(path.join(target, 'SKILL.md'), 'utf8')).toContain('name: partiful');
    expect(JSON.parse(fs.readFileSync(path.join(target, marker), 'utf8'))).toMatchObject({ installer: 'partiful-cli', skill: 'partiful' });
  });

  it('is idempotent for an installer-owned copy', () => {
    run(['skill', 'install', 'hermes'], { env: env() });
    const result = run(['skill', 'install', 'hermes'], { env: env() });
    expect(result.data.state).toBe('already_installed');
  });

  it('preserves a locally modified owned copy unless forced', () => {
    run(['skill', 'install', 'hermes'], { env: env() });
    const skillFile = path.join(destination('hermes'), 'SKILL.md');
    fs.appendFileSync(skillFile, '\nlocal edit\n');

    const refused = runRaw(['skill', 'install', 'hermes'], { env: env() });
    expect(refused.exitCode).toBe(3);
    expect(JSON.parse(refused.stdout).error.message).toMatch(/modified after installation/i);
    expect(fs.readFileSync(skillFile, 'utf8')).toContain('local edit');

    const preview = run(['skill', 'install', 'hermes', '--force', '--dry-run'], { env: env() });
    expect(preview.data.state).toBe('would_update');
    expect(fs.readFileSync(skillFile, 'utf8')).toContain('local edit');

    const replaced = run(['skill', 'install', 'hermes', '--force'], { env: env() });
    expect(replaced.data.state).toBe('updated');
    expect(fs.readFileSync(skillFile, 'utf8')).not.toContain('local edit');
  });

  it('refuses an existing unowned destination unless forced', () => {
    const target = destination('claude');
    fs.mkdirSync(target, { recursive: true });
    fs.writeFileSync(path.join(target, 'mine.txt'), 'keep');

    const rejected = runRaw(['skill', 'install', 'claude'], { env: env() });
    expect(rejected.exitCode).toBe(3);
    expect(JSON.parse(rejected.stdout).error.message).toMatch(/already exists/i);
    expect(fs.readFileSync(path.join(target, 'mine.txt'), 'utf8')).toBe('keep');

    const forced = run(['skill', 'install', 'claude', '--force'], { env: env() });
    expect(forced.data.state).toBe('installed');
    expect(fs.existsSync(path.join(target, 'mine.txt'))).toBe(false);
    expect(fs.existsSync(path.join(target, marker))).toBe(true);
  });

  it('uninstalls an owned copy but refuses unowned content', () => {
    run(['skill', 'install', 'copilot'], { env: env() });
    const removed = run(['skill', 'uninstall', 'copilot'], { env: env() });
    expect(removed.data.state).toBe('removed');
    expect(fs.existsSync(destination('copilot'))).toBe(false);

    const target = destination('copilot');
    fs.mkdirSync(target, { recursive: true });
    fs.writeFileSync(path.join(target, 'SKILL.md'), 'user content');
    const rejected = runRaw(['skill', 'uninstall', 'copilot'], { env: env() });
    expect(rejected.exitCode).toBe(3);
    expect(JSON.parse(rejected.stdout).error.message).toMatch(/not installed by partiful-cli/i);
    expect(fs.existsSync(target)).toBe(true);

    const forced = runRaw(['skill', 'uninstall', 'copilot', '--force'], { env: env() });
    expect(forced.exitCode).toBe(3);
    expect(fs.existsSync(target)).toBe(true);
  });

  it('preserves a locally modified owned copy during uninstall unless forced', () => {
    run(['skill', 'install', 'hermes'], { env: env() });
    const target = destination('hermes');
    const skillFile = path.join(target, 'SKILL.md');
    fs.appendFileSync(skillFile, '\nlocal edit\n');

    const rejected = runRaw(['skill', 'uninstall', 'hermes'], { env: env() });
    expect(rejected.exitCode).toBe(3);
    expect(JSON.parse(rejected.stdout).error.message).toMatch(/modified after installation/i);
    expect(fs.existsSync(target)).toBe(true);

    const removed = run(['skill', 'uninstall', 'hermes', '--force'], { env: env() });
    expect(removed.data.state).toBe('removed');
    expect(fs.existsSync(target)).toBe(false);
  });

  it('returns not_installed when no owned copy exists', () => {
    const result = run(['skill', 'uninstall', 'claude'], { env: env() });
    expect(result.data).toMatchObject({ action: 'uninstall', agent: 'claude', state: 'not_installed' });
  });

  it('dry-runs uninstall without removing an owned copy', () => {
    run(['skill', 'install', 'hermes'], { env: env() });
    const result = run(['skill', 'uninstall', 'hermes', '--dry-run'], { env: env() });

    expect(result.data).toMatchObject({ action: 'uninstall', state: 'would_remove', dryRun: true });
    expect(fs.existsSync(destination('hermes'))).toBe(true);
  });

  it('cleans only package-owned legacy OpenClaw symlinks', () => {
    const workspace = path.join(home, 'legacy-workspace');
    const workspaceSkills = path.join(workspace, 'skills');
    fs.mkdirSync(workspaceSkills, { recursive: true });
    const legacy = path.join(workspaceSkills, 'partiful-events');
    const sameNameUnowned = path.join(workspaceSkills, 'partiful-guests');
    const checkoutUnowned = path.join(workspaceSkills, 'partiful-posters');
    const unrelated = path.join(workspaceSkills, 'partiful-personal');
    const oldPackageSkills = path.join(home, 'old-node', 'lib', 'node_modules', 'partiful-cli', 'skills');
    fs.symlinkSync(path.join(oldPackageSkills, 'partiful-events'), legacy);
    fs.symlinkSync(path.join(home, 'my-skills', 'partiful-guests'), sameNameUnowned);
    fs.symlinkSync(path.join(home, 'projects', 'partiful-cli', 'skills', 'partiful-posters'), checkoutUnowned);
    fs.symlinkSync(path.join(home, 'my-skills', 'partiful-personal'), unrelated);

    const result = run(['skill', 'uninstall', 'openclaw', '--workspace', workspace], { env: env() });

    expect(result.data.legacyRemoved).toEqual([legacy]);
    expect(fs.existsSync(legacy)).toBe(false);
    expect(fs.lstatSync(sameNameUnowned).isSymbolicLink()).toBe(true);
    expect(fs.lstatSync(checkoutUnowned).isSymbolicLink()).toBe(true);
    expect(fs.lstatSync(unrelated).isSymbolicLink()).toBe(true);
  });

  it('dry-runs legacy cleanup and force-removes ambiguous legacy symlinks', () => {
    const workspace = path.join(home, 'legacy-workspace');
    const workspaceSkills = path.join(workspace, 'skills');
    fs.mkdirSync(workspaceSkills, { recursive: true });
    const ambiguous = path.join(workspaceSkills, 'partiful-guests');
    fs.symlinkSync(path.join(home, 'renamed-checkout', 'skills', 'partiful-guests'), ambiguous);

    const preview = run(['skill', 'uninstall', 'openclaw', '--workspace', workspace, '--force', '--dry-run'], { env: env() });
    expect(preview.data.legacyRemoved).toEqual([ambiguous]);
    expect(fs.lstatSync(ambiguous).isSymbolicLink()).toBe(true);

    const removed = run(['skill', 'uninstall', 'openclaw', '--workspace', workspace, '--force'], { env: env() });
    expect(removed.data.legacyRemoved).toEqual([ambiguous]);
    expect(fs.existsSync(ambiguous)).toBe(false);
  });

  it('returns a structured validation error for unsupported agents', () => {
    const result = runRaw(['skill', 'install', 'cursor'], { env: env() });

    expect(result.exitCode).toBe(3);
    expect(JSON.parse(result.stdout).error).toMatchObject({ type: 'validation_error' });
  });
});
