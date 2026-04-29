# VX6 Discovery Model

## Current State

VX6 now supports a distributed bootstrap discovery stage.

A known VX6 node can act as a registry for signed endpoint records. Other nodes can:

- publish their current signed endpoint record
- resolve another node by name or node ID
- save the resolved address into their local peer list
- pull a snapshot of known records into a local cached registry
- answer lookups from their local cached registry

This is enough to test the address-change problem in a practical distributed-bootstrap way:

1. a node changes IPv6 address
2. the daemon republishes a new signed endpoint record to configured bootstrap nodes
3. another node resolves the record again
4. the resolved address replaces the stale local peer entry

## What This Solves

- removes repeated manual sharing of raw IPv6 addresses after first bootstrap contact
- allows a known bootstrap node to act as a discovery point for several machines
- verifies endpoint claims cryptographically before accepting them
- allows nodes to keep a cached registry snapshot even if the bootstrap becomes temporarily unavailable
- allows nodes to query cached peers when a bootstrap is temporarily unavailable

## What This Does Not Solve Yet

- there is no full Kademlia-style DHT
- there is no recursive peer-to-peer search across every cached node
- there is no quorum, conflict resolution, or multi-bootstrap merge logic
- nodes do not yet automatically promote discovered peers into a full mesh search fabric

## Practical Interpretation

This is a distributed bootstrap registry with local cache and peer-assisted lookup, not a fully decentralized DHT yet.

It is still the correct next step because it proves the signed-record model and allows multi-device testing before the project takes on the complexity of a real DHT.
