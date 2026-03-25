import { describe, it, expect } from 'vitest';
import { run, runRaw } from './helpers.js';

describe('posters list', () => {
  it('returns success with array and expected fields', () => {
    const res = run(['posters', 'list', '--limit', '3']);
    expect(res.status).toBe('success');
    expect(Array.isArray(res.data)).toBe(true);
    expect(res.data.length).toBeLessThanOrEqual(3);
    const p = res.data[0];
    expect(p).toHaveProperty('id');
    expect(p).toHaveProperty('name');
    expect(p).toHaveProperty('contentType');
    expect(p).toHaveProperty('categories');
    expect(p).toHaveProperty('tags');
    expect(p).toHaveProperty('width');
    expect(p).toHaveProperty('height');
    expect(p).toHaveProperty('url');
    expect(p).toHaveProperty('thumbnail');
    expect(p).toHaveProperty('bgColor');
  });

  it('respects --limit', () => {
    const res = run(['posters', 'list', '--limit', '5']);
    expect(res.data.length).toBeLessThanOrEqual(5);
  });

  it('metadata includes count and totalAvailable', () => {
    const res = run(['posters', 'list', '--limit', '2']);
    expect(res.metadata).toHaveProperty('count');
    expect(res.metadata).toHaveProperty('totalAvailable');
    expect(res.metadata.count).toBe(res.data.length);
    expect(res.metadata.totalAvailable).toBeGreaterThan(0);
  });

  it('filters by --category Birthday', () => {
    const res = run(['posters', 'list', '--category', 'Birthday', '--limit', '50']);
    expect(res.status).toBe('success');
    for (const p of res.data) {
      expect(p.categories.map(c => c.toLowerCase())).toContain('birthday');
    }
  });
});

describe('posters search', () => {
  it('finds results with score field', () => {
    const res = run(['posters', 'search', 'birthday']);
    expect(res.status).toBe('success');
    expect(res.data.length).toBeGreaterThan(0);
    expect(res.data[0]).toHaveProperty('score');
    expect(res.data[0].score).toBeGreaterThan(0);
  });

  it('respects --limit', () => {
    const res = run(['posters', 'search', 'party', '--limit', '3']);
    expect(res.data.length).toBeLessThanOrEqual(3);
  });

  it('returns empty array for no matches', () => {
    const res = run(['posters', 'search', 'xyzzyflurble123']);
    expect(res.status).toBe('success');
    expect(res.data).toEqual([]);
  });
});

describe('posters get', () => {
  it('returns full poster by ID', () => {
    const res = run(['posters', 'get', 'piscesairbrush.png']);
    expect(res.status).toBe('success');
    expect(res.data.id).toBe('piscesairbrush.png');
  });

  it('returns not_found error for missing ID', () => {
    const { stdout, exitCode } = runRaw(['posters', 'get', 'does-not-exist-xyz']);
    const res = JSON.parse(stdout.trim());
    expect(res.status).toBe('error');
    expect(res.error.type).toBe('not_found');
    expect(exitCode).toBe(4);
  });
});
