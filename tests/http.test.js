import { afterEach, describe, it, expect, vi } from 'vitest';
import {
  apiRequest,
  firestoreGetDocument,
  firestoreRequest,
  firestoreListDocuments,
} from '../src/lib/http.js';

afterEach(() => {
  vi.unstubAllGlobals();
});

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

  it('bounds Firestore document reads with an abort signal', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: async () => '{}',
    });
    vi.stubGlobal('fetch', fetchMock);

    await firestoreGetDocument('events/EV1/guests/G7', 'token');

    expect(fetchMock).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });
});
