/** Canonical cohost lifecycle helpers. */

import { apiRequest, firestoreRequest, firestoreListDocuments, firestoreGetDocument } from './http.js';
import { wrapPayload } from './auth.js';
import type { PartifulConfig } from './auth.js';
import type { GetContactsData } from './api/endpoints.js';
import { NotFoundError, ValidationError } from './errors.js';

interface GetContactsEnvelope {
  result?: { data?: GetContactsData };
}

export interface CohostContact {
  userId?: string;
  name?: string;
}

export type CohostStatus = 'pending' | 'accepted' | 'declined' | 'stale';
export interface CohostState {
  userId: string;
  status: CohostStatus;
  name?: string | null;
}

interface RawCohostRequest {
  userId: string;
  status: string;
}

interface FirestoreValue {
  stringValue?: string;
  timestampValue?: string;
}

interface FirestoreDocument {
  name?: string;
  fields?: Record<string, FirestoreValue>;
}

interface FirestoreListEnvelope {
  documents?: FirestoreDocument[];
}

export type CanonicalCohostCall = (endpoint: string, params: Record<string, string>) => Promise<unknown>;

/** Resolve every supplied name uniquely. Any miss or ambiguity fails the command. */
export function resolveContactNames(names: string[], contacts: CohostContact[]): string[] {
  const ids: string[] = [];
  for (const name of names ?? []) {
    const query = name.trim().toLowerCase();
    if (!query) throw new ValidationError('Co-host name cannot be empty');

    const usable = contacts.filter((contact) => contact.userId && contact.name);
    const exact = usable.filter((contact) => contact.name!.trim().toLowerCase() === query);
    const candidates = exact.length > 0
      ? exact
      : usable.filter((contact) => contact.name!.toLowerCase().includes(query));

    if (candidates.length === 0) {
      throw new ValidationError(`Could not resolve co-host "${name}" from contacts`, { query: name });
    }
    if (candidates.length > 1) {
      throw new ValidationError(`Ambiguous co-host name "${name}"`, {
        query: name,
        candidates: candidates.map(({ userId, name: candidateName }) => ({ userId, name: candidateName })),
      });
    }

    const id = candidates[0]!.userId!;
    if (!ids.includes(id)) ids.push(id);
  }
  return ids;
}

export async function getContacts(
  token: string,
  config: PartifulConfig,
  verbose = false,
): Promise<GetContactsData> {
  const payload = {
    data: wrapPayload(config, {
      params: {},
      amplitudeSessionId: Date.now(),
      userId: config.userId,
    }),
  };
  const result = (await apiRequest('POST', '/getContacts', token, payload, verbose)) as GetContactsEnvelope;
  return result.result?.data || [];
}

export async function resolveCohostNames(
  names: string[],
  token: string,
  config: PartifulConfig,
  verbose = false,
): Promise<string[]> {
  if (!names || names.length === 0) return [];
  return resolveContactNames(names, await getContacts(token, config, verbose));
}

interface FirestoreEventDoc {
  fields?: {
    cohostIds?: { arrayValue?: { values?: Array<{ stringValue?: string }> } };
  };
}

export async function getCohostIds(eventId: string, token: string, verbose = false): Promise<string[]> {
  const doc = (await firestoreRequest('GET', eventId, null, token, [], verbose)) as FirestoreEventDoc;
  const values = doc.fields?.cohostIds?.arrayValue?.values || [];
  return values.map((value) => value.stringValue).filter((value): value is string => Boolean(value));
}

/** @deprecated Lifecycle commands must use canonical callables, not raw membership writes. */
export async function setCohostIds(
  eventId: string,
  ids: string[],
  token: string,
  verbose = false,
): Promise<void> {
  const unique = [...new Set(ids.filter(Boolean))];
  const fields = { cohostIds: { arrayValue: { values: unique.map((id) => ({ stringValue: id })) } } };
  await firestoreRequest('PATCH', eventId, { fields }, token, ['cohostIds'], verbose);
}

export async function getCohostRequests(eventId: string, token: string, verbose = false): Promise<RawCohostRequest[]> {
  const result = (await firestoreListDocuments(
    `events/${eventId}/cohostRequests`, token, 100, null, verbose,
  )) as FirestoreListEnvelope;
  return (result.documents ?? []).map((document) => ({
    userId: document.fields?.targetUserId?.stringValue || document.fields?.cohostId?.stringValue || document.name?.split('/').pop() || '',
    status: document.fields?.status?.stringValue || 'PENDING',
  })).filter((request) => Boolean(request.userId));
}

export function mergeCohostState(requests: RawCohostRequest[], cohostIds: string[]): CohostState[] {
  const states: CohostState[] = requests.map((request) => {
    const normalized = request.status.toLowerCase();
    const status: CohostStatus = normalized === 'accepted' || normalized === 'declined' ? normalized : 'pending';
    return { userId: request.userId, status };
  });
  const requestIds = new Set(states.map((state) => state.userId));
  for (const userId of cohostIds) {
    if (!requestIds.has(userId)) states.push({ userId, status: 'stale' });
  }
  return states;
}

