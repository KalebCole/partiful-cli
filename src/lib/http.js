/**
 * HTTP module for Partiful CLI.
 * Handles API requests with retry logic and error classification.
 */

import { AuthError, NotFoundError, ApiError } from './errors.js';

const API_BASE = 'https://api.partiful.com';
const FIRESTORE_BASE = 'https://firestore.googleapis.com';
const FIRESTORE_PROJECT = 'getpartiful';

const RETRYABLE_CODES = new Set([429, 500, 502, 503, 504]);
const MAX_RETRIES = parseInt(process.env.PARTIFUL_MAX_RETRIES || '3', 10);

function classifyError(statusCode, message, body) {
  if (statusCode === 401 || statusCode === 403) {
    return new AuthError(message || `Auth failed (${statusCode})`, { statusCode, body });
  }
  if (statusCode === 404) {
    return new NotFoundError(message || 'Not found', { statusCode, body });
  }
  return new ApiError(message || `API error (${statusCode})`, { statusCode, body });
}

async function withRetry(fn, verbose = false) {
  let lastError;
  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    try {
      const resp = await fn();

      if (resp.ok || !RETRYABLE_CODES.has(resp.status)) {
        return resp;
      }

      // Retryable status
      lastError = resp;
      if (attempt < MAX_RETRIES) {
        const retryAfter = resp.headers?.get('retry-after');
        const delay = retryAfter
          ? Math.min(30, parseFloat(retryAfter)) * 1000
          : Math.min(30000, (2 ** attempt + Math.random()) * 1000);
        if (verbose) console.error(`Retry ${attempt + 1}/${MAX_RETRIES} after ${Math.round(delay)}ms...`);
        await new Promise(r => setTimeout(r, delay));
      }
    } catch (err) {
      lastError = err;
      if (attempt < MAX_RETRIES) {
        const delay = Math.min(30000, (2 ** attempt + Math.random()) * 1000);
        await new Promise(r => setTimeout(r, delay));
      }
    }
  }

  // Exhausted retries
  if (lastError instanceof Response || (lastError && lastError.status)) {
    const body = await lastError.text().catch(() => '');
    throw classifyError(lastError.status, `Request failed after ${MAX_RETRIES} retries`, body);
  }
  throw lastError instanceof Error ? lastError : new ApiError('Request failed after retries');
}

export async function apiRequest(method, endpoint, token, body = null, verbose = false) {
  const resp = await withRetry(() =>
    fetch(`${API_BASE}${endpoint}`, {
      method,
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        'Accept': 'application/json, text/plain, */*',
        'Origin': 'https://partiful.com',
        'Referer': 'https://partiful.com/',
      },
      ...(body ? { body: JSON.stringify(body) } : {}),
    }),
    verbose
  );

  if (!resp.ok) {
    const text = await resp.text().catch(() => '');
    throw classifyError(resp.status, `API ${method} ${endpoint} failed`, text);
  }

  const text = await resp.text();
  return text ? JSON.parse(text) : {};
}

export async function firestoreRequest(method, eventId, body, token, updateFields = [], verbose = false) {
  let fsPath = `/v1/projects/${FIRESTORE_PROJECT}/databases/(default)/documents/events/${eventId}`;
  if (method === 'PATCH' && updateFields.length > 0) {
    fsPath += '?' + updateFields.map(f => `updateMask.fieldPaths=${f}`).join('&');
  }

  const resp = await withRetry(() =>
    fetch(`${FIRESTORE_BASE}${fsPath}`, {
      method,
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        'Referer': 'https://partiful.com/',
      },
      ...(body ? { body: JSON.stringify(body) } : {}),
    }),
    verbose
  );

  if (!resp.ok) {
    const text = await resp.text().catch(() => '');
    throw classifyError(resp.status, `Firestore ${method} failed`, text);
  }

  const text = await resp.text();
  return text ? JSON.parse(text) : {};
}

export async function firestoreListDocuments(collectionPath, token, pageSize = 100, pageToken = null, verbose = false) {
  let fsPath = `/v1/projects/${FIRESTORE_PROJECT}/databases/(default)/documents/${collectionPath}?pageSize=${pageSize}`;
  if (pageToken) fsPath += `&pageToken=${encodeURIComponent(pageToken)}`;

  const resp = await withRetry(() =>
    fetch(`${FIRESTORE_BASE}${fsPath}`, {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Referer': 'https://partiful.com/',
      },
    }),
    verbose
  );

  if (!resp.ok) {
    const text = await resp.text().catch(() => '');
    throw classifyError(resp.status, `Firestore list failed`, text);
  }

  const text = await resp.text();
  return text ? JSON.parse(text) : {};
}
