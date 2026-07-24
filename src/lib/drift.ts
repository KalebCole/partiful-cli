/**
 * Drift detection (T6).
 *
 * Partiful is an unofficial API — the vendor changes response shapes without
 * notice. Every response schema in api/endpoints.ts is authored with Zod
 * `.passthrough()`, so unknown fields survive parsing instead of being
 * stripped. This module diffs a raw response against the known field surface
 * of its schema and reports any fields the spec does NOT yet declare. Over
 * time, real traffic reveals the vendor's true shape and keeps types-as-spec
 * honest.
 *
 * OPT-IN: logging is silent unless `PARTIFUL_DRIFT_LOG` is set (or verbose is
 * passed explicitly). Set `PARTIFUL_DRIFT_LOG=1` to log to stderr, or
 * `PARTIFUL_DRIFT_LOG=<path>` to append NDJSON records to a file. This keeps
 * the default JSON-on-stdout contract clean for agent consumers.
 */

import { appendFileSync } from 'fs';
import { z } from 'zod';
import { responseSchemas, type ApiMethod } from './api/endpoints.js';

/** A single drift observation: fields present in the response but not in the spec. */
export interface DriftRecord {
  method: ApiMethod;
  unknownFields: string[];
  observedAt: string;
}

/**
 * Enumerate the declared (known) top-level keys of a Zod schema. Resolves
 * through an array wrapper (arrays expose their element's keys) so the diff
 * compares against a single record's shape. Non-object/array schemas yield an
 * empty set (no field surface → no drift reported).
 */
function knownKeys(schema: z.ZodTypeAny): Set<string> {
  let s: unknown = schema;
  // Unwrap array element schemas so we compare a single record's keys.
  const def = s as { element?: unknown; shape?: Record<string, unknown> };
  if (def.element) s = def.element as z.ZodTypeAny;
  const inner = s as { shape?: Record<string, unknown> };
  if (inner.shape && typeof inner.shape === 'object') return new Set(Object.keys(inner.shape));
  return new Set();
}

/**
 * A few endpoints nest their real payload one level deeper than the callable
 * envelope's `result.data`, so the schema in `responseSchemas` describes that
 * inner value, not `data` itself. Map those methods to the sub-key the drift
 * checker must descend into before diffing; methods absent from this map are
 * diffed at `result.data` directly. Keeping this here (next to the schemas it
 * pairs with) means the http wiring stays a dumb, uniform caller.
 */
const PAYLOAD_UNWRAP: Partial<Record<ApiMethod, string>> = {
  getEventInfo: 'event', // result.data.event
  getMyUpcomingEventsForHomePage: 'upcomingEvents', // result.data.upcomingEvents[]
  getMyPastEventsForHomePage: 'pastEvents', // result.data.pastEvents[]
};

/**
 * Given the already-unwrapped `result.data` for a method, descend into the
 * method-specific sub-key when one is declared (returning that inner value),
 * otherwise return `data` unchanged. Never throws — a missing/oddly-shaped
 * key falls back to the original value so drift detection stays advisory.
 */
export function unwrapPayload(method: ApiMethod, data: unknown): unknown {
  const key = PAYLOAD_UNWRAP[method];
  if (!key) return data;
  if (data && typeof data === 'object' && !Array.isArray(data) && key in (data as Record<string, unknown>)) {
    return (data as Record<string, unknown>)[key];
  }
  return data;
}

/**
 * Return the top-level keys of an observed response record. Array responses
 * are reduced to the union of keys across their elements (objects only).
 */
function observedKeys(value: unknown): Set<string> {
  const keys = new Set<string>();
  const records = Array.isArray(value) ? value : [value];
  for (const rec of records) {
    if (rec && typeof rec === 'object' && !Array.isArray(rec)) {
      for (const k of Object.keys(rec as Record<string, unknown>)) keys.add(k);
    }
  }
  return keys;
}

/**
 * Compare a raw response against the spec's schema for `method`. Returns the
 * set of fields present in the response but absent from the schema, or an
 * empty array when the response conforms (or the method has no schema).
 */
export function detectDrift(method: ApiMethod, response: unknown): string[] {
  const schema = responseSchemas[method];
  if (!schema) return [];
  const known = knownKeys(schema);
  if (known.size === 0) return []; // schema declares no fields to diff against
  const observed = observedKeys(response);
  return [...observed].filter((k) => !known.has(k)).sort();
}

/**
 * Detect drift for `method` and, if any unknown fields are found AND drift
 * logging is enabled, emit a record. Enabled when `PARTIFUL_DRIFT_LOG` is set
 * or `force` is true. `PARTIFUL_DRIFT_LOG=1|true|stderr` → stderr; any other
 * value is treated as a file path and NDJSON records are appended to it.
 *
 * Always returns the DriftRecord when drift is found (even if logging is off),
 * so callers/tests can assert on it; returns null when the response conforms.
 * Never throws — drift detection must never break a real request.
 */
export function reportDrift(method: ApiMethod, response: unknown, force = false): DriftRecord | null {
  let unknownFields: string[];
  try {
    unknownFields = detectDrift(method, response);
  } catch {
    return null;
  }
  if (unknownFields.length === 0) return null;

  const record: DriftRecord = { method, unknownFields, observedAt: new Date().toISOString() };

  const sink = process.env.PARTIFUL_DRIFT_LOG;
  const enabled = force || (sink != null && sink !== '' && sink !== '0' && sink !== 'false');
  if (enabled) {
    const line = JSON.stringify({ drift: record });
    if (!sink || sink === '1' || sink === 'true' || sink === 'stderr' || force) {
      console.error(`[drift] ${method}: unknown fields ${unknownFields.join(', ')}`);
    } else {
      try {
        appendFileSync(sink, line + '\n');
      } catch {
        console.error(`[drift] ${method}: unknown fields ${unknownFields.join(', ')} (log write failed)`);
      }
    }
  }
  return record;
}
