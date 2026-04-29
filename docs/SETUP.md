# VX6 Setup Guide

This guide covers the common working setups:

- bootstrap node
- normal network node
- direct IPv6 share without bootstrap
- hidden service
- file transfer
- background reload

## One Rule For IPv6 Endpoints

Always use this format:

```text
'[ipv6]:port'
```

Examples:

- `'[2001:db8::1]:4242'`
- `'[2406:abcd:1::10]:5000'`

## 1. Bootstrap Node

Build:

```bash
go build -o ./vx6 ./cmd/vx6
```

Initialize:

```bash
./vx6 init \
  --name bootstrap \
  --listen '[::]:4242' \
  --advertise '[2001:db8::1]:4242'
```

Run:

```bash
./vx6 node
```

Check:

```bash
./vx6 identity
./vx6 status
```

## 2. Normal Network Node

Initialize:

```bash
./vx6 init \
  --name bob \
  --listen '[::]:4242' \
  --advertise '[2001:db8::20]:4242' \
  --bootstrap '[2001:db8::1]:4242'
```

Add a service:

```bash
./vx6 service add --name ssh --target 127.0.0.1:22
```

Start the node:

```bash
./vx6 node
```

If the node is already running, publish the new service immediately:

```bash
./vx6 reload
```

Useful checks:

```bash
./vx6 service
./vx6 list --user bob
./vx6 debug registry
./vx6 debug dht-get --service bob.ssh
```

## 3. Client Node

Initialize:

```bash
./vx6 init \
  --name client \
  --listen '[::]:4242' \
  --advertise '[2001:db8::30]:4242' \
  --bootstrap '[2001:db8::1]:4242'
```

Start:

```bash
./vx6 node
```

Connect to a published service:

```bash
./vx6 connect --service bob.ssh --listen 127.0.0.1:2222
```

Then use the local forwarded port:

```bash
ssh -p 2222 user@127.0.0.1
```

## 4. Direct IPv6 Share Without Bootstrap

This is the simplest IPv6 use case.

Host:

```bash
./vx6 init \
  --name host \
  --listen '[::]:4242' \
  --advertise '[2001:db8::10]:4242'

./vx6 service add --name ssh --target 127.0.0.1:22
./vx6 node
```

Client:

```bash
./vx6 connect --service ssh --addr '[2001:db8::10]:4242' --listen 127.0.0.1:2222
```

No bootstrap, peer book, or VX6 name is required for this path.

## 5. Hidden Service

Host:

```bash
./vx6 init \
  --name ghost \
  --listen '[::]:4242' \
  --advertise '[2001:db8::40]:4242' \
  --bootstrap '[2001:db8::1]:4242' \
  --hidden-node

./vx6 service add \
  --name admin \
  --target 127.0.0.1:22 \
  --hidden \
  --alias hs-admin \
  --profile fast \
  --intro-mode random
```

Run:

```bash
./vx6 node
```

Client:

```bash
./vx6 connect --service hs-admin --listen 127.0.0.1:2222
```

Hidden service notes:

- clients use the alias only
- the service endpoint is not published
- the default hidden profile is `fast`
- `balanced` uses more relay hops

## 6. File Transfer

Send by node name:

```bash
./vx6 send --file ./archive.tar --to bob
```

Send directly by IPv6 address:

```bash
./vx6 send --file ./archive.tar --addr '[2001:db8::20]:4242'
```

## 7. Proxy Path

Use the relay path for a direct service:

```bash
./vx6 connect --service bob.ssh --listen 127.0.0.1:2222 --proxy
```

This needs enough known relay nodes in the local registry.

## 8. Background Reload

If `vx6 node` is already running and you edit config or add services:

```bash
./vx6 reload
```

This tells the running node to refresh config and republish immediately.

## 9. Where Config Lives

Default paths:

```text
~/.config/vx6/config.json
~/.config/vx6/identity.json
~/.config/vx6/node.pid
```

## 10. Useful Commands

```bash
./vx6 help
./vx6 identity
./vx6 status
./vx6 list
./vx6 service
./vx6 peer
./vx6 bootstrap
./vx6 debug registry
./vx6 debug dht-get --node bob
./vx6 debug dht-get --service bob.ssh
./vx6 debug ebpf-status
```
