/**
 * Shared Firebase-callable RPC request/response envelopes.
 *
 * Every firebase-callable endpoint (POST api.partiful.com) wraps its params in
 * the SAME shape. This is that shape, specified ONCE as a generic and referenced
 * per-endpoint — never re-inlined. See docs/TYPESCRIPT-PORT-GUIDE.md §5.
 */

/** The Firebase-callable RPC request envelope. `P` = the endpoint's params shape. */
export interface CallableEnvelope<P> {
  data: {
    params: P;
    /** Device fingerprint sent on every call. */
    amplitudeDeviceId: string;
    /** Present on identity-scoped calls (rsvp, cohosts, contacts). */
    amplitudeSessionId?: number;
    /** Backfilled from the Firebase JWT; may be null on legacy auth files. */
    userId?: string | null;
  };
}

/** Firebase-callable responses nest the real payload under `result.data`. */
export interface CallableResult<D> {
  result?: { data?: D };
}
