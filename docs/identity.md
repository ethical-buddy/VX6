# VX6 Identity Model

VX6 now maintains a persistent Ed25519 identity for each node.

## Current Shape

Each initialized node has:

- a human-readable node name
- a stable node ID derived from the public key
- an Ed25519 public/private keypair
- a signed endpoint record describing where that node can currently be reached

The node ID is derived from the public key, so it stays stable even if the node changes IPv6 addresses.

## Why This Matters

The earlier peer alias feature was convenient but not trustworthy. A decentralized lookup system cannot safely exchange endpoint data unless peers can verify who signed a record and whether that record has expired.

The current record format gives VX6:

- a stable cryptographic identity
- a signed claim over `[ipv6]:port`
- issue and expiry times
- a public key that other peers can verify

## What It Does Not Solve Yet

Identity alone does not provide discovery.

VX6 still needs:

- bootstrap peers that can exchange records
- storage and refresh of the latest valid records
- conflict handling for old or stale endpoint claims
- service records layered on top of node endpoint records
