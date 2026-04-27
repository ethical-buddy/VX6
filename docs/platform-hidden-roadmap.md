# VX6 Cross-Platform and Hidden-Mode Roadmap

## Goals

1. Keep `direct` mode fast enough for daily use on Linux, Windows, and macOS.
2. Keep Linux as the high-performance relay tier with eBPF acceleration.
3. Redesign `--hidden` so relay operators learn far less than they do today.
4. Move sensitive runtime state out of casual CLI/config views.
5. Make desktop users consumers by default, not silent relays by default.

This plan is intentionally split between immediate hardening and protocol redesign. The immediate work is small and safe. The protocol redesign is larger and must stay compatible with staged releases.

## Branch Strategy

Create and maintain these long-lived branches:

- `platform/linux-ebpf`
- `platform/windows`
- `platform/macos`

Branch roles:

- `main`
  - shared protocol
  - shared CLI
  - shared discovery and hidden-mode logic
  - shared tests
- `platform/linux-ebpf`
  - eBPF/XDP fast path
  - Linux service manager
  - Linux relay benchmarks
- `platform/windows`
  - Windows service management
  - Windows packaging
  - Windows firewall and path handling
- `platform/macos`
  - `launchd` integration
  - macOS packaging
  - keychain integration

Rule: protocol changes land in `main` first, then platform branches rebase or merge from `main`.

## Delivery Order

1. Finish current CLI and config hardening.
2. Introduce a local control API so reload/status do not depend on Unix signals.
3. Add transport abstraction so direct mode can support TCP and QUIC.
4. Redesign hidden descriptors and relay circuits.
5. Add encrypted local state storage.
6. Add Windows service packaging.
7. Add macOS app and service packaging.
8. Keep Linux relay optimization separate in the Linux branch.

## What Direct Mode Means Today

Direct mode is not invisible. It is private in a narrower sense:

- the real app stays on `127.0.0.1`
- the client connects to a local VX6 listener
- VX6 forwards the stream to the remote VX6 node
- the remote VX6 node connects to the local app

That means:

- the remote service port is hidden from the public internet
- the remote VX6 node address is still visible to the client network and ISP
- the traffic payload is protected by the VX6 secure session
- the network path itself is not hidden

So in direct mode:

- local machines on the same network can observe that a VX6 connection exists
- the ISP can observe source and destination IPs, timing, and packet sizes
- they should not be able to read the encrypted VX6 payload itself

## Hidden Mode Target

The target for `--hidden` is:

- hide the owner IP from clients
- hide the client IP from the owner
- hide both from the rendezvous point
- limit what any single relay learns
- keep latency acceptable even if it is slower than direct mode

The target is not "perfect invisibility". The target is "no single helper node can trivially map both ends".

## Current Hidden-Mode Problems

Current hidden mode still leaks too much:

- hidden control traffic is plaintext JSON
- relay path extension exposes `next_hop` in plaintext
- guards learn the owner address too directly
- intro requests are too direct
- relay operators can inspect timing and some control metadata

Before we call hidden mode strong, these must change.

## Hidden-Mode Redesign

### 1. Mode Split

Support three explicit operating modes:

- `direct`
  - fastest
  - endpoint IP visible
  - payload encrypted
- `private-relay`
  - 1 to 2 relay hops
  - better exposure reduction
  - modest slowdown
- `hidden`
  - guard + middle + rendezvous path on both sides
  - strongest anonymity
  - slowest

### 2. Hidden Descriptors

Move hidden-service publishing to short-lived signed descriptors:

- blinded service identifier
- descriptor epoch
- intro set identifiers
- capability flags
- expiry
- signature

Do not publish owner IP inside the descriptor.

### 3. Owner Path

Owner flow for a hidden service:

1. Pick a small stable guard set.
2. Build owner circuits through `guard -> middle -> intro`.
3. Register intro descriptors through those circuits.
4. Build a separate owner circuit to a rendezvous point only when needed.

Result:

- intro nodes do not need the raw owner IP
- middle nodes see only adjacent hops
- guard nodes remain the most sensitive trust point for the owner

### 4. Client Path

Client flow for a hidden service:

1. Resolve the hidden descriptor from the DHT or directory layer.
2. Build a client circuit through `guard -> middle -> intro`.
3. Send an intro request through that circuit.
4. Build a separate client circuit through `guard -> middle -> rendezvous`.
5. Complete the rendezvous only after the owner joins.

