# VX6 Windows Porting Guide

## Purpose

This document explains the current VX6 architecture in implementation terms so a
Windows team can build on the same protocol instead of inventing a second one.

The key rule is:

- the VX6 network protocol stays shared across Linux, Windows, and macOS
- only runtime integration, packaging, and fast-path differences should become
  platform-specific

## Current VX6 Architecture

VX6 is a service-oriented peer-to-peer network, not a full layer-3 VPN.

The default model is:

```text
local app -> local VX6 -> VX6 network -> remote VX6 -> remote local app
```

The important consequence is:

- the real application can stay on `127.0.0.1`
- the remote user also connects from `127.0.0.1`
- VX6 carries the bytes between those endpoints

That gives a simpler user and operator model than exposing raw public service
ports or provisioning full virtual subnets for one service.

## Core Modes

VX6 currently has three major network modes.

### 1. Direct Mode

Flow:

```text
client app -> local VX6 -> secure direct VX6 session -> remote VX6 -> localhost target
```

Properties:

- fastest path
- endpoint IPs are visible to each other
- payload is encrypted
- no anonymity claim

### 2. Relay Mode

Flow:

```text
client app -> local VX6 -> relays -> remote VX6 -> localhost target
```

Properties:

- useful when direct path is undesirable or unavailable
- more latency than direct mode
- distributes traffic through selected relays

### 3. Hidden Mode

Flow:

```text
client -> client guard -> middle -> rendezvous <- middle <- owner guard <- owner
```

Properties:

- client and owner do not directly learn each other’s IP
- rendezvous sees only adjacent relays
- first guard on each side still sees that side’s source IP
- slower than direct mode

## Protocol Layers

VX6 currently uses multiple layers of protection.

### Layer A: Identity Layer

Files:

- `internal/identity/store.go`

Current primitive:

- `Ed25519`

Use:

- node identity
- service signing
- handshake identity verification
- circuit setup signatures

Meaning:

- every node has a long-term identity keypair
- this is not the same thing as the short-lived transport or circuit keys

### Layer B: Neighbor Secure Session

Files:

- `internal/secure/session.go`

Current primitives:

- `X25519` for ephemeral key agreement
- `Ed25519` for authenticating the peer identity
- `AES-GCM` for direct neighbor transport encryption
- `SHA-256` for session key derivation

How it works:

1. each side generates a fresh X25519 ephemeral key
2. each side sends a signed hello that binds:
   - protocol kind
   - node ID
   - ephemeral key
3. each side verifies the Ed25519 signature
4. both derive the shared secret with X25519
5. both derive the transport key
6. all future bytes on that direct socket are encrypted with AES-GCM

What this protects:

- raw TCP bytes between two directly connected VX6 nodes

What it does not protect:

- source IP from the directly connected neighbor
- destination IP of that neighbor
- traffic timing and size metadata

This layer is used in:

- direct VX6 sessions
- relay extension links
- hidden control paths

### Layer C: Hidden Control Protection

Files:

- `internal/hidden/hidden.go`

Current mechanism:

- hidden control rides on secure VX6 connections
- messages are authenticated to the secure peer identity
- messages include:
  - `sender_node_id`
  - `epoch`
  - `nonce`

Current timing model:

- control epoch: `30s`
- lease duration: `30s`
- ping interval: `10s`

Purpose:

- stop raw plaintext hidden-control messages
- reject replayed control messages
- keep intro/guard registrations alive and renewable

### Layer D: Onion Circuit Layer

Files:

- `internal/onion/cell.go`
- `internal/onion/onion.go`
- `internal/onion/circuit.go`

Current primitives:

- `X25519` per hop
- `Ed25519` signature on `CREATED` payload
- `HKDF-SHA256` for per-hop forward/backward key derivation
- `AES-GCM` for per-hop relay-layer encryption

Current cell model:

- fixed-size `1024` byte cells
- forward and backward directions
- layered relay commands inside the cell payload

