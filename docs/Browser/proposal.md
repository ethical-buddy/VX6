# VX6 Browser — Architecture Proposal

> **Status:** Draft — open for contribution and discussion  
> **Scope:** A VX6-native browser shell that understands peer service names, hidden aliases, and the VX6 node runtime — wrapped for Linux, macOS, and Windows.

---

## Why This Exists

Right now, if we want to reach a VX6 service from a browser, we have to manually run `vx6 connect`, figure out which local port it forwarded to, and type `http://127.0.0.1:PORT` into the browser. That works for developers who live in the terminal, but it is not how most people use software.

The VX6 browser closes that gap. It understands what `vx6://alice.dashboard` means, handles the DHT lookup, sets up port forwarding, and connects — then the page simply loads. No manual steps. No exposed ports. No confusion about why `127.0.0.1:8080` stopped working after a reconnect.

It is also the right place for us to surface node health, peer status, and service discovery in a way non-technical users can actually read.

---

## Goals

- Let users navigate to VX6 services using `vx6://service-name` the same way they navigate to websites
- Keep the VX6 node runtime decoupled from the UI — the node runs whether the browser is open or not
- Work on Linux, macOS, and Windows without separate codebases
- Be small — this is a network tool, not a general-purpose browser; binary size and resource usage matter
- Make the security model explicit and auditable — hidden services, invite secrets, and service isolation need to be correct from day one, not bolted on later

---

## What This Is Not

This is not a replacement for Firefox or Chrome. We are not trying to render the full public web better than existing browsers. For regular `https://` URLs it just passes through to the OS webview. The VX6-specific behaviour is the contribution here.

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      User Interface                      │
│         (HTML / CSS / JS — React or Svelte)              │
│                                                          │
│   address bar · tabs · service panel · node status       │
└───────────────────────┬─────────────────────────────────┘
                        │  IPC (Tauri commands)
┌───────────────────────▼─────────────────────────────────┐
│                   Native Backend                         │
│                    (Rust — Tauri)                        │
│                                                          │
│   protocol handler · port allocator · process manager   │
│   control socket client · storage scoping layer         │
└───────────────────────┬─────────────────────────────────┘
                        │  Unix socket / named pipe
┌───────────────────────▼─────────────────────────────────┐
│                   VX6 Node Runtime                       │
│                  (Go — existing binary)                  │
│                                                          │
│   DHT · registry · service forwarding · hidden services  │
└─────────────────────────────────────────────────────────┘
```

Three layers. Each one can fail independently without taking the others down. If the UI crashes, the node keeps running. If the node restarts, we do not lose open tabs.

---

## Tech Stack Decisions

### Shell — Tauri

We believe Tauri is the right choice here for a few concrete reasons.

VX6 already has a Go runtime that handles all the heavy networking. The browser shell does not need to reimplement any of that — it just needs to talk to it. Tauri's Rust backend gives a thin, fast bridge to the OS without shipping a second copy of Chromium. The final installer ends up around 10–15 MB instead of 150 MB.

The other reason is that Tauri's IPC model maps cleanly onto the VX6 control socket pattern. We write a Rust command, it opens a socket to the Go runtime, and returns JSON to the frontend. That is a short, auditable path.

Electron is a valid alternative if the team has stronger JavaScript/Node experience and binary size is not a concern. The architecture described here applies to both — swap "Tauri command" for "ipcMain handler" and "Rust" for "Node.js" and everything else stays the same.

### Rendering — OS Native Webview

| Platform | Webview Engine | Notes                                                              |
| -------- | -------------- | ------------------------------------------------------------------ |
| Linux    | WebKitGTK      | Version varies by distro — test on Ubuntu LTS and Arch             |
| macOS    | WKWebView      | Safari engine — consistent across macOS versions we care about     |
| Windows  | Edge WebView2  | Requires WebView2 runtime — handle missing runtime at install time |

We do not ship Chromium. We use what the OS provides. This means there will be minor rendering differences across platforms — that is acceptable for a network tool. If pixel-perfect cross-platform rendering becomes a hard requirement later, Chromium embedding (via CEF) can be revisited.

### Frontend — Svelte (recommended) or React

Svelte produces smaller bundles and has less runtime overhead. For a browser shell where the UI is relatively modest (address bar, tabs, sidebar), this matters. React is fine if contributors are more comfortable there — the architecture does not depend on the choice.

### Build & Packaging — Tauri CLI + GitHub Actions

Tauri's CLI handles platform packaging. GitHub Actions with a three-OS matrix (ubuntu-latest, macos-latest, windows-latest) produces all installers in CI. No manual packaging steps.

---

## Component Breakdown

### 1. Address Bar & Navigation

The address bar is the main VX6-aware component. It needs to understand three kinds of input:

- `vx6://alice.dashboard` — a VX6 service name, route through the VX6 protocol handler
- `https://example.com` — a regular URL, pass directly to the webview
- `alice.dashboard` (bare name, no scheme) — treat as `vx6://alice.dashboard` if it does not look like a regular hostname or search query

