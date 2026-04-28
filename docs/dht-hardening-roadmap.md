# VX6 DHT Hardening Roadmap

This note separates:

- what is implemented now
- what is still recommended next
- which outside designs influenced the current direction

It is deliberately blunt. The DHT is materially stronger than before, but it is
not "finished forever."

## Implemented Now

### 1. Multi-source confirmation for trusted VX6 keys

For these key families:

- `node/name/*`
- `node/id/*`
- `service/*`
- `hidden/*`

VX6 now requires confirmation from multiple DHT sources before trusting a
resolved value.

Current rule:

- at least `2` exact supporting sources
- total confirmation weight at least `4`
- at least `2` network groups when possible
- loopback-only tests are allowed to confirm on multiple local sources

This is enforced in:

- `internal/dht/value.go`

### 2. Signed DHT envelopes

Trusted VX6 keys are now stored as signed DHT envelopes when the publishing node
has a VX6 identity available.

The envelope carries:

- key
- wrapped value
- origin node id
- publisher node id
- publisher public key
- version
- issued_at
- expires_at
- observed_at
- publisher signature

This lets lookup code distinguish:

- the record owner
- the node that published the DHT copy
- the freshness window of the wrapped record

Implementation:

- `internal/dht/envelope.go`

### 3. Version and conflict tracking

The DHT server now tracks:

- current stored version
- bounded previous versions
- bounded conflicting versions

This is local state for observability and debugging, not a distributed ledger.

Implementation:

- `internal/dht/dht.go`

### 4. Bounded replication

Store replication is now bounded to a smaller replica set instead of blindly
using all `K = 20` closest nodes.

Current replication factor:

- `5`

Replica selection also prefers network diversity before falling back to same-
network peers.

Implementation:

- `internal/dht/dht.go`

### 5. Source diversity checks

Lookup confirmation now tracks network-group diversity:

- IPv6 grouped by `/64`
- IPv4 grouped by `/24`
- loopback tracked separately

This reduces the chance that multiple copies from the same host or same local
network are treated as fully independent evidence.

Implementation:

- `internal/dht/value.go`

### 6. Trust weighting

Confirmation is no longer pure source counting.

Each exact supporting source contributes weighted evidence based on:

- base source weight
- whether the value is in a signed DHT envelope
- whether the publisher is authoritative for the wrapped record

This is not a complete Sybil defense, but it is better than "two nodes said so."

Implementation:

- `internal/dht/value.go`

### 7. Routing-table replacement cache and failure eviction

The routing table no longer only appends or ignores.

When a bucket is full:

- new nodes enter a bounded replacement cache

When a live node repeatedly fails:

- it is evicted
- a cached replacement is promoted

This improves churn handling and reduces long-lived stale-bucket bias.

Implementation:

- `internal/dht/table.go`

## What This Improves

These changes raise the bar against:

- first-response trust
- malformed-value poisoning
- stale-value replay
- single-node DHT lies
- same-host duplicate confirmation
- unbounded replica fanout
- stale routing buckets that never recover

## What This Still Does Not Solve

These changes do not make the DHT magically perfect.

Not fully solved yet:

- large Sybil sets with many valid identities
- Eclipse attacks by topology control
- colluding diverse peers returning the same bad but valid value
- write-amplification or publish flooding
- WAN-scale tuning under churn

## Best Next Steps

### 1. Disjoint-path lookups

Current lookup fanout is parallel, but not fully disjoint-path aware.

Recommended next step:

- S/Kademlia-style disjoint lookup paths for better adversarial resilience

Why:

- it reduces the chance that one attacked region of the routing table controls
  the whole lookup

### 2. Admission weighting or proof-of-work for publishers

Current trust weighting uses identity-bound signatures and multi-source
confirmation, but not admission cost.

Recommended next step:

- optional publisher proof-of-work or admission token
- higher confirmation threshold for low-cost identities

Why:

- it raises the cost of spinning large numbers of DHT publishers

### 3. Write tokens for stores

Recommended next step:

- tokenized store admission similar to established DHT designs

Why:

- it reduces blind third-party store abuse

### 4. Refresh and republish policy

Current store replication is bounded, but refresh behavior is still simple.

Recommended next step:

- periodic refresh on closest live replicas
- bounded re-replication when the closest set changes

### 5. Negative caching and backoff

Recommended next step:

- cache recent misses briefly
- rate-limit repeated unresolved lookups

Why:

- it lowers lookup amplification under repeated failed queries

## Design References

These designs are the main reference points for the current and next-stage
choices:

- Kademlia for XOR routing and bucketed neighbor selection
- S/Kademlia for secure/disjoint routing ideas
- BitTorrent BEP 5 for practical operational DHT behavior
- BitTorrent BEP 42 for stronger node-id / network-position checks
- Coral DHT for bounded, locality-aware replication ideas

VX6 is not copying any one of these wholesale, but those are the right
comparison points for the DHT evolution.
