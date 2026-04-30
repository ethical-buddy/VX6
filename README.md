<p align="right">
<b>SPONSORED BY</b><br>
HackitiseLabs Pvt. Ltd.<br>
<a href="https://hackitiselabs.in">hackitiselabs.in</a> | 
<br>Dailker<br>
<a href="https://github.com/dailker">GitHub</a>
</p>

<h1 align="center">VX6</h1>

<p align="center">
  <strong>Peer-to-peer service networking for real local applications.</strong><br>
  Signed discovery, encrypted sessions, DHT-backed lookup, relay paths, hidden services, file transfer, and a small GUI.
</p>

<p align="center">
  This branch is the <strong>Windows-compatible</strong> release branch.<br>
  Build <code>vx6.exe</code> and <code>vx6-gui.exe</code> from here for Windows 11 and Windows Server deployments.
</p>

## What VX6 Is

VX6 is built around a simple idea:

- your application stays local
- VX6 publishes a signed record for it
- another VX6 node resolves that record
- VX6 opens an encrypted stream between the two nodes
- the remote side reaches the service through a local forwarder instead of exposing the raw app directly

In practice, that means you can keep building on:

- `127.0.0.1:22`
- `127.0.0.1:8080`
- `127.0.0.1:5432`
- other local admin panels, APIs, dashboards, and internal tools

VX6 then makes those services reachable across a peer network in a controlled way.

## What This Branch Is For

This branch is meant for:

- Windows 11
- Windows Server class deployments
- building `vx6.exe`
- building `vx6-gui.exe`

Protocol and feature behavior are kept aligned with `main`.  
The goal is:

- same network behavior
- same DHT behavior
- same hidden-service behavior
- same CLI flags
- same GUI surface

The difference is platform runtime compatibility, not protocol design.

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
- production-grade Windows installer, service manager, and firewall automation
- production-grade macOS packaging

## Platform Notes

### Windows

This branch is intended for Windows builds.

Current Windows expectations:

- build `vx6.exe`
- build `vx6-gui.exe`
- run the same protocol and service features as Linux
- use TCP transport
- use the local runtime control channel instead of Unix signal flow

Linux-only features that do not become native Windows features just by building:

- systemd integration
- eBPF/XDP attach and active status management

### Linux

Linux remains fully supported by the protocol and runtime model as well, but the Linux-first branch for packaging and Linux release work is `main`.

## Security Model In Plain Language

VX6 is not claiming Tor-grade anonymity across the whole system.

What it does provide today:

- signed records for node and service identity
- encrypted peer-to-peer sessions
- encrypted hidden-service descriptors
- blinded rotating hidden lookup keys
- relay-based hidden-service paths

What it does not fully solve yet:

- perfect traffic-analysis resistance
- seamless hidden-stream continuation after relay failure
- hardened large-scale adversarial DHT admission

## Transport

VX6 is currently TCP-only in production behavior.

The config surface may still mention `quic` for forward compatibility, but this build does not activate a real QUIC transport yet.

## GUI

`vx6-gui` is included in this branch.

It is a local web UI that:

- starts on your own machine
- calls the `vx6` binary underneath
- exposes the same core features through forms instead of shell commands

On Windows, this is the easiest way to use VX6 if you do not want to drive everything from the terminal.

## Quick Start

### Build

```powershell
go build -o vx6.exe ./cmd/vx6
go build -o vx6-gui.exe ./cmd/vx6-gui
```

Or:

```powershell
make build
```

### Initialize a node

```powershell
.\vx6.exe init --name alice --listen "[::]:4242"
```

### Run the node

```powershell
.\vx6.exe node
```

### Add a public service

```powershell
.\vx6.exe service add --name web --target 127.0.0.1:8080
```

### Inspect status

```powershell
.\vx6.exe status
```

### Open the GUI

```powershell
.\vx6-gui.exe
```

## Documentation

- [Setup](./docs/SETUP.md)
- [Windows Guide](./docs/WINDOWS.md)
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
- still in need of hardening, packaging polish, and more adversarial testing
