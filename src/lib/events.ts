/**
 * Shared event-building helpers.
 * Extracted from commands/events.js and commands/bulk.js to eliminate duplication.
 */

import readline from 'readline';
import { parseDateTime, stripMarkdown } from './dates.js';
import { NotFoundError, ValidationError } from './errors.js';
import type { EventDraft, EventLink } from './api/endpoints.js';
import type { PartifulConfig } from './auth.js';
import type { Poster, PosterImage } from './posters.js';

/**
 * Default guest status counts for new events.
 */
export const DEFAULT_GUEST_STATUS_COUNTS: Record<string, number> = {
  READY_TO_SEND: 0, SENDING: 0, SENT: 0, SEND_ERROR: 0,
  DELIVERY_ERROR: 0, INTERESTED: 0, MAYBE: 0, GOING: 0,
  DECLINED: 0, WAITLIST: 0, PENDING_APPROVAL: 0, APPROVED: 0,
  WITHDRAWN: 0, RESPONDED_TO_FIND_A_TIME: 0,
  WAITLISTED_FOR_APPROVAL: 0, REJECTED: 0,
};

/**
 * Prompt user for yes/no confirmation on stderr.
 */
export async function confirm(question: string): Promise<boolean> {
  const rl = readline.createInterface({ input: process.stdin, output: process.stderr });
  return new Promise((resolve) => {
    rl.question(question + ' [y/N]: ', (answer) => {
      rl.close();
      resolve(answer.toLowerCase() === 'y' || answer.toLowerCase() === 'yes');
    });
  });
}

/**
 * Allowed image extensions for upload.
 */
export const ALLOWED_IMAGE_EXTENSIONS = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.avif'];

/**
 * Check if a string is an HTTP(S) URL.
 */
export function isUrl(str: string): boolean {
  return !!str && (str.startsWith('http://') || str.startsWith('https://'));
}

/** CLI options accepted by buildBaseEvent() and friends. */
export interface EventOptions {
  title: string;
  date: string;
  endDate?: string;
  timezone?: string;
  theme?: string;
  effect?: string;
  titleFont?: string;
  private?: boolean;
  location?: string;
  address?: string;
  description?: string;
  capacity?: number;
  poster?: string;
  posterSearch?: string;
  [extra: string]: unknown;
}

/** The return shape of buildBaseEvent(). */
export interface BuiltBaseEvent {
  event: EventDraft;
  startDate: Date;
  endDate: Date | null;
}

/**
 * Build a base event object for creation (used by create, clone, bulk).
 */
export function buildBaseEvent(opts: EventOptions): BuiltBaseEvent {
  const startDate = parseDateTime(opts.date, opts.timezone);
  const endDate = opts.endDate ? parseDateTime(opts.endDate, opts.timezone) : null;

  const event: EventDraft = {
    title: opts.title,
    startDate: startDate.toISOString(),
    timezone: opts.timezone || 'America/Los_Angeles',
    displaySettings: {
      theme: opts.theme || 'oxblood',
      effect: opts.effect || 'sunbeams',
      titleFont: opts.titleFont || 'display',
    },
    showHostList: true,
    showGuestCount: true,
    showGuestList: true,
    showActivityTimestamps: true,
    displayInviteButton: true,
    visibility: opts.private ? 'private' : 'public',
    allowGuestPhotoUpload: true,
    enableGuestReminders: true,
    rsvpsEnabled: true,
    allowGuestsToInviteMutuals: true,
    rsvpButtonGlyphType: 'emojis',
    status: 'UNSAVED',
    guestStatusCounts: { ...DEFAULT_GUEST_STATUS_COUNTS },
  };

  if (endDate) event.endDate = endDate.toISOString();
  if (opts.location) event.location = opts.location;
  if (opts.address) event.address = opts.address;
  if (opts.description) event.description = stripMarkdown(opts.description);
  if (opts.capacity) {
    event.guestLimit = opts.capacity;
    event.enableWaitlist = true;
  }

  return { event, startDate, endDate };
}

/**
 * Build links array from CLI options.
 */
export function buildLinks(
  linkUrls: string[] | undefined,
  linkTexts: string[] | undefined,
): EventLink[] | null {
  if (!linkUrls || linkUrls.length === 0) return null;
  return linkUrls.map((url, i) => ({
    url,
    text: linkTexts?.[i] || url,
  }));
}

/**
 * Resolve poster image from --poster or --poster-search options.
 * Returns image object or null. Throws on not-found.
 */
export async function resolvePosterImage(
  opts: { poster?: string; posterSearch?: string },
  fetchCatalog: () => Promise<Poster[]>,
  searchPosters: (catalog: Poster[], query: string) => Poster[],
  buildPosterImage: (poster: Poster) => PosterImage,
): Promise<PosterImage | null> {
  if (!opts.poster && !opts.posterSearch) return null;

  const catalog = await fetchCatalog();

  if (opts.poster) {
    const poster = catalog.find((p) => p.id === opts.poster);
    if (!poster) {
      throw new NotFoundError(`Poster not found: "${opts.poster}". Use "partiful posters search <term>" to find posters.`);
    }
    return buildPosterImage(poster);
  }

  const results = searchPosters(catalog, opts.posterSearch!);
  if (results.length === 0) {
    throw new NotFoundError(`No posters found matching "${opts.posterSearch}". Try "partiful posters search <term>".`);
  }
  return buildPosterImage(results[0]!);
}

