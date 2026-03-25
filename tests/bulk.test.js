/**
 * Tests for bulk commands and series creation.
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { run, runRaw } from './helpers.js';
import fs from 'fs';
import path from 'path';
import os from 'os';

let tmpDir;

describe('bulk create', () => {
  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'partiful-bulk-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('--dry-run previews events from JSON file', () => {
    const events = [
      { title: 'Event 1', date: '2026-04-15 7pm', location: 'Place A' },
      { title: 'Event 2', date: '2026-04-22 7pm', location: 'Place B' },
    ];
    const jsonFile = path.join(tmpDir, 'events.json');
    fs.writeFileSync(jsonFile, JSON.stringify(events));

    const out = run(['bulk', 'create', jsonFile, '--dry-run']);
    expect(out.status).toBe('success');
    expect(out.data.length).toBe(2);
    expect(out.data[0].title).toBe('Event 1');
    expect(out.data[1].title).toBe('Event 2');
    expect(out.metadata.action).toBe('dry_run');
  });

  it('--dry-run previews events from CSV file', () => {
    const csv = 'title,date,location,capacity\nParty A,2026-05-01 8pm,Venue X,20\nParty B,2026-05-08 8pm,Venue Y,30';
    const csvFile = path.join(tmpDir, 'events.csv');
    fs.writeFileSync(csvFile, csv);

    const out = run(['bulk', 'create', csvFile, '--dry-run']);
    expect(out.status).toBe('success');
    expect(out.data.length).toBe(2);
    expect(out.data[0].title).toBe('Party A');
    expect(out.data[0].location).toBe('Venue X');
  });

  it('rejects empty JSON array', () => {
    const jsonFile = path.join(tmpDir, 'empty.json');
    fs.writeFileSync(jsonFile, '[]');

    const { stdout } = runRaw(['bulk', 'create', jsonFile, '--dry-run']);
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('error');
  });

  it('rejects missing file', () => {
    const { stdout } = runRaw(['bulk', 'create', '/tmp/nonexistent-xyz-12345.json', '--dry-run']);
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('error');
    expect(out.error.message).toContain('not found');
  });

  it('validates rows have required fields', () => {
    const events = [{ title: 'No Date Event' }];
    const jsonFile = path.join(tmpDir, 'bad.json');
    fs.writeFileSync(jsonFile, JSON.stringify(events));

    const { stdout } = runRaw(['bulk', 'create', jsonFile, '--dry-run']);
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('error');
    expect(out.error.message).toContain('missing "date"');
  });
});

describe('events create --repeat (series)', () => {
  it('--dry-run with --repeat and --count shows series info', () => {
    const out = run(['events', 'create', '--title', 'Weekly Game Night', '--date', '2026-04-15 7pm', '--repeat', 'weekly', '--count', '4', '--dry-run']);
    expect(out.status).toBe('success');
    expect(out.data.dryRun).toBe(true);
    expect(out.data.series.repeat).toBe('weekly');
    expect(out.data.series.count).toBe(4);
  });
});

describe('events create --template', () => {
  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'partiful-tpl-create-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('--template fills in fields from saved template', () => {
    // Save a template
    run(['template', 'save', '--name', 'game-night', '--title', 'Game Night', '--location', 'My Place', '--capacity', '10'], {
      env: { PARTIFUL_TEMPLATES_FILE: path.join(tmpDir, 'templates.json') },
    });

    // Create event using template
    const out = run(['events', 'create', '--template', 'game-night', '--date', '2026-04-15 7pm', '--dry-run'], {
      env: { PARTIFUL_TEMPLATES_FILE: path.join(tmpDir, 'templates.json') },
    });
    expect(out.status).toBe('success');
    expect(out.data.dryRun).toBe(true);
    expect(out.data.payload.data.params.event.title).toBe('Game Night');
    expect(out.data.payload.data.params.event.location).toBe('My Place');
  });

  it('CLI opts override template values', () => {
    run(['template', 'save', '--name', 'game-night', '--title', 'Game Night', '--location', 'My Place'], {
      env: { PARTIFUL_TEMPLATES_FILE: path.join(tmpDir, 'templates.json') },
    });

    const out = run(['events', 'create', '--template', 'game-night', '--title', 'Custom Title', '--date', '2026-04-15 7pm', '--dry-run'], {
      env: { PARTIFUL_TEMPLATES_FILE: path.join(tmpDir, 'templates.json') },
    });
    expect(out.data.payload.data.params.event.title).toBe('Custom Title');
  });

  it('--template with missing template returns error', () => {
    const { stdout } = runRaw(['events', 'create', '--template', 'nonexistent', '--date', '2026-04-15 7pm'], {
      env: { PARTIFUL_TEMPLATES_FILE: path.join(tmpDir, 'templates.json') },
    });
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('error');
    expect(out.error.type).toBe('not_found');
  });

  it('--template with --var substitutes variables', () => {
    run(['template', 'save', '--name', 'weekly', '--title', 'Game Night Week {{week}}', '--location', '{{venue}}'], {
      env: { PARTIFUL_TEMPLATES_FILE: path.join(tmpDir, 'templates.json') },
    });

    const out = run(['events', 'create', '--template', 'weekly', '--date', '2026-04-15 7pm', '--var', 'week=15', '--var', 'venue=My House', '--dry-run'], {
      env: { PARTIFUL_TEMPLATES_FILE: path.join(tmpDir, 'templates.json') },
    });
    expect(out.data.payload.data.params.event.title).toBe('Game Night Week 15');
    expect(out.data.payload.data.params.event.location).toBe('My House');
  });
});
