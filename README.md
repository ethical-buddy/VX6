# VX6

VX6 is an IPv6-first transport and service fabric for direct, host-to-host connectivity without central tunnel infrastructure.

The project is built around one executable: `vx6`. The same binary is intended to act as a node, transfer endpoint, and later a routing and proxy participant. The current stage is intentionally small: a node can listen on `tcp6`, accept file transfers, and another node can send files to it using a simple framed protocol.

## Current Stage

- Single `vx6` executable for node and sender roles
- IPv6-only file transfer over `tcp6`
- Human-readable node names included in transfer metadata
- Persistent Ed25519 node identity with a stable VX6 node ID
- Signed endpoint record generation for future discovery
- Bootstrap discovery registry for publish and resolve through known VX6 nodes
- Automatic publish and bootstrap sync when the daemon runs with advertise and bootstrap config
- Simple CLI with no external dependencies
- Linux-first development baseline
- Clean repository structure for future identity, discovery, and routing work

## Repository Layout

```text
VX6/
├── cmd/vx6/              # CLI entrypoint
├── docs/                 # Architecture and roadmap documents
├── internal/             # Non-exported application packages
├── .gitignore
├── CONTRIBUTING.md
├── LICENSE
├── go.mod
└── README.md
```

## Quick Start

Build the binary:

```bash
go build ./cmd/vx6
```

Initialize node state once on each machine:

```bash
./vx6 init --name receiver-lab --listen '[::]:4242' --advertise '[2001:db8::10]:4242' --bootstrap '[2001:db8::1]:4242' --data-dir ./data/inbox
```

Inspect the local identity:

```bash
./vx6 identity show
```

Start the node:

```bash
./vx6 node
```

Add another bootstrap later if needed:

```bash
./vx6 bootstrap add --addr '[2001:db8::2]:4242'
```

Send a file to a name:

```bash
./vx6 send --file ./example.bin --to receiver-lab
```

Print a signed endpoint record for the current node:

```bash
./vx6 record print
```

Administrative discovery commands still exist when you want to inspect or force a publish:

```bash
./vx6 discover publish --via bootstrap --addr '[2001:db8::10]:4242'
./vx6 discover resolve --via bootstrap --name receiver-lab --save-peer
```

The receive side and send side use the same binary. Transfers include a small metadata header carrying the sender node name, file name, and file size before the payload stream.

## Practical User Flow

For normal use, the useful commands are:

- `vx6 init`
- `vx6 bootstrap add`
- `vx6 node`
- `vx6 send --to <name>`

When `vx6 node` runs with a configured advertise address and bootstrap addresses, it now:

- publishes its signed endpoint record on startup
- republishes periodically
- pulls a snapshot of known records from bootstrap nodes into a local registry cache

When `vx6 send --to <name>` runs, VX6 now:

- checks the local peer book first
- if needed, resolves the name from configured bootstrap nodes
- saves the resolved address locally
- retries with the refreshed address if a stale local address fails

## Service Sharing Direction

The current milestone moves files, not arbitrary TCP services. The intended service model is:

- every machine runs one `vx6` node
- the node owns naming, peer state, and connection policy
- services such as SSH, HTTP, or custom TCP applications are published through the node
- remote users connect to a VX6 service name, and the node resolves that to the current endpoint or forwarding path

For SSH specifically, the later shape would look more like `vx6 expose ssh --target 127.0.0.1:22 --name lab-ssh` and `vx6 connect lab-ssh`, with VX6 handling endpoint lookup and the raw IPv6 address staying out of the user flow.

## Current Discovery Model

Right now, VX6 has a bootstrap discovery stage. A known VX6 node can temporarily act as a registry for signed endpoint records, and other nodes can publish or resolve through it.

That means:

- if you set up VX6 on several devices today, they can use one or more known bootstrap nodes to publish and resolve signed endpoint records
- VX6 can now help with address changes because the daemon republishes and other nodes can re-resolve by name
- nodes now keep a local cached registry snapshot from bootstrap nodes
- there is still no true DHT, recursive peer flood lookup, quorum, or fully automatic cross-peer search mesh yet

The earlier design documents point toward a decentralized discovery layer. That is still the target. The expected path is:

1. local node identity and peer state
2. stable naming and signed endpoint records
3. bootstrap publish and resolve
4. decentralized lookup and endpoint refresh
5. service publication and forwarding

So the answer today is no: you do not yet get zero-configuration communication between two fresh devices without exchanging an address somehow. The end goal is also not a permanent central server. It is a decentralized discovery system, but that layer still needs to be built.

## Design Principles

- IPv6 is a first-class constraint, not an optional fallback.
- The codebase should stay small, inspectable, and easy to reason about.
- Initial transport primitives should be reliable before higher-level discovery, naming, and routing layers are added.
- Identity and discovery data should be verifiable, not just convenient.
- Documentation should describe the system plainly and precisely.

## Status

VX6 is at an early bootstrap stage. The current milestone establishes one executable, one daemon-style node runtime, one local config model, one persistent cryptographic identity, one signed endpoint record format, automatic bootstrap publish/sync, and a bootstrap discovery path that the larger system can build on.

## Run as a Service

For Linux service operation, see [docs/systemd.md](./docs/systemd.md) and the example unit in `deployments/systemd/vx6.service`.

## Contributing

Contribution guidelines are in [CONTRIBUTING.md](./CONTRIBUTING.md).
