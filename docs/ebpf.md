# eBPF and XDP

VX6 includes Linux eBPF/XDP work, but it is still experimental.

## What Exists

- embedded relay bytecode
- runtime attach and detach commands
- runtime status reporting

## What Does Not Exist Yet

- a complete proven fast path for the current encrypted relay data plane
- a finished cross-platform acceleration design

## Release Guidance

Treat eBPF/XDP as optional experimental work.

The stable network path today is still:

- userspace
- TCP
- signed records
- encrypted sessions

That is the correct reliability-first view for the current release.
