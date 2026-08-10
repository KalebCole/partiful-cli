---
status: proposed
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
