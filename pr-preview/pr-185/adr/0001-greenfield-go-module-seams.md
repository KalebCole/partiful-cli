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
request projection, and a digest of every pre-read fact. Apply reacquires the
fingerprint and preconditions before consuming the record. After comparison,
it consumes the record immediately before dispatch. It permits exactly one
remote mutation attempt and never retries automatically after an ambiguous
completion; any uncertain transport outcome requires a new plan.

For RSVP, mutation authority binds only the current-guest identity and status,
or an explicit no-current-guest marker. It does not widen the event-read seam
to obtain party limits, questionnaire versions, password state, ticketing
state, or other unreviewed event preconditions.

The remote seam distinguishes a callable protocol completion from verified
business state. For RSVP mutations, it validates only the reviewed HTTP
status, callable result envelope, and client-required completion fields. The
application can return only the product-permitted submitted projection; it
cannot claim stored RSVP state, delivery, or another side effect without a
separately reviewed post-write read. The proposed RSVP result uses only
`eventId`, intent, and `submitted: true`; no post-write read is performed.
Current-client null-safe selection can distinguish a present guest from the
explicit absent marker without claiming that a null remote response was
observed. The command remains outside the releasable catalog until delegated
review approves that selection and submitted-request boundary.
