# Remote API contract

`spec/partiful.openapi.json` is proposed revision `2026-08-10.1` of the remote
transport snapshot. It is pending explicit owner approval. It describes only
network operations and wire shapes. It does not prescribe commands, output,
credentials, mutation safeguards, or implementation architecture.

## Authority and change process

Live, privacy-safe observations outrank this proposed snapshot. The owner must
explicitly approve it before it becomes a reviewed contract. A proposed change
must update the contract and `spec/partiful.api-evidence.json`, then run
`npm test -- tests/remote-api-contract.test.js`. TypeScript, historical drafts,
tests, and endpoint notes are research evidence, never automatic authority.
The CLI product contract is separate and remains the authority for user-facing
behavior.

The evidence ledger records the classification and source for each operation,
each request/response claim, and each component schema. `explicit-unknown`
means the contract intentionally declines to claim precision. Never replace it
with guessed fields, captured credentials, real identifiers, or personal data.
`docs/research/2026-08-10-partiful-api-contract-evidence-ledger.md` is the
human-readable companion to the machine-readable ledger.

## Historical provenance

The 27-operation historical draft in commit
`17e9800753ada577408074bbbcadbae8cc8eacf0` was recovered as research input;
it was not copied as an approved contract. Its product extensions were
excluded. The nested `createTextBlast.message` object comes instead from the
dated observation recorded in
`docs/research/2026-03-24-text-blast-endpoint.md`; the historical string
message/`recipientStatuses` representation is superseded.
