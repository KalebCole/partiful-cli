/**
 * THE API SPEC (types-as-spec).
 *
 * One entry per Partiful endpoint the lib layer calls. Each entry co-locates:
 *   - a fully-typed request interface (§3.5) referencing the shared envelope (§5),
 *   - a Zod `.passthrough()` response schema + `z.infer` type (§4),
 *   - an introspectable metadata record (`{ method, host, path, transport, ... }`)
 *     so T5's `schema api.<method>` can surface the spec from one place.
 *
 * Typing these IS authoring the API spec — there is no separate spec file to
 * drift. See docs/TYPESCRIPT-PORT-GUIDE.md.
 */

import { z } from 'zod';
import type { CallableEnvelope, CallableResult } from './envelope.js';

// ---------------------------------------------------------------------------
// Transport tags
// ---------------------------------------------------------------------------

export type Transport = 'firebase-callable' | 'firestore' | 'firebase-auth';

/** Metadata common to every spec'd endpoint (introspectable via `schema api.*`). */
export interface EndpointMeta {
  method: string;
  host: string;
  path: string;
  transport: Transport;
  /** Names of the request params the endpoint accepts (documentation surface). */
  requestParams: readonly string[];
  /** Known (non-exhaustive) response field names, derived from the Zod schema. */
  responseFields: readonly string[];
}

// ---------------------------------------------------------------------------
// Shared internal shapes (plain interfaces — we construct these, §3.4)
// ---------------------------------------------------------------------------

/** A single event link ({ url, text }). */
export interface EventLink {
  url: string;
  text: string;
}

/** Display settings on an event draft. */
export interface EventDisplaySettings {
  theme: string;
  effect: string;
  titleFont: string;
}

/**
 * The event object we POST to /createEvent. Built by buildBaseEvent() +
 * per-command additions; known fields are enumerated, unknown extension fields
 * are permitted (Partiful accepts a broad event shape).
 */
export interface EventDraft {
  title: string;
  startDate: string;
  endDate?: string;
  timezone: string;
  displaySettings: EventDisplaySettings;
  showHostList: boolean;
  showGuestCount: boolean;
  showGuestList: boolean;
  showActivityTimestamps: boolean;
  displayInviteButton: boolean;
  visibility: 'public' | 'private';
  allowGuestPhotoUpload: boolean;
  enableGuestReminders: boolean;
  rsvpsEnabled: boolean;
  allowGuestsToInviteMutuals: boolean;
  rsvpButtonGlyphType: string;
  status: string;
  guestStatusCounts: Record<string, number>;
  location?: string;
  address?: string;
  description?: string;
  guestLimit?: number;
  enableWaitlist?: boolean;
  links?: EventLink[];
  image?: unknown;
  [extra: string]: unknown;
}

/** RSVP params ride inside /addGuest under `rsvp`. */
export interface RsvpDraft {
  name: string;
  count: number;
  plusOnes: string[];
  message: string | null;
  emailInvitationId: string | null;
  status: string;
  guestId: string | null;
  timezone: string;
  password: string | null;
  questionnaireResponse?: {
    questionnaireVersion: number;
    answers: Record<string, string>;
  };
}

// ===========================================================================
// firebase-callable endpoints (POST api.partiful.com)
// ===========================================================================

const HOST_CALLABLE = 'api.partiful.com';

// --- createEvent -----------------------------------------------------------
export interface CreateEventParams {
  event: EventDraft;
  cohostIds: string[];
}
export type CreateEventRequest = CallableEnvelope<CreateEventParams>;
export const CreateEventResponseSchema = z
  .object({
    id: z.string(),
    title: z.string().optional(),
    status: z.string().optional(),
    startDate: z.string().optional(),
  })
  .passthrough();
export type CreateEventData = z.infer<typeof CreateEventResponseSchema>;
export type CreateEventResponse = CallableResult<CreateEventData>;

// --- cancelEvent -----------------------------------------------------------
export interface CancelEventParams {
  eventId: string;
}
export type CancelEventRequest = CallableEnvelope<CancelEventParams>;
export const CancelEventResponseSchema = z.object({}).passthrough();
export type CancelEventData = z.infer<typeof CancelEventResponseSchema>;
export type CancelEventResponse = CallableResult<CancelEventData>;

