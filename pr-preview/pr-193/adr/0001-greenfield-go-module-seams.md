---
status: accepted
---

# Keep product orchestration behind one application seam

The greenfield Go CLI will use one Go module with `cmd/partiful` as composition
only and one deep internal application module as the interface used by the
executable and black-box tests. Command definitions, schema projection,
envelopes, diagnostics, and the event, guest, RSVP, contact, cohost, blast,
and poster capabilities stay behind that seam. Authentication, mutation
authority, and reviewed remote protocol handling are separate internal modules
because each owns durable state or a true external seam; callable, Firestore,
Firebase, and poster transports remain adapters private to the remote module.

We rejected packages that mirror commands or remote endpoints because their
interfaces would repeat the product and transport contracts while spreading
safety, privacy, and protocol-drift behavior across callers. We also rejected
one undifferentiated package because credential state, persistent single-use
plans, and external protocol validation need independent locality and
substitution in tests. Missing response evidence prevents a command definition
from entering a releasable catalog; it is never represented as a runtime
fallback.

The authentication seam supplies mutation authority with an opaque, private,
stable account fingerprint. It is stable across token refresh for one account
and changes when the authenticated account changes. Mutation authority never
derives identity from command input, and the fingerprint never enters public
output, diagnostics, or a remote request. The remote adapter receives the
bearer session separately.

Mutation authority owns persistent five-minute, single-use plan records. A
record binds the account fingerprint, command, normalized input, exact remote
request projection, and a digest of every pre-read fact. For RSVP, those facts
include the current-guest marker, ID, status, and count. They also include the
normalized event safeguard snapshot. The public plan redacts the guest ID,
account fingerprint, account ID, and user ID while the private record binds
the exact values.

Apply reacquires the fingerprint and performs the same event and current-guest
reads once. After comparison, it consumes the record immediately before
dispatch. It permits exactly one remote mutation attempt and never retries
automatically after an ambiguous completion. Any uncertain outcome requires a
new plan.

The remote seam distinguishes a callable protocol completion from verified
business state. For RSVP mutations, it validates only the reviewed HTTP
status, callable result envelope, and client-required completion fields. The
application can return the normalized submitted request, but it cannot claim
stored RSVP state, delivery, or another side effect without a separately
reviewed post-write read. The RSVP application returns only the minimal
submitted request projection: event ID, intent, and `submitted: true`. It
performs no post-write read. RSVP enters the releasable catalog only with the
approved dated object/null current-guest variants, event safeguard boundary,
and matching product and remote revision constants.

Event creation follows the same submitted-only boundary. Its standard plan
has no existing-event precondition. It binds the exact callable request and
the complete, digest-bound built-in poster record. The current create client
uses callable completion data as an event ID but does not validate or re-read
a complete Event, so the application returns only `submitted: true`.

Event update keeps the broad official Firestore Document grammar private to
the remote seam. The application owns a closed product-to-field projection
and never accepts a raw field path, update mask, typed value, or document
reference. Its standard plan binds current ownership, raw status, each target
field's presence/null/value state, and date safeguards when relevant. Apply
re-reads `getEventInfo` once before consuming the plan. A successful PATCH
Document is protocol completion only; the result contains the event ID,
sorted submitted product fields, and `submitted: true`.

Event cancellation uses a consequential plan. Its public form states the
exact event, message, guest-notification choice, and effects. Its private form
also binds ownership, status, start, guest-count facts, the exact callable
request, and the account fingerprint. Apply must re-read those facts and
receive the exact confirmation token. Callable completion returns only the
event ID, notification choice, and `submitted: true`; it does not claim
cancellation or notification delivery.
