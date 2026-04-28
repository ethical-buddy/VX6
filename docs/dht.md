# VX6 DHT Specification

Version:

- current implementation after strict confirmation, signed envelopes, bounded
  replication, and routing-table hardening

Purpose:

- define what the VX6 DHT is trusted for
- define how lookups are validated
- define freshness, conflict, replication, and confirmation behavior

This is an implementation note, not a full academic protocol paper.

## Scope

The VX6 DHT is used for decentralized discovery of:

- node endpoint records
- service records
- hidden-service alias records

It is not intended to be a generic globally trusted key/value store.

## Key Families

VX6 currently uses:

- `node/name/<nodeName>`
- `node/id/<nodeID>`
- `service/<nodeName.serviceName>`
- `hidden/<alias>`

Helper constructors are in:

- `internal/dht/dht.go`

## Routing Model

Current routing model:

- Kademlia-like XOR routing
- `256` buckets
- `K = 20` entries per bucket
- replacement cache per bucket
- repeated-failure eviction
- recursive `find_node`
- recursive `find_value`

Implementation:

- `internal/dht/table.go`
- `internal/dht/dht.go`

## Stored Values

The DHT still transports strings at the wire level, but trusted VX6 discovery
keys are now normally stored as signed DHT envelopes around the existing signed
record payloads.

Trusted wrapped payloads include:

- endpoint records for `node/name/*` and `node/id/*`
- service records for `service/*`
- hidden service records for `hidden/*`

The DHT does not create trust by itself.

Trust now comes from:

- inner VX6 record signatures
- DHT envelope signatures
- expiry checks
- key/value consistency checks
- version tracking
- multi-source confirmation
- source diversity checks

## Signed DHT Envelopes

When a publishing node has a VX6 identity, it stores trusted VX6 keys as a
signed envelope containing:

- `key`
- `value`
- `origin_node_id`
- `publisher_node_id`
- `publisher_public_key`
- `version`
- `issued_at`
- `expires_at`
- `observed_at`
- `signature`

The inner `value` remains the original VX6 signed record.

Implementation:

- `internal/dht/envelope.go`

## Record Verification Rules

### Node endpoint keys

For `node/name/<name>`:

- value must decode as a valid signed endpoint record
- `record.NodeName` must equal `<name>`

For `node/id/<id>`:

- value must decode as a valid signed endpoint record
- `record.NodeID` must equal `<id>`

### Service keys

For `service/<node.service>`:

- value must decode as a valid signed service record
- `record.FullServiceName(NodeName, ServiceName)` must match the key suffix

### Hidden alias keys

For `hidden/<alias>`:

- value must decode as a valid signed hidden service record
- `record.IsHidden` must be true
- `record.Alias` must equal the key suffix

### Envelope checks

If the value is wrapped in a DHT envelope:

- envelope signature must verify
- publisher node id must match the publisher public key
- envelope key must match the lookup key
- envelope version must match the wrapped record issue time
- envelope origin must match the wrapped record owner
- envelope freshness must still be valid

### Unknown keys

Unknown key families are treated as raw values:

- they can still be stored and looked up
- they do not receive the same signature-based guarantees

## Store Behavior

Current store path is conservative for trusted VX6 keys.

Accepted writes:

- first valid value for a key
- newer valid value from the same record family
- exact same value again

Rejected writes:

- invalid signed records
- invalid signed envelopes
- key/value mismatches
- conflicting valid values from a different family for the same trusted key

## Version and Conflict Tracking

Each local DHT server now tracks bounded metadata per key:

- current version
- previous accepted versions
- conflicting versions seen and rejected

This is not distributed consensus. It is local observability and conflict memory.

Implementation:

- `internal/dht/dht.go`

## Freshness Rules

For trusted VX6 records:

- expired records are invalid
- newer `IssuedAt` wins inside the same record family
- same-timestamp ties fall back to fingerprint ordering
- older copies are stale, not authoritative

## Lookup Behavior

The lookup path no longer trusts the first answer.

Current behavior:

1. query multiple nearby DHT nodes in parallel
2. keep walking even if one node already has a value
3. validate returned values for trusted VX6 keys
4. reject malformed, expired, or mismatched values
5. merge stale and fresh versions within the same record family
6. require exact multi-source confirmation before trusting a verified value
7. fail with a conflict error if multiple verified families remain

## Confirmation Rules

Trusted VX6 lookups now require all of the following:

- at least `2` exact supporting sources
- total confirmation weight at least `4`
- at least `2` source network groups when possible
- loopback-only environments may satisfy diversity with multiple local sources

Confirmation weight increases when:

- the value is wrapped in a signed DHT envelope
- the publisher is authoritative for the wrapped record

Implementation:

- `internal/dht/value.go`

## Replication

Store replication is now bounded.

Current policy:

- replicate to `5` nearby nodes
- prefer distinct network groups first
- fall back to same-network replicas only if needed

This reduces unnecessary fanout and improves replica diversity.

Implementation:

- `internal/dht/dht.go`

## Routing-Table Maintenance

Current bucket behavior:

- live entries are kept in LRU-like order
- full buckets keep a bounded replacement cache
- nodes that fail repeatedly are evicted
- replacements are promoted when stale entries are removed

This reduces long-lived stale-bucket bias.

Implementation:

- `internal/dht/table.go`

## Current Guarantees

The hardened VX6 DHT is now:

- bounded
- key-aware
- envelope-aware
- version-aware
- conflict-aware
- multi-source confirmed for trusted keys
- more conservative under poisoning attempts

## Current Limits

The DHT is stronger than before, but not magically Sybil-proof.

Still not fully solved:

- large Sybil sets with many valid identities
- strong Eclipse attacks
- colluding diverse peers serving the same bad but valid value
- WAN-scale parameter tuning under heavy churn
- tokenized store admission
- disjoint-path secure lookups

See also:

- `docs/dht-hardening-roadmap.md`