// --- getEventInfo ----------------------------------------------------------
export interface GetEventInfoParams {
  eventId: string;
}
export type GetEventInfoRequest = CallableEnvelope<GetEventInfoParams>;
export const GetEventInfoResponseSchema = z
  .object({
    id: z.string().optional(),
    title: z.string().optional(),
    startDate: z.string().optional(),
    endDate: z.string().nullable().optional(),
    location: z.string().nullable().optional(),
    status: z.string().optional(),
    ownerIds: z.array(z.string()).optional(),
    guestStatusCounts: z.record(z.string(), z.number()).optional(),
    links: z.array(z.object({ url: z.string(), text: z.string().optional() }).passthrough()).optional(),
  })
  .passthrough();
export type GetEventInfoData = z.infer<typeof GetEventInfoResponseSchema>;
export type GetEventInfoResponse = CallableResult<GetEventInfoData>;

// --- getContacts -----------------------------------------------------------
export interface GetContactsParams {
  // intentionally empty — server reads identity from the token/envelope
}
export type GetContactsRequest = CallableEnvelope<GetContactsParams>;
export const ContactSchema = z
  .object({
    userId: z.string().optional(),
    name: z.string().optional(),
    sharedEventCount: z.number().optional(),
  })
  .passthrough();
/** getContacts returns the contact array directly under result.data. */
export const GetContactsResponseSchema = z.array(ContactSchema);
export type GetContactsData = z.infer<typeof GetContactsResponseSchema>;
export type GetContactsResponse = CallableResult<GetContactsData>;

// --- createTextBlast -------------------------------------------------------
export interface CreateTextBlastParams {
  eventId: string;
  message: string;
  recipientStatuses?: string[];
}
export type CreateTextBlastRequest = CallableEnvelope<CreateTextBlastParams>;
export const CreateTextBlastResponseSchema = z.object({}).passthrough();
export type CreateTextBlastData = z.infer<typeof CreateTextBlastResponseSchema>;
export type CreateTextBlastResponse = CallableResult<CreateTextBlastData>;

// --- addInvitedGuestsAsHost ------------------------------------------------
export interface AddInvitedGuestsAsHostParams {
  eventId: string;
  guests: Array<Record<string, unknown>>;
}
export type AddInvitedGuestsAsHostRequest = CallableEnvelope<AddInvitedGuestsAsHostParams>;
export const AddInvitedGuestsAsHostResponseSchema = z.object({}).passthrough();
export type AddInvitedGuestsAsHostData = z.infer<typeof AddInvitedGuestsAsHostResponseSchema>;
export type AddInvitedGuestsAsHostResponse = CallableResult<AddInvitedGuestsAsHostData>;

// --- canonical cohost lifecycle -------------------------------------------
export interface CohostRequestParams {
  eventId: string;
  targetUserId: string;
}
export type CreateCohostRequestRequest = CallableEnvelope<CohostRequestParams>;
export const CreateCohostRequestResponseSchema = z.object({}).passthrough();
export type CreateCohostRequestData = z.infer<typeof CreateCohostRequestResponseSchema>;
export type CreateCohostRequestResponse = CallableResult<CreateCohostRequestData>;

export type DeleteCohostRequestRequest = CallableEnvelope<CohostRequestParams>;
export const DeleteCohostRequestResponseSchema = z.object({}).passthrough();
export type DeleteCohostRequestData = z.infer<typeof DeleteCohostRequestResponseSchema>;
export type DeleteCohostRequestResponse = CallableResult<DeleteCohostRequestData>;

export type RemoveCohostRequest = CallableEnvelope<CohostRequestParams>;
export const RemoveCohostResponseSchema = z.object({}).passthrough();
export type RemoveCohostData = z.infer<typeof RemoveCohostResponseSchema>;
export type RemoveCohostResponse = CallableResult<RemoveCohostData>;

export interface EventCohostLinkParams {
  eventId: string;
}
export type GenerateEventCohostLinkRequest = CallableEnvelope<EventCohostLinkParams>;
export const GenerateEventCohostLinkResponseSchema = z.object({ path: z.string().optional() }).passthrough();
export type GenerateEventCohostLinkData = z.infer<typeof GenerateEventCohostLinkResponseSchema>;
export type GenerateEventCohostLinkResponse = CallableResult<GenerateEventCohostLinkData>;

