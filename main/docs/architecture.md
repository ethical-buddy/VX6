# VX6 Architecture

## Current Baseline

The repository starts with one executable, `vx6`, and a narrow set of responsibilities:

1. Run a node listener on `tcp6`.
2. Accept inbound file transfers into a local data directory.
3. Open outbound `tcp6` connections to other nodes.
4. Send files using a small framed protocol with node metadata.
5. Maintain a persistent Ed25519 identity and sign endpoint records.
6. Publish and resolve endpoint records through a known bootstrap node.

This is intentionally narrow. VX6 should earn complexity rather than declare it.

## Near-Term Direction

The node and transfer primitives in this repository are expected to evolve into a larger IPv6-first system with the following layers:

- stable node naming
- endpoint handling
- node identity
- signed endpoint records
- bootstrap discovery
- service publication
- discovery
- forwarding and routing

Each layer should remain independently testable. Direct connectivity is the default path; any relay or proxy behavior should be an explicit extension, not an implicit fallback.

## Implementation Rules

- Keep IPv6-only behavior explicit in code and documentation.
- Prefer streaming interfaces over whole-file buffering.
- Keep package boundaries small and responsibility-driven.
- Add protocol surface gradually and document it as it appears.
- Keep the single-binary operational model intact as features are added.
- Treat node identity and endpoint claims as signed data, not trusted local strings.
