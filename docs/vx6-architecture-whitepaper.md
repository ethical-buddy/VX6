# VX6 Architecture Whitepaper

Version:
- current implementation as of commit `a0b9a94`

Status:
- working prototype with implemented direct, relay, and hidden-service paths
- cryptographic and routing architecture is in place
- remaining work is mostly hardening, failover, abuse control, and later transport optimization

## Abstract

VX6 is an IPv6-first peer-to-peer service network built around a simpler idea than most overlay VPNs:

- applications stay on `127.0.0.1`
- users connect from their own `127.0.0.1`
- VX6 carries the stream between peers

The system supports three major access styles:

1. direct encrypted peer-to-peer service access
2. multi-hop relay access
3. hidden rendezvous access with layered relay encryption

Unlike a full layer-3 VPN, VX6 is service-oriented. Instead of exposing a whole virtual subnet by default, it publishes named services and hidden aliases while keeping the actual application ports local to each machine.

## Problem Statement

Most software is easiest to run on localhost, but hard to share safely across machines.

Common alternatives each have tradeoffs:

- public service exposure is simple but expands attack surface
- VPN overlays expose whole hosts or subnets when only one service is needed
- anonymity systems are powerful but often too slow or operationally heavy for normal internal services

VX6 tries to sit between those models:

- keep apps local
- make them reachable by name
- allow direct fast paths when possible
- allow relays when needed
- allow hidden access when endpoint privacy matters more than raw speed

## Design Goals

VX6 is designed around these goals:

1. localhost-to-localhost service access
2. direct encrypted connectivity when possible
3. named service discovery without a permanent central traffic hub
4. optional relay and hidden modes without changing the application
5. IPv6-first addressing and routing
6. user-facing simplicity: names and aliases instead of raw endpoint handling
7. support for all major desktop/server platforms, with Linux retaining the best fast-path options

## Non-Goals

VX6 is not currently trying to be:

- a full Tor replacement
- a full layer-3 enterprise SD-WAN
- a censorship-resistant mixnet
- a fully anonymous network against a global passive adversary

The current hidden mode aims to reduce endpoint exposure and limit what relays learn. It does not claim Tor-grade anonymity.

## High-Level Model

The core model is:

```text
local app -> local VX6 -> VX6 network -> remote VX6 -> remote local app
```

Direct mode:

```text
client app -> 127.0.0.1:port -> VX6 ==encrypted direct path==> VX6 -> 127.0.0.1:target
```

Relay mode:

```text
client app -> local VX6 -> relay chain -> remote VX6 -> target app
```

Hidden mode:

```text
client -> guard -> middle -> rendezvous <- middle <- guard <- owner
```

## System Components

### 1. Identity

Each VX6 node has a long-term identity.

Current implementation:
- `Ed25519` identity keys
- identity generation and storage in `internal/identity`

Role:
- authenticate nodes
- sign service records and control material
- bind secure sessions and relay circuits to node identity

### 2. Discovery and Naming

VX6 uses signed peer and service records so peers can resolve:

- nodes by name
- services like `alice.ssh`
- hidden aliases

The bootstrap node is only the first known live node, not the permanent center of the traffic path.

### 3. Local Service Proxying

VX6 does not require the real application to listen publicly.

Instead:
- the owner service stays on localhost
- the client also uses localhost
- VX6 proxies the stream between those points

This is the main reason VX6 feels simpler than layer-3 overlays for service publishing.

### 4. Runtime Control Plane

VX6 now exposes a local runtime control channel instead of relying only on Unix signals.

That gives a common base for:
- Linux
- Windows
- macOS

This is important because the transport and node runtime no longer need to assume `SIGHUP`-style control forever.

### 5. Relay Governor

VX6 includes a relay resource governor so relaying does not overwhelm the user’s own machine.

Current default policy:
- relay participation enabled
- relay work capped by a resource percentage

This matters because VX6 is designed so any node can participate in relaying, not only Linux servers.

## Transport and Session Security

### Neighbor Session Layer

Every direct VX6-to-VX6 TCP link is protected by the secure session layer.

Current cryptographic stack:
- `X25519` for ephemeral key agreement
- `Ed25519` for identity verification
- `AES-GCM` for encrypted transport
- `SHA-256` or `HKDF-SHA256` in key derivation paths

Purpose:
- protect bytes on the direct socket between neighboring VX6 nodes
- prevent intermediate network devices or ISPs from reading payload bytes

What this does not hide:
- source IP to first-hop peer
- destination IP of the directly connected peer
- timing and packet size side channels

### Why VX6 Does Not Depend On Raw IPv6 Security

IPv6 does not automatically make application traffic private.

VX6 security comes from the VX6 protocol itself:
- authenticated key exchange
- authenticated encryption
- signed identities
- circuit-layer encryption in relay mode

