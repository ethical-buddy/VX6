# VX6 Roadmap

## Phase 0

- establish repository structure
- provide a single working Go executable
- implement node listen mode over `tcp6`
- implement IPv6 file streaming over a defined VX6 wire format
- define documentation and contribution standards

## Phase 1

- harden transfer metadata framing
- add checksums and transfer diagnostics
- add tests for IPv6 parsing and stream handling
- add graceful shutdown and node-level logging controls

## Phase 2

- introduce stable local node configuration
- define human-readable naming beyond raw IPv6 endpoints
- introduce node identity
- generate signed endpoint records
- formalize peer connection handling

## Phase 3

- add endpoint record exchange between peers
- define bootstrap peer flow
- add service advertisement primitives
- define discovery records
- document bootstrap and lookup strategy

## Phase 4

- build decentralized endpoint lookup
- add endpoint refresh and re-sign flows
- evaluate forwarding, proxying, and policy controls
- add observability for routing and transfer paths
- prepare for multi-node integration testing