Result:

- intro nodes do not directly see the client IP
- the last hop before intro sees only the previous hop
- the rendezvous sees only the last relay on each side

### 5. Layered Cells

Replace plaintext relay extension messages with fixed-size encrypted cells.

Per circuit:

- derive a hop key for every hop
- wrap the routing instruction in layers
- each hop peels only its own layer
- no hop learns the full path

This is the main protocol change needed to stop "hop N knows hop N+1 in clear" behavior.

### 6. Control-Plane Encryption

All hidden control messages should move behind authenticated encryption:

- intro registration
- intro requests
- rendezvous setup
- path extension
- teardown
- health checks

Use authenticated encryption for the control plane even when the service payload already has end-to-end VX6 encryption.

### 7. Fixed-Size Framing and Padding

For hidden mode:

- use fixed-size cells
- batch small writes
- optionally add padding
- optionally add low-rate cover traffic for long-lived hidden services

This reduces easy size correlation. It will cost bandwidth and latency, so it should stay configurable.

### 8. Guard Strategy

Use stable guards per node instead of full random relay selection for every circuit.

Reason:

- full randomness sounds good, but in practice it increases exposure
- repeated random first hops let more nodes learn the real client or owner IP over time
- stable guards reduce the number of nodes that ever see the real endpoint

So the hidden-mode target is:

- small rotating guard set
- frequent middle/rendezvous rotation
- intro rotation by descriptor epoch

### 9. Client and Owner IP Hiding

With the redesign:

- the owner IP is primarily exposed to the owner guard set
- the client IP is primarily exposed to the client guard set
- intro nodes do not need either raw IP
- rendezvous nodes should see neither raw IP

That is the correct trust split.

## Transport Plan

## Why QUIC Helps Even For TCP Apps

QUIC is useful because VX6 is carrying application streams, not raw IP packets.

For a TCP service:

- the local app still speaks normal TCP to the local VX6 process
- VX6 transports the byte stream over a reliable QUIC stream between VX6 peers
- the remote VX6 process replays it into a local TCP socket

Benefits:

- one long-lived encrypted session between neighbors
- multiple streams without head-of-line blocking between unrelated streams
- faster reconnects and resumptions
- better mobility for laptops switching networks

QUIC is not "raw UDP and hope for the best". QUIC adds:

- reliability
- retransmission
- congestion control
- flow control
- encryption

That makes it suitable for file transfer and normal TCP-like application streams.

## Transport Policy

Expose transport choice explicitly:

- `--transport tcp`
- `--transport quic`
- `--transport auto`

Recommendation:

- `direct`: `auto`, prefer QUIC if both ends support it
- `private-relay`: QUIC between relay neighbors
- `hidden`: QUIC between relay neighbors with fixed-size cells on top

TCP stays available as a compatibility fallback.

## Performance Plan

### Linux

Linux remains the fast relay tier:

- eBPF/XDP for early filtering and relay acceleration
- keep high-bandwidth public relays on Linux
- measure relay throughput and CPU per connection

### Windows

Windows should be treated as:

- client endpoint
- service owner endpoint
- optional light relay

Not ideal as the highest-capacity public relay tier.

Structural work:

- replace `SIGHUP` control path with a local API or named pipe
- replace Unix file locking with a portable lock strategy
- add Windows Service support
- add Windows firewall onboarding
- package via MSI and `winget`

Expected performance:

- direct mode should be close to Linux for typical desktop usage
- hidden mode will be slower mainly because of extra hops, not because Windows lacks eBPF
- heavy relay throughput will still trail Linux

### macOS

macOS should be treated similarly to Windows:

- client endpoint
- service owner endpoint
- optional light relay

Structural work:

- replace signal-based control with a local API
- add `launchd` integration
- add Keychain-backed secret storage
- package via signed app bundle, PKG, and Homebrew cask

Expected performance:

- direct mode should be near-normal for interactive use
- hidden mode slowdown is dominated by circuit design, not the OS itself
- large public relay loads should stay on Linux

### Cross-Platform Performance Work

Apply these on all OSes:

- reuse neighbor connections
- pool buffers
- avoid per-stream full handshakes
- use stream multiplexing
- prewarm standby relay paths for hidden services
- cache descriptor lookups
- use fast reconnect and session resumption

