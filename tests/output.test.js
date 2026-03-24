import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { jsonOutput, jsonError, formatTable, formatCsv, EXIT } from '../src/lib/output.js';

describe('jsonOutput', () => {
  let stdout;
  beforeEach(() => {
    stdout = '';
    vi.spyOn(process.stdout, 'write').mockImplementation((chunk) => { stdout += chunk; });
  });
  afterEach(() => vi.restoreAllMocks());

  it('wraps data in success envelope', () => {
    jsonOutput({ id: '123', title: 'Party' });
    const parsed = JSON.parse(stdout);
    expect(parsed.status).toBe('success');
    expect(parsed.data.id).toBe('123');
  });

  it('includes metadata when provided', () => {
    jsonOutput({ events: [] }, { count: 0 });
    const parsed = JSON.parse(stdout);
    expect(parsed.metadata.count).toBe(0);
  });
});

describe('jsonError', () => {
  let stdout;
  let exitCode;
  beforeEach(() => {
    stdout = '';
    vi.spyOn(process.stdout, 'write').mockImplementation((chunk) => { stdout += chunk; });
    vi.spyOn(process, 'exit').mockImplementation((code) => { exitCode = code; throw new Error('EXIT'); });
  });
  afterEach(() => vi.restoreAllMocks());

  it('outputs error envelope and exits with code', () => {
    try { jsonError('Not found', 4, 'not_found'); } catch {}
    const parsed = JSON.parse(stdout);
    expect(parsed.status).toBe('error');
    expect(parsed.error.code).toBe(4);
    expect(parsed.error.type).toBe('not_found');
    expect(exitCode).toBe(4);
  });
});

describe('formatTable', () => {
  it('formats rows into aligned columns', () => {
    const rows = [{ name: 'Alice', age: '30' }, { name: 'Bob', age: '25' }];
    const result = formatTable(rows, ['name', 'age']);
    expect(result).toContain('Alice');
    expect(result).toContain('Bob');
  });
  it('returns no results for empty array', () => {
    expect(formatTable([], ['a'])).toBe('(no results)');
  });
});

describe('formatCsv', () => {
  it('formats as CSV with headers', () => {
    const rows = [{ name: 'Alice', age: '30' }];
    const result = formatCsv(rows, ['name', 'age']);
    expect(result).toBe('name,age\nAlice,30');
  });
  it('escapes commas in values', () => {
    const rows = [{ name: 'Cole, Kaleb', age: '22' }];
    const result = formatCsv(rows, ['name', 'age']);
    expect(result).toContain('"Cole, Kaleb"');
  });
});

describe('EXIT codes', () => {
  it('has all expected codes', () => {
    expect(EXIT.SUCCESS).toBe(0);
    expect(EXIT.API_ERROR).toBe(1);
    expect(EXIT.AUTH_ERROR).toBe(2);
    expect(EXIT.VALIDATION_ERROR).toBe(3);
    expect(EXIT.NOT_FOUND).toBe(4);
    expect(EXIT.INTERNAL_ERROR).toBe(5);
  });
});