export type RevokeEventCohostLinkRequest = CallableEnvelope<EventCohostLinkParams>;
export const RevokeEventCohostLinkResponseSchema = z.object({}).passthrough();
export type RevokeEventCohostLinkData = z.infer<typeof RevokeEventCohostLinkResponseSchema>;
export type RevokeEventCohostLinkResponse = CallableResult<RevokeEventCohostLinkData>;

// --- getMyUpcomingEventsForHomePage ----------------------------------------
export interface GetMyUpcomingEventsParams {
  // empty params
}
export type GetMyUpcomingEventsRequest = CallableEnvelope<GetMyUpcomingEventsParams>;
export const HomePageEventSchema = z
  .object({
    id: z.string(),
    title: z.string().optional(),
    startDate: z.string().optional(),
    endDate: z.string().nullable().optional(),
    location: z.string().nullable().optional(),
    status: z.string().optional(),
    ownerIds: z.array(z.string()).optional(),
    guest: z.object({ status: z.string().optional() }).passthrough().nullable().optional(),
    guestStatusCounts: z.record(z.string(), z.number()).optional(),
  })
  .passthrough();
/** Home-page endpoints return an event array under result.data. */
export const GetMyUpcomingEventsResponseSchema = z.array(HomePageEventSchema);
export type GetMyUpcomingEventsData = z.infer<typeof GetMyUpcomingEventsResponseSchema>;
export type GetMyUpcomingEventsResponse = CallableResult<GetMyUpcomingEventsData>;

// --- getMyPastEventsForHomePage --------------------------------------------
export interface GetMyPastEventsParams {
  // empty params
}
export type GetMyPastEventsRequest = CallableEnvelope<GetMyPastEventsParams>;
export const GetMyPastEventsResponseSchema = z.array(HomePageEventSchema);
export type GetMyPastEventsData = z.infer<typeof GetMyPastEventsResponseSchema>;
export type GetMyPastEventsResponse = CallableResult<GetMyPastEventsData>;

// --- addGuest (self-RSVP) --------------------------------------------------
export interface AddGuestParams {
  eventId: string;
  rsvp: RsvpDraft;
}
export type AddGuestRequest = CallableEnvelope<AddGuestParams>;
export const AddGuestResponseSchema = z
  .object({
    guestId: z.string().optional(),
    status: z.string().optional(),
  })
  .passthrough();
export type AddGuestData = z.infer<typeof AddGuestResponseSchema>;
export type AddGuestResponse = CallableResult<AddGuestData>;

// --- markEventInterest -----------------------------------------------------
export interface MarkEventInterestParams {
  eventId: string;
  interested: boolean;
  source: string;
}
export type MarkEventInterestRequest = CallableEnvelope<MarkEventInterestParams>;
export const MarkEventInterestResponseSchema = z.object({}).passthrough();
export type MarkEventInterestData = z.infer<typeof MarkEventInterestResponseSchema>;
export type MarkEventInterestResponse = CallableResult<MarkEventInterestData>;

// --- getCurrentGuest -------------------------------------------------------
export interface GetCurrentGuestParams {
  eventId: string;
}
export type GetCurrentGuestRequest = CallableEnvelope<GetCurrentGuestParams>;
export const CurrentGuestSchema = z
  .object({
    id: z.string().optional(),
    status: z.string().optional(),
    name: z.string().optional(),
  })
  .passthrough();
export const GetCurrentGuestResponseSchema = z
  .object({
    currentGuest: CurrentGuestSchema.nullable().optional(),
  })
  .passthrough();
export type GetCurrentGuestData = z.infer<typeof GetCurrentGuestResponseSchema>;
export type GetCurrentGuestResponse = CallableResult<GetCurrentGuestData>;

// ===========================================================================
// firestore endpoints (GET / PATCH firestore.googleapis.com, doc format)
// ===========================================================================

const HOST_FIRESTORE = 'firestore.googleapis.com';

/** A Firestore document in the typed-value format Partiful returns. */
export const FirestoreDocumentSchema = z
  .object({
    name: z.string().optional(),
    fields: z.record(z.string(), z.unknown()).optional(),
    createTime: z.string().optional(),
    updateTime: z.string().optional(),
  })
  .passthrough();
export type FirestoreDocument = z.infer<typeof FirestoreDocumentSchema>;

/** firestore list response ({ documents, nextPageToken }). */
export const FirestoreListResponseSchema = z
  .object({
    documents: z.array(FirestoreDocumentSchema).optional(),
    nextPageToken: z.string().optional(),
  })
  .passthrough();
