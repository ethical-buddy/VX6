# VX6 DHT Architecture And Scaling

This document explains the VX6 DHT in one place.

It covers:

- what is implemented now
- what is planned next
- what each technical term means
- how the lookup works
- how public, private, and hidden services should work
- why the DHT scales better than "everyone stores everything"
- the main formulas behind storage, lookup cost, and routing size

This is a technical note, but it is written to stay readable.

## 1. Main Idea

The VX6 DHT is a distributed lookup system.

Its job is:

- help a node find another node
- help a node find a public service
- help a node find a hidden-service alias

It should **not** behave like:

- one giant global registry copied to every machine
- a broadcast where every node asks every other node

The scalable design is:

- each node stores only a small useful subset
- each lookup asks only a small number of peers at a time
- records are stored only on a small number of responsible nodes

## 2. Important Terms

### 2.1 DHT

`DHT` means `Distributed Hash Table`.

It is a decentralized key-value system where:

- the key is something like `service/acme.web`
- the value is the signed VX6 record for that service

### 2.2 Key

A `key` is the name used for lookup.

Examples:

- `node/name/alice`
- `node/id/vx6_abcd1234`
- `service/acme.web`
- `hidden/ghost`

### 2.3 Value

A `value` is the record stored for that key.

Usually this is:

- an endpoint record
- a service record
- a hidden service record

### 2.4 Hash

A `hash` turns a name into a fixed-size number.

Example idea:

- `service/acme.web` -> `H(service/acme.web)`

VX6 then uses that number to decide which nodes are "closest" to that key.

### 2.5 XOR distance

This is how Kademlia-like DHTs measure closeness.

If:

- `a` = hashed node id
- `b` = hashed key

then distance is:

`d(a, b) = a XOR b`

Smaller XOR distance means "closer."

This is not geographic distance.
It is not ping time.
It is logical keyspace distance.

### 2.6 Routing table

A `routing table` is the list of peers a node knows about for DHT routing.

It answers:

- "who should I ask next?"

It is not the same as:

- all users in the world
- all services in the world

### 2.7 Replica

A `replica` is one stored copy of a DHT record.

If replication factor is `r = 5`, then one record is stored on about `5` nodes.

### 2.8 Local registry / local cache

This is a convenience store on your own node.

It keeps:

- recently used records
- locally known peers
- previously resolved users/services

It is not the full DHT.

## 3. Current VX6 DHT: What Is Implemented Now

The current DHT is already working and hardened compared with the original version.

### 3.1 Current key families

Implemented now:

- `node/name/<nodeName>`
- `node/id/<nodeID>`
- `service/<nodeName.serviceName>`
- `hidden/<alias>`

### 3.2 Current routing table size

The routing table uses:

- `256` buckets
- `K = 20` active entries per bucket

So the maximum number of active routing peers is:

`ActivePeersMax = 256 * 20 = 5120`

The current replacement cache is also bounded at:

- `20` spare entries per bucket

So the spare capacity upper bound is:

`SparePeersMax = 256 * 20 = 5120`

That means one node does **not** track millions of peers.

### 3.3 Current lookup behavior

Current lookup settings:

- parallel fanout `alpha = 3`
- total query budget `beta = 12`

So in the current implementation:

- a node asks at most `3` peers in one round
- and at most about `12` peers total in a lookup

This is why lookups stay bounded.

### 3.4 Current confirmation rules

For trusted VX6 records, the DHT now requires:

- at least `2` exact matching supporting sources
- total confirmation weight at least `4`
- at least `2` network groups when possible

This is much safer than trusting the first answer.

### 3.5 Current replication

The current public replication factor is:

- `r = 5`

That means one service record is stored on about `5` nearby responsible nodes,
not on all nodes.

### 3.6 Current version and conflict behavior

The DHT now tracks:

- current accepted version
- previous accepted versions
- conflicting versions

And the rule is:

- newer signed record wins within the same family
- conflicting valid families fail safely instead of guessing

### 3.7 Current envelope behavior

Trusted VX6 values can now be stored in a signed DHT envelope that includes:

- key
- wrapped record
- origin node id
- publisher node id
- publisher public key
- version
- issue time
- expiry time
- observation time
- signature

This means VX6 can now reason about:

- whether the record itself is valid
- who published the DHT copy
- whether the copy is fresh

## 4. How Lookup Works Now

Suppose a client wants:

- `service/acme.web`

The flow is:

1. check local cache / local registry first
2. if not found, ask a few peers already in the routing table
3. each peer replies with:
   - the value if it has it
   - or peers that are closer to the key
