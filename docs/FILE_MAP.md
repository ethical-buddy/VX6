# File Map

This file explains what the important files and directories do.

## Commands

- `cmd/vx6/main.go`
  - CLI entrypoint
- `cmd/vx6/signals_unix.go`
  - Unix/Linux process signal setup
- `cmd/vx6/signals_windows.go`
  - Windows process signal setup
- `cmd/vx6-gui/main.go`
  - local web GUI front-end that wraps the CLI binary

## Core Runtime

- `internal/cli/app.go`
  - CLI command handling
- `internal/config/`
  - config file loading, defaults, runtime paths
- `internal/node/node.go`
  - main node runtime and background publish/sync loop
- `internal/runtimectl/runtimectl.go`
  - local runtime control channel used by `status` and `reload`

## Identity and Records

- `internal/identity/store.go`
  - local identity creation and storage
- `internal/record/endpoint.go`
  - signed endpoint records
- `internal/record/service.go`
  - signed service records

## Discovery and DHT

- `internal/discovery/discovery.go`
  - local registry and peer snapshot exchange
- `internal/dht/dht.go`
  - main DHT server and lookup flow
- `internal/dht/value.go`
  - DHT lookup validation, trust weighting, and confirmation logic
- `internal/dht/private_catalog.go`
  - private per-user catalog format
- `internal/dht/hidden_descriptor.go`
  - hidden descriptor encoding, encryption, and validation
- `internal/dht/hidden_lookup.go`
  - hidden descriptor cache, polling, and cover lookup logic
- `internal/dht/privacy_transport.go`
  - anonymous relay transport for hidden descriptor DHT traffic
- `internal/dht/replica_status.go`
  - publish health tracking and DHT status summaries
- `internal/dht/table.go`
  - routing table implementation

## Hidden Services and Relays

- `internal/hidden/hidden.go`
  - hidden intro, guard, rendezvous, and failover logic
- `internal/onion/cell.go`
  - fixed-size encrypted relay cells
- `internal/onion/circuit.go`
  - relay-side circuit handling
- `internal/onion/onion.go`
  - client-side circuit build and planned circuit dialing

## Secure Sessions and Service Forwarding

- `internal/secure/session.go`
  - authenticated encrypted VX6 session layer
- `internal/serviceproxy/proxy.go`
  - direct and hidden local forwarder behavior
- `internal/transfer/transfer.go`
  - file send and receive logic
- `internal/transport/transport.go`
  - current TCP transport abstraction

## Linux Experimental Acceleration

- `internal/onion/ebpf.go`
  - eBPF/XDP status and attach lifecycle
- `internal/ebpf/onion_relay.c`
  - embedded relay bytecode source

## Tests

- `internal/*/*_test.go`
  - unit tests
- `internal/integration/swarm_test.go`
  - multi-node integration tests