Navigation lifecycle states the UI must handle:

```
idle
  → resolving        (DHT lookup in progress)
  → connecting       (port forward being established)
  → loading          (webview fetching the page)
  → loaded           (done)
  → error            (one of several failure modes — see Error Pages section)
```

The distinction between "resolving" and "connecting" matters. A slow DHT lookup is a VX6 network problem. A slow page load after the port is up is a service problem. Users should be able to tell the difference.

### 2. VX6 Protocol Handler

This is the core custom piece. When the address bar sees a `vx6://` URL:

1. Extract the service name from the URL
2. Call the native backend to request a local forwarded port for that service name
3. The backend calls `vx6 connect --service <name> --listen 127.0.0.1:<random-port>`
4. Once the port is ready, navigate the webview to `http://127.0.0.1:<port>`
5. When the tab closes, call the backend to release the port and stop the forwarder

The webview never sees the `vx6://` URL. It only ever loads `http://127.0.0.1:<port>`. The protocol handler is entirely in the native backend.

```rust
// Tauri command (simplified)
#[tauri::command]
async fn resolve_vx6_service(service_name: String) -> Result<u16, String> {
    let port = allocate_port();
    spawn_vx6_connect(&service_name, port).await?;
    Ok(port)
}
```

**Important:** Register `vx6://` as a custom URI scheme in `tauri.conf.json`. This prevents regular web pages loaded in the browser from triggering VX6 navigations — only the browser's own UI can initiate them.

### 3. Port Allocator

A small module in the Rust backend that:

- Picks a random port in the high range (49152–65535) for each new service connection
- Tracks which ports are in use and by which tab/service
- Releases ports when tabs close or connections drop
- Never reuses a port until it has been confirmed released

This must be a proper allocator, not just a counter. If we increment a counter and the previous forwarder on that port has not fully closed yet, the new connection will fail silently.

### 4. Storage Scoping Layer

This is the part that is easiest to get wrong.

The webview will try to scope cookies, localStorage, and IndexedDB to the origin it sees — which is `http://127.0.0.1:PORT`. That port changes every session. Every reconnect would wipe the service's stored state from the browser's perspective.

The scoping layer sits between the frontend and the webview and remaps storage origins:

```
vx6://alice.dashboard  →  storage origin: vx6://alice.dashboard
  (even though the webview loads http://127.0.0.1:52341)
```

In Tauri this is handled by intercepting webview data directory configuration and using the VX6 service identity as the storage partition key. In Electron it is `session.fromPartition('vx6:alice.dashboard')`.

Bookmarks and history must also store the `vx6://` URL, not the localhost URL.

### 5. VX6 Runtime Process Manager

The browser bundles the compiled `vx6` binary for each target platform. On first launch:

1. Copy the bundled binary to the user's data directory
2. Check if a VX6 config exists — if not, run `vx6 init` with sensible defaults and prompt the user to review
3. Start `vx6 node` as a background subprocess
4. Connect to the control socket and verify the node is healthy

On subsequent launches, check if the node is already running (via the PID file or control socket ping) before starting a new one.

The process manager must handle:

- Node crashes — detect via process exit or socket timeout, surface a "node offline" banner, offer restart
- Node updates — when the browser updates and ships a new `vx6` binary version, replace the old one and restart the node
- Clean shutdown — when the browser exits, gracefully stop all port forwarders before killing the node process

