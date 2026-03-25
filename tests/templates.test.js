/**
 * Tests for template commands and library.
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { run, runRaw } from './helpers.js';
import fs from 'fs';
import path from 'path';
import os from 'os';

let tmpDir;

function tplRun(args, extraEnv = {}) {
  return run(args, {
    env: {
      PARTIFUL_TEMPLATES_FILE: path.join(tmpDir, 'templates.json'),
      ...extraEnv,
    },
  });
}

function tplRunRaw(args, extraEnv = {}) {
  return runRaw(args, {
    env: {
      PARTIFUL_TEMPLATES_FILE: path.join(tmpDir, 'templates.json'),
      ...extraEnv,
    },
  });
}

describe('template commands', () => {
  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'partiful-tpl-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('template list returns empty array when no templates', () => {
    const out = tplRun(['template', 'list']);
    expect(out.status).toBe('success');
    expect(out.data).toEqual([]);
    expect(out.metadata.total).toBe(0);
  });

  it('template save creates a template', () => {
    const out = tplRun(['template', 'save', '--name', 'game-night', '--title', 'Game Night', '--location', 'My Place', '--capacity', '10']);
    expect(out.status).toBe('success');
    expect(out.data.title).toBe('Game Night');
    expect(out.data.location).toBe('My Place');
    expect(out.data.capacity).toBe(10);
    expect(out.metadata.name).toBe('game-night');
  });

  it('template list shows saved templates', () => {
    tplRun(['template', 'save', '--name', 'game-night', '--title', 'Game Night', '--location', 'My Place']);
    const out = tplRun(['template', 'list']);
    expect(out.data.length).toBe(1);
    expect(out.data[0].name).toBe('game-night');
    expect(out.data[0].title).toBe('Game Night');
  });

  it('template show returns template details', () => {
    tplRun(['template', 'save', '--name', 'game-night', '--title', 'Game Night', '--capacity', '10']);
    const out = tplRun(['template', 'show', 'game-night']);
    expect(out.status).toBe('success');
    expect(out.data.title).toBe('Game Night');
    expect(out.data.capacity).toBe(10);
  });

  it('template show returns error for missing template', () => {
    const { stdout, exitCode } = tplRunRaw(['template', 'show', 'nonexistent']);
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('error');
    expect(out.error.type).toBe('not_found');
  });

  it('template edit updates fields', () => {
    tplRun(['template', 'save', '--name', 'game-night', '--title', 'Game Night', '--capacity', '10']);
    const out = tplRun(['template', 'edit', 'game-night', '--capacity', '20', '--location', 'New Place']);
    expect(out.data.capacity).toBe(20);
    expect(out.data.location).toBe('New Place');
    expect(out.data.title).toBe('Game Night'); // preserved
  });

  it('template edit --rename works', () => {
    tplRun(['template', 'save', '--name', 'old-name', '--title', 'Test']);
    const out = tplRun(['template', 'edit', 'old-name', '--rename', 'new-name']);
    expect(out.metadata.name).toBe('new-name');
    expect(out.metadata.renamedFrom).toBe('old-name');

    const list = tplRun(['template', 'list']);
    expect(list.data.length).toBe(1);
    expect(list.data[0].name).toBe('new-name');
  });

  it('template delete removes template', () => {
    tplRun(['template', 'save', '--name', 'game-night', '--title', 'Game Night']);
    const out = tplRun(['template', 'delete', 'game-night']);
    expect(out.metadata.action).toBe('deleted');

    const list = tplRun(['template', 'list']);
    expect(list.data.length).toBe(0);
  });

  it('template save --force overwrites', () => {
    tplRun(['template', 'save', '--name', 'game-night', '--title', 'V1']);
    const out = tplRun(['template', 'save', '--name', 'game-night', '--title', 'V2', '--force']);
    expect(out.data.title).toBe('V2');
  });

  it('template save without --force rejects duplicate', () => {
    tplRun(['template', 'save', '--name', 'game-night', '--title', 'V1']);
    const { stdout } = tplRunRaw(['template', 'save', '--name', 'game-night', '--title', 'V2']);
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('error');
    expect(out.error.type).toBe('validation_error');
  });
});

describe('template variable substitution', () => {
  it('replaces {{variables}} in template fields', async () => {
    const { applyVariables } = await import('../src/lib/templates.js');
    const tpl = { title: 'Game Night Week {{week_number}}', location: '{{venue}}' };
    const result = applyVariables(tpl, { week_number: '15', venue: 'My Place' });
    expect(result.title).toBe('Game Night Week 15');
    expect(result.location).toBe('My Place');
  });

  it('preserves unmatched variables', async () => {
    const { applyVariables } = await import('../src/lib/templates.js');
    const tpl = { title: '{{greeting}} {{unknown}}' };
    const result = applyVariables(tpl, { greeting: 'Hi' });
    expect(result.title).toBe('Hi {{unknown}}');
  });
});
