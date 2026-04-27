# VX6
<p align="right">
<b>SPONSORED BY</b><br>
HackitiseLabs Pvt. Ltd.<br>
<a href="https://hackitiselabs.in">hackitiselabs.in</a>|
<br>Dalker<br>
<a href="https://github.com/dailker">GitHub: https://github.com/dailker </a>
</p>

[![IPv6 First](https://img.shields.io/badge/IPv6-first-0F766E?style=for-the-badge)](./docs/architecture.md)
[![Peer to Peer](https://img.shields.io/badge/peer--to--peer-service_network-1D4ED8?style=for-the-badge)](./docs/discovery.md)
[![Localhost to Localhost](https://img.shields.io/badge/localhost-to_localhost-F59E0B?style=for-the-badge)](./docs/services.md)
[![Linux Ready](https://img.shields.io/badge/Linux-systemd_ready-7C3AED?style=for-the-badge)](./docs/systemd.md)

VX6 is an IPv6-first peer-to-peer network for real services.  
It turns local apps into network-reachable services without forcing them to stop being local.  
Share SSH, APIs, web apps, databases, dashboards, internal tools, files, and hidden aliases across peers.  
Build on localhost. Share by peer. Stay direct.

> VX6 brings localhost to the network.

## Why VX6

Most apps already work on `127.0.0.1`. VX6 keeps that model intact.

Instead of redesigning your stack, opening raw ports everywhere, or depending on one fixed center, VX6 lets peers reach services through direct IPv6, named discovery, relay paths, and hidden aliases.

That makes it a strong fit for:

- SSH and admin access
- internal APIs and dashboards
- databases and tooling
- CTF infrastructure
- distributed systems
- collaboration apps
- peer-to-peer products built on top of VX6

## The Core Idea

```text
  local app -> VX6 -> peer network -> VX6 -> local app

  build on localhost
  share by peer
  stay direct
```

## Localhost to Localhost

This is the main VX6 story.

Your service can stay on localhost:

- `127.0.0.1:22`
- `127.0.0.1:8080`
- `127.0.0.1:5432`

Another peer can reach that service from its own localhost.

```text
  Bob's Machine                             Alice's Machine
  ------------------                        ------------------
  sshd -> 127.0.0.1:22                      ssh -p 2222 user@127.0.0.1
        |                                           ^
        v                                           |
      [ VX6 ] <========== peer network =========> [ VX6 ]
```

This is why VX6 feels simple:

- the app stays local
- the user connects to a local port
- the service does not need to be redesigned

## Highlights

- IPv6-first peer-to-peer service network
- localhost-to-localhost service sharing
- signed node identity, signed service records, and encrypted service/file sessions
- direct IPv6 access when you already know the target address
- named services like `alice.ssh` and `team.api`
- hidden services by alias
- multi-hop relay paths
- file transfer between VX6 nodes
- Linux systemd support
- eBPF-assisted Linux fast-path design
- builder-friendly model for apps and distributed systems

## Discovery Without a Permanent Center

In VX6, a bootstrap is not a forever-central server.

It is simply the first live node you know.

That can be:

- your own VPS
- a team node
- a friend's node
- any trusted live node already in the network

Once connected, VX6 learns peers, services, and aliases from the network and keeps moving through signed records, peer sync, and DHT-backed lookups.

```text
               first contact

            [ any known node ]
                 /   |   \
                /    |    \
               v     v     v
           [NodeA][NodeB][NodeC]
              \      |      /
               \     |     /
                \    |    /
                 \   |   /
                  [ peer mesh ]
```

## Hidden Services

VX6 hidden services are reached by alias instead of by public service endpoint.

They use:

- 3 active intro nodes
- 2 standby intro nodes
- 2 guard nodes
- 3 rendezvous candidates

Profiles:

- `fast`: `3 + X + 3`
- `balanced`: `5 + X + 5`

Hidden-service flow:

1. the service publishes an alias
2. the client resolves the alias
3. intro nodes and guards carry routing signals
4. client and owner build paths toward a rendezvous point
5. the service stream is carried through the relay topology

```text
                  active intros
              [I1] [I2] [I3]
                  \   |   /
                   \  |  /
                    \ | /
                [ hidden alias ]

             standby intros: [S1] [S2]

  client side path                     owner side path

  Client -> A -> B -> C -> X <- D <- E <- F <- Hidden Owner

  X = rendezvous node
```

## Fast Paths, Relays, and eBPF

VX6 supports direct service access, relay paths, and Linux kernel-assisted networking.

Why it feels fast:

- direct IPv6 is available when possible
- services stay close to localhost
- peers can talk directly
- relay paths avoid forcing every workload through one center
- Linux eBPF is used as a fast-path layer for VX6 traffic handling

```text
  NIC
   |
   v
 [eBPF / XDP]
   |
   +--> classify VX6 traffic
   +--> fast relay decisions
   +--> keep hot path lightweight
   |
   v
 [ VX6 runtime ]
```

## What You Can Build

VX6 is not only something to use. It is something to build on.

Examples:

- peer-to-peer video sharing
- private meeting rooms
- team chat
- collaborative editors
- remote control panels
- distributed compute control planes
- edge coordination systems
- internal service meshes

For the right topology, a VX6-based video app can move media more directly between participants and feel lighter than stacks built around more centralized traffic flow.

```text
  [User A Camera/Mic] -> [VX6 media app] -> [VX6] ==== peer path ==== [VX6] -> [VX6 media app] -> [User B Screen/Speakers]

  direct media path
  lighter middle layer
  good fit for small-group real-time apps
```

Think of VX6 as the network layer under your app:

```text
      your app
        |
        +--> UI
        +--> state
        +--> media
        +--> collaboration
        |
        v
       VX6
        |
        +--> naming
        +--> localhost sharing
        +--> relay paths
        +--> hidden aliases
        +--> peer discovery
```

## Use Cases

### Team Infrastructure

A frontend can live on one node, an API on another, and a database on a third.  
Each service can stay local while VX6 connects the whole stack.

```text
   Users
    |
    v
 [Frontend Node]
       |
       v
   [API Node]
       |
       v
   [DB Node]

  Each service can stay local on its own machine.
  VX6 connects the whole stack.
```

### CTF Teams

VX6 works well for:

- challenge hosting
- scoreboards
- admin panels
- internal tools
- controlled team access

```text
       [Scoreboard]
        /   |   \
       /    |    \
 [Challenge1][Challenge2][Challenge3]
       \      |       /
        \     |      /
         [Admin Tools]
              |
         [10 Team Members]
```

### Distributed Computing

VX6 fits worker meshes, schedulers, controllers, and internal metrics systems.

```text
           [ Controller ]
            /   |   \
           /    |    \
          v     v     v
      [Worker][Worker][Worker]
          \      |      /
           \     |     /
            \    |    /
             [ Metrics ]
```

### Collaboration Tools

VX6 is a good base for:

- team chat
- private dashboards
- shared notes
- internal meeting tools
- collaborative workspaces

## Quick Start

### 1. Build

```bash
go build -o ./vx6 ./cmd/vx6
```

### 2. Initialize a Node

```bash
./vx6 init \
  --name alice \
  --listen '[::]:4242' \
  --advertise '[2001:db8::10]:4242' \
  --bootstrap '[2001:db8::1]:4242'
```

`--bootstrap` means a known live VX6 node. It does not need to be a permanent central server.

### 3. Start the Node

```bash
./vx6 node
```

### 4. Share a Local Service

```bash
./vx6 service add --name ssh --target 127.0.0.1:22
./vx6 reload
```

### 5. Connect From Another Node

```bash
./vx6 connect --service alice.ssh --listen 127.0.0.1:2222
ssh -p 2222 user@127.0.0.1
```

`--listen` is your own local port. VX6 uses exactly the port you give.

## Direct IPv6 Mode

If two people already know the host IPv6 address, they can connect directly.

Host:

```bash
./vx6 init --name host --listen '[::]:4242' --advertise '[2001:db8::10]:4242'
./vx6 service add --name ssh --target 127.0.0.1:22
./vx6 node
```

Client:

```bash
./vx6 connect --service ssh --addr '[2001:db8::10]:4242' --listen 127.0.0.1:2222
```

## Hidden Service Example

Host:

```bash
./vx6 init \
  --name ghost \
  --listen '[::]:4242' \
  --advertise '[2001:db8::30]:4242' \
  --bootstrap '[2001:db8::1]:4242' \
  --hidden-node

./vx6 service add \
  --name admin \
  --target 127.0.0.1:22 \
  --hidden \
  --alias hs-admin \
  --profile fast

./vx6 node
```

Client:

```bash
./vx6 connect --service hs-admin --listen 127.0.0.1:2222
```

## File Transfer

Send a file:

```bash
./vx6 send --file ./backup.tar --to bob
```

Received files go to:

```text
~/Downloads
```

## Background Operation

VX6 is designed to stay running.

Foreground:

```bash
./vx6 node
```

Systemd user service:

```bash
systemctl --user enable --now vx6
systemctl --user status vx6
systemctl --user reload vx6
```

## Default Paths

```text
~/.config/vx6/config.json
~/.config/vx6/identity.json
~/.config/vx6/node.pid
~/.local/share/vx6
~/Downloads
```

## Endpoint Format

VX6 IPv6 endpoints always look like this:

```text
'[ipv6]:port'
```

Example:

```text
'[2401:db8::10]:4242'
```

Rules:

- always include square brackets
- always include the port
- quote the full endpoint in shell commands

## Included App

- [apps/chat](./apps/chat/README.md): decentralized web chat with direct and group messaging over VX6

## Documentation

- [docs/USAGE.md](./docs/USAGE.md)
- [docs/SETUP.md](./docs/SETUP.md)
- [docs/COMMANDS.md](./docs/COMMANDS.md)
- [docs/systemd.md](./docs/systemd.md)
- [docs/services.md](./docs/services.md)
- [docs/discovery.md](./docs/discovery.md)
- [docs/architecture.md](./docs/architecture.md)
- [README_PROXY.md](./README_PROXY.md)
- [website-content](./website-content/README.md)

## Build and Test

```bash
make build
make test
```

or:

```bash
go test ./...
```

## License

[LICENSE](./LICENSE)
