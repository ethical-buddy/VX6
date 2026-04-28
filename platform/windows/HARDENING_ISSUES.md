# VX6 Hardening And Contributor Issues

This file is written so it can be posted or adapted as a public contributor
backlog and issue source.

The goal is to attract contributors to concrete security, resilience, and
cross-platform work, not vague “help wanted” items.

For a maintainer-facing diagnosis and fix-plan, also see:

- [docs/current-limitations-and-fix-plan.md](/home/suryansh/Projects/test/docs/current-limitations-and-fix-plan.md:1)

## Project State

VX6 already has:

- direct encrypted service access
- relay mode
- hidden rendezvous mode
- encrypted onion cells
- layered relay hop processing
- replay-protected hidden control
- benchmark coverage
- integration coverage

What remains is mainly hardening and production maturity.

## How To Use This File

You can use this document in three ways:

1. copy one issue block into a GitHub issue directly
2. split issues into milestones:
   - docs/trust
   - hidden resilience
   - abuse resistance
   - Windows/macOS runtime
   - performance
3. label newcomer-safe work using the `Beginner First Issue` section at the end

Each issue below includes:

- a suggested title
- why it matters
- proposed solution
- acceptance criteria
- suggested labels

## Priority 0: Honest Status And Trust Documentation

### Issue 1: Clarify eBPF/XDP status in README and docs

Suggested title:

- `docs: mark eBPF/XDP as experimental acceleration, not shipped fast path`

Problem:

- the repo currently has embedded eBPF bytecode and a presence test
- it does not yet prove runtime attach, active path use, or benchmarked speedup
- docs can therefore overstate the feature

Why it matters:

- avoids misleading users and contributors
- improves trust

Proposed solution:

- downgrade eBPF language to `experimental` or `Linux acceleration track`
- add one explicit note saying current correctness does not depend on eBPF

Acceptance criteria:

- README no longer reads like eBPF acceleration is fully shipped
- whitepaper and architecture docs match implementation reality

Suggested labels:

- `docs`
- `architecture`
- `good-first-issue`

### Issue 2: Write a formal threat model

Suggested title:

- `security: add formal VX6 threat model for direct, relay, and hidden modes`

Problem:

- users do not currently have a precise statement of attacker capabilities and non-goals

Why it matters:

- prevents unsafe assumptions
- makes later security work measurable

Proposed solution:

- add a `docs/threat-model.md` file covering:
  - attacker classes
  - per-mode protections
  - explicit non-protections
  - trust assumptions around guards, exits, and bootstraps

Acceptance criteria:

- threat model doc exists
- README links to it
- direct/relay/hidden each have explicit privacy boundaries

Suggested labels:

- `security`
- `docs`
- `hardening`

### Issue 3: Document bootstrap trust and poisoning limits

Suggested title:

- `security: document bootstrap trust model and peer-discovery poisoning risks`

Problem:

- bootstrap is not a permanent traffic center, but it still strongly influences discovery

Why it matters:

- users need to understand bootstrap bias and omission risks

Proposed solution:

- add a bootstrap trust section to the architecture docs
- recommend multi-bootstrap operation
- explain what signatures prevent and what they do not prevent

Acceptance criteria:

- docs clearly distinguish:
  - record forgery resistance
  - discovery bias risk

Suggested labels:

- `security`
- `docs`
- `discovery`

## Priority 1: Hidden-Circuit Resilience

### 1. Active Hidden-Circuit Failover

Suggested title:

- `hidden-mode: add active hidden-circuit failover and fast rebuild`

Problem:

- hidden registrations rotate and rebuild
- active hidden data circuits still need faster live failover if a relay dies

Needed work:

- detect relay death mid-session
- rebuild working circuit quickly
- retry rendezvous attachment cleanly
- preserve user-facing behavior as much as possible

Good contributor profile:

- Go networking
- connection state machines
- fault recovery

### 2. Relay Churn Soak Testing

Suggested title:

- `testing: add high-churn hidden relay soak tests`

Problem:

- current tests prove correctness
- they do not yet simulate long high-churn relay populations deeply enough

Needed work:

- repeated relay joins/leaves
- hidden session continuity checks
- churn-driven leak checks
- timing and failure telemetry

Good contributor profile:

- distributed systems testing
- Go integration harnesses

## Priority 1: Abuse Resistance

### 3. Intro Flood Controls

Suggested title:

- `security: add intro-node flood controls and per-alias rate limiting`

Problem:

- intro nodes are exposed to request flooding

Needed work:

- per-alias rate limits
- per-node rate limits
- request budgeting
- bounded pending-intro state

Possible extensions:

- adaptive backoff
- proof-of-work or token gating for stressed paths