export type FirestoreListResponse = z.infer<typeof FirestoreListResponseSchema>;

// ===========================================================================
// firebase-auth endpoint (POST securetoken.googleapis.com)
// ===========================================================================

const HOST_AUTH = 'securetoken.googleapis.com';

export interface RefreshTokenRequest {
  grant_type: 'refresh_token';
  refresh_token: string;
}
export const RefreshTokenResponseSchema = z
  .object({
    id_token: z.string().optional(),
    refresh_token: z.string().optional(),
    expires_in: z.string().optional(),
    user_id: z.string().optional(),
    error: z.object({ message: z.string().optional() }).passthrough().optional(),
  })
  .passthrough();
export type RefreshTokenResponse = z.infer<typeof RefreshTokenResponseSchema>;

// ===========================================================================
// Introspectable endpoint registry — surfaced by `schema api.<method>` (T5)
// ===========================================================================

function fieldsOf(schema: z.ZodTypeAny): readonly string[] {
  // Best-effort: enumerate object keys for object schemas; arrays expose their
  // element's keys; everything else has no enumerable field surface.
  const def = schema as unknown as { shape?: Record<string, unknown> };
  if (def.shape && typeof def.shape === 'object') return Object.keys(def.shape);
  return [];
}

export const apiEndpoints = {
  createEvent: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/createEvent',
    transport: 'firebase-callable',
    requestParams: ['event', 'cohostIds'],
    responseFields: fieldsOf(CreateEventResponseSchema),
  },
  cancelEvent: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/cancelEvent',
    transport: 'firebase-callable',
    requestParams: ['eventId'],
    responseFields: fieldsOf(CancelEventResponseSchema),
  },
  getEventInfo: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/getEventInfo',
    transport: 'firebase-callable',
    requestParams: ['eventId'],
    responseFields: fieldsOf(GetEventInfoResponseSchema),
  },
  getContacts: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/getContacts',
    transport: 'firebase-callable',
    requestParams: [],
    responseFields: fieldsOf(ContactSchema),
  },
  createTextBlast: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/createTextBlast',
    transport: 'firebase-callable',
    requestParams: ['eventId', 'message', 'recipientStatuses'],
    responseFields: fieldsOf(CreateTextBlastResponseSchema),
  },
  addInvitedGuestsAsHost: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/addInvitedGuestsAsHost',
    transport: 'firebase-callable',
    requestParams: ['eventId', 'guests'],
    responseFields: fieldsOf(AddInvitedGuestsAsHostResponseSchema),
  },
  createCohostRequest: {
    method: 'POST', host: HOST_CALLABLE, path: '/createCohostRequest', transport: 'firebase-callable',
    requestParams: ['eventId', 'targetUserId'], responseFields: fieldsOf(CreateCohostRequestResponseSchema),
  },
  deleteCohostRequest: {
    method: 'POST', host: HOST_CALLABLE, path: '/deleteCohostRequest', transport: 'firebase-callable',
    requestParams: ['eventId', 'targetUserId'], responseFields: fieldsOf(DeleteCohostRequestResponseSchema),
  },
  removeCohost: {
    method: 'POST', host: HOST_CALLABLE, path: '/removeCohost', transport: 'firebase-callable',
    requestParams: ['eventId', 'targetUserId'], responseFields: fieldsOf(RemoveCohostResponseSchema),
  },
  generateEventCohostLink: {
    method: 'POST', host: HOST_CALLABLE, path: '/generateEventCohostLink', transport: 'firebase-callable',
    requestParams: ['eventId'], responseFields: fieldsOf(GenerateEventCohostLinkResponseSchema),
  },
  revokeEventCohostLink: {
    method: 'POST', host: HOST_CALLABLE, path: '/revokeEventCohostLink', transport: 'firebase-callable',
    requestParams: ['eventId'], responseFields: fieldsOf(RevokeEventCohostLinkResponseSchema),
  },
  getMyUpcomingEventsForHomePage: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/getMyUpcomingEventsForHomePage',
    transport: 'firebase-callable',
    requestParams: [],
    responseFields: fieldsOf(HomePageEventSchema),
  },
  getMyPastEventsForHomePage: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/getMyPastEventsForHomePage',
    transport: 'firebase-callable',
    requestParams: [],
    responseFields: fieldsOf(HomePageEventSchema),
  },
  addGuest: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/addGuest',
    transport: 'firebase-callable',
    requestParams: ['eventId', 'rsvp'],
    responseFields: fieldsOf(AddGuestResponseSchema),
  },
  markEventInterest: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/markEventInterest',
    transport: 'firebase-callable',
    requestParams: ['eventId', 'interested', 'source'],
    responseFields: fieldsOf(MarkEventInterestResponseSchema),
  },
  getCurrentGuest: {
    method: 'POST',
    host: HOST_CALLABLE,
    path: '/getCurrentGuest',
    transport: 'firebase-callable',
    requestParams: ['eventId'],
    responseFields: fieldsOf(GetCurrentGuestResponseSchema),
  },
  firestoreGetEvent: {
    method: 'GET',
    host: HOST_FIRESTORE,
    path: '/v1/projects/getpartiful/databases/(default)/documents/events/{eventId}',
    transport: 'firestore',
    requestParams: ['eventId'],
    responseFields: fieldsOf(FirestoreDocumentSchema),
  },
  firestoreGetGuest: {
    method: 'GET',
    host: HOST_FIRESTORE,
    path: '/v1/projects/getpartiful/databases/(default)/documents/events/{eventId}/guests/{guestId}',
    transport: 'firestore',
    requestParams: ['eventId', 'guestId'],
    responseFields: fieldsOf(FirestoreDocumentSchema),
  },
  firestorePatchEvent: {
    method: 'PATCH',
    host: HOST_FIRESTORE,
    path: '/v1/projects/getpartiful/databases/(default)/documents/events/{eventId}',
    transport: 'firestore',
    requestParams: ['eventId', 'fields', 'updateMask.fieldPaths'],
    responseFields: fieldsOf(FirestoreDocumentSchema),
  },
  firestoreListDocuments: {
    method: 'GET',
    host: HOST_FIRESTORE,
    path: '/v1/projects/getpartiful/databases/(default)/documents/{collectionPath}',
    transport: 'firestore',
    requestParams: ['collectionPath', 'pageSize', 'pageToken'],
    responseFields: fieldsOf(FirestoreListResponseSchema),
  },
  refreshToken: {
    method: 'POST',
    host: HOST_AUTH,
    path: '/v1/token',
    transport: 'firebase-auth',
    requestParams: ['grant_type', 'refresh_token'],
    responseFields: fieldsOf(RefreshTokenResponseSchema),
  },
} as const satisfies Record<string, EndpointMeta>;

