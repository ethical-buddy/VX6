# VX6 Architecture

## Current Baseline

The repository starts with a single transport primitive:

1. Open a `tcp6` connection to a remote IPv6 endpoint.
2. Stream file bytes over that connection.
3. Close cleanly after transfer completion or surface the error.

This is intentionally narrow. VX6 should earn complexity rather than declare it.

## Near-Term Direction

The transport primitive in this repository is expected to evolve into a larger IPv6-first system with the following layers:

- endpoint handling
- node identity
- service publication
- discovery
- forwarding and routing

Each layer should remain independently testable. Direct connectivity is the default path; any relay or proxy behavior should be an explicit extension, not an implicit fallback.

## Implementation Rules

- Keep IPv6-only behavior explicit in code and documentation.
- Prefer streaming interfaces over whole-file buffering.
- Keep package boundaries small and responsibility-driven.
- Add protocol surface gradually and document it as it appears.