### 6. Node Status Panel

Always visible. Never hidden in a submenu. Shows:

- Node running / stopped / restarting
- Connected peer count
- DHT health (publishing, degraded, offline)
- Active service connections (which tabs are forwarding which services)
- Identity (node name, truncated node ID)

This panel is the user's only window into whether the decentralized network is actually working. It needs to be honest — "0 peers connected" should look like a warning, not a normal state.

### 7. Service Discovery Panel

A sidebar or drawer that shows services the node knows about:

- Services from the local registry (directly known peers)
- Services found via recent DHT lookups

Clicking a service navigates to `vx6://service-name`. This is how users discover what is available on the network without needing to know exact names in advance.

### 8. Error Pages

Custom error pages for each failure mode — not the webview's default "ERR_CONNECTION_REFUSED":

| Error                       | Cause                                  | What to Show                                                                       |
| --------------------------- | -------------------------------------- | ---------------------------------------------------------------------------------- |
| `vx6://service-not-found`   | DHT lookup returned nothing            | "No service with this name was found on the network. Check the name or try again." |
| `vx6://service-unreachable` | Service found but connection failed    | "This service is known but not currently reachable. The peer may be offline."      |
| `vx6://timeout`             | Resolution or connection took too long | "Connection timed out. The network may be degraded."                               |
| `vx6://invite-required`     | Hidden service needs an invite secret  | Form to enter the invite secret                                                    |
| `vx6://signature-invalid`   | DHT record failed signature check      | "This service record could not be verified. It may have been tampered with."       |
| `vx6://node-offline`        | Local VX6 node is not running          | "The VX6 node is not running." + restart button                                    |

The signature-invalid error is important to surface explicitly. It should not silently fall back to "unreachable" — the user needs to know the difference between a missing service and a potentially compromised one.

### 9. Hidden Service Flow

Hidden services require extra UI care because they involve invite secrets.

When a user navigates to a hidden service alias:

1. Show a dedicated connection screen — not just a loading spinner
2. If an invite secret is required, show a prompt for it
3. Never store the invite secret in browser history, bookmarks, or logs
4. Show clearly that this is a hidden service connection (distinct visual treatment from regular VX6 services)
5. After successful connection, the tab loads normally

The invite secret prompt should behave like a password field — masked input, no autocomplete, cleared from memory after use.

### 10. Tray Icon

The VX6 node runs as a background process. Users need a way to know it is running and interact with it without opening the full browser window.

Tray icon menu:

```
VX6 — 4 peers connected
─────────────────────────
Open Browser
─────────────────────────
Node: Running  ●
DHT: Healthy   ●
─────────────────────────
Restart Node
Stop Node
─────────────────────────
Quit
```

On Linux this uses the system tray API (AppIndicator or StatusNotifierItem). On macOS it uses NSStatusBar. On Windows it uses the system notification area. Tauri provides a cross-platform tray API that handles all three.

---

## Security Model

### Origin Isolation

Each VX6 service is a separate origin: `vx6://alice.dashboard` and `vx6://bob.api` cannot access each other's storage, cookies, or DOM. This is enforced by the storage scoping layer and by keeping each service in a separate webview partition.

### Protocol Handler Restriction

The `vx6://` protocol can only be triggered by the browser's own UI. Web content loaded inside the webview cannot navigate to a `vx6://` URL and cannot call the native backend. This prevents a malicious service from pivoting to other VX6 services or to the node control socket.

### Localhost Port Security

Forwarded ports are bound to `127.0.0.1` on randomly allocated high ports. They are not predictable. Each port is only open for the duration of the tab's session. No port is reused within a session.

A separate local process could theoretically probe random high ports and land on one of these. This is an acceptable risk for a v1 — full mitigation would require per-connection authentication tokens on the forwarded port, which can be added later.

### Hidden Service Secrets

Invite secrets are: held in memory only during the connection handshake, never written to disk, never appear in URLs, never appear in logs. The browser's history stores `vx6://alias-name`, not the resolved hidden descriptor or the invite secret.

### Signature Verification

