# Setup

## Requirements

- Go 1.22 or newer to build from source
- IPv6 enabled on the machines that will run VX6
- firewall rules that allow the VX6 listen port

## Build

```bash
go build ./cmd/vx6
go build ./cmd/vx6-gui
```

## First-Time Node Setup

Create a config and identity:

```bash
vx6 init --name alice --listen '[::]:4242'
```

Optional:

- add `--advertise` if you already know the public IPv6 address
- add `--bootstrap` to seed discovery from a known node
- add `--hidden-node` if you do not want to publish an endpoint record

## Run the Node

```bash
vx6 node
```

## Check Status

```bash
vx6 status
```

## Open the GUI

```bash
vx6-gui
```

The GUI opens a local browser page and calls the `vx6` binary underneath.
