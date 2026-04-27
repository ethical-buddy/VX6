# Windows Compatibility Changes + Manifest Alignment

## What Was Changed

### 1) CLI process signaling compatibility
Updated both:
- `internal/cli/app.go`
- `internal/internal/cli/app.go`

Changes:
- Removed direct `syscall` usage from shared CLI code.
- Added OS-specific signal/process helpers:
  - `internal/cli/process_unix.go`
  - `internal/cli/process_windows.go`
  - `internal/internal/cli/process_unix.go`
  - `internal/internal/cli/process_windows.go`
- `runNode` now calls `registerReloadSignal(...)`.
- `runReload` now calls `processExists(...)` and `sendReloadSignal(...)`.

Windows behavior:
- No SIGHUP subscription.
- Fallback signal reload returns a clear error message when control-channel reload is unavailable.

### 2) Runtime lock compatibility
Updated both:
- `internal/cli/app.go`
- `internal/internal/cli/app.go`

Changes:
- Replaced Unix flock-based runtime lock with cross-platform exclusive lock-file creation (`O_CREATE|O_EXCL`).
- Added stale lock recovery:
  - If lock exists and PID process is dead, remove stale PID/lock and retry.
- Keeps lock file open until `Close()` and cleans PID/control/lock files on shutdown.

### 3) Cross-platform test fixes
Updated tests in both package trees:
- `internal/config/config_test.go`
- `internal/internal/config/config_test.go`
- `internal/identity/store_test.go`
- `internal/internal/identity/store_test.go`

Changes:
- Replaced hard-coded Unix paths with `filepath.FromSlash`, `filepath.Dir`, `filepath.Join`.
- Set both `HOME` and `USERPROFILE` in home-path tests.
- Expected values now resolve correctly on Windows path semantics.

## Manifest Alignment Status (Current + Implementation Track)

### Already preserved from existing VX6 architecture
- Linux-side eBPF/XDP references remain intact (`internal/ebpf/onion_relay.c` and debug paths).
- Runtime control-channel reload path remains first-class and is now explicit across platforms.
- No feature removal was made from the existing code line; changes were compatibility-focused.

### Windows implementation track (research-backed)
The architecture below is the concrete path to add Windows kernel/data-plane support while keeping current VX6 logic and command surface.

1. **Control Plane (Go daemon, user mode)**
  - Keep VX6 policy/state in Go (`internal/node`, `internal/discovery`, `internal/dht`).
  - Add a Windows control adapter that writes route/filter/introduction metadata to a driver-facing channel.
  - Preferred initial channel: local control socket + structured JSON, followed by IOCTL bridge process once driver API is stable.

2. **Windows Fast Path (XDP for Windows)**
  - Use `xdp.sys` with Generic XDP first (no NIC-specific changes), then Native XDP where NIC support exists.
  - Use header-only XDP/AF_XDP APIs (new apps should use `XDP_API_VERSION_3+`; `xdpapi.dll` is backward-compat only).
  - Steer traffic to AF_XDP rings for user-mode VX6 packet classification/encapsulation when needed.

3. **Windows eBPF Execution Path (ebpf-for-windows)**
  - Compile VX6 packet program(s) as eBPF bytecode and load with libbpf-compatible APIs / `ebpf_api.h`.
  - For production/HVCI-safe deployments, use native mode (`bpf2c` -> signed `.sys`) for eBPF program packaging.
  - Keep JIT/interpreter only for local dev/test where appropriate.

4. **QUIC Transport Path (MsQuic)**
  - Introduce transport abstraction in VX6 so TCP and QUIC backends can coexist.
  - Start with user-mode `msquic.dll` integration for peer sessions and hidden-service streams.
  - Configure UDP firewall + keepalive settings to survive NAT rebinding patterns on Windows deployments.
  - Use Schannel baseline first; plan OpenSSL-backed mode only if 0-RTT requirement is strict for your deployment profile.

## Concrete Work Breakdown (Implement, not defer)

