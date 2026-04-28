# VX6 eBPF / XDP Status

Purpose:
- describe what VX6 currently supports for eBPF/XDP
- explain what the new attach/status lifecycle does
- state the current benchmark limitation honestly

## What Exists Now

VX6 now has a real XDP lifecycle manager in:

- `internal/onion/ebpf.go`

It supports:

- checking whether embedded bytecode exists
- attaching the embedded XDP program to a Linux interface
- detaching it
- querying live kernel state for an interface

CLI commands:

```bash
vx6 debug ebpf-status --iface eth0
vx6 debug ebpf-attach --iface eth0
vx6 debug ebpf-detach --iface eth0
```

If you omit `--iface` from `ebpf-status`, VX6 prints only embedded-bytecode
status and the current compatibility warning.

## What `xdp_attached` Means

`xdp_attached=true` means:

- some XDP program is attached to the interface

It does not automatically mean:

- VX6 is using that program as the current relay fast path

## What `vx6_active` Means

`vx6_active=true` means:

- an attached XDP program is present
- its kernel-reported program name matches the embedded VX6 XDP program

That is stronger than just “some XDP program is attached.”

## Important Compatibility Warning

The current embedded XDP program still targets the legacy VX6 onion header.

That means:

- attach/status lifecycle is real
- kernel state reporting is real
- but the program is not yet the active fast path for the current encrypted
  relay data path

Why:

- current VX6 relay traffic is carried as encrypted userspace TCP streams
- the embedded XDP program expects an older visible onion header layout
- XDP therefore cannot currently accelerate the real encrypted relay path
  without further design work

## What Was Implemented In This Pass

Implemented:

- live kernel XDP status reporting
- attach fallback:
  - native mode first
  - generic mode second
- detach confirmation
- unit tests for:
  - JSON status parsing
  - text fallback parsing
  - attach fallback behavior
  - detach behavior
  - active-vs-attached distinction

## Why There Is No Honest Relay Fast-Path Benchmark Yet

The benchmark requirement has a hard architectural blocker:

- the current embedded XDP program is not yet on the same data path as the
  current encrypted VX6 relay flow

So a benchmark claiming:

- “userspace relay only”
- vs
- “eBPF-assisted current relay”

would be misleading today.

The honest current benchmark status is:

- attach/status lifecycle can be tested
- current encrypted relay performance can be benchmarked
- true current relay acceleration benchmarking must wait until the XDP program
  is aligned with the current relay transport format

## What Must Happen Before A Real eBPF Relay Benchmark

To benchmark actual VX6 relay acceleration honestly, VX6 needs:

1. an XDP/eBPF program that matches the current relay framing
2. a defined place where kernel-visible metadata can safely guide forwarding
3. an end-to-end benchmark comparing:
   - current userspace relay path
   - real kernel-assisted relay path

Until then, VX6 should describe eBPF as:

- experimental Linux acceleration work

not:

- a fully shipped fast path for the current relay architecture