## Service Access Modes

### Mode 1: Direct

How it works:
- client resolves target node or service
- client opens a secure VX6 session to the target node
- target VX6 opens a localhost connection to the real service

Benefits:
- fastest mode
- simplest path
- low CPU overhead
- good for normal admin, API, chat, and dashboard use

Tradeoffs:
- the remote VX6 node knows the client IP
- the network can see which VX6 node is being contacted
- not anonymous

### Mode 2: Relay

How it works:
- the client reaches the service through selected relays
- relay path can be direct proxying or onion-style circuit usage depending on the flow

Benefits:
- allows connectivity when direct path is not desirable or not available
- reduces direct exposure of endpoints
- can distribute forwarding work

Tradeoffs:
- more latency than direct mode
- more state, more moving parts

### Mode 3: Hidden

How it works:
- service is published by hidden alias
- owner maintains intro/guard registrations
- client resolves alias and uses intro and rendezvous flow
- both sides build relay paths toward a rendezvous node

Goal:
- owner IP hidden from client
- client IP hidden from owner
- rendezvous sees only adjacent relays

Current trust reality:
- a node always exposes its IP to its first directly connected hop
- therefore the first guard remains the most sensitive trust point on each side

## Hidden-Service Control Architecture

VX6 has already replaced the older direct owner-address model with callback-style guard registration.

That means:
- guards no longer need to store a raw `ownerAddr` as the core registration primitive
- owners maintain callback control channels to guards
- intro and guard leases are rotated and rebuilt
- hidden control messages carry sender identity, epoch, and nonce protections

This closes a major privacy gap from the earlier design.

## Onion Relay Architecture

### Why VX6 Added Onion Cells

Earlier relay extension designs exposed too much routing metadata in plaintext.

The current design moves relay traffic to:
- fixed-size cells
- layered per-hop encryption
- explicit forward and backward circuit directions

### Core Idea

The relay path uses two protection layers:

1. outer neighbor secure sessions
2. inner onion cells for circuit-layer routing

This means:
- networks and ISPs cannot read direct VX6 socket payloads
- earlier relays cannot see deeper routing instructions

### Current Cell Design

Implemented in `internal/onion/cell.go`.

Conceptually:
- the client builds a circuit hop by hop
- each hop gets its own ephemeral shared secret
- that shared secret derives forward and backward keys
- relay commands are packed into fixed-size cells
- each hop peels only its own forward layer
- backward responses gain only the backward layer for that hop

Current relay command families include:
- `CREATE`
- `CREATED`
- `EXTEND`
- `BEGIN`
- `DATA`
- `END`

### Circuit Build

The client-side builder is in `internal/onion/onion.go`.

The flow is:

1. open secure connection to first hop
2. send `CREATE`
3. receive signed `CREATED`
4. derive hop keys
5. send layered `EXTEND` to next hop
6. repeat until full path exists
7. send layered `BEGIN`
8. carry service bytes as `DATA` cells

### Relay Processing

Relay-side processing is in `internal/onion/circuit.go`.

Each relay does only three important things:

1. decrypt one forward layer
2. decide whether the cell is for itself
3. either process it or forward the still-protected inner payload

That means a middle relay should not learn:
- the original sender
- the final target
- the entire route

Only the exit hop learns the target needed for local delivery.

## What Has Been Verified

VX6 is not only documented; the important parts were tested live.

The most important verification pieces are:

- direct multi-node service and proxy path tests
- hidden rendezvous integration tests
- live relay visibility inspection tests
- unit tests for cell wrapping and unwrapping
- replay and epoch checks for hidden control

The custom relay visibility test proved that:
- first hop saw only its own next hop
- middle hop saw only its own next hop
- only the exit hop saw the final target

That is the strongest current proof that the layered relay boundary is functioning correctly in the implemented system.

## Current Security Properties

### What VX6 Hides Well Right Now

- service payloads on neighbor VX6 links
- deeper relay instructions from earlier relays
- owner IP from the client in hidden mode
- client IP from the owner in hidden mode
- casual user-facing exposure of raw peer/service addresses in normal CLI surfaces

### What VX6 Does Not Fully Hide

- the client IP from the first client-side guard
- the owner IP from the first owner-side guard
- that a VX6 connection exists at all
- traffic timing and packet size patterns
- the exit target from the exit hop

### What Still Needs Hardening

The architecture is largely in place, but not fully hardened.

Remaining major hardening items:

1. active hidden-circuit failover when a relay dies mid-session
2. intro and rendezvous abuse limits
3. circuit setup throttling and broader DoS protection
4. more transport/session reuse optimization
5. later QUIC transport if adopted

That means the right description today is:

