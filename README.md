<p align="right">
<b>SPONSORED BY</b><br>
HackitiseLabs Pvt. Ltd.<br>
<a href="https://hackitiselabs.in">hackitiselabs.in</a> | 
<br>Dailker<br>
<a href="https://github.com/dailker">GitHub</a>
</p>

<h1 align="center">VX6</h1>

<p align="center">
  <strong>Linux-first peer-to-peer service networking for real local applications.</strong><br>
  Signed discovery, encrypted sessions, DHT-backed lookup, relay paths, hidden services, file transfer, and a small GUI.
</p>

<p align="center">
  This branch is the <strong>Linux-first</strong> release branch.<br>
  Build <code>vx6</code> and <code>vx6-gui</code> from here for Linux hosts and Linux-centered deployments.
</p>

## What VX6 Is

VX6 is a service network built for applications that already work locally.

The model is simple:

- your service stays on localhost
- VX6 publishes a signed record for that service
- another VX6 node resolves the record
- VX6 creates an encrypted stream between the two nodes
- the remote machine reaches the service through its own local forwarder

That means you do not have to redesign your application just to share it across a peer network.

Good fits include:

- SSH
- internal APIs
- dashboards
- development tools
- databases
- private admin services
- hidden internal services

## What This Branch Is For

This branch is meant for:

- Linux builds
- Linux test deployments
- Linux-first production-style staging
- the reference protocol and security behavior for VX6

Typical binaries here are:

- `vx6`
- `vx6-gui`

Windows support should follow the same protocol and feature behavior, but the Windows-oriented release branch is `Windows-compatible`.

## Connection Modes

VX6 currently supports three access styles:

1. `direct`
   Connect to a known VX6 node address directly.

2. `named`
   Resolve a public or private service through discovery and the DHT.

3. `hidden`
   Resolve a hidden service through blinded DHT keys and relay paths.

## What Works Right Now

- signed node identity with Ed25519
- encrypted node-to-node sessions
- public service publishing and lookup
- private per-user service catalogs
- hidden services with:
  - encrypted hidden descriptors
  - blinded rotating DHT keys
  - invite secrets
  - anonymous descriptor store and lookup over relay paths
- relay budgeting so transit work does not consume all local capacity
- file transfer with local receive policy
- runtime status and reload over a local control channel
- TCP-based transport across the whole system
- `vx6-gui` as a local web UI over the same CLI/runtime

## What Is Still In Progress

- real QUIC transport
- seamless mid-stream hidden TCP failover after relay loss
- stronger anti-Sybil DHT store admission
- a proven active eBPF/XDP fast path for the current encrypted relay plane
- production-grade Windows installer and service automation
- production-grade macOS packaging

## Platform Notes

### Linux

This is the branch you should use if your target environment is Linux.

Current Linux expectations:

- build `vx6`
- build `vx6-gui`
- run the full current VX6 protocol feature set
- use TCP transport
- use the local runtime control channel
- optionally use Linux-only eBPF/XDP status and attach commands

Important:

- eBPF/XDP is still experimental
- it should be treated as optional work, not the reason to adopt VX6

### Windows

Windows is expected to follow the same protocol and service behavior, but the Windows-oriented branch is `Windows-compatible`.

That branch is intended for:

- `vx6.exe`
- `vx6-gui.exe`
- Windows 11
- Windows Server class deployments

## Security Model In Plain Language

VX6 is not claiming full Tor-equivalent anonymity.

What it does provide today:

- signed node and service identity
- encrypted peer-to-peer sessions
- encrypted hidden-service descriptors
- blinded rotating hidden lookup keys
- relay-based hidden-service paths

What it still does not fully solve:

- perfect traffic-analysis resistance
- seamless hidden-stream continuation after relay failure
- hardened large-scale adversarial DHT admission

## Transport

VX6 is currently TCP-only in production behavior.

The config surface may still mention `quic` for forward compatibility, but the current build does not activate a real QUIC transport.

## GUI

`vx6-gui` is included in this branch.

It is a local web UI that:

- starts on your own machine
- calls the `vx6` binary underneath
- exposes the same core features through forms instead of shell commands

This keeps the GUI aligned with the CLI and avoids splitting the protocol logic into two different apps.

## Quick Start

### Build

```bash
make build
```

Or:

```bash
go build ./cmd/vx6
go build ./cmd/vx6-gui
```

### Initialize a node

```bash
vx6 init --name alice --listen '[::]:4242'
```

### Run the node

```bash
vx6 node
```

### Add a public service

```bash
vx6 service add --name web --target 127.0.0.1:8080
```

### Inspect status

```bash
vx6 status
```

### Open the GUI

```bash
vx6-gui
```

## Documentation

- [Setup](./docs/SETUP.md)
- [Linux Guide](./docs/LINUX.md)
- [Usage](./docs/USAGE.md)
- [Commands](./docs/COMMANDS.md)
- [Architecture](./docs/architecture.md)
- [Discovery](./docs/discovery.md)
- [DHT](./docs/dht.md)
- [Services](./docs/services.md)
- [Identity](./docs/identity.md)
- [GUI](./docs/GUI.md)
- [eBPF Status](./docs/ebpf.md)
- [Systemd](./docs/systemd.md)
- [Status](./docs/STATUS.md)
- [File Map](./docs/FILE_MAP.md)

## Release Position

VX6 is already a working system for controlled testing and temporary internal deployment.

It is best described as:

- a strong working prototype
- protocol-complete enough for real usage
- still in need of hardening, failover polish, and deeper adversarial testing
