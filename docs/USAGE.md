# VX6 Usage Guide

This guide explains VX6 in plain language.

## What VX6 Does

VX6 lets one machine share a local TCP service with another machine over IPv6.

Examples:

- SSH
- web servers
- databases
- any other TCP port

VX6 can work in three common ways:

- by name through a VX6 network
- by hidden alias through relay nodes
- directly by raw IPv6 address without joining a VX6 network

## The Basic Idea

There are only three things to remember:

1. `vx6 node` keeps your node online.
2. `vx6 service add` tells VX6 which local port to share.
3. `vx6 connect` creates a local port on your own machine that forwards to the remote service.

## Very Important: The `--listen` Port

When you run:

```bash
vx6 connect --service bob.ssh --listen 127.0.0.1:3333
```

VX6 listens on `127.0.0.1:3333` on your own machine.

That means:

- `3333` is your local port
- the remote machine does not need to use `3333`
- you can choose any free local port you want

Examples:

- SSH on local `2222`
- web on local `9000`
- database on local `15432`

If you do not give `--listen`, VX6 uses the default:

```text
127.0.0.1:2222
```

Both command styles work:

```bash
vx6 connect --service bob.ssh --listen 127.0.0.1:3333
vx6 connect bob.ssh --listen 127.0.0.1:3333
```

## One Rule For IPv6 Addresses

Always write IPv6 endpoints like this:

```text
'[ipv6]:port'
```

Example:

```text
'[2401:db00::10]:4242'
```

Use quotes in shell commands.

## Normal VX6 Network

This is the common setup when you have a bootstrap node.

### Bootstrap Node

```bash
./vx6 init \
  --name bootstrap \
  --listen '[::]:4242' \
  --advertise '[2001:db8::1]:4242'

./vx6 node
```

### Normal Node

```bash
./vx6 init \
  --name bob \
  --listen '[::]:4242' \
  --advertise '[2001:db8::20]:4242' \
  --bootstrap '[2001:db8::1]:4242'

./vx6 node
```

### Share a Local Service

If Bob wants to share SSH:

```bash
./vx6 service add --name ssh --target 127.0.0.1:22
./vx6 reload
```

That means:

- service name inside VX6: `ssh`
- real local target on Bob's machine: `127.0.0.1:22`

### Access It From Another Node

On the client machine:

```bash
./vx6 connect --service bob.ssh --listen 127.0.0.1:2222
```

Then use the forwarded local port:

```bash
ssh -p 2222 user@127.0.0.1
```

## Web Example

On the service host:

```bash
./vx6 service add --name web --target 127.0.0.1:8080
./vx6 reload
```

On the client:

```bash
./vx6 connect --service bob.web --listen 127.0.0.1:9000
curl http://127.0.0.1:9000
```

## Hidden Service

Hidden service means:

- the public registry does not show the real service endpoint
- clients connect by alias
- VX6 uses relay nodes in the background

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

## Direct IPv6 Sharing Without A VX6 Network

This is the simplest case.

If your friend already knows your public IPv6 address, they can connect directly.

Host:

```bash
./vx6 init \
  --name host \
  --listen '[::]:4242' \
  --advertise '[2001:db8::10]:4242'

./vx6 service add --name ssh --target 127.0.0.1:22
./vx6 node
```

Friend:

```bash
./vx6 connect --service ssh --addr '[2001:db8::10]:4242' --listen 127.0.0.1:2222
```

## File Transfer

Send by VX6 name:

```bash
./vx6 send --file ./backup.tar --to bob
```

Send directly by IPv6:

```bash
./vx6 send --file ./backup.tar --addr '[2001:db8::20]:4242'
```

## Background Use

You do not need to stop and start the node every time.

If the node is already running and you change config or add a service:

```bash
./vx6 reload
```

If you use systemd:

```bash
systemctl --user enable --now vx6
systemctl --user reload vx6
systemctl --user status vx6
```

See [systemd.md](./systemd.md) for the full setup.

## Useful Commands

```bash
./vx6 help
./vx6 identity
./vx6 status
./vx6 service
./vx6 list
./vx6 list --user bob
./vx6 list --hidden
./vx6 debug registry
./vx6 debug dht-get --service bob.ssh
./vx6 debug dht-get --service hs-admin
```

## Troubleshooting

### I can ping the other machine but VX6 does not connect

That usually means:

- `tcp/4242` is blocked by firewall
- the machine is not actually listening on IPv6
- the advertised IPv6 address is wrong

Ping is not enough. VX6 needs IPv6 TCP reachability.

### My chosen local port is not working

Remember:

- `--listen` is your own local port
- choose any free port
- if you do not set it, VX6 uses `127.0.0.1:2222`

Examples:

```bash
./vx6 connect bob.ssh --listen 127.0.0.1:3333
./vx6 connect bob.web --listen 127.0.0.1:9000
./vx6 connect bob.pg --listen 127.0.0.1:15432
```
