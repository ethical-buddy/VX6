# VX6 DHT Specification

Version:
- current implementation as of the DHT hardening pass on `main`

Purpose:
- describe what the VX6 DHT currently is
- describe what it is trusted for
- define conflict, freshness, and poisoning behavior

This is a practical implementation note, not a full academic protocol paper.

## Scope

The VX6 DHT is currently used for lightweight decentralized discovery of:

- node endpoint records
- service records
- hidden-service alias records

It is not currently designed as a generic globally trusted database.

## Keys

VX6 uses these main key families:

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
- `K = 20` nodes per bucket
- recursive `find_node`
- recursive `find_value`

The routing table is in:

- `internal/dht/table.go`

Important implication:

- a node does not need to know all users in the network
- it only needs a bounded routing table plus recently used records

## Stored Values

The DHT stores raw strings.

For real VX6 discovery keys, those strings are expected to contain signed JSON
records:

- endpoint records for `node/name/*` and `node/id/*`
- service records for `service/*` and `hidden/*`

The DHT itself does not create trust.

Trust comes from:

- record signatures
- record expiry
- key-to-record consistency checks
- multi-source lookup behavior

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
- `record.Alias` must equal `<alias>`

### Unknown keys

Unknown key families are treated as unverified raw values.

That means:

- they can still be looked up
- they do not receive the same signature-based trust guarantees

## Store Behavior

Current store path is conservative for VX6 record keys.

### Accepted writes

- first valid value for a key
- newer valid value from the same record family
- exact same value again

### Rejected writes

- invalid signed records
- key/value mismatches
- conflicting valid values from a different family for the same trusted VX6 key

Examples:

- newer service record for the same node/service is accepted
- a different signed endpoint record claiming the same node name but a different
  node identity is rejected as conflicting

## Freshness Rules

For signed VX6 records:

- expired records are invalid
- for the same record family, newer `IssuedAt` wins
- older valid copies are treated as stale, not authoritative

This mirrors the freshness rule already used in the registry layer.

## Lookup Behavior

The hardened lookup path no longer trusts the first answer blindly.

Current behavior:

1. query multiple nearby DHT nodes
2. validate returned values if the key is a trusted VX6 discovery key
3. reject invalid or mismatched values
4. merge stale and fresh versions within the same record family
5. return the newest valid value if only one valid family remains
6. return a conflict error if multiple valid families remain

For unknown keys:

- the lookup chooses the most-supported exact raw value
- ties become conflicts

## Multi-Source Checks

The current lookup path queries multiple nodes before finalizing a result when
possible.

Current tuning:

- batch fanout: `3`
- query budget: `8`
- early success target: at least `2` supporting sources

Important:

- a single valid value can still be returned if that is all the network can
  provide
- but when multiple sources exist, the lookup tries to confirm rather than
  trust the first response

## Poisoning Resistance

Current poisoning resistance comes from:

- signature verification
- expiry checks
- key/value consistency checks
- conflicting-family rejection
- multi-source lookup

What this protects against reasonably:

- malformed record injection
- stale record replay as authoritative state
- simple forged-value poisoning without the private key

What it does not fully solve:

- Sybil attacks
- bootstrap bias
- coordinated malicious peers returning the same wrong but valid conflict family
- global discovery trust under strong adversaries

## Conflict Semantics

For trusted VX6 keys, a conflict means:

- more than one valid record family exists for the same lookup key

Examples:

- two different valid endpoint identities for the same `node/name/alice`
- two different valid hidden-service owners for the same alias

In those cases the lookup should fail with a conflict error rather than guess.

That is safer than returning whichever value arrived first.

## Current Limitations

The DHT is better than before, but still not “finished.”

Remaining limitations:

- no Sybil resistance
- no quorum signatures
- no reputation/trust weighting
- no WAN-scale evaluation yet
- no formal anti-Eclipse strategy

So the honest description is:

- bounded and functional
- key-aware and conflict-aware
- signature-checked for VX6 discovery records
- not yet a hardened large-scale adversarial DHT

## Files

Core implementation:

- `internal/dht/dht.go`
- `internal/dht/table.go`
- `internal/dht/value.go`

Tests:

- `internal/dht/dht_network_test.go`

Benchmarks:

- `internal/dht/bench_test.go`