export type ApiMethod = keyof typeof apiEndpoints;

// ===========================================================================
// Response-schema registry — consumed by the drift detector (T6).
// Maps each spec'd method to its Zod `.passthrough()` response schema so raw
// API responses can be diffed against the known field surface at parse time.
// Methods whose real response is an array expose the element schema (the unit
// whose keys we compare); firebase-auth/refreshToken uses its object schema.
// ===========================================================================

export const responseSchemas = {
  createEvent: CreateEventResponseSchema,
  cancelEvent: CancelEventResponseSchema,
  getEventInfo: GetEventInfoResponseSchema,
  getContacts: ContactSchema,
  createTextBlast: CreateTextBlastResponseSchema,
  addInvitedGuestsAsHost: AddInvitedGuestsAsHostResponseSchema,
  createCohostRequest: CreateCohostRequestResponseSchema,
  deleteCohostRequest: DeleteCohostRequestResponseSchema,
  removeCohost: RemoveCohostResponseSchema,
  generateEventCohostLink: GenerateEventCohostLinkResponseSchema,
  revokeEventCohostLink: RevokeEventCohostLinkResponseSchema,
  getMyUpcomingEventsForHomePage: HomePageEventSchema,
  getMyPastEventsForHomePage: HomePageEventSchema,
  addGuest: AddGuestResponseSchema,
  markEventInterest: MarkEventInterestResponseSchema,
  getCurrentGuest: GetCurrentGuestResponseSchema,
  firestoreGetEvent: FirestoreDocumentSchema,
  firestoreGetGuest: FirestoreDocumentSchema,
  firestorePatchEvent: FirestoreDocumentSchema,
  firestoreListDocuments: FirestoreListResponseSchema,
  refreshToken: RefreshTokenResponseSchema,
} as const satisfies Record<ApiMethod, z.ZodTypeAny>;
