# DHT

The VX6 DHT is the distributed lookup layer behind public, private, and hidden discovery.

## What It Stores

- node records by name
- node records by node ID
- public service records
- private per-user catalogs
- hidden service descriptors

## What It Does Today

- multi-source lookup confirmation
- conflict detection
- bounded replication
- refresh tracking
- hidden descriptor caching and cover lookups
- blinded rotating hidden keys
- encrypted hidden descriptor payloads

## What It Does Not Do Yet

- strong anti-Sybil store admission
- real ASN/provider diversity
- disk-backed large-scale value storage
- full Tor-grade traffic-analysis resistance

## Hidden Descriptor Notes

Hidden descriptors are stronger than plain alias lookup because:

- the lookup key is blinded
- the descriptor payload is encrypted
- the invite carries the secret lookup part
- descriptor store and lookup can be relayed anonymously

Still, the responsible DHT holders can observe timing and volume on a blinded descriptor key. That is one of the main remaining privacy limits.
