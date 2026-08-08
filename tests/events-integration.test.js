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
      expect(out.data.endpoint).toBe('/getEventInfo');
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

    it('events create --rsvp-deadline encodes a closed-after-deadline policy', () => {
      const out = run([
        'events', 'create',
        '--title', 'Sunset Picnic',
        '--date', '2026-08-22 3pm',
        '--timezone', 'America/Los_Angeles',
        '--rsvp-deadline', '2026-08-20 12pm',
        '--dry-run',
      ]);
      const event = out.data.payload.data.params.event;
      expect(event.rsvpDeadline).toBe('2026-08-20T19:00:00.000Z');
      expect(event.allowResponsesAfterRsvpDeadline).toBe(false);
    });

    it('events create rejects an RSVP deadline after the event starts', () => {
      const { stdout, exitCode } = runRaw([
        'events', 'create',
        '--title', 'Bad Deadline',
        '--date', '2026-08-22 3pm',
        '--timezone', 'America/Los_Angeles',
        '--rsvp-deadline', '2026-08-23 12pm',
        '--dry-run',
      ]);
      expect(exitCode).not.toBe(0);
      const out = JSON.parse(stdout.trim());
      expect(out.status).toBe('error');
      expect(out.error.type).toBe('validation_error');
      expect(out.error.message).toContain('before the event starts');
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

    it('events create --image with URL shows download note in dry-run', () => {
      const out = run([
        'events', 'create',
        '--title', 'URL Image Test',
        '--date', '2026-06-01 7pm',
        '--image', 'https://example.com/test.png',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      const image = out.data.payload.data.params.event.image;
      expect(image.url).toBe('https://example.com/test.png');
      expect(image.note).toBe('URL will be downloaded and uploaded on real run');
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

    it('events update --image with URL in dry-run', () => {
      const out = run([
        'events', 'update', 'test-event-123',
        '--image', 'https://example.com/test.png',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      expect(out.data.dryRun).toBe(true);
      expect(out.data.fields).toContain('image');
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
      expect(out.data.parameters['--rsvp-deadline'].required).toBe(false);
      expect(out.data.parameters['--rsvp-deadline'].type).toBe('string');
    });

    it('schema events.clone exposes date and shift alternatives', () => {
      const out = run(['schema', 'events.clone']);
      expect(out.status).toBe('success');
      expect(out.data.command).toBe('events clone <eventId>');
      expect(out.data.parameters['--date'].required).toBe(false);
      expect(out.data.parameters['--shift'].default).toBe(7);
    });

    it('schema unknown path returns error', () => {
      const { stdout, exitCode } = runRaw(['schema', 'nonexistent']);
      expect(exitCode).not.toBe(0);
      const out = JSON.parse(stdout.trim());
      expect(out.status).toBe('error');
      expect(out.error.type).toBe('not_found');
    });
  });

  describe('--link flags', () => {
    it('events create --link --dry-run includes links in payload', () => {
      const out = run([
        'events', 'create',
        '--title', 'Link Party',
        '--date', '2026-06-01 7pm',
        '--link', 'https://zoom.us/j/123',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      const event = out.data.payload.data.params.event;
      expect(event.links).toEqual([{ url: 'https://zoom.us/j/123', text: 'https://zoom.us/j/123' }]);
    });

    it('events create with multiple --link flags', () => {
      const out = run([
        'events', 'create',
        '--title', 'Multi Link',
        '--date', '2026-06-01 7pm',
        '--link', 'https://zoom.us/j/123',
        '--link', 'https://docs.google.com/doc',
        '--dry-run',
      ]);
      const event = out.data.payload.data.params.event;
      expect(event.links).toHaveLength(2);
      expect(event.links[0].url).toBe('https://zoom.us/j/123');
      expect(event.links[1].url).toBe('https://docs.google.com/doc');
    });

    it('events create with --link + --link-text pairing', () => {
      const out = run([
        'events', 'create',
        '--title', 'Named Links',
        '--date', '2026-06-01 7pm',
        '--link', 'https://zoom.us/j/123',
        '--link-text', 'Zoom',
        '--link', 'https://docs.google.com/doc',
        '--link-text', 'Agenda',
        '--dry-run',
      ]);
      const event = out.data.payload.data.params.event;
      expect(event.links).toEqual([
        { url: 'https://zoom.us/j/123', text: 'Zoom' },
        { url: 'https://docs.google.com/doc', text: 'Agenda' },
      ]);
    });

    it('events update --link --dry-run includes links in update', () => {
      const out = run([
        'events', 'update', 'test-event-123',
        '--link', 'https://example.com',
        '--link-text', 'Example',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      expect(out.data.fields).toContain('links');
      const linksField = out.data.body.fields.links;
      expect(linksField.arrayValue.values).toHaveLength(1);
      expect(linksField.arrayValue.values[0].mapValue.fields.url.stringValue).toBe('https://example.com');
      expect(linksField.arrayValue.values[0].mapValue.fields.text.stringValue).toBe('Example');
    });

    it('events update --rsvp-deadline encodes Firestore fields', () => {
      const out = run([
        'events', 'update', 'test-event-123',
        '--rsvp-deadline', '2026-08-20 12pm',
        '--timezone', 'America/Los_Angeles',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      expect(out.data.fields).toContain('rsvpDeadline');
      expect(out.data.fields).toContain('allowResponsesAfterRsvpDeadline');
      expect(out.data.body.fields.rsvpDeadline.timestampValue).toBe('2026-08-20T19:00:00.000Z');
      expect(out.data.body.fields.allowResponsesAfterRsvpDeadline.booleanValue).toBe(false);
    });
  });
});

describe('events clone', () => {
  it('events clone --dry-run produces createEvent payload with clonedFrom', () => {
    const out = run([
      'events', 'clone', 'test-event-123',
      '--date', '2026-06-01 7pm',
      '--dry-run',
    ]);
    expect(out.status).toBe('success');
    expect(out.data.dryRun).toBe(true);
    expect(out.data.endpoint).toBe('/createEvent');
    expect(out.data.clonedFrom).toBe('test-event-123');
    expect(out.data.payload.data.params.event).toBeDefined();
    expect(out.data.payload.data.params.event.startDate).toBeDefined();
  });

  it('events clone --dry-run with --title override applies override', () => {
    const out = run([
      'events', 'clone', 'test-event-123',
      '--date', '2026-06-01 7pm',
      '--title', 'Override Title',
      '--dry-run',
    ]);
    expect(out.status).toBe('success');
    const event = out.data.payload.data.params.event;
    expect(event.title).toBe('Override Title');
  });

  it('legacy clone alias resolves to events clone', () => {
    const { exitCode } = runRaw([
      'clone', 'test-event-123', '--date', '2026-06-01 7pm', '--dry-run',
    ]);
    expect(exitCode).toBe(0);
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
