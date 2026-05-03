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
- conservative store admission with signed trusted writes, authoritative publisher checks, stale-write rejection, and per-source throttling
- ASN-aware diversity when a local ASN map is present
- hidden descriptor caching and cover lookups
- blinded rotating hidden keys
- encrypted hidden descriptor payloads

## What It Does Not Do Yet

- disk-backed large-scale value storage
- full Tor-grade traffic-analysis resistance
- operator-managed publish tokens for high-trust deployments
- perfect live migration of an already-running hidden TCP stream after relay loss

## Hidden Descriptor Notes

Hidden descriptors are stronger than plain alias lookup because:

- the lookup key is blinded
- the descriptor payload is encrypted
- the invite carries the secret lookup part
- descriptor store and lookup can be relayed anonymously

Still, the responsible DHT holders can observe timing and volume on a blinded descriptor key. That is one of the main remaining privacy limits.

VX6 reduces the obvious alias leak, but it does not hide all metadata from a powerful observer yet.

## ASN Diversity

VX6 can use an offline ASN map to improve DHT diversity checks.

When the map is present:

- lookup confirmation prefers independent ASNs first
- replica selection spreads records across ASNs before falling back to prefix diversity

When the map is missing or incomplete:

- VX6 falls back to the current prefix-based provider grouping
- the DHT still works normally

The ASN map is optional and local. It is not fetched over the network.

## Store Admission

Trusted keys are handled carefully.

That means:

- the record must verify correctly
- the envelope must be from the authoritative publisher for trusted keys
- stale verified values are rejected
- repeat writes from the same source are rate limited

This keeps the DHT from accepting arbitrary trusted writes just because they are signed.