/**
 * Handle image upload from file path or URL.
 * Returns image object for the event payload.
 */
export async function resolveUploadImage(
  imagePath: string,
  token: string,
  config: PartifulConfig,
  verbose: boolean | undefined,
  dryRun: boolean | undefined,
): Promise<Record<string, unknown> | import('./upload.js').UploadImage> {
  const imageIsUrl = isUrl(imagePath);

  if (!imageIsUrl) {
    const { extname } = await import('path');
    const ext = extname(imagePath).toLowerCase();
    if (!ALLOWED_IMAGE_EXTENSIONS.includes(ext)) {
      throw new Error(`Unsupported image type "${ext}". Allowed types: ${ALLOWED_IMAGE_EXTENSIONS.join(', ')}`);
    }
  }

  if (dryRun) {
    return imageIsUrl
      ? { source: 'upload', url: imagePath, note: 'URL will be downloaded and uploaded on real run' }
      : { source: 'upload', file: imagePath, note: 'File will be uploaded on real run' };
  }

  const { basename } = await import('path');

  if (imageIsUrl) {
    const { downloadToTemp, uploadEventImage, buildUploadImage } = await import('./upload.js');
    const { tempPath, cleanup } = await downloadToTemp(imagePath);
    try {
      const uploadData = await uploadEventImage(tempPath, token, config, verbose);
      return buildUploadImage(uploadData, basename(tempPath));
    } finally {
      cleanup();
    }
  }

  const { uploadEventImage, buildUploadImage } = await import('./upload.js');
  const uploadData = await uploadEventImage(imagePath, token, config, verbose);
  return buildUploadImage(uploadData, basename(imagePath));
}

/**
 * Validate that at most one image option is set.
 * Returns the count of image options provided.
 */
export function validateImageOptions(...imageOpts: unknown[]): number {
  const count = imageOpts.filter(Boolean).length;
  if (count > 1) {
    throw new ValidationError('Use only one of --poster, --poster-search, or --image.');
  }
  return count;
}

/**
 * Canonical public URL for an event's Partiful page.
 * Single source of truth for the `partiful.com/e/<id>` format.
 */
export function buildEventUrl(id: string): string {
  return `https://partiful.com/e/${id}`;
}

/** A raw home-page event object as returned by the list endpoints. */
export interface RawHomePageEvent {
  id: string;
  title?: string;
  startDate?: string;
  endDate?: string | null;
  location?: string | null;
  status?: string;
  ownerIds?: string[];
  guest?: { status?: string } | null;
  guestStatusCounts?: Record<string, number>;
  [extra: string]: unknown;
}

/** The compact summary shape returned by `events list`. */
export interface EventSummary {
  id: string;
  title?: string;
  startDate?: string;
  endDate: string | null;
  location: string | null;
  status?: string;
  isHost: boolean;
  myRsvp: string | null;
  going: number;
  maybe: number;
  url: string;
}

/**
 * Map a raw event object from the Partiful home-page endpoints into the compact
 * summary shape returned by `events list`. Pure function — no I/O.
 */
export function mapEventSummary(e: RawHomePageEvent, me: string | null): EventSummary {
  return {
    id: e.id,
    title: e.title,
    startDate: e.startDate,
    endDate: e.endDate || null,
    location: e.location || null,
    status: e.status,
    isHost: (me != null && e.ownerIds?.includes(me)) || false,
    myRsvp: e.guest?.status ?? null,
    going: e.guestStatusCounts?.GOING || 0,
    maybe: e.guestStatusCounts?.MAYBE || 0,
    url: buildEventUrl(e.id),
  };
}

/** A Firestore typed-value field. */
export type FirestoreValue =
  | { stringValue: string }
  | { integerValue: string }
  | { doubleValue: number }
  | { booleanValue: boolean }
  | { arrayValue: { values: FirestoreValue[] } }
  | { mapValue: { fields: Record<string, FirestoreValue> } };

/**
 * Convert a plain JS object to Firestore field format (recursive).
 */
export function toFirestoreMap(obj: Record<string, unknown>): Record<string, FirestoreValue> {
  const fields: Record<string, FirestoreValue> = {};
  for (const [key, value] of Object.entries(obj)) {
    if (value === null || value === undefined) continue;
    if (typeof value === 'string') fields[key] = { stringValue: value };
    else if (typeof value === 'number') {
      fields[key] = Number.isInteger(value)
        ? { integerValue: String(value) }
        : { doubleValue: value };
    } else if (typeof value === 'boolean') fields[key] = { booleanValue: value };
    else if (Array.isArray(value)) {
      fields[key] = {
        arrayValue: {
          values: value.map((v): FirestoreValue => {
            if (typeof v === 'string') return { stringValue: v };
            if (typeof v === 'number') {
              return Number.isInteger(v) ? { integerValue: String(v) } : { doubleValue: v };
            }
            if (typeof v === 'object' && v !== null)
              return { mapValue: { fields: toFirestoreMap(v as Record<string, unknown>) } };
            return { stringValue: String(v) };
          }),
        },
      };
    } else if (typeof value === 'object') {
      fields[key] = { mapValue: { fields: toFirestoreMap(value as Record<string, unknown>) } };
    }
  }
  return fields;
}
