# VX6 Hardening And Contributor Issues

This file is written so it can be posted or adapted as a public contributor
backlog.

The goal is to attract contributors to concrete security, resilience, and
cross-platform work, not vague “help wanted” items.

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

## Priority 0: Hidden-Circuit Resilience

### 1. Active Hidden-Circuit Failover

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

Problem:

- rendezvous points can be targeted with half-open or abandoned state creation

Needed work:

- bounded rendezvous tables
- expiration strategy
- cleanup on peer death
- structured error reporting

### 5. Circuit Setup Throttling

Problem:

- onion circuit setup is more expensive than plain forwarding

Needed work:

- setup concurrency caps
- per-peer limits
- overload rejection strategy
- fairness under relay pressure

## Priority 2: Cryptographic Hardening

### 6. Replay Window Review

Problem:

- hidden control has epoch/nonce replay protection
- the circuit layer should be reviewed again for replay edge cases and counter misuse

Needed work:

- explicit replay test corpus
- wraparound analysis
- malformed cell fuzzing
- invalid counter progression testing

### 7. Session Separation Audit

Problem:

- direct sessions, hidden control, and onion hop keys must stay clearly separated

Needed work:

- audit all key derivation contexts
- document key separation invariants
- ensure no accidental key reuse across protocol roles

### 8. Traffic Analysis Surface Review

Problem:

- encryption does not hide timing and size metadata

Needed work:

- measure current observable patterns
- evaluate optional padding strategies
- evaluate batching tradeoffs
- produce realistic guidance, not inflated privacy claims

## Priority 3: Platform Hardening

### 9. Portable Runtime Locking

Problem:

- current runtime lock path still depends on Unix file locking behavior

Needed work:

- abstract runtime locking
- implement Windows-compatible lock path
- keep single-instance guarantees per config

### 10. Windows Service Control Integration

Problem:

- the network protocol is cross-platform
- background service lifecycle still needs Windows-native integration

Needed work:

- SCM integration
- clean start/stop/status flow
- reload via local control API

### 11. OS-Aware Path Handling

Problem:

- config and runtime paths are still POSIX-shaped

Needed work:

- platform-specific config/data/runtime locations
- migration behavior if old paths exist
- test coverage on Windows and macOS

## Priority 4: Performance Hardening

### 12. Connection Reuse

Problem:

- path setup cost is acceptable but still worth reducing in steady state

Needed work:

- reuse neighbor sessions more aggressively
- reduce repeated setup work
- keep correctness under reconnect

### 13. Relay Queue Tuning

Problem:

- hidden mode cost is dominated by path length, RTT, and queueing

Needed work:

- queue depth instrumentation
- backpressure tuning
- fairness between local work and relay work

### 14. Memory Profiling

Problem:

- benchmark allocations are acceptable now
- but production relay churn may expose more heap pressure

Needed work:

- long-run heap profiles
- allocation hotspot reduction
- pooled buffer review where safe

## Priority 5: Verification And Auditability

### 15. Fuzzing The Onion Cell Layer

Problem:

- fixed-size cells and layered decoding are safety-critical

Needed work:

- malformed cell fuzzing
- relay envelope fuzzing
- corrupted `CREATED` payload fuzzing
- invalid length and version fuzzing

### 16. Mixed-OS Integration Matrix

Problem:

- protocol correctness must survive mixed Linux/Windows/macOS deployments

Needed work:

- Windows client -> Linux owner
- Linux client -> Windows owner
- Windows relay in hidden path
- mixed relay pools under churn

### 17. Security Review And Threat Model Cleanup

Problem:

- the implementation is ahead of the formal threat model document

Needed work:

- document attacker classes
- document what each hop can learn
- document what is and is not hidden from ISPs, guards, exits, and rendezvous points

## Good First Issues

- add stronger integration assertions around hidden-circuit rebuild timing
- add benchmarks for relay churn recovery
- add tests for invalid epoch rollover and stale callback leases
- add docs that map message types to threat model claims
- add path selection telemetry for relay health decisions

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
