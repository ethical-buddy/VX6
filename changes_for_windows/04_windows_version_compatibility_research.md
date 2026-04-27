# Windows Version Compatibility Research (Pre-Implementation)

## Scope
Requested compatibility target review for:
- Windows XP
- Windows Vista
- Windows 7
- Windows 8 / 8.1
- Windows 10
- Windows 11

Stack reviewed:
- Go toolchain/runtime constraints
- eBPF for Windows
- XDP for Windows
- MsQuic
- WFP baseline availability

## Findings Matrix

| OS | Go (current VX6 line) | eBPF for Windows | XDP for Windows | MsQuic | WFP | Practical VX6 Status |
|---|---|---|---|---|---|---|
| XP | ❌ (unsupported by modern Go) | ❌ | ❌ | ❌ | ❌ (Vista+) | Not feasible |
| Vista | ❌ for modern Go lines (requires very old Go history) | ❌ | ❌ | ❌ | ✅ | Not feasible for current VX6 |
| 7 | ❌ on Go 1.21+ (Go 1.20 last supported line) | ❌ | ❌ | Not a support goal | ✅ | Legacy-only lane possible (user-space only) |
| 8 / 8.1 | ❌ on Go 1.21+ (Go 1.20 last supported line) | ❌ | ❌ | Not a support goal | ✅ | Legacy-only lane possible (user-space only) |
| 10 | ✅ | ⚠️ not the stated support baseline | ⚠️ not in stated prerequisites | ⚠️ possible via OpenSSL mode, but not primary target | ✅ | Partial/conditional support |
| 11 | ✅ | ✅ | ⚠️ docs emphasize Server 2019/2022 x64 path | ✅ | ✅ | Primary modern target |

Notes:
    - `eBPF for Windows` README Getting Started: Windows 11+ / Server 2022+.
    - `XDP for Windows` usage prerequisites: Windows Server 2019 or 2022, x64.
    - `MsQuic` platform docs: Schannel path needs Windows 11 / Server 2022 for TLS 1.3 baseline; OpenSSL path can run on most Windows 10 versions, older versions are not a support goal.
    - `WFP` exists since Vista/Server 2008, but does not alone provide eBPF/XDP parity.

## Key Answer to Your Question
"Can we support all Windows versions and also do eBPF?"

- For **XP/Vista/7/8** with current VX6 architecture and modern toolchain: **No full parity path exists**.
- For **Windows 10**: core VX6 can run; eBPF/XDP availability is conditional and not guaranteed as a first-class target from upstream docs.
- For **Windows 11/Server 2022+**: this is the realistic full-feature target for eBPF/XDP/MsQuic.

## Recommended Support Policy

### Official support lanes
1. **Full lane**: Windows 11 + Server 2022+
2. **Core lane**: Windows 10 + Server 2016/2019 (without claiming full eBPF/XDP parity)
3. **Legacy lane**: Windows 7/8 with a frozen legacy branch/toolchain and user-space transport only

### Explicit non-target
- Windows XP and Vista for the modern branch.

## Before-Implementation Gate
Do not implement Windows kernel acceleration work until this policy is accepted, because it affects:
- CI matrix
- toolchain pinning
- packaging and installer targets
- fallback behavior in CLI and runtime docs
