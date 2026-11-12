# Partiful CLI

This context separates what Partiful exposes remotely, what the CLI promises
to its users, and what the Go program implements.

## Language

**Remote API contract**:  
A reviewed, versioned description of the Partiful, Firebase, Firestore,
upload, and poster operations the CLI may call, including known request,
response, and failure behavior. It records verified remote facts; it does not
define CLI commands.

**CLI product contract**:  
The reviewed command, input, JSON output, failure, and mutation-safety behavior
that `partiful` promises to users and agents.

**Research evidence**:  
Information that can support or challenge remote facts but is not authoritative
until it is reviewed into the remote API contract.

**Implementation gate**:  
A remote-evidence condition that must be satisfied before a public command can
ship. A command does not use an implementation gate as a runtime fallback.

**Unavailable-not-claimed**:
A documented nullable product field that the current implementation
intentionally does not project because no reviewed remote mapping exists.
Null does not claim that the remote field is absent and is not protocol drift.

**Protocol change**:  
A difference between current Partiful behavior and the remote API contract
that a released command was built against.
_Avoid_: Remote unknown, API issue

**Dry-run preview**:
A no-effect description of one proposed remote change, including its normalized
request and any relevant preconditions. It does not authorize a later command.

**Consequential action**:  
A mutation that contacts a person, removes access, exposes access, or cancels
an event. Destructive actions require an interactive confirmation or `--force`.

**Implementation**:  
The Go code that is subordinate to the CLI product contract and remote API
contract. Implementation behavior does not silently redefine either contract.

**Cutover**:  
The point when the greenfield Go CLI replaces the TypeScript project and Node
and npm leave installation, runtime, development, and CI.
