import { describe, it, expect } from 'vitest';
import { run, runRaw } from './helpers.js';

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

    it('events create --poster includes image in payload', () => {
      const out = run([
        'events', 'create',
        '--title', 'Poster Test',
        '--date', '2026-06-01 7pm',
        '--poster', 'piscesairbrush.png',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      const event = out.data.payload.data.params.event;
      expect(event.image).toBeDefined();
      expect(event.image.source).toBe('partiful_posters');
      expect(event.image.poster.id).toBe('piscesairbrush.png');
      expect(event.image.url).toContain('assets.getpartiful.com');
    });

    it('events create --poster errors on unknown poster', () => {
      const { stdout } = runRaw([
        'events', 'create',
        '--title', 'Bad Poster',
        '--date', '2026-06-01 7pm',
        '--poster', 'nonexistent-poster-xyz',
        '--dry-run',
      ]);
      const out = JSON.parse(stdout.trim());
      expect(out.status).toBe('error');
      expect(out.error.type).toBe('not_found');
    });

    it('events create --poster-search finds and uses best match', () => {
      const out = run([
        'events', 'create',
        '--title', 'Search Test',
        '--date', '2026-06-01 7pm',
        '--poster-search', 'birthday',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      const event = out.data.payload.data.params.event;
      expect(event.image).toBeDefined();
      expect(event.image.source).toBe('partiful_posters');
    });

    it('events create errors when both --poster and --poster-search given', () => {
      const { stdout } = runRaw([
        'events', 'create',
        '--title', 'Conflict',
        '--date', '2026-06-01 7pm',
        '--poster', 'piscesairbrush.png',
        '--poster-search', 'birthday',
        '--dry-run',
      ]);
      const out = JSON.parse(stdout.trim());
      expect(out.status).toBe('error');
    });

    it('events create --image validates file extension', () => {
      const { stdout } = runRaw([
        'events', 'create',
        '--title', 'Upload Test',
        '--date', '2026-06-01 7pm',
        '--image', '/tmp/not-an-image.txt',
        '--dry-run',
      ]);
      const out = JSON.parse(stdout);
      expect(out.status).toBe('error');
      expect(out.error.message).toContain('Unsupported');
    });

    it('events create errors when --poster and --image used together', () => {
      const { stdout } = runRaw([
        'events', 'create',
        '--title', 'Conflict',
        '--date', '2026-06-01 7pm',
        '--poster', 'piscesairbrush.png',
        '--image', '/tmp/test.png',
        '--dry-run',
      ]);
      const out = JSON.parse(stdout);
      expect(out.status).toBe('error');
    });
  });

  describe('JSON envelope shape - events update', () => {
    it('events update --poster in dry-run', () => {
      const out = run([
        'events', 'update', 'test-event-123',
        '--poster', 'piscesairbrush.png',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      expect(out.data.dryRun).toBe(true);
      expect(out.data.fields).toContain('image');
    });

    it('events update --image validates extension', () => {
      const { stdout } = runRaw([
        'events', 'update', 'test-event-123',
        '--image', '/tmp/bad-file.txt',
        '--dry-run',
      ]);
      const out = JSON.parse(stdout);
      expect(out.status).toBe('error');
      expect(out.error.message).toContain('Unsupported');
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
    const { stdout, exitCode } = runRaw(['version']);
    expect(exitCode).toBe(0);
    const out = JSON.parse(stdout.trim());
    expect(out.status).toBe('success');
    expect(out.data.cli).toBe('partiful');
    expect(out.data.version).toBeTruthy();
    expect(out.data.node).toMatch(/^v/);
  });
});
