# VX6

VX6 is an IPv6-first node, service, and relay runtime.

It lets you:

- share files between VX6 nodes
- publish local TCP services such as SSH, HTTP, and databases
- connect to remote services by name instead of raw IP
- route traffic through a 5-hop relay path when you want a proxy path
- publish hidden services by alias without exposing the service endpoint
- run as a background service under systemd

## Endpoint Format

VX6 uses IPv6 endpoints in this form:

```text
'[2001:db8::10]:4242'
```

Rules:

- always include square brackets around the IPv6 address
- always include the port
- in shell commands, quote the full endpoint

## Current Features

- IPv6-only transport
- persistent Ed25519 node identity
- signed node and service records
- bootstrap-based discovery with local cache
- DHT-backed node and service lookup
- encrypted file transfer
- encrypted direct service forwarding
- 5-hop proxy forwarding
- hidden services with:
  - alias-only lookup
  - 3 active intro nodes
  - 2 standby intro nodes
  - 2 guard nodes
  - 3 rendezvous candidates
  - `fast` profile: `3 + X + 3`
  - `balanced` profile: `5 + X + 5`
- direct IPv6 service sharing without joining a VX6 network
- Linux systemd service support
- reload of a running background node with `vx6 reload`

## Quick Start

Build:

```bash
go build -o ./vx6 ./cmd/vx6
```

Initialize a node:

```bash
./vx6 init \
  --name alice \
  --listen '[::]:4242' \
  --advertise '[2001:db8::10]:4242' \
  --bootstrap '[2001:db8::1]:4242'
```

Start the node:

```bash
./vx6 node
```

Expose a local service:

```bash
./vx6 service add --name ssh --target 127.0.0.1:22
./vx6 reload
```

Connect from another node:

```bash
./vx6 connect --service alice.ssh --listen 127.0.0.1:2222
ssh -p 2222 user@127.0.0.1
```

## Common Use Cases

### 1. Direct IPv6 Service Share

No bootstrap. No VX6 naming. Just one friend and one IPv6 address.

On the host:

```bash
./vx6 init --name host --listen '[::]:4242' --advertise '[2001:db8::10]:4242'
./vx6 service add --name ssh --target 127.0.0.1:22
./vx6 node
```

On the client:

```bash
./vx6 connect --service ssh --addr '[2001:db8::10]:4242' --listen 127.0.0.1:2222
```

### 2. Named Service Over a VX6 Network

On the bootstrap:

```bash
./vx6 init --name bootstrap --listen '[::]:4242' --advertise '[2001:db8::1]:4242'
./vx6 node
```

On a normal node:

```bash
./vx6 init \
  --name bob \
  --listen '[::]:4242' \
  --advertise '[2001:db8::20]:4242' \
  --bootstrap '[2001:db8::1]:4242'
./vx6 service add --name web --target 127.0.0.1:8080
./vx6 node
```

On a client:

```bash
./vx6 connect --service bob.web --listen 127.0.0.1:9000
curl http://127.0.0.1:9000
```

### 3. Hidden Service

On the host:

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

On the client:

```bash
./vx6 connect --service hs-admin --listen 127.0.0.1:2222
```

### 4. File Transfer

```bash
./vx6 send --file ./backup.tar --to bob
```

## Background Operation

VX6 is designed to stay running in the background.

- foreground run: `./vx6 node`
- reload changed config or services: `./vx6 reload`
- status: `./vx6 status`

For systemd setup, see [docs/systemd.md](./docs/systemd.md).

## Documentation

- [docs/SETUP.md](./docs/SETUP.md): operator setup guide
- [docs/COMMANDS.md](./docs/COMMANDS.md): full command reference
- [docs/systemd.md](./docs/systemd.md): background service setup
- [docs/services.md](./docs/services.md): service and hidden-service model
- [docs/discovery.md](./docs/discovery.md): discovery and DHT model
- [docs/architecture.md](./docs/architecture.md): runtime architecture
- [README_PROXY.md](./README_PROXY.md): relay and hidden-service path notes

## Build and Test

```bash
make build
make test
```

Or:

```bash
go test ./...
```

## Status

VX6 is usable now for IPv6 file transfer, named service access, hidden services, direct-by-address sharing, and background node operation.

eBPF support is currently limited to the embedded XDP object and attach/detach tooling. The working relay and service transport still run in user space.

## License

[LICENSE](./LICENSE)