Current command families:

- `CREATE`
- `CREATED`
- `RELAY`
- relay subcommands:
  - `EXTEND`
  - `EXTENDED`
  - `BEGIN`
  - `CONNECTED`
  - `DATA`
  - `END`
  - `ERROR`

How it works:

1. client chooses relay path
2. client opens secure session to first hop
3. client sends `CREATE`
4. first relay returns signed `CREATED`
5. both derive per-hop keys
6. client sends layered `EXTEND` for next hop
7. process repeats until full circuit exists
8. client sends layered `BEGIN`
9. stream bytes flow as layered `DATA` cells

What each relay should know:

- previous hop
- next hop
- its local circuit state

What a relay should not know unless it is the exit:

- final destination
- entire route
- original sender beyond its own direct peer

## What Is Encrypted Where

This is the simplest accurate summary.

### Direct Mode

Encrypted:

- all VX6 bytes between the two directly connected VX6 nodes

Not hidden:

- each node can see the other node’s IP
- network path can see which two VX6 nodes are communicating

### Relay Mode

Encrypted:

- each direct VX6-to-VX6 neighbor link
- relay instructions inside onion cells

Visible:

- first hop sees the source IP of the peer that connected to it
- exit hop sees the final target it must open
- middle relays do not see the final target

### Hidden Mode

Encrypted:

- neighbor VX6 links
- hidden control messages
- layered relay instructions
- service stream through the VX6 path

Visible:

- first guard on each side sees that side’s source IP
- rendezvous sees only adjacent relays
- client and owner should not directly see each other’s IP

## Windows-Relevant Split: Shared vs Platform-Specific

### Shared Cross-Platform Core

These should stay shared:

- `internal/identity`
- `internal/secure`
- `internal/onion`
- `internal/hidden`
- `internal/serviceproxy`
- `internal/discovery`
- `internal/dht`
- `internal/record`
- most of `internal/node`

Reason:

- these implement protocol and network behavior
- Windows should not fork the protocol

### Platform-Specific Runtime Layer

These are the areas that need Windows adaptation:

- process lifecycle
- service manager integration
- lock handling
- default paths
- firewall integration
- packaging
- secure local storage options

## Linux eBPF vs Windows

### What eBPF Means In VX6 Today

Files:

- `internal/ebpf/onion_relay.c`
- `internal/onion/ebpf.go`
- embedded object: `internal/onion/onion_relay.o`

Current reality:

- the binary embeds Linux eBPF bytecode
- `IsEBPFAvailable()` currently only reports whether the bytecode is present
- eBPF is a Linux fast-path and future kernel acceleration story
- it is not the core security boundary of VX6

### What eBPF Is Responsible For

eBPF should only be treated as:

- fast classification
- low-overhead relay help
- kernel-side packet decision support

It should not be treated as:

- the source of VX6 encryption
- the source of VX6 authentication
- required for correctness

### Windows Equivalent Strategy

Do not try to “replace eBPF” one-for-one in the first Windows phase.

Windows should instead do:

- correct userspace transport
- correct userspace relay routing
- correct resource governor behavior
- efficient socket and goroutine management

Then later, if needed, evaluate:

- IOCP tuning
- socket buffer tuning
- dual-stack listener tuning
- optional WFP-based policy integration

But those are performance and integration topics, not protocol prerequisites.

## Exact Windows Port Tasks

### 1. Entry and Process Lifecycle

Current Unix-shaped code:

- `cmd/vx6/main.go` uses `os/signal` with `SIGTERM`
- `internal/cli/app.go` still listens for `SIGHUP` in `vx6 node`
- `vx6 reload` falls back to `SIGHUP` if the local control channel is unavailable

Windows work:

- keep the local runtime control channel as the primary reload/status path
- remove Windows dependence on Unix signals entirely
- later integrate with Windows Service Control Manager

Goal:

- `vx6 reload` should work through the runtime control API only on Windows

### 2. Runtime Locking