- working
- architecturally coherent
- verified as a functioning prototype
- not yet fully production-hardened

## Is The Architecture Ready?

Yes, with an important qualifier.

The current VX6 architecture is ready in the sense that:
- the direct path works
- the relay path works
- hidden rendezvous works
- the encrypted cell layer works
- the layered hop processing works
- the benchmark harness exists
- the main remaining work is hardening and optimization rather than fundamental protocol invention

The qualifier is that this is still better described as:
- an advanced prototype

not yet:
- a mature production anonymity network

So the straight answer is:

- yes, the architecture works
- yes, it is already a real system
- no, it is not “done forever”
- the main unfinished work is resilience and abuse hardening, not core design

## Related Work

VX6 sits near several existing families, but does not exactly match any one of them.

### Tor

Closest overlap:
- rendezvous-based hidden services
- relay cells
- onion-style layered forwarding

Difference:
- Tor is built around anonymity first
- VX6 is built around localhost service networking first, with hidden mode as one operating mode

### I2P

Closest overlap:
- layered routed privacy network
- tunnel-based design
- hidden-service style thinking

Difference:
- I2P is a broader anonymity network with garlic routing and unidirectional tunnels
- VX6 is simpler and service-oriented, not a general anonymous Internet replacement

### Tailscale

Closest overlap:
- direct encrypted connectivity
- relay fallback
- peer relay concept

Difference:
- Tailscale is a WireGuard-based device network with coordination and relay infrastructure
- VX6 is service-oriented and explicitly built around localhost-to-localhost publication and hidden aliases

### NetBird

Closest overlap:
- direct encrypted connectivity
- relay service
- cross-platform overlay operation

Difference:
- NetBird is a managed layer-3 style private network using WireGuard, management, signal, and relay services
- VX6 focuses on named services and hidden access patterns rather than full network overlay management

### Nebula

Closest overlap:
- encrypted peer-to-peer overlay
- lighthouse-assisted discovery
- optional relays

Difference:
- Nebula is a layer-3 overlay with host certificates and virtual network semantics
- VX6 is application-service-first, not TUN-first

### ZeroTier

Closest overlap:
- peer-to-peer virtual networking
- relay fallback when direct connectivity is blocked

Difference:
- ZeroTier is a virtual network platform
- VX6 is closer to a service publication and hidden-access system

### Yggdrasil

Closest overlap:
- encrypted IPv6-oriented overlay mindset

Difference:
- Yggdrasil is an encrypted IPv6 mesh where nodes are routable and anonymity is not the goal
- VX6 tries to keep actual apps local and selectively expose services

## What Is Actually Unique In VX6

The novelty in VX6 is not that each primitive is new.

The stronger claim is this combination:

1. localhost-to-localhost service networking as the default mental model
2. service-level publication instead of full host/subnet exposure
3. direct, relay, and hidden modes in one stack
4. IPv6-first peer naming and service discovery
5. peer-operated relay participation with a local resource governor
6. onion-style hidden service routing applied to a service overlay rather than a whole browsing network

That combination is distinctive.

What is not unique by itself:
- onion routing
- rendezvous points
- encrypted overlay networking
- peer relays
- public-key identity

So VX6 is:
- not a completely new category
- but also not just a clone of one existing project

## Can VX6 Be Published In A Big Conference?

Potentially yes, but not just because the code exists.

Based on current venue scopes, VX6 is in range for systems/security/networking venues if framed correctly:
- security/privacy systems venues
- networking architecture venues
- deployment-experience venues

However, a top-tier conference paper would need more than “we built a thing.”

For a serious submission, VX6 would need:

1. a crisp threat model
2. a precise claim of novelty
3. formal comparison against Tor, Tailscale, Nebula, NetBird, ZeroTier, and Yggdrasil where relevant
4. WAN-scale evaluation, not only local or synthetic integration runs
5. failure analysis under churn and attack
6. clear discussion of what hidden mode does and does not protect
7. reproducible artifacts and benchmark/evaluation tooling

Strong paper angles could be:

- a systems paper on service-oriented hidden overlays
- a deployment paper on localhost-to-localhost peer service networking
- a security paper on balancing service usability and endpoint privacy

Weak paper angle:
- claiming onion routing itself as the novelty

That would not survive comparison with Tor and I2P.

My honest assessment:

- publishable as an idea: yes
- publishable at top-tier today as-is: probably no
- publishable after stronger evaluation and sharper framing: yes, realistically possible

## Benchmark Methodology

Benchmark harness files:

- `internal/identity/identity_bench_test.go`
- `internal/secure/session_bench_test.go`
- `internal/onion/bench_test.go`

The benchmark report is in:

- `docs/encrypted-onion-benchmark-report.md`

### What The Benchmarks Measure

