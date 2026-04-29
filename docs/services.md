# VX6 Services

VX6 exposes local TCP services through a VX6 node.

The local target stays local, for example:

- `127.0.0.1:22` for SSH
- `127.0.0.1:8080` for HTTP
- `127.0.0.1:5432` for PostgreSQL

## Direct Services

Direct services are named as:

```text
username.servicename
```

Example:

```text
alice.ssh
alice.web
alice.pg
```

Flow:

1. the service owner runs `vx6 service add`
2. the node publishes a signed service record
3. another node resolves `username.servicename`
4. VX6 opens an encrypted node-to-node session
5. the remote VX6 node forwards the TCP stream to the local target

## Hidden Services

Hidden services are resolved by alias only.

Example:

```text
hs-admin
```

Flow:

1. the owner publishes a hidden service descriptor
2. the descriptor contains intro nodes, standby intro nodes, and routing profile
3. the client looks up the alias
4. the client and owner build relay paths to a rendezvous node
5. the service is reached without publishing the service endpoint

Current hidden-service topology:

- 3 active intro nodes
- 2 standby intro nodes
- 2 guard nodes
- 3 rendezvous candidates

Profiles:

- `fast`: `3 + X + 3`
- `balanced`: `5 + X + 5`

## Direct By IPv6 Address

VX6 also supports a simple no-network mode:

```bash
vx6 connect --service ssh --addr '[2001:db8::10]:4242' --listen 127.0.0.1:2222
```

This is useful when:

- you do not want to run a bootstrap node
- you already know the host IPv6 address
- you want VX6 only for service forwarding

## Proxy Path

For direct services, VX6 can route over a 5-hop relay path:

```bash
vx6 connect --service alice.ssh --listen 127.0.0.1:2222 --proxy
```

This requires enough known relay candidates in the local registry.

## Transport Notes

- service traffic is end-to-end encrypted between VX6 nodes
- hidden-service relay routing is plain multi-hop TCP, not Tor-style per-hop onion encryption
- the eBPF/XDP tooling exists, but the working service path still uses the VX6 user-space transport
