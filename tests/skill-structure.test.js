import { describe, it, expect } from 'vitest';
import { execFileSync } from 'child_process';
import fs from 'fs';
import os from 'os';
import path from 'path';
import { fileURLToPath } from 'url';
import { runRaw } from './helpers.js';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const skillsRoot = path.join(repoRoot, 'skills');
const skillRoot = path.join(skillsRoot, 'partiful');

function skillDirectories() {
  return fs.readdirSync(skillsRoot)
    .filter((entry) => fs.statSync(path.join(skillsRoot, entry)).isDirectory())
    .sort();
}

describe('bundled Partiful skill', () => {
  it('ships one model-invoked skill named partiful', () => {
    expect(skillDirectories()).toEqual(['partiful']);

    const skill = fs.readFileSync(path.join(skillRoot, 'SKILL.md'), 'utf8');
    expect(skill).toMatch(/^---\nname: partiful\n/);
    expect(skill).toMatch(/description: .*Partiful/i);
    expect(skill).not.toMatch(/disable-model-invocation:\s*true/);
  });

  it('describes user intents that should invoke the skill', () => {
    const skill = fs.readFileSync(path.join(skillRoot, 'SKILL.md'), 'utf8');
    const description = skill.match(/^description: (.+)$/m)?.[1] ?? '';

    for (const intent of ['check Partiful', 'discover events', 'RSVP', 'create or manage an event']) {
      expect(description).toContain(intent);
    }
  });

  it('has no broken markdown context pointers', () => {
    const skill = fs.readFileSync(path.join(skillRoot, 'SKILL.md'), 'utf8');
    const links = [...skill.matchAll(/\[[^\]]+\]\((references\/[^)]+\.md)\)/g)]
      .map((match) => match[1]);

    expect(links.length).toBeGreaterThan(0);
    for (const link of links) {
      expect(fs.existsSync(path.join(skillRoot, link))).toBe(true);
    }
  });

  it('routes every supported task branch to focused reference documentation', () => {
    const skill = fs.readFileSync(path.join(skillRoot, 'SKILL.md'), 'utf8');
    const expectedReferences = [
      'events.md',
      'rsvps-and-interest.md',
      'guests-invitations-and-cohosts.md',
      'posters-and-images.md',
      'text-blasts.md',
      'authentication.md',
      'cli-output-and-safety.md',
    ];
    const routedReferences = [...skill.matchAll(/^\|[^\n]+\]\(references\/([^)]+\.md)\) \|$/gm)]
      .map((match) => match[1]);

    expect(routedReferences).toHaveLength(7);
    expect(new Set(routedReferences).size).toBe(7);
    expect(routedReferences.sort()).toEqual(expectedReferences.sort());
    for (const reference of expectedReferences) {
      expect(skill).toContain(`references/${reference}`);
    }
  });

  it('documents helper commands at their real top-level paths', () => {
    const references = fs.readdirSync(path.join(skillRoot, 'references'))
      .map((file) => fs.readFileSync(path.join(skillRoot, 'references', file), 'utf8'))
      .join('\n');

    expect(references).toContain('partiful +clone');
    expect(references).toContain('partiful +watch');
    expect(references).toContain('partiful +export');
    expect(references).toContain('partiful +share');
    expect(references).toContain('--plus-one');
    expect(references).toContain('--no-show-on-event-page');
    expect(references).not.toMatch(/partiful (?:events|guests) \+(?:clone|watch|export|share)/);

    for (const command of [
      ['+clone', '--help'],
      ['+watch', '--help'],
      ['+export', '--help'],
      ['+share', '--help'],
      ['events', 'rsvp', 'set', '--help'],
      ['blasts', 'send', '--help'],
    ]) {
      const { stdout, exitCode } = runRaw(command);
      expect(exitCode, `${command.join(' ')} should resolve`).toBe(0);
      expect(stdout).toContain('Usage: partiful');
    }

    expect(runRaw(['events', 'rsvp', 'set', '--help']).stdout).toContain('--plus-one');
    expect(runRaw(['blasts', 'send', '--help']).stdout).toContain('--no-show-on-event-page');
  });

  it('publishes SKILL.md and exactly seven routed reference files', () => {
    const packDir = fs.mkdtempSync(path.join(os.tmpdir(), 'partiful-pack-'));
    try {
      const raw = execFileSync('npm', ['pack', '--dry-run', '--json'], {
        cwd: repoRoot,
        encoding: 'utf8',
        env: { ...process.env, npm_config_cache: path.join(packDir, 'npm-cache') },
      });
      const files = JSON.parse(raw)[0].files.map((entry) => entry.path);
      const shippedSkillFiles = files.filter((file) => file.startsWith('skills/'));
      const shippedReferences = files.filter((file) => file.startsWith('skills/partiful/references/'));

      expect(shippedSkillFiles).toContain('skills/partiful/SKILL.md');
      expect(shippedReferences).toHaveLength(7);
      expect(shippedSkillFiles).toHaveLength(8);
      expect(shippedSkillFiles.every((file) => file.startsWith('skills/partiful/'))).toBe(true);
    } finally {
      fs.rmSync(packDir, { recursive: true, force: true });
    }
  });

  it('exposes the agent-neutral skill installer and retires setup openclaw', () => {
    expect(fs.existsSync(path.join(repoRoot, 'src', 'commands', 'setup.ts'))).toBe(false);
    const cli = fs.readFileSync(path.join(repoRoot, 'src', 'cli.ts'), 'utf8');
    expect(cli).toMatch(/registerSkillCommands/);
    expect(cli).not.toMatch(/registerSetupCommands|commands\/setup/);

    for (const command of [
      ['skill', '--help'],
      ['skill', 'install', '--help'],
      ['skill', 'uninstall', '--help'],
    ]) {
      const { stdout, exitCode } = runRaw(command);
      expect(exitCode, `${command.join(' ')} should resolve`).toBe(0);
      expect(stdout).toContain('Usage: partiful skill');
    }

    const legacy = runRaw(['setup', 'openclaw']);
    expect(legacy.exitCode).not.toBe(0);
    expect(legacy.stdout).not.toContain('"status":"success"');
  });
});
