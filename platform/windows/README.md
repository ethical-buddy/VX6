# VX6 Windows Track

This directory is the Windows port workspace for VX6.

It is intentionally documentation-first right now so the team can port the
current protocol and runtime model without changing the network architecture.

Files:

- `PORTING_GUIDE.md`
  - full Windows port architecture
  - current crypto model
  - protocol layering
  - Linux eBPF role vs Windows userspace role
  - exact areas that must be adapted on Windows

- `HARDENING_ISSUES.md`
  - contributor-facing hardening backlog
  - security, abuse-resistance, failover, test, and portability work

Scope of the Windows track:

- keep the current VX6 protocol
- keep the current direct / relay / hidden architecture
- port runtime, packaging, and platform integration
- preserve behavior without depending on Linux-only features

Non-goal of the first Windows phase:

- do not redesign VX6 into a different network model
- do not make Windows depend on eBPF-like kernel hooks
- do not change the cryptographic protocol just to fit platform differences
