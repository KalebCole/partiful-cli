# Vision

`partiful-cli` is an agent-friendly CLI for Partiful: every command speaks JSON, so agents and people can run Partiful event workflows from a terminal without guessing at a private API.
The same repo hosts the agent skills that teach agents to use it, packaged as a plugin.
Three ideas decide every change:

1. The OpenAPI spec (`spec/partiful.openapi.json`) is the source of truth for CLI commands. A command exists only for a call the spec defines.
2. The CLI follows agentic CLI principles (see "Agent-native by design" below).
3. The agent skills plugin stays in step with the commands, so an agent always learns the real surface.

## The catalog is the product

The public product is the command catalog returned by `partiful schema`.
A command ships only when its input, success, failure, and safety behavior are enforced by Go tests and reviewed schema snapshots.
The existence of a remote endpoint or a note in research files is not enough to add a command.
Public behavior changes update goldens deliberately; accidental golden drift is not a merge.
JSON is the default and promised output for people and agents; a second output dialect is a product change, not a formatting tweak.

## Evidence before shipping, fail closed

`spec/partiful.openapi.json` is the only authority for remote transport.
Every shipped call carries dated evidence; material under `docs/research` and `spec/research` stays research until it is reviewed into that spec.
When live Partiful behavior disagrees with the contract a released command was built against, the CLI fails closed as a protocol change.
It does not guess, retry, or paper over an unreviewed response.

## Mutations are reviewable and happen once

A mutation validates, performs its required reads, and dispatches at most once.
`--dry-run` previews a redacted request and does not authorize a later write.
Destructive or consequential actions prompt on a TTY unless `--force` is set, and they fail safely under `--no-input` without it.
An accepted remote request is not proof of delivery, stored state, or a side effect the contract does not claim.
Uncertain or malformed completion never triggers an automatic retry.

## Privacy and money stay outside the line the CLI will not cross

The CLI never prints phone numbers, email addresses, access tokens, or Partiful user IDs.
Tests and release checks never use live credentials or mutate real events.
Contacts may show display names and shared-event counts, nothing more.
The CLI never pays.
Chip-ins and suggested donations are handles Partiful displays; paying them belongs in the payment app.
Stripe ticket checkout is unsupported, and honor-system `ticketPaid` flags are not charges.

## Native Go, nothing else in the runtime

Installation, development, CI, and release are Go-only.
A change that brings Node or npm back into those paths is refused.
Auth login is human-attended.
Generic escape hatches, including raw HTTP, callable, Firebase, or Firestore access from the command line, are refused, because they bypass domain safeguards.

## Agent-native by design

Every command runs without prompts when there is no terminal, with one consistent flag for confirmation bypass.
Output is structured JSON by default.
Errors say what went wrong and list the valid values when an input is invalid.
Mutations are safe to retry, offer dry-run, use explicit flags for destructive actions, and return identifiers.
Responses are bounded by default, with explicit flags to widen them.
Command names follow common CLI vocabulary (get, list, create, update, delete), and help is discoverable in layers.
A change that breaks one of these is resisted; a change that improves one aligns.

## Scope

partiful-cli talks only to Partiful. It never calls Stripe or any other third-party system, even when Partiful itself uses one behind the scenes.
partiful-cli is not a Partiful app, not a payments product, and not a general Firebase or Firestore client.
It is not a template, clone, export, watch, or bulk-mutation orchestrator.
It is not an image generator, and it does not upload custom event images.
The repo ships its agent skills as a plugin that teaches agents to use the CLI; the skills are not the contributor guide.

A change aligns when it makes a reviewed workflow safer, more accurate, or more complete without widening the catalog past evidenced remote behavior, tightens fail-closed or privacy behavior, keeps mutations one-shot and previewable, or strengthens the Go-only release path.
A change should be resisted when it ships a command without reviewed product tests and remote evidence, reopens a generic transport hatch, processes a payment, exposes identity or contact PII, retries an uncertain write, adds persistence or multi-event orchestration beyond a single reviewed remote call, or trades the JSON contract for an untested convenience format.
