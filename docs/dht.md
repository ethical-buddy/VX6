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
- conflict candidate reporting for same-name lookups
- interactive user choice support in the CLI when multiple candidates are returned
- bounded replication
- adaptive replication target under churn/failure pressure
- refresh tracking
- conservative store admission with signed trusted writes, authoritative publisher checks, stale-write rejection, and per-source throttling
- ASN-aware diversity when a local ASN map is present
- hidden descriptor caching, jittered refresh polling, and background cover lookups
- tracked hidden-invite warmers to reduce purely on-demand lookup patterns
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

## WAN/Churn Adaptation

The DHT now adjusts lookup and replication behavior from live lookup failure signals:

- lookup fanout (`alpha`) increases when recent failures rise
- lookup query budget (`beta`-style budget) increases under churn
- replication target increases under churn to keep availability stable

This reduces timeout risk in unstable WAN conditions without permanently paying high bandwidth cost in healthy periods.

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

## Name Conflicts

VX6 treats the node keypair and NodeID as the real identity.

If two different nodes try to use the same human-readable node name:

- the shared registry rejects the second publisher
- DHT lookup can report every conflicting candidate instead of silently picking one
- the CLI can show the full set of candidates and let the user choose which identity is correct
- each running node periodically checks its own name key and logs a clash warning if another identity claims the same name

This is intentional. Human names are labels. NodeID is the identity.

At init time and during explicit rename, VX6 can also wait and probe the network before accepting a name. If another identity is already using that name, the CLI stops and shows the candidates instead of silently taking over the name.
