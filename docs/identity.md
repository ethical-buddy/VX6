# Identity

Each VX6 node has one long-term Ed25519 identity.

That identity is used to:

- derive the node ID
- sign endpoint records
- sign service records
- sign private catalogs
- sign DHT envelopes

The identity is local to the machine and is stored on disk.

If you reuse the same identity after restart, other nodes still recognize you as the same VX6 node.
