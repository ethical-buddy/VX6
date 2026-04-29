# VX6 Architecture

VX6 is a single-binary system with a small set of runtime layers.

## Main Parts

### 1. Identity

Each node has:

- a persistent Ed25519 keypair
- a stable VX6 node ID

Default storage:

- `~/.config/vx6/identity.json`

### 2. Node Runtime

`vx6 node` runs the IPv6 listener and handles:

- file transfer
- discovery requests
- DHT requests
- direct service connections
- relay path extension
- hidden-service rendezvous control

### 3. Discovery

The local registry stores:

- known node records
- known service records

Bootstrap nodes and known peers exchange snapshots and signed updates.

### 4. DHT

The DHT stores lookup keys for:

- nodes by name
- nodes by ID
- direct services
- hidden service aliases

### 5. Service Proxy

Direct services work like this:

1. client resolves a service record
2. client opens a VX6 session to the remote node
3. remote node forwards the stream to the local TCP target

### 6. Hidden Services

Hidden services add:

- intro nodes
- guard nodes
- rendezvous selection
- relay planning

The working transport is plain multi-hop TCP plus the VX6 secure session. It is not Tor-style layered onion encryption.

### 7. Background Operation

VX6 can run under systemd and reload changed config with:

```bash
vx6 reload
```

Reload is meant for:

- new services
- changed bootstrap list
- changed advertise address
- changed hidden-service settings

Changing the listen address still requires a restart.
