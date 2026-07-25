/**
 * Shared co-host helpers: contact resolution, Firestore read/write.
 */

import { apiRequest, firestoreRequest } from './http.js';
import { wrapPayload } from './auth.js';
import type { PartifulConfig } from './auth.js';
import type { GetContactsData } from './api/endpoints.js';

/** A firebase-callable result wrapping the getContacts array. */
interface GetContactsEnvelope {
  result?: { data?: GetContactsData };
}

/**
 * Resolve co-host names to Partiful user IDs via the contacts API.
 * Tries exact match first, then substring. Warns on stderr for misses.
 * @returns resolved user IDs
 */
export async function resolveCohostNames(
  names: string[],
  token: string,
  config: PartifulConfig,
  verbose = false,
): Promise<string[]> {
  if (!names || names.length === 0) return [];

  const payload = {
    data: wrapPayload(config, {
      params: {},
      amplitudeSessionId: Date.now(),
      userId: config.userId,
    }),
  };
  const result = (await apiRequest('POST', '/getContacts', token, payload, verbose)) as GetContactsEnvelope;
  const contacts = result.result?.data || [];

  const ids: string[] = [];
  for (const name of names) {
    const q = name.toLowerCase();
    const match =
      contacts.find((c) => (c.name || '').toLowerCase() === q) ||
      contacts.find((c) => (c.name || '').toLowerCase().includes(q));
    if (match?.userId) {
      if (!ids.includes(match.userId)) ids.push(match.userId);
    } else {
      process.stderr.write(`Warning: could not resolve co-host "${name}" from contacts — skipping\n`);
    }
  }
  return ids;
}

/** A Firestore event doc, narrowed to the cohostIds array field we read. */
interface FirestoreEventDoc {
  fields?: {
    cohostIds?: {
      arrayValue?: {
        values?: Array<{ stringValue?: string }>;
      };
    };
  };
}

/**
 * Read cohostIds array from a Firestore event document.
 */
export async function getCohostIds(
  eventId: string,
  token: string,
  verbose = false,
): Promise<string[]> {
  const doc = (await firestoreRequest('GET', eventId, null, token, [], verbose)) as FirestoreEventDoc;
  const values = doc.fields?.cohostIds?.arrayValue?.values || [];
  return values.map((v) => v.stringValue).filter((v): v is string => Boolean(v));
}

/**
 * Write cohostIds array to a Firestore event document.
 */
export async function setCohostIds(
  eventId: string,
  ids: string[],
  token: string,
  verbose = false,
): Promise<void> {
  const unique = [...new Set(ids.filter(Boolean))];
  const fields = {
    cohostIds: {
      arrayValue: { values: unique.map((id) => ({ stringValue: id })) },
    },
  };
  await firestoreRequest('PATCH', eventId, { fields }, token, ['cohostIds'], verbose);
}
