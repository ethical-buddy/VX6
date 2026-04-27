# VX6 Command Reference

All IPv6 endpoints must use this format:

```text
'[ipv6]:port'
```

Examples:

- `'[2001:db8::1]:4242'`
- `'[2406:abcd::10]:5000'`

## Core Commands

### Initialize a Node

```bash
vx6 init --name NAME [--listen '[::]:4242'] [--advertise '[ipv6]:port'] [--bootstrap '[ipv6]:port'] [--hidden-node] [--data-dir DIR] [--downloads-dir DIR]
```

Example:

```bash
vx6 init --name alice --listen '[::]:4242' --advertise '[2001:db8::10]:4242' --bootstrap '[2001:db8::1]:4242'
```

Default Linux paths:

- config: `~/.config/vx6/config.json`
- identity: `~/.config/vx6/identity.json`
- runtime state: `~/.local/share/vx6`
- received files: `~/Downloads`

### Start the Node

```bash
vx6 node
```

### Reload a Running Node

```bash
vx6 reload
```

### Identity

```bash
vx6 identity
```

### Status

```bash
vx6 status
```

## Services

### Add a Direct Service

```bash
vx6 service add --name NAME --target HOST:PORT
```

Example:

```bash
vx6 service add --name ssh --target 127.0.0.1:22
```

### Add a Hidden Service

```bash
vx6 service add \
  --name NAME \
  --target HOST:PORT \
  --hidden \
  --alias ALIAS \
  [--profile fast|balanced] \
  [--intro-mode random|manual|hybrid] \
  [--intro NODE_OR_ENDPOINT]
```

Example:

```bash
vx6 service add \
  --name admin \
  --target 127.0.0.1:22 \
  --hidden \
  --alias hs-admin \
  --profile fast \
  --intro-mode random
```

### List Local Services

```bash
vx6 service
```

## Service Access

### Connect to a Named Service

```bash
vx6 connect --service USER.SERVICE --listen 127.0.0.1:LOCALPORT
```

Example:

```bash
vx6 connect --service alice.ssh --listen 127.0.0.1:2222
```

You can also put the service name first:

```bash
vx6 connect alice.ssh --listen 127.0.0.1:3333
```

`--listen` is the local port on your own machine. VX6 uses exactly the port you give here. `127.0.0.1:2222` is only the default when `--listen` is omitted.

### Connect to a Hidden Service

```bash
vx6 connect --service ALIAS --listen 127.0.0.1:LOCALPORT
```

Example:

```bash
vx6 connect --service hs-admin --listen 127.0.0.1:2222
```

### Connect Directly By IPv6 Address

```bash
vx6 connect --service SERVICE --addr '[ipv6]:port' --listen 127.0.0.1:LOCALPORT
```

Example:

```bash
vx6 connect --service ssh --addr '[2001:db8::10]:4242' --listen 127.0.0.1:2222
```

### Force the 5-Hop Proxy Path

```bash
vx6 connect --service USER.SERVICE --listen 127.0.0.1:LOCALPORT --proxy
```

## File Transfer

### Send to a Named Node

```bash
vx6 send --file PATH --to NODE
```

### Send Directly By IPv6 Address

```bash
vx6 send --file PATH --addr '[ipv6]:port'
```

### Send Through the Proxy Path

```bash
vx6 send --file PATH --to NODE --proxy
```

## Discovery and Listing

### General List View

```bash
vx6 list
```

### Show One User's Direct Services

```bash
vx6 list --user USER
```

### Show Hidden Aliases Seen Locally

```bash
vx6 list --hidden
```

## Peer Book

### Add a Peer

```bash
vx6 peer add --name NAME --addr '[ipv6]:port'
```

### List Peers

```bash
vx6 peer
```

## Bootstraps

### Add a Bootstrap

```bash
vx6 bootstrap add --addr '[ipv6]:port'
```

### List Bootstraps

```bash
vx6 bootstrap
```

## Debug Commands

### Registry Snapshot

```bash
vx6 debug registry
```

### DHT Lookup

By service:

```bash
vx6 debug dht-get --service alice.ssh
vx6 debug dht-get --service hs-admin
```

By node:

```bash
vx6 debug dht-get --node alice
vx6 debug dht-get --node-id vx6_deadbeef123456
```

By raw key:

```bash
vx6 debug dht-get --key 'service/alice.ssh'
```

### eBPF / XDP

```bash
vx6 debug ebpf-status
vx6 debug ebpf-attach --iface eth0
vx6 debug ebpf-detach --iface eth0
```

`ebpf-attach` and `ebpf-detach` usually require root or the needed Linux capabilities.

## Help

```bash
vx6 help
```
