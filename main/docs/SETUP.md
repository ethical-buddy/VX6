# VX6 Setup Guide

This file shows the current working setup for:

- a bootstrap node
- normal nodes
- file transfer
- service exposure
- service access
- where bootstrap IPs are configured today

## Bootstrap Node

Build VX6:

```bash
go build -o ./vx6 ./cmd/vx6
```

Initialize the bootstrap node:

```bash
./vx6 init \
  --name bootstrap \
  --listen '[::]:4242' \
  --data-dir ./data/inbox
```

Optional explicit advertise address:

```bash
./vx6 init \
  --name bootstrap \
  --listen '[::]:4242' \
  --advertise '[2401:db8::10]:4242' \
  --data-dir ./data/inbox
```

If no `--advertise` address is set, `vx6 node` tries to detect a global IPv6 address automatically when it starts.

Start the daemon:

```bash
./vx6 node
```

## Normal Node

Initialize a normal node with a bootstrap:

```bash
./vx6 init \
  --name surya \
  --listen '[::]:4242' \
  --bootstrap '[2401:db8::10]:4242' \
  --data-dir ./data/inbox
```

Optional explicit advertise address:

```bash
./vx6 init \
  --name surya \
  --listen '[::]:4242' \
  --advertise '[2401:db8::20]:4242' \
  --bootstrap '[2401:db8::10]:4242' \
  --data-dir ./data/inbox
```

Add more bootstraps later:

```bash
./vx6 bootstrap add --addr '[2401:db8::11]:4242'
./vx6 bootstrap list
```

Start the daemon:

```bash
./vx6 node
```

When the daemon runs, it now:

- auto-detects a global IPv6 advertise address if one is not pinned
- publishes its signed node record
- publishes its signed service records
- syncs registry snapshots from known discovery targets
- serves encrypted file and service sessions

## File Transfer

Send by node name:

```bash
./vx6 send --file ./example.bin --to receiver-lab
```

VX6 now resolves through:

- local peer entries
- configured bootstrap nodes
- cached peer/registry addresses

## Expose a Local Service

Expose local SSH:

```bash
./vx6 service add --name ssh --target 127.0.0.1:22
./vx6 service list
```

Keep `./vx6 node` running. The service will be published as:

```text
surya.ssh
```

## Access a Remote Service

Create a local forwarder:

```bash
./vx6 connect --service surya.ssh --listen 127.0.0.1:2222
```

Then use it locally:

```bash
ssh -p 2222 localhost
```

The VX6 node-to-node hop is encrypted and authenticated.

## Useful Commands

```bash
./vx6 init --name <node> --listen '[::]:4242' --bootstrap '[bootstrap_ipv6]:4242'
./vx6 identity show
./vx6 node

./vx6 bootstrap add --addr '[ipv6]:4242'
./vx6 bootstrap list

./vx6 peer add --name <peer> --addr '[ipv6]:4242'
./vx6 peer list

./vx6 send --file ./file.bin --to <peer>

./vx6 service add --name ssh --target 127.0.0.1:22
./vx6 service list
./vx6 connect --service <node>.<service> --listen 127.0.0.1:2222
```

## Where The Bootstrap IP Is Hardcoded Today

Bootstrap IPs are configured in:

```text
~/.config/vx6/config.json
```

The field is:

```json
{
  "node": {
    "bootstrap_addrs": [
      "[2401:db8::10]:4242"
    ]
  }
}
```

You set it in two ways:

1. during init with `--bootstrap '[ipv6]:4242'`
2. later with `vx6 bootstrap add --addr '[ipv6]:4242'`

That is where the bootstrap IP is effectively hardcoded today.

## Notes

- In `zsh`, always quote bracketed IPv6 endpoints.
- `--listen` is the local bind address.
- `--advertise` is the explicit public address to publish. If omitted, VX6 tries to detect one automatically.
- The bootstrap node does not need a bootstrap address unless you want bootstrap-to-bootstrap sync.