The browser UI surfaces signature failures explicitly (see Error Pages). It does not silently ignore them or fall back to "unreachable". A signature failure is treated as a trust failure, not a network failure.

---

## Cross-Platform Packaging

### Build Matrix

```yaml
# .github/workflows/build.yml (simplified)
strategy:
  matrix:
    include:
      - os: ubuntu-latest
        target: x86_64-unknown-linux-gnu
        go_target: linux/amd64
      - os: ubuntu-latest
        target: aarch64-unknown-linux-gnu
        go_target: linux/arm64
      - os: macos-latest
        target: x86_64-apple-darwin
        go_target: darwin/amd64
      - os: macos-latest
        target: aarch64-apple-darwin
        go_target: darwin/arm64
      - os: windows-latest
        target: x86_64-pc-windows-msvc
        go_target: windows/amd64
```

Each job compiles the `vx6` Go binary for its target, then runs `tauri build` with that binary bundled.

### Output Formats

| Platform       | Format                      | Notes                                               |
| -------------- | --------------------------- | --------------------------------------------------- |
| Linux x86_64   | `.deb`, `.AppImage`, `.rpm` | AppImage is the most portable — no install required |
| Linux arm64    | `.deb`, `.AppImage`         | For Raspberry Pi and ARM servers                    |
| macOS x86_64   | `.dmg`, `.app`              | Intel Macs                                          |
| macOS arm64    | `.dmg`, `.app`              | Apple Silicon                                       |
| Windows x86_64 | `.msi`, `.exe` (NSIS)       | MSI for enterprise, NSIS for general users          |

### Code Signing

**macOS** requires signing and notarization. Without it, every user gets a Gatekeeper warning on first launch. We need an Apple Developer account and the notarization step in CI. This is non-negotiable for any release that goes to users outside the dev team.

**Windows** SmartScreen will show a warning for unsigned executables. A code signing certificate reduces (but does not eliminate) this. EV certificates suppress the warning immediately; standard OV certificates need to accumulate reputation first.

**Linux** does not require signing but `.deb` and `.rpm` packages should be signed with GPG for repository distribution.

### WebView2 on Windows

Edge WebView2 may not be installed on older Windows 10 machines. The installer must check for it and offer to download the WebView2 bootstrapper if missing. Tauri handles this in the NSIS installer template — make sure it is enabled in `tauri.conf.json`:

```json
"windows": {
  "webviewInstallMode": {
    "type": "downloadBootstrapper"
  }
}
```

### Auto-Update

We should ship with auto-update enabled from the first release. The update check should happen on launch, and updates should install in the background with a "restart to apply" prompt. Tauri's updater plugin handles the mechanism — we need to host a `latest.json` update manifest at a stable URL and sign update artifacts with a separate keypair.

Define the update keypair before the first release and keep it offline. Losing it means we cannot ship signed updates.

---

## Repository Structure

```
vx6-browser/
├── src/                        # Frontend (Svelte or React)
│   ├── components/
│   │   ├── AddressBar.svelte
│   │   ├── Tab.svelte
│   │   ├── TabBar.svelte
│   │   ├── NodeStatusPanel.svelte
│   │   ├── ServiceDiscoveryPanel.svelte
│   │   ├── TrayMenu.svelte
│   │   └── error-pages/
│   │       ├── ServiceNotFound.svelte
│   │       ├── InviteRequired.svelte
│   │       └── SignatureInvalid.svelte
│   ├── lib/
│   │   ├── vx6-protocol.ts     # vx6:// URL handling logic
│   │   ├── navigation.ts       # History stack, tab state
│   │   └── node-status.ts      # Control socket polling
│   └── App.svelte
│
├── src-tauri/                  # Rust backend (Tauri)
│   ├── src/
│   │   ├── main.rs
│   │   ├── commands/
│   │   │   ├── resolve.rs      # vx6:// resolution command
│   │   │   ├── node.rs         # Node start/stop/status
│   │   │   └── storage.rs      # Storage scoping
│   │   ├── port_allocator.rs
│   │   ├── process_manager.rs
│   │   ├── control_socket.rs   # VX6 runtime socket client
│   │   └── tray.rs
│   ├── binaries/               # Bundled vx6 binaries per platform
│   │   ├── vx6-x86_64-unknown-linux-gnu
│   │   ├── vx6-aarch64-unknown-linux-gnu
│   │   ├── vx6-x86_64-apple-darwin
│   │   ├── vx6-aarch64-apple-darwin
│   │   └── vx6-x86_64-pc-windows-msvc.exe
│   └── tauri.conf.json
│
├── .github/
│   └── workflows/
│       ├── build.yml           # Cross-platform build matrix
│       └── release.yml         # Signed release + update manifest
│
└── docs/
    ├── architecture.md         # This document
    ├── security.md             # Threat model and security decisions
    ├── contributing.md         # How to contribute
    └── platform-notes.md       # Per-platform quirks and workarounds
```

