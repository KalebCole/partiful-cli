import { describe, it, expect } from 'vitest';
import { apiRequest, firestoreRequest, firestoreListDocuments, firestoreGetDocument } from '../src/lib/http.js';

describe('http module exports', () => {
  it('exports apiRequest as function', () => {
    expect(typeof apiRequest).toBe('function');
  });
  it('exports firestoreRequest as function', () => {
    expect(typeof firestoreRequest).toBe('function');
  });
  it('exports firestoreListDocuments as function', () => {
    expect(typeof firestoreListDocuments).toBe('function');
  });
  it('exports firestoreGetDocument as function', () => {
    expect(typeof firestoreGetDocument).toBe('function');
  });
});
