# VX6 Roadmap

## Phase 0

- establish repository structure
- provide a working Go CLI
- implement IPv6 file streaming over `tcp6`
- define documentation and contribution standards

## Phase 1

- add receiver-side protocol support in Go
- define transfer metadata framing
- add checksums and transfer diagnostics
- add tests for IPv6 parsing and stream handling

## Phase 2

- introduce node identity
- persist local configuration
- formalize peer connection handling

## Phase 3

- add service advertisement primitives
- define discovery records
- document bootstrap and lookup strategy

## Phase 4

- evaluate forwarding, proxying, and policy controls
- add observability for routing and transfer paths
- prepare for multi-node integration testing