### Multi-Process and Resource Control

For user machines, add a runtime governor:

- CPU budget
- memory budget
- socket budget
- max relay bandwidth
- max concurrent relayed streams

Suggested policy:

- default desktop relay mode is off
- if desktop relay mode is enabled, cap relay resource use at a configured ceiling such as 25% to 33%
- stop admitting new relay work before the machine becomes unstable

Implementation ideas:

- one control process
- one network worker pool
- separate relay workers for heavy forwarding
- per-worker connection sharding
- health-based load shedding

## Fast Failover

For hidden and private-relay modes:

- keep at least one warm standby circuit
- track RTT, error rate, and recent throughput per relay
- switch to standby on loss or timeout
- republish intro sets before expiry

This is better than waiting for a failed relay to time out before rebuilding the whole path.

## Security and Hardening

### DHT and Directory

Harden the record layer against poisoning:

- short TTLs
- sequence numbers or version counters
- signature checks everywhere
- node-name validation
- service-name validation
- replay-window checks
- per-node publish rate limits
- per-IP publish rate limits for public entry points
- record replacement rules that require newer signed state

### Naming

Keep node names short and strict:

- max 15 characters
- lowercase letters
- digits
- hyphen

Reason:

- simpler UX
- safer parsing
- lower spoofing surface
- better terminal and UI rendering

### File Transfer

File transfer should be opt-in:

- default receive mode off
- allow all only by explicit operator action
- allow trusted senders by explicit operator action
- reject unauthorized senders before writing the file

### Public vs Private Publication

Split publication intent:

- default named publication for known peers
- explicit public publication for world-visible services

Future CLI shape:

- `vx6 service add --name admin --target 127.0.0.1:8080`
- `vx6 service publish --name admin --public`

This avoids accidental global exposure.

### Hiding IPs From Local UX

Realistically, a local administrator with full disk access can always recover state somehow. The goal is to remove casual exposure, not to promise impossible secrecy.

Implementation plan:

- keep static config for local preferences only
- move peer and registry state into encrypted local storage
- encrypt the local state database with a key protected by OS facilities:
  - Linux keyring or file key with locked permissions
  - Windows DPAPI
  - macOS Keychain
- CLI shows names by default
- raw addresses require an explicit debug flag

## What We Should Not Do

- do not claim "completely invisible"
- do not rely on random relays alone for anonymity
- do not make security depend on Linux eBPF
- do not use packet striping as the main anonymity story
- do not make Windows and macOS public relay defaults

## About Multipath

Multipath can still be useful, but for performance and resilience:

- stripe fixed encrypted cells across multiple circuits
- use it for large transfers
- use optional forward error correction for lossy paths

Do not present this as the core anonymity feature. Timing correlation will still exist.

## Latency Expectations

These are directional targets, not guarantees:

- direct mode
  - about normal internet path plus small local VX6 overhead
- private-relay mode
  - usually one extra regional RTT
- hidden mode
  - typically several extra RTTs and lower throughput

Rough practical expectation:

- direct: near native for SSH, chat, dashboards, and normal web traffic
- private-relay: noticeable but acceptable for admin traffic
- hidden: slower, but still usable for admin panels, messaging, and low-volume service access

## Immediate Work Already Worth Doing

1. Keep file receiving disabled unless explicitly allowed.
2. Keep raw peer and registry addresses hidden in normal CLI output.
3. Enforce short node names.
4. Separate public and hidden service views cleanly.
5. Replace signal-only runtime control with a local admin API.

## First Major Milestones

### Milestone 1: Cross-Platform Runtime Base

- portable control API
- portable locking
- portable service lifecycle
- Windows and macOS CLI builds

### Milestone 2: Direct-Mode Transport Upgrade

- transport abstraction
- QUIC neighbor sessions
- session resumption
- benchmark direct mode

### Milestone 3: Hidden-Mode Protocol Upgrade

- encrypted cells
- guard-based circuit building
- encrypted descriptor flow
- intro and rendezvous redesign

### Milestone 4: Desktop Packaging

- Windows service + MSI + `winget`
- macOS app + `launchd` + Homebrew cask
- Linux packages and systemd polish

### Milestone 5: Relay Tier Optimization

- Linux eBPF relay fast path
- relay scoring
- warm failover circuits
- resource governor
