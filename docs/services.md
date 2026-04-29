# VX6 Service Model

## Current State

VX6 currently supports one application-level operation: file transfer between nodes.

This is not yet a general service proxy. There is no SSH forwarding command, no service registry, and no automatic peer lookup.

## Intended Direction

VX6 is meant to sit in front of local services and provide:

- a stable service name
- endpoint resolution
- policy around which peers can connect
- optional forwarding when direct paths are not available

For a local SSH daemon, the eventual model is:

1. a node publishes a service such as `lab-ssh`
2. the service points at a local target like `127.0.0.1:22`
3. another node resolves `lab-ssh`
4. VX6 connects to the current endpoint and carries the TCP stream

The same shape applies to HTTP, databases, and other TCP services. The major difference between current VX6 and that future state is the missing discovery and routing layer.

## Why Raw IPv6 Is Still Visible Today

Direct IPv6 connectivity is the transport base, but it is not by itself a naming or discovery system.

To hide changing IPv6 addresses cleanly, VX6 needs:

- a persistent node identity
- signed endpoint records
- a way to publish and refresh those records
- a way for other peers to look them up

VX6 now has the first three pieces in bootstrap form: persistent identity, signed endpoint records, and publish/lookup through a known VX6 node. It still lacks decentralized replication, automatic refresh, and service-level routing.
