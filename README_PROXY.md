# VX6 Relay and Hidden-Service Paths

VX6 supports two relay-based paths:

- a 5-hop proxy path for direct services and file transfer
- a hidden-service rendezvous path

## 5-Hop Proxy Path

Direct services and file transfer can be forced through a relay chain:

```bash
vx6 connect --service alice.ssh --listen 127.0.0.1:2222 --proxy
vx6 send --file ./archive.tar --to alice --proxy
```

Current behavior:

- VX6 picks 5 relay nodes from the local registry
- each hop extends the circuit to the next hop
- the final hop reaches the target VX6 node
- the service session still uses the VX6 secure transport end to end

This is a relay path, not Tor-style onion encryption.

## Hidden-Service Path

Hidden services are looked up by alias, not by `username.service`.

Current hidden-service flow:

1. the hidden service publishes an alias descriptor
2. the descriptor contains 3 active intro nodes and 2 standby intro nodes
3. the client chooses an intro path and rendezvous candidate
4. the owner builds a separate path to the same rendezvous node
5. the rendezvous node joins the two streams

Profiles:

- `fast`: `3 + X + 3`
- `balanced`: `5 + X + 5`

## eBPF

The repository includes an embedded XDP/eBPF object and attach tooling:

```bash
vx6 debug ebpf-status
vx6 debug ebpf-attach --iface eth0
vx6 debug ebpf-detach --iface eth0
```

Current truth:

- the eBPF object is present
- attach and detach are supported
- the working VX6 relay and service transport still run in user space

The eBPF path should be treated as an acceleration hook, not as the main active transport yet.