---

## What Needs to Be Built — In Order

This is the recommended sequence for contributors. Each step produces something testable before the next one starts.

**Step 1 — Tauri scaffold + VX6 binary launch**
Set up the Tauri project. Bundle the `vx6` binary. Write a Rust command that starts the node process and reads its stdout. Verify the node starts and produces output. No UI yet.

**Step 2 — Control socket client**
Write the Rust control socket client. Connect to the VX6 runtime socket. Send a status ping. Parse the response. Return it to the frontend via a Tauri command. Verify you can read node status from Rust.

**Step 3 — Protocol handler + port allocator**
Implement `vx6://` interception. Write the port allocator. Wire them together so navigating to `vx6://test-service` calls `vx6 connect` and returns a local port. Verify with a real VX6 service on the local machine.

**Step 4 — Minimal UI**
Address bar, single webview, basic tab. Navigation to `vx6://` URLs works. Navigation to `https://` URLs works. Loading states are shown. Error pages for the two most common failures (not found, offline).

**Step 5 — Storage scoping**
Implement the storage partition layer. Verify that cookies and localStorage for `vx6://alice.x` are not visible to `vx6://bob.y`. Verify that cookies survive a reconnect (port change).

**Step 6 — Node status panel + tray**
Surface node health in the UI. Add the tray icon. Test on all three platforms.

**Step 7 — Tab management**
Multi-tab support. Port release on tab close. History per tab.

**Step 8 — Hidden service UI**
Invite secret prompt. Visual distinction for hidden service tabs. Secret handling (no logs, no history, cleared after use).

**Step 9 — Cross-platform packaging**
GitHub Actions matrix. Signing setup. WebView2 install check on Windows. AppImage on Linux. DMG on macOS.

**Step 10 — Auto-update**
Update manifest server. Update keypair. In-app update check and install flow.

---

## Open Questions

These are decisions that need to be made before or during implementation. They are listed here so contributors know what is not yet settled:

- **Multi-tab via multiple webview instances or navigation swap?** Multiple instances give better isolation but cost more memory. Navigation swap is simpler. Recommendation: navigation swap for v1, migrate to separate instances in v2.

- **Should the browser manage the VX6 node lifecycle, or assume the node is already running?** Managing it is more user-friendly but adds complexity. Assuming it is running is simpler but requires users to start it separately. Recommendation: manage it, but detect an already-running node gracefully.

- **How should the service discovery panel be populated?** Polling the VX6 control socket? Subscribing to registry events? Recommendation: polling with a 5-second interval for v1, event subscription when the control protocol supports it.

- **What is the right update cadence?** The `vx6` binary inside the browser and the VX6 node installed separately could get out of sync. Should the browser enforce a minimum node version? Recommendation: yes — check node version on startup and warn if it is below the minimum supported version.

---

## Contributing

If you are picking this up, the best place to start is Step 1 — get the Tauri scaffold running and the `vx6` binary launching from it. That unblocks every other step and does not require deep knowledge of VX6 internals.

The `src-tauri/control_socket.rs` module is the highest-leverage contribution after that. Everything the UI needs flows through it — node status, service resolution, peer count, DHT health. Getting that right early makes the rest of the UI straightforward to build.

If you find something in this document that is wrong, incomplete, or that you disagree with — open an issue or edit the document directly. This architecture is a starting point, not a final word.

---

_Document version: draft-01 — May 2026_