export async function getCohostState(eventId: string, token: string, verbose = false): Promise<CohostState[]> {
  const [requests, ids] = await Promise.all([
    getCohostRequests(eventId, token, verbose),
    getCohostIds(eventId, token, verbose),
  ]);
  return mergeCohostState(requests, ids);
}

export type CohostInviteAction = 'invite' | 'repair' | 'reinvite' | 'noop';
export function planCohostInvite(state?: CohostState): CohostInviteAction {
  if (!state) return 'invite';
  if (state.status === 'stale') return 'repair';
  if (state.status === 'declined') return 'reinvite';
  return 'noop';
}

export type CohostRemovalAction = 'delete_request' | 'remove_cohost';
export function planCohostRemoval(state?: CohostState): CohostRemovalAction {
  if (!state) throw new NotFoundError('User is not a co-host of this event');
  return state.status === 'pending' || state.status === 'declined' ? 'delete_request' : 'remove_cohost';
}

export async function inviteCohost(
  eventId: string,
  userId: string,
  state: CohostState | undefined,
  call: CanonicalCohostCall,
  repairStale?: () => Promise<void>,
): Promise<{ userId: string; outcome: string }> {
  const action = planCohostInvite(state);
  if (action === 'noop') return { userId, outcome: state!.status };
  if (action === 'repair') {
    if (!repairStale) throw new ValidationError('Stale co-host membership requires an explicit repair hook');
    // Both canonical endpoints return INTERNAL for a legacy ID with no request.
    await repairStale();
  }
  await call('/createCohostRequest', { eventId, targetUserId: userId });
  const outcome = action === 'repair' ? 'stale_repair' : action === 'reinvite' ? 'reinvited' : 'invited';
  return { userId, outcome };
}

export async function inviteCohostBatch(
  eventId: string,
  userIds: string[],
  states: CohostState[],
  call: CanonicalCohostCall,
  repairStale?: (userId: string) => Promise<void>,
): Promise<{
  succeeded: Array<{ userId: string; outcome: string }>;
  failed: Array<{ userId: string; error: string }>;
}> {
  const succeeded: Array<{ userId: string; outcome: string }> = [];
  const failed: Array<{ userId: string; error: string }> = [];
  for (const userId of userIds) {
    const state = states.find((item) => item.userId === userId);
    try {
      succeeded.push(await inviteCohost(
        eventId,
        userId,
        state,
        call,
        state?.status === 'stale' && repairStale ? () => repairStale(userId) : undefined,
      ));
    } catch (error) {
      failed.push({ userId, error: error instanceof Error ? error.message : String(error) });
    }
  }
  return { succeeded, failed };
}

export async function removeCohostCanonical(
  eventId: string,
  state: CohostState | undefined,
  call: CanonicalCohostCall,
  removeStale?: () => Promise<void>,
): Promise<{ userId: string; outcome: 'removed' }> {
  const action = planCohostRemoval(state);
  if (state?.status === 'stale') {
    if (!removeStale) throw new ValidationError('Stale co-host membership requires an explicit removal hook');
    await removeStale();
  } else {
    const endpoint = action === 'delete_request' ? '/deleteCohostRequest' : '/removeCohost';
    await call(endpoint, { eventId, targetUserId: state!.userId });
  }
  return { userId: state!.userId, outcome: 'removed' };
}

export function cohostPathToUrl(path: string): string {
  if (!/^\/e\/[^/?#]+\?[^#]*\baccept-cohost=[^&#]+/.test(path)) {
    throw new ValidationError('Invalid cohost invite-link path returned by Partiful', { path });
  }
  return `https://partiful.com${path}`;
}

interface CohostSecretDocument {
  fields?: { path?: { stringValue?: string } };
}

export interface CohostLinkState {
  enabled: boolean;
  url: string | null;
}

export function parseCohostLinkDocument(document: CohostSecretDocument | null): CohostLinkState {
  if (!document) return { enabled: false, url: null };
  const path = document.fields?.path?.stringValue;
  if (!path) throw new ValidationError('Invalid cohost invite-link document: path missing');
  return { enabled: true, url: cohostPathToUrl(path) };
}

export async function getCohostLink(eventId: string, token: string, verbose = false): Promise<CohostLinkState> {
  const document = await firestoreGetDocument(`events/${eventId}/private/cohostSecret`, token, verbose);
  return parseCohostLinkDocument(document as CohostSecretDocument | null);
}

export type CohostLinkRequest = 'inspect' | 'enable' | 'disable';
export type CohostLinkAction = 'inspect' | 'generate' | 'revoke' | 'noop';
export function planCohostLinkAction(requested: CohostLinkRequest, enabled: boolean): CohostLinkAction {
  if (requested === 'inspect') return 'inspect';
  if (requested === 'enable') return enabled ? 'noop' : 'generate';
  return enabled ? 'revoke' : 'noop';
}
