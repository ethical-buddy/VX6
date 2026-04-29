# VX6 Discovery Model

## Current State

VX6 now supports a bootstrap discovery stage.

A known VX6 node can act as a registry for signed endpoint records. Other nodes can:

- publish their current signed endpoint record
- resolve another node by name or node ID
- save the resolved address into their local peer list

This is enough to test the address-change problem in a limited way:

1. a node changes IPv6 address
2. that node republishes a new signed endpoint record to the bootstrap node
3. another node resolves the record again
4. the resolved address replaces the stale local peer entry

## What This Solves

- removes repeated manual sharing of raw IPv6 addresses after first bootstrap contact
- allows a known bootstrap node to act as a discovery point for several machines
- verifies endpoint claims cryptographically before accepting them

## What This Does Not Solve Yet

- there is no real DHT
- there is no gossip or peer-to-peer replication of records
- registry data is not persisted across bootstrap node restarts
- nodes do not automatically refresh changed addresses in the background
- there is no quorum, conflict resolution, or multi-bootstrap merge logic

## Practical Interpretation

This is a bootstrap registry, not a decentralized discovery fabric yet.

It is still the correct next step because it proves the signed-record model and allows multi-device testing before the project takes on the complexity of a real DHT.
