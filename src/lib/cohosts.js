/**
 * Shared co-host helpers: contact resolution, Firestore read/write.
 */

import { apiRequest, firestoreRequest } from './http.js';
import { wrapPayload } from './auth.js';

/**
 * Resolve co-host names to Partiful user IDs via the contacts API.
 * Tries exact match first, then substring. Warns on stderr for misses.
 * @returns {string[]} resolved user IDs
 */
export async function resolveCohostNames(names, token, config, verbose = false) {
  if (!names || names.length === 0) return [];

  const payload = {
    data: wrapPayload(config, {
      params: {},
      amplitudeSessionId: Date.now(),
      userId: config.userId,
    }),
  };
  const result = await apiRequest('POST', '/getContacts', token, payload, verbose);
  const contacts = result.result?.data || [];

  const ids = [];
  for (const name of names) {
    const q = name.toLowerCase();
    const match =
      contacts.find(c => (c.name || '').toLowerCase() === q) ||
      contacts.find(c => (c.name || '').toLowerCase().includes(q));
    if (match?.userId) {
      if (!ids.includes(match.userId)) ids.push(match.userId);
    } else {
      process.stderr.write(`Warning: could not resolve co-host "${name}" from contacts — skipping\n`);
    }
  }
  return ids;
}

/**
 * Read cohostIds array from a Firestore event document.
 * @returns {string[]}
 */
export async function getCohostIds(eventId, token, verbose = false) {
  const doc = await firestoreRequest('GET', eventId, null, token, [], verbose);
  const values = doc.fields?.cohostIds?.arrayValue?.values || [];
  return values.map(v => v.stringValue).filter(Boolean);
}

/**
 * Write cohostIds array to a Firestore event document.
 */
export async function setCohostIds(eventId, ids, token, verbose = false) {
  const unique = [...new Set(ids.filter(Boolean))];
  const fields = {
    cohostIds: {
      arrayValue: { values: unique.map(id => ({ stringValue: id })) },
    },
  };
  await firestoreRequest('PATCH', eventId, { fields }, token, ['cohostIds'], verbose);
}
