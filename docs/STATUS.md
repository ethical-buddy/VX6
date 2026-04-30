# Current Status

VX6 is usable now for controlled testing and temporary internal deployment.

## Working Today

- TCP transport
- signed discovery
- direct service access
- public service lookup
- private service catalogs
- hidden services with encrypted descriptors
- blinded hidden lookup keys
- invite-secret based hidden lookup
- file transfer with local permission policy
- runtime status and reload
- GUI over the same CLI/runtime surface

## Platform Position

### Linux

- Linux is the main protocol and release reference branch
- systemd documentation exists
- eBPF/XDP controls exist but remain experimental

### Windows

- Windows builds are supported through the Windows-compatible branch
- protocol behavior is kept aligned with `main`
- CLI and GUI build successfully for Windows targets
- runtime compatibility is in place for Windows 11 and Windows Server class builds
- Windows installer/service/firewall polish is still unfinished

## Still In Progress

- seamless hidden mid-stream failover
- stronger DHT store admission
- real QUIC transport
- real eBPF/XDP fast path for the current encrypted relay plane
- polished Windows and macOS packaging and service lifecycle work

## Honest Summary

VX6 is no longer just an architecture draft.

It is a real working prototype with:

- functioning service discovery
- functioning encrypted sessions
- functioning hidden services
- functioning DHT-backed lookup

The biggest remaining work is hardening, failover, packaging, and adversarial-scale polish.