Identity benchmark:
- long-term `Ed25519` identity generation cost

Secure benchmarks:
- full VX6 neighbor handshake cost
- 4 KB AES-GCM seal and open costs

Onion benchmarks:
- ephemeral circuit key generation
- single-hop onion setup
- layered 3-hop payload wrapping and unwrapping
- fixed-size cell encode/decode cost

### How The Go Benchmark Harness Works

The benchmark commands use:

```bash
go test -run '^$' -bench 'PATTERN' -benchmem ./package
```

Meaning:
- `-run '^$'` disables normal tests
- `-bench` selects benchmark functions
- `-benchmem` prints heap allocation statistics

The reported fields mean:

- `ns/op`: average time per benchmark operation
- `B/op`: average bytes allocated per operation
- `allocs/op`: average heap allocations per operation

These are operation-level costs, not full process RAM usage.

### How To Reproduce The Benchmarks

If your normal Go cache works:

```bash
go test -run '^$' -bench 'BenchmarkIdentityGenerate$' -benchmem ./internal/identity
go test -run '^$' -bench 'BenchmarkSecureHandshake$' -benchmem ./internal/secure
go test -run '^$' -bench 'BenchmarkSecureChunk(RoundTrip|Seal|Open)4K$' -benchmem ./internal/secure
go test -run '^$' -bench 'BenchmarkOnion' -benchmem ./internal/onion
```

If your environment has restricted default Go caches, use temporary caches:

```bash
GOCACHE=/tmp/vx6-go-build GOMODCACHE=/tmp/vx6-go-mod go test -run '^$' -bench 'BenchmarkIdentityGenerate$' -benchmem ./internal/identity
GOCACHE=/tmp/vx6-go-build GOMODCACHE=/tmp/vx6-go-mod go test -run '^$' -bench 'BenchmarkSecureHandshake$' -benchmem ./internal/secure
GOCACHE=/tmp/vx6-go-build GOMODCACHE=/tmp/vx6-go-mod go test -run '^$' -bench 'BenchmarkSecureChunk(RoundTrip|Seal|Open)4K$' -benchmem ./internal/secure
GOCACHE=/tmp/vx6-go-build GOMODCACHE=/tmp/vx6-go-mod go test -run '^$' -bench 'BenchmarkOnion' -benchmem ./internal/onion
```

### How To Reproduce Functional Verification

Run the full suite:

```bash
GOCACHE=/tmp/vx6-go-build GOMODCACHE=/tmp/vx6-go-mod go test ./...
```

Run the key integration checks:

```bash
GOCACHE=/tmp/vx6-go-build GOMODCACHE=/tmp/vx6-go-mod go test ./internal/integration -run TestSixteenNodeSwarmServiceAndProxy -v
GOCACHE=/tmp/vx6-go-build GOMODCACHE=/tmp/vx6-go-mod go test ./internal/integration -run TestHiddenServiceRendezvousPlainTCP -v
GOCACHE=/tmp/vx6-go-build GOMODCACHE=/tmp/vx6-go-mod go test ./internal/integration -run TestOnionRelayInspectVisibility -v
```

What to look for:

- direct and relay traffic still succeeds
- hidden rendezvous still succeeds
- visibility analysis still shows only the exit hop learning the final target

## Current Bottom Line

VX6 is already a working architecture, not only a sketch.

The right summary today is:

- the protocol path is real
- the encryption path is real
- the layered relay path is real
- the system has benchmark and integration coverage
- the remaining work is mostly hardening and production maturity

That is a solid place to be.

## External Positioning References

Official references useful for comparing VX6 to existing systems:

- Tor rendezvous protocol:
  - `https://spec.torproject.org/rend-spec/rendezvous-protocol.html`
- Tor relay cells:
  - `https://spec.torproject.org/tor-spec/relay-cells.html`
- Tailscale DERP and peer relays:
  - `https://tailscale.com/docs/reference/derp-servers`
  - `https://tailscale.com/docs/features/peer-relay`
- NetBird architecture:
  - `https://docs.netbird.io/about-netbird/how-netbird-works`
- Nebula overview and relays:
  - `https://nebula.defined.net/docs/`
  - `https://nebula.defined.net/docs/config/relay/`
- Yggdrasil FAQ:
  - `https://yggdrasil-network.github.io/faq.html`
- I2P garlic routing:
  - `https://geti2p.net/en/docs/how/garlic-routing`
- ZeroTier TCP relay:
  - `https://docs.zerotier.com/relay/`
- USENIX Security 2026 CFP:
  - `https://www.usenix.org/conference/usenixsecurity26/call-for-papers`
- SIGCOMM 2026 CFP:
  - `https://conferences.sigcomm.org/sigcomm/2026/cfp/`
