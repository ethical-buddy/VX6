# VX6

VX6 is an IPv6-first transport and service fabric for direct, host-to-host connectivity without central tunnel infrastructure.

This repository currently ships the first executable building block: a small Go CLI that opens a `tcp6` connection and streams a file to a remote IPv6 listener. The long-term project direction is larger than that, but the codebase starts with a narrow, working transport primitive.

## Current Scope

- IPv6-only file transfer over `tcp6`
- Simple CLI with no external dependencies
- Linux-first development baseline
- Clean repository structure for future networking components

## Repository Layout

```text
VX6/
├── cmd/vx6/              # CLI entrypoint
├── docs/                 # Architecture and roadmap documents
├── internal/             # Non-exported application packages
├── .gitignore
├── CONTRIBUTING.md
├── go.mod
└── README.md
```

## Quick Start

Build the binary:

```bash
go build ./cmd/vx6
```

Start a receiver on another machine with an IPv6 listener on port `4242`:

```bash
nc -6 -l 4242 > received.bin
```

Send a file:

```bash
./vx6 send --file ./example.bin --addr [2001:db8::10]:4242
```

The current sender writes raw file bytes to the socket. The receiving side can be any IPv6-capable TCP listener that reads from standard input.

## Design Principles

- IPv6 is a first-class constraint, not an optional fallback.
- The codebase should stay small, inspectable, and easy to reason about.
- Initial transport primitives should be reliable before higher-level discovery and routing layers are added.
- Documentation should describe the system plainly and precisely.

## Status

VX6 is at an early bootstrap stage. The present milestone establishes repository conventions and a working IPv6 transfer command that the larger system can build on.

## Contributing

Contribution guidelines are in [CONTRIBUTING.md](./CONTRIBUTING.md).