### 4. Rendezvous State Caps

Suggested title:

- `security: bound rendezvous state and clean up abandoned joins`

Problem:

- rendezvous points can be targeted with half-open or abandoned state creation

Needed work:

- bounded rendezvous tables
- expiration strategy
- cleanup on peer death
- structured error reporting

### 5. Circuit Setup Throttling

Suggested title:

- `security: throttle onion circuit setup under relay pressure`

Problem:

- onion circuit setup is more expensive than plain forwarding

Needed work:

- setup concurrency caps
- per-peer limits
- overload rejection strategy
- fairness under relay pressure

## Priority 2: Cryptographic Hardening

### 6. Replay Window Review

Suggested title:

- `security: expand replay and counter misuse tests for hidden and onion paths`

Problem:

- hidden control has epoch/nonce replay protection
- the circuit layer should be reviewed again for replay edge cases and counter misuse

Needed work:

- explicit replay test corpus
- wraparound analysis
- malformed cell fuzzing
- invalid counter progression testing

### 7. Session Separation Audit

Suggested title:

- `security: audit key separation across direct, hidden-control, and onion layers`

Problem:

- direct sessions, hidden control, and onion hop keys must stay clearly separated

Needed work:

- audit all key derivation contexts
- document key separation invariants
- ensure no accidental key reuse across protocol roles

### 8. Traffic Analysis Surface Review

Suggested title:

- `research: document traffic-analysis surface and evaluate padding options`

Problem:

- onion cells are fixed-size, but VX6 does not yet claim full traffic-analysis resistance
- timing and some higher-level metadata still leak

Needed work:

- measure current observable patterns
- evaluate optional padding strategies
- evaluate batching tradeoffs
- produce realistic guidance, not inflated privacy claims

Acceptance criteria:

- one doc explains exactly what fixed-size cells do and do not hide
- at least one benchmark or experiment compares padded vs unpadded hidden traffic

Suggested labels:

- `security`
- `research`
- `hidden-mode`

## Priority 3: Platform Hardening

### 9. Portable Runtime Locking

Suggested title:

- `windows: replace Unix-only runtime locking with portable lock abstraction`

Problem:

- current runtime lock path still depends on Unix file locking behavior

Needed work:

- abstract runtime locking
- implement Windows-compatible lock path
- keep single-instance guarantees per config

Acceptance criteria:

- `syscall.Flock` is no longer required in shared CLI logic
- Windows-specific lock implementation exists
- Linux behavior is preserved

Suggested labels:

- `windows`
- `runtime`
- `portability`

### 10. Windows Service Control Integration

Suggested title:

- `windows: add Service Control Manager integration for vx6 node lifecycle`

Problem:

- the network protocol is cross-platform
- background service lifecycle still needs Windows-native integration

Needed work:

- SCM integration
- clean start/stop/status flow
- reload via local control API

Acceptance criteria:

- Windows node can run as a service
- `reload` uses local control channel
- no Unix signal dependency on Windows

Suggested labels:

- `windows`
- `runtime`
- `service-management`

### 11. OS-Aware Path Handling

Suggested title:

- `portability: move config/data/runtime paths behind OS-aware path layer`

Problem:

- config and runtime paths are still POSIX-shaped

Needed work:

- platform-specific config/data/runtime locations
- migration behavior if old paths exist
- test coverage on Windows and macOS

Acceptance criteria:

- shared config code no longer hardcodes Linux path conventions
- Windows and macOS path rules are tested

Suggested labels:

- `windows`
- `macos`
- `portability`

### Issue 12: Add mixed-OS quickstarts and smoke checks

Suggested title:

- `docs/testing: add Windows and macOS quickstarts plus mixed-OS smoke matrix`

Problem:

- cross-platform architecture exists, but operator guidance is Linux-heavy

Why it matters:

- reduces friction for early testers
- helps validate the port as soon as code starts landing

Proposed solution:

- add quickstart docs for Windows and macOS
- define smoke matrix:
  - Windows client -> Linux owner
  - Linux client -> Windows owner
  - Windows relay in hidden path

Acceptance criteria:

- quickstarts exist
- smoke matrix is documented

Suggested labels:

- `docs`
- `windows`
- `macos`
- `testing`

## Priority 4: Performance Hardening

### 12. Connection Reuse

Suggested title:

- `performance: reuse neighbor sessions more aggressively`

Problem:

- path setup cost is acceptable but still worth reducing in steady state

Needed work:

- reuse neighbor sessions more aggressively
- reduce repeated setup work
- keep correctness under reconnect

### 13. Relay Queue Tuning

Suggested title:

- `performance: instrument relay queues and tune backpressure`

Problem:

