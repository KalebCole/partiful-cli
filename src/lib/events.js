/**
 * Shared event-building helpers.
 * Extracted from commands/events.js and commands/bulk.js to eliminate duplication.
 */

import readline from 'readline';
import { parseDateTime, stripMarkdown } from './dates.js';
import { NotFoundError, ValidationError } from './errors.js';

/**
 * Default guest status counts for new events.
 */
export const DEFAULT_GUEST_STATUS_COUNTS = {
  READY_TO_SEND: 0, SENDING: 0, SENT: 0, SEND_ERROR: 0,
  DELIVERY_ERROR: 0, INTERESTED: 0, MAYBE: 0, GOING: 0,
  DECLINED: 0, WAITLIST: 0, PENDING_APPROVAL: 0, APPROVED: 0,
  WITHDRAWN: 0, RESPONDED_TO_FIND_A_TIME: 0,
  WAITLISTED_FOR_APPROVAL: 0, REJECTED: 0,
};

/**
 * Prompt user for yes/no confirmation on stderr.
 */
export async function confirm(question) {
  const rl = readline.createInterface({ input: process.stdin, output: process.stderr });
  return new Promise(resolve => {
    rl.question(question + ' [y/N]: ', answer => {
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
export function isUrl(str) {
  return str && (str.startsWith('http://') || str.startsWith('https://'));
}

/**
 * Build a base event object for creation (used by create, clone, bulk).
 */
export function buildBaseEvent(opts) {
  const startDate = parseDateTime(opts.date, opts.timezone);
  const endDate = opts.endDate ? parseDateTime(opts.endDate, opts.timezone) : null;

  const event = {
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
export function buildLinks(linkUrls, linkTexts) {
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
export async function resolvePosterImage(opts, fetchCatalog, searchPosters, buildPosterImage) {
  if (!opts.poster && !opts.posterSearch) return null;

  const catalog = await fetchCatalog();

  if (opts.poster) {
    const poster = catalog.find(p => p.id === opts.poster);
    if (!poster) {
      throw new NotFoundError(`Poster not found: "${opts.poster}". Use "partiful posters search <term>" to find posters.`);
    }
    return buildPosterImage(poster);
  }

  const results = searchPosters(catalog, opts.posterSearch);
  if (results.length === 0) {
    throw new NotFoundError(`No posters found matching "${opts.posterSearch}". Try "partiful posters search <term>".`);
  }
  return buildPosterImage(results[0]);
}

/**
 * Handle image upload from file path or URL.
 * Returns image object for the event payload.
 */
export async function resolveUploadImage(imagePath, token, config, verbose, dryRun) {
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
export function validateImageOptions(...imageOpts) {
  const count = imageOpts.filter(Boolean).length;
  if (count > 1) {
    throw new ValidationError('Use only one of --poster, --poster-search, or --image.');
  }
  return count;
}

/**
 * Convert a plain JS object to Firestore field format (recursive).
 */
export function toFirestoreMap(obj) {
  const fields = {};
  for (const [key, value] of Object.entries(obj)) {
    if (value === null || value === undefined) continue;
    if (typeof value === 'string') fields[key] = { stringValue: value };
    else if (typeof value === 'number') {
      fields[key] = Number.isInteger(value)
        ? { integerValue: String(value) }
        : { doubleValue: value };
    }
    else if (typeof value === 'boolean') fields[key] = { booleanValue: value };
    else if (Array.isArray(value)) {
      fields[key] = { arrayValue: { values: value.map(v => {
        if (typeof v === 'string') return { stringValue: v };
        if (typeof v === 'number') {
          return Number.isInteger(v) ? { integerValue: String(v) } : { doubleValue: v };
        }
        if (typeof v === 'object') return { mapValue: { fields: toFirestoreMap(v) } };
        return { stringValue: String(v) };
      })}};
    }
    else if (typeof value === 'object') {
      fields[key] = { mapValue: { fields: toFirestoreMap(value) } };
    }
  }
  return fields;
}