Current Unix-shaped code:

- `internal/cli/app.go` uses `syscall.Flock`

Windows work:

- replace that with a portable lock abstraction
- implement:
  - Linux/macOS: current file locking or a wrapped equivalent
  - Windows: Win32-compatible file lock or single-instance lock file strategy

Goal:

- one node instance per config
- no Unix syscall leakage into the shared CLI path

### 3. Default Paths

Current defaults are POSIX-oriented:

- config under `~/.config/vx6`
- data under `~/.local/share/vx6`
- downloads under `~/Downloads`

Windows work:

- introduce OS-aware path resolution
- likely map to:
  - config: `%AppData%\\vx6` or `%LocalAppData%\\vx6`
  - data: `%LocalAppData%\\vx6\\data`
  - downloads: Windows known Downloads folder if available

Goal:

- no Linux path assumptions in shared config code

### 4. Service Management

Linux today:

- `deployments/systemd/vx6.service`

Windows work:

- build a service wrapper or SCM integration
- define lifecycle:
  - install service
  - start service
  - stop service
  - query status
  - forward reload via local control API

Goal:

- no user has to keep `vx6 node` open in a console forever

### 5. Firewall Integration

Windows work:

- document or automate inbound allow rules for the VX6 listener
- keep localhost-only service targets untouched

Goal:

- allow VX6 node reachability
- avoid asking users to open every application port

### 6. Transport Behavior

Files:

- `internal/transport/transport.go`

Current state:

- `auto`, `tcp`, `quic` modes exist at config level
- effective transport is still TCP
- listener and dialer are currently `tcp6`

Windows work:

- validate `tcp6` behavior on Windows carefully
- ensure dual-stack and IPv6-only behavior are predictable
- keep transport code shared
- do not fork protocol behavior by OS

Goal:

- Windows should join the same network with the same session and onion logic

### 7. Resource Governance

Files:

- `internal/node/relay_governor.go`

Current behavior:

- relay work is capped by configured percentage
- default is `33%`
- capacity is derived from `GOMAXPROCS`

Windows work:

- preserve the same model
- later enrich with memory, socket, and latency-aware backpressure if needed

Goal:

- Windows nodes can relay
- user machine stays responsive

## Recommended Windows Code Split

Introduce a platform adapter layer instead of scattering OS checks everywhere.

Suggested packages:

- `internal/platform/runtime`
  - process control
  - single-instance lock
  - service hooks

- `internal/platform/paths`
  - config/data/runtime path selection

- `internal/platform/firewall`
  - optional firewall integration

- `internal/platform/service`
  - SCM or service manager wrappers

This keeps:

- protocol code stable
- OS conditionals out of security-critical paths

## What The Windows Team Must Not Change

Do not change these just to make the port easier:

- node identity model
- X25519 + Ed25519 + AES-GCM secure session model
- HKDF-derived onion hop keys
- hidden control epoch/nonce validation model
- fixed-size onion cells
- relay governor semantics

If those change on Windows, you risk protocol divergence.

## Validation Targets For Windows

The Windows port should prove the same behavior as Linux in these cases:

1. node starts and stays single-instance
2. `vx6 status` works through local control
3. `vx6 reload` works without Unix signals
4. direct service connect works
5. relay path works
6. hidden rendezvous works
7. layered relay visibility remains unchanged
8. file receive permissions still behave correctly

Minimum parity test matrix:

- Windows client -> Linux service owner
- Linux client -> Windows service owner
- Windows relay in middle of hidden path
- mixed relay set with Windows and Linux nodes

## Current Bottom Line For Windows

Windows can be built on the current architecture.

Why:

- the cryptographic core is userspace and cross-platform
- the relay and hidden logic are userspace and cross-platform
- Linux eBPF is optional acceleration, not correctness
- the main remaining Windows work is runtime and OS integration

So the Windows phase should be treated as:

- a platform adaptation project

not:

- a protocol redesign project
