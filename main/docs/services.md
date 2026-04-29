# VX6 Service Model

## Current State

VX6 now supports:

- file transfer between nodes
- local service registration
- service publication through discovery
- local forwarding into remote services such as SSH

## Intended Direction

VX6 is meant to sit in front of local services and provide:

- a stable service name
- endpoint resolution
- policy around which peers can connect
- optional forwarding when direct paths are not available

For a local SSH daemon, the current model is:

1. a node registers a service such as `ssh`
2. the service points at a local target like `127.0.0.1:22`
3. the daemon publishes `surya.ssh` through discovery
4. another node resolves `surya.ssh`
5. VX6 connects to the current endpoint and carries the TCP stream over an encrypted node-to-node session

The same shape applies to HTTP, databases, and other TCP services. The major difference between current VX6 and that future state is the missing discovery and routing layer.

## Naming Direction

The intended naming pattern is a composition of node identity and service identity, for example:

- `surya.ssh`
- `surya.blog`
- `surya.files`

The exact wire format and naming policy are not fixed yet, but the operational goal is simple: the user chooses a node name and service name, and VX6 resolves the current reachable endpoint behind that name.

## Why Raw IPv6 Is Still Visible Today

Direct IPv6 connectivity is the transport base, but it is not by itself a naming or discovery system.

To hide changing IPv6 addresses cleanly, VX6 needs:

- a persistent node identity
- signed endpoint records
- a way to publish and refresh those records
- a way for other peers to look them up

VX6 now has the first distributed-bootstrap pieces: persistent identity, signed endpoint records, publish/lookup through known VX6 nodes, automatic republish on daemon start, cached registry snapshots, and encrypted file/service sessions. It still lacks policy-rich service routing and a full decentralized DHT.

Current service traffic is encrypted and authenticated between VX6 nodes. The local target behind the remote VX6 node remains the normal local TCP service you exposed, for example `127.0.0.1:22`.
