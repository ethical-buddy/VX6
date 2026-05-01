# Threat Model (Current)

VX6 provides secure service connectivity and discovery between peers, but its threat model is still evolving and not yet formally defined.

## Assumptions

- The network may contain untrusted or malicious nodes
- Local registry data may be incomplete or stale
- DHT participants may behave adversarially
- Network observers may measure timing and traffic patterns

## Protections

- Each node has a cryptographic identity (Ed25519); only the holder of the private key can produce valid signatures for that identity
- Node and service records are signed and verifiable, preventing impersonation
- Peer-to-peer sessions are encrypted, protecting against passive eavesdropping
- Hidden services use blinded lookup keys and encrypted descriptors to avoid direct exposure of endpoints
- Service access does not require exposing public ports

## Known Limitations

- The DHT does not yet provide strong anti-Sybil protection, allowing attackers to introduce many fake nodes and influence service discovery and lookup routing
- Malicious DHT participants may return incorrect or biased results, or drop/slow responses
- Lookup timing and access patterns may be observable, enabling traffic analysis
- Hidden-service anonymity is not equivalent to Tor-level guarantees
- Local registry data is a cached view of the network and may be incomplete or outdated, which can affect service discovery until refreshed via the DHT
- Relay path stability and seamless mid-stream failover are still evolving; connections may drop if relays fail
- eBPF/XDP acceleration is experimental and not part of the active relay data path
- No formal or externally reviewed threat model has been completed yet