4. the requester asks the next closest peers
5. stop when enough matching signed answers arrive

Important:

- the requester controls the lookup
- the network does **not** flood everyone
- the request does **not** go to all 20 million nodes

## 5. Why "Closer" Works

Each node has an ID.
Each lookup key is hashed.

If:

- `n` = node id hash
- `k` = key hash

then:

`DistanceToKey = n XOR k`

A node that has a smaller `DistanceToKey` is considered closer to that key.

So each peer can say:

- "here are the closest peers I know to that key"

That is how the search moves through the network.

## 6. Public Service Model

This is the right scalable model for public services.

### 6.1 Public publish

When someone publishes a public service:

1. create a signed service record
2. compute the service key hash
3. store the record on a small set of nearby responsible nodes
4. keep refreshing it before expiry

### 6.2 Public lookup

When someone wants a public service:

1. check local cache
2. if not found, start DHT lookup
3. walk toward nodes closer to the key
4. accept the result only when enough matching replies confirm it
5. cache the result locally

### 6.3 Public storage cost

Let:

- `N` = number of live nodes
- `M_pub` = number of public service records
- `r_pub` = replication factor for public services

Then the average number of public service records stored per node is:

`AvgPublicRecordsPerNode = (M_pub * r_pub) / N`

Example:

- `N = 20,000,000`
- `M_pub = 20,000,000`
- `r_pub = 5`

Then:

`AvgPublicRecordsPerNode = (20,000,000 * 5) / 20,000,000 = 5`

So on average, each node stores about `5` public records from the global DHT
responsibility set.

This is why the design scales much better than global replication.

## 7. Private Service Model: Planned Simpler Version

This part is the simplified plan we discussed.
It is **not fully implemented as described below yet**.

### 7.1 Goal

Private services should:

- not appear in public global search
- appear only when someone explicitly checks a specific user

### 7.2 Simple model

Public search:

- only public services are returned

Per-user search:

- if someone checks `user/alice/services`
- they can see Alice's service list
- that list may include Alice's private services

This is simple and practical.

Important honesty:

- this is not strong access control
- it is "hidden from global listing"
- it is not "cryptographically invisible"

### 7.3 Planned per-user flow

If `bob` wants to inspect Alice:

1. resolve Alice's node record
2. ask for Alice's per-user service catalog
3. show services under Alice only

That means:

- private services are not globally indexed
- they are only visible when the user is explicitly queried

## 8. Hidden Service Model: Planned Lookup Shape

This is also part of the planned search model.

### 8.1 Hidden service naming

Hidden services should use exact alias lookup, for example:

- `hidden/ghost`
- or a stronger alias like `hidden/h-ghost`

### 8.2 Hidden service visibility

Hidden aliases should:

- not appear in a global public listing
- resolve only when the exact alias is known

This is similar to how many websites are practically discovered:

- not from a universal public list
- but because someone knows the name and searches for it

Important honesty:

- exact-alias-only lookup is not the same as secrecy
- DHT holders for that key can still know the alias record exists

## 9. Why We Should Not Store Everything Everywhere

A naive design would say:

- every node stores every public service
- every node stores every user

That fails for:

- bandwidth
- storage
- churn
- refresh traffic
- privacy

Let:

- `N = 20,000,000` nodes
- `M = 20,000,000` public records
- `S = average bytes per record`

If everyone stored everything:

`TotalBytesPerNode = M * S`

If `S = 1 KB`, then:

`TotalBytesPerNode = 20,000,000 KB ≈ 20 GB`

per node, just for one record family.

That is clearly wrong for a lightweight peer node.

With bounded replication:

`TotalBytesPerNodeAvg = (M * r * S) / N`

If:

- `M = 20,000,000`
- `r = 5`
- `S = 1 KB`
- `N = 20,000,000`

then:

`TotalBytesPerNodeAvg = (20,000,000 * 5 * 1 KB) / 20,000,000 = 5 KB`

That is the scaling advantage of DHT-style responsibility storage.

## 10. RAM Versus Disk

You specifically wanted big datasets not to sit in RAM forever.

That is the correct direction.

### 10.1 What should stay in RAM

Best items to keep in RAM:

- routing table
- replacement cache
- current live lookups
- hot recent-record cache

These are small and performance-sensitive.

### 10.2 What should go to disk

Best items to keep on disk:

- local registry cache
- older resolved public services
- per-user service catalogs
- DHT records this node is responsible for
- expiry metadata
- version/conflict history

### 10.3 Current honest status

Today:

- the routing table is in RAM
- the DHT value map is in RAM
- the local registry already has a disk-backed JSON path