### Phase A — Windows platform adapters in VX6 (short-term)
1. Add `internal/transport/quic_msquic_windows.go` (build-tagged) with a minimal connection/listener lifecycle.
2. Add `internal/ebpf/windows/` package for:
  - capability detection (xdp + ebpf runtime presence),
  - loader interface (`LoadProgram`, `Attach`, `Detach`, `Stats`),
  - AF_XDP ring setup wrapper for packet redirection mode.
3. Extend `vx6 debug` with Windows runtime visibility:
  - XDP runtime state,
  - eBPF program attach state,
  - transport backend (tcp/quic-msquic).

### Phase B — Driver/control-channel bridge (mid-term)
1. Add a small Windows bridge process (or package) that maps VX6 control records to IOCTL payloads.
2. Define schema for route/action entries (drop/pass/redirect/encap).
3. Add versioned compatibility contract between Go daemon and bridge.

### Phase C — End-to-end hidden-service acceleration (mid/long-term)
1. Route intro/guard/rendezvous packet classes through XDP/eBPF rules first, user-mode fallback second.
2. Add QUIC stream multiplexing for multiple services over fewer guard links.
3. Add path metrics feedback loop from transport to DHT path selector.

## Acceptance Criteria Per Phase

### Phase A Done When
- `go test ./...` passes on Windows.
- `vx6 debug ebpf status` reports Windows adapter state without panics.
- A feature-gated MsQuic backend can establish at least one encrypted stream in integration tests.

### Phase B Done When
- VX6 can push one policy update and observe kernel/runtime effect confirmation.
- Driver bridge version mismatch fails closed with clear diagnostics.

### Phase C Done When
- Hidden service path setup works with QUIC backend under packet loss.
- Relay throughput and median latency improve versus TCP baseline in repeatable benchmark runs.

## Research Notes Used for This Plan
- **XDP for Windows usage/architecture**: XDP driver model, Generic vs Native mode, AF_XDP API, header-only user APIs via IOCTL path, runtime installation flow.
- **eBPF for Windows architecture**: libbpf compatibility, attach/helpers model, native mode (`bpf2c`) for production/HVCI scenarios.
- **MsQuic platform/deployment docs**: Windows TLS/runtime constraints, firewall/UDP operational requirements, keepalive/client-migration considerations.
- **WFP docs**: optional policy/filter integration layer if a deeper Windows network control path is required.

## Legacy Windows Compatibility Research (XP / Vista / 7 / 8)

### Hard constraints from toolchain and platform APIs
- **Go toolchain**:
  - Go 1.21+ requires **Windows 10 / Server 2016+**.
  - Go 1.20 is the last release supporting Windows 7/8/Server 2008/2012.
  - Go documentation history indicates very old systems (Vista and below) require much older Go lines (Go 1.10 is documented as the last line for Vista or below).
- **eBPF for Windows**: Getting Started states support on **Windows 11+ / Server 2022+**.
- **XDP for Windows**: usage prerequisites list **Windows Server 2019 or 2022, x64**.
- **MsQuic**:
  - Schannel path requires Windows 11 / Server 2022 for TLS 1.3 baseline.
  - OpenSSL path may run on most Windows 10 versions; older versions are explicitly not a support goal.
- **WFP**: available from **Vista / Server 2008+**, but this alone does not provide eBPF/XDP parity.

### Conclusion
Supporting "all Windows versions" with one modern binary and full eBPF/XDP/MsQuic feature set is not technically achievable with current upstream support boundaries.

### Practical support model (recommended)
1. **Tier A (Full Feature)**: Windows 11 / Server 2022+
  - VX6 core + Windows eBPF/XDP path + MsQuic transport.
2. **Tier B (Core + Partial Acceleration)**: Windows 10 / Server 2016/2019
  - VX6 core user-space networking.
  - Optional MsQuic (OpenSSL mode where operationally valid).
  - No guarantee of full eBPF/XDP parity.
3. **Tier C (Legacy Compatibility Lane)**: Windows 7/8/Server 2008/2012
  - Separate legacy build lane with older Go toolchain.
  - No eBPF/XDP path; fallback user-space transport only.
4. **Tier D (XP/Vista)**
  - Not feasible for current VX6 architecture.
  - Would require a historically frozen legacy fork and obsolete toolchain baseline, with significant security and maintenance risk.
