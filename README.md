# VX6

VX6 is a TCP-based peer-to-peer service network for real local applications.

The main idea is simple:

- your app stays on `127.0.0.1`
- VX6 publishes a signed record for that app
- another VX6 node resolves the record
- VX6 opens an encrypted stream between the two nodes
- the remote app is reached through a local forwarder, not by exposing the app itself

VX6 can do this in three ways:

- `direct`: connect straight to a known VX6 node address
- `named`: resolve a public or private service through discovery and the DHT
- `hidden`: resolve a hidden service through blinded DHT keys and relay paths

This repository currently treats:

- `main` as the Linux-first source of truth for protocol and security behavior
- Windows support as a compatibility effort that should follow `main`, not fork it

## What Works Right Now

- signed node identity with Ed25519
- encrypted node-to-node sessions with authenticated key exchange
- public service publishing and lookup
- private per-user service catalogs
- hidden services with:
  - encrypted hidden descriptors
  - blinded rotating DHT keys
  - invite secrets
  - anonymous descriptor store and lookup over relay paths
- local runtime control channel for `status` and `reload`
- file transfer with local receive policy
- relay budgeting so transit traffic does not consume all local capacity
- TCP-only transport across the whole system

## What Does Not Work Yet

- real QUIC transport
- seamless live failover of an already-active hidden TCP stream after a relay dies
- production-ready Windows service packaging
- production-ready macOS service packaging
- a proven active eBPF/XDP fast path for the current encrypted relay data path
- strong anti-Sybil store admission in the DHT

## Release Status

This is a strong working prototype for controlled use and testing.

It is not yet a finished large-scale production network.

The protocol and main security model are in place. The main remaining work is:

- hardening
- failover
- mixed-OS runtime polish
- DHT admission controls
- WAN tuning

## Transport

VX6 currently runs on TCP only.

The config surface still accepts `transport=quic` for forward compatibility, but this build does not activate QUIC. The effective transport is TCP.

## Hidden Services

VX6 hidden services are not listed publicly by default.

Important details:

- hidden lookups use blinded rotating keys, not plain alias keys
- the hidden descriptor payload is encrypted
- lookups can be relayed over anonymous onion-style paths
- invite secrets make alias guessing much harder

Important limitation:

- this is stronger than plain hidden-alias lookup, but it is not full Tor-equivalent traffic-analysis protection

## eBPF

Linux eBPF/XDP work is still experimental in this repo.

There is embedded bytecode and status reporting, but not a complete proven acceleration path for the current encrypted relay plane. Stability is more important than speed in the current release.

## GUI

This repo now includes a small GUI front-end:

- `vx6-gui`

It is a local web UI that calls the `vx6` binary underneath and exposes the main features as forms instead of shell commands.

## Quick Start

Initialize a node:

```bash
vx6 init --name alice --listen '[::]:4242'
```

Run it:

```bash
vx6 node
```

Add a public service:

```bash
vx6 service add --name web --target 127.0.0.1:8080
```

List visible services:

```bash
vx6 list
```

Open the GUI:

```bash
vx6-gui
```

## Documentation

- [Setup](./docs/SETUP.md)
- [Usage](./docs/USAGE.md)
- [Commands](./docs/COMMANDS.md)
- [Architecture](./docs/architecture.md)
- [Discovery](./docs/discovery.md)
- [DHT](./docs/dht.md)
- [Services](./docs/services.md)
- [Identity](./docs/identity.md)
- [eBPF Status](./docs/ebpf.md)
- [Systemd](./docs/systemd.md)
- [GUI](./docs/GUI.md)
- [Status](./docs/STATUS.md)
- [File Map](./docs/FILE_MAP.md)