So the next storage improvement is:

- move the main DHT responsibility store to disk-backed storage
- keep only hot indexes and current lookups in RAM

## 11. Exact Numbers To Keep The System Light

These are practical target numbers.

### 11.1 Current implemented routing numbers

- active routing peers max: `5120`
- spare routing peers max: `5120`
- lookup fanout: `3`
- lookup query budget: `12`
- public replica count: `5`

### 11.2 Good next resource targets

Recommended hot RAM cache:

- `1,000` to `5,000` recent records

Recommended disk cache:

- `50,000` to `200,000` recent records

These are operational targets, not current enforced limits.

## 12. Lookup Time Intuition

The DHT lookup cost is not linear in total users.

It should behave much closer to logarithmic growth:

`LookupSteps ≈ O(log N)`

In practice, VX6 caps it with a query budget.

Current rough upper bound:

`MaxQueriedNodes = beta = 12`

If median round-trip time to peers is `RTT`, then rough lookup wall time is:

`LookupTime ≈ LookupRounds * RTT + VerificationCost`

With:

- `alpha = 3`
- `beta = 12`

the number of rounds is at most about:

`LookupRoundsMax ≈ beta / alpha = 12 / 3 = 4`

So even with strict confirmation, the lookup stays bounded.

## 13. Why Requester-Controlled Lookup Is Better Than Flood Search

We discussed an alternative idea:

- node A asks B
- B asks others
- those ask others
- when answer is found, everyone sends stop packets backwards

That sounds efficient at first, but it has problems:

- too much temporary state on intermediate nodes
- harder cleanup logic
- cancel packets can be lost
- attackers can abuse search trees
- more bandwidth under bad conditions

The better model is:

- requester-controlled iterative search
- each node only replies with:
  - value if known
  - closer peers otherwise

This is simpler, safer, and easier to bound.

## 14. Planned Public / Private / Hidden Separation

### 14.1 Public

Public services should:

- be globally discoverable by exact key
- be stored on nearby responsible nodes only
- be locally cached after successful lookup

### 14.2 Private

Private services in the simple model should:

- not appear in public global search
- appear only when a specific user's services are requested

### 14.3 Hidden

Hidden services should:

- use exact alias lookup
- not appear in global listings
- resolve only for people who know the alias

## 15. Optional Search Engines On Top

The base DHT should stay simple:

- exact lookup
- signed records
- bounded replication

If someone wants "Google-like" behavior:

- they can run an indexer node
- that node can crawl public records
- then expose a search service over VX6

That gives:

- decentralized core
- optional search engines
- no need for the core DHT to behave like a heavy full-text search system

## 16. Suggested Final Architecture

### 16.1 Current implemented core

- Kademlia-like XOR routing
- bounded routing table
- replacement cache
- failure eviction
- signed VX6 records
- signed DHT envelopes
- strict multi-source confirmation
- bounded replication

### 16.2 Next storage evolution

- keep routing and hot cache in RAM
- move DHT responsibility storage to disk
- keep local registry and service cache on disk

### 16.3 Next discovery evolution

- public exact-name DHT lookup
- private per-user catalog lookup
- hidden alias-only lookup
- optional higher-layer search/index services

## 17. Summary Formulas

### Routing table size

`ActivePeersMax = BucketCount * K`

Current:

`ActivePeersMax = 256 * 20 = 5120`

### Replacement cache size

`SparePeersMax = BucketCount * ReplacementPerBucket`

Current:

`SparePeersMax = 256 * 20 = 5120`

### Average DHT responsibility storage per node

`AvgRecordsPerNode = (TotalRecords * ReplicationFactor) / LiveNodes`

### Approximate per-node storage bytes

`AvgStorageBytesPerNode = AvgRecordsPerNode * AvgRecordBytes`

### Approximate lookup wall time

`LookupTime ≈ LookupRounds * RTT + VerifyCost`

### Max lookup rounds from configured budget

`LookupRoundsMax ≈ QueryBudget / Fanout`

Current:

`LookupRoundsMax ≈ 12 / 3 = 4`

## 18. Final Honest Status

What VX6 DHT already does well:

- bounded routing
- no global replication of everything
- safe record verification
- stricter trust than first-answer DHTs
- practical scaling direction for millions of nodes

What is still planned:

- disk-backed DHT responsibility storage
- cleaner public/private/hidden discovery split
- optional search-index services above the DHT
- stronger Sybil and Eclipse resistance later

So the design direction is:

- global discovery without global storage
- bounded RAM usage
- disk-backed long-lived cache/storage
- exact lookup in the core
- optional richer search on top
