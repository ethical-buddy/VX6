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
- ASN-aware DHT diversity when a local ASN map is provided
- file transfer with local permission policy
- runtime status and reload
- GUI over the same CLI/runtime surface

## Linux Position

`main` is the Linux-first release branch.

That means:

- Linux is the primary reference environment
- systemd documentation exists
- Linux runtime behavior is the baseline used to shape the protocol branch
- eBPF/XDP controls exist here, but they are still experimental

## Other Platforms

- Windows should follow the same protocol and feature behavior through the `Windows-compatible` branch
- macOS packaging and runtime polish are still behind Linux

## Still In Progress

- seamless hidden mid-stream failover
- stronger DHT store admission
- real QUIC transport
- real eBPF/XDP fast path for the current encrypted relay plane
- polished Windows and macOS packaging and service lifecycle work
- richer ASN map tooling and operator data sources

## Honest Summary

VX6 is no longer just an architecture sketch.

It already has:

- functioning service discovery
- functioning encrypted sessions
- functioning DHT-backed lookup
- functioning hidden services

The biggest remaining work is hardening, failover, packaging, and large-scale adversarial polish.