- hidden mode cost is dominated by path length, RTT, and queueing

Needed work:

- queue depth instrumentation
- backpressure tuning
- fairness between local work and relay work

### 14. Memory Profiling

Suggested title:

- `performance: profile long-run relay churn memory behavior`

Problem:

- benchmark allocations are acceptable now
- but production relay churn may expose more heap pressure

Needed work:

- long-run heap profiles
- allocation hotspot reduction
- pooled buffer review where safe

## Priority 5: Verification And Auditability

### 15. Fuzzing The Onion Cell Layer

Suggested title:

- `security: fuzz fixed-size onion cells and relay envelopes`

Problem:

- fixed-size cells and layered decoding are safety-critical

Needed work:

- malformed cell fuzzing
- relay envelope fuzzing
- corrupted `CREATED` payload fuzzing
- invalid length and version fuzzing

Acceptance criteria:

- fuzz targets exist for cell and relay envelope decoding
- malformed input cannot panic shared code paths

Suggested labels:

- `security`
- `testing`
- `fuzzing`

### 16. Mixed-OS Integration Matrix

Suggested title:

- `testing: add mixed-OS integration matrix for direct and hidden modes`

Problem:

- protocol correctness must survive mixed Linux/Windows/macOS deployments

Needed work:

- Windows client -> Linux owner
- Linux client -> Windows owner
- Windows relay in hidden path
- mixed relay pools under churn

### 17. Security Review And Threat Model Cleanup

Suggested title:

- `security: align docs, tests, and code with an explicit threat model`

Problem:

- the implementation is ahead of the formal threat model document

Needed work:

- document attacker classes
- document what each hop can learn
- document what is and is not hidden from ISPs, guards, exits, and rendezvous points

### Issue 18: Specify and harden the DHT

Suggested title:

- `discovery: specify current DHT behavior and add poisoning/conflict handling`

Problem:

- the DHT works today but is weakly specified and lightly defended against adversarial inputs

Why it matters:

- decentralization claims depend on trustworthy discovery behavior

Proposed solution:

- write a DHT spec note
- add conflict handling and multi-source lookup checks
- add adversarial tests for conflicting or poisoned values

Acceptance criteria:

- DHT spec exists
- stale/conflicting value behavior is defined
- at least one poisoning-style test exists

Suggested labels:

- `discovery`
- `security`
- `dht`

### Issue 19: Add malicious relay integration tests

Suggested title:

- `testing: add malicious relay scenarios for onion and hidden paths`

Problem:

- current tests prove correct behavior, not malicious relay behavior

Why it matters:

- relay integrity and trust boundaries need adversarial validation

Proposed solution:

- introduce test relays that:
  - drop cells selectively
  - send malformed responses
  - abandon circuits mid-flow

Acceptance criteria:

- at least three adversarial relay scenarios exist
- behavior under failure is asserted, not just observed

Suggested labels:

- `testing`
- `security`
- `hidden-mode`
- `onion`

## Beginner First Issue

This one is safe for a newcomer and still useful.

Suggested title:

- `docs: add honest status note for eBPF, threat model, and platform support`

Why this is a real beginner issue:

- it does not require protocol changes
- it improves trust immediately
- it helps newcomers understand the project correctly

Work:

- update README to say:
  - eBPF is an experimental Linux acceleration track
  - Windows/macOS support is in progress
  - formal threat model is still being written
- link to:
  - `docs/vx6-architecture-whitepaper.md`
  - `docs/current-limitations-and-fix-plan.md`
  - `platform/windows/PORTING_GUIDE.md`

Acceptance criteria:

- README language is accurate
- no feature is described more strongly than the code supports
- links are added to the right docs

Suggested labels:

- `docs`
- `good-first-issue`
- `beginner-friendly`

## Other Good First Issues

- add tests for invalid epoch rollover and stale callback leases
- add docs that map message types to threat model claims
- add a support matrix doc for Windows/macOS/Linux targets
- add issue labels and milestone suggestions to the contributor docs

## Contributor Expectations

Good contributions should:

- preserve protocol compatibility
- include tests
- avoid broad refactors unrelated to the issue
- avoid weakening current privacy guarantees for convenience

Good issue labels:

- `security`
- `hardening`
- `hidden-mode`
- `windows`
- `performance`
- `good-first-issue`
- `integration-tests`
- `needs-benchmark`

## Suggested Public Summary

If you want a short public pitch for contributors, use this:

> VX6 already has working direct, relay, and hidden encrypted service paths.
> We are now focused on hardening: failover, abuse resistance, mixed-OS support,
> performance tuning, and stronger verification. Contributors who like Go,
> distributed systems, networking, security, and cross-platform runtime work are
> highly relevant here.
