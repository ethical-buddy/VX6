# VX6 as a Service

VX6 is intended to run continuously as a background node on Linux.

The repository includes an example unit file at `deployments/systemd/vx6.service`.

## Typical Setup

1. build `vx6`
2. initialize the node with its advertise address and bootstrap addresses
3. copy the example unit into `/etc/systemd/system/vx6.service`
4. adjust `User`, `WorkingDirectory`, and `ExecStart`
5. enable the service with `systemctl enable --now vx6`

## Why This Matters

Running VX6 as a service allows it to:

- keep listening for inbound file transfers
- publish its current endpoint record on start
- periodically republish and refresh bootstrap state
- maintain a local cached registry while the machine stays online
