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
- conservative store admission with signed trusted writes, stale-write rejection, and per-source throttling
- ASN-aware diversity when a local ASN map is present
- hidden descriptor caching and cover lookups
- blinded rotating hidden keys
- encrypted hidden descriptor payloads

## What It Does Not Do Yet

- disk-backed large-scale value storage
- full Tor-grade traffic-analysis resistance
- operator-managed publish tokens for high-trust deployments

## Hidden Descriptor Notes

Hidden descriptors are stronger than plain alias lookup because:

- the lookup key is blinded
- the descriptor payload is encrypted
- the invite carries the secret lookup part
- descriptor store and lookup can be relayed anonymously

Still, the responsible DHT holders can observe timing and volume on a blinded descriptor key. That is one of the main remaining privacy limits.

## ASN Diversity

VX6 can use an offline ASN map to improve DHT diversity checks.

When the map is present:

- lookup confirmation prefers independent ASNs first
- replica selection spreads records across ASNs before falling back to prefix diversity

When the map is missing or incomplete:

- VX6 falls back to the current prefix-based provider grouping
- the DHT still works normally

The ASN map is optional and local. It is not fetched over the network.
