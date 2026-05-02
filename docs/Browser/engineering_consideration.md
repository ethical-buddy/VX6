## 1. Rendering & Layout

The hardest part of any browser. Even if you embed WebKit/Blink, you need to understand what you're getting:

Webview quirks per OS — WebKitGTK on Linux, WKWebView on macOS, and Edge WebView2 on Windows all behave slightly differently. CSS rendering, font rendering, scroll behavior, and JS engine versions will differ. You cannot assume pixel-perfect consistency across platforms. WebView2 on Windows requires a runtime — Edge WebView2 must be installed on the user's machine. If it's not there, your app won't render. You need to either bundle the fixed-version WebView2 runtime or handle the "not installed" case gracefully with a download prompt. WebKitGTK on Linux has version fragmentation — Ubuntu 20.04 ships a very different WebKitGTK version than Arch. Test on multiple distros. Font rendering — each OS has its own font stack. Don't hardcode font names. Use system font stacks and test on each OS.

## 2. Navigation Model

Things every browser needs that are easy to overlook:

History stack — back/forward buttons require a proper history stack, not just a URL string. Each tab has its own stack. Session restore — if the app crashes or is closed, can the user reopen with their previous tabs? Navigation lifecycle — you need to handle: page load started, DOM ready, page fully loaded, navigation error, navigation blocked (by your VX6 protocol handler). Each state needs a UI signal (spinner, error page, etc.). Redirect chains — vx6:// resolves to http://127.0.0.1:PORT which may itself redirect. Your protocol handler must not break redirect chains. Cancelled navigation — user types a new URL before the current one finishes. You need to cancel the in-flight request cleanly.

## 3. Security Model

This is the most critical area and the easiest to get wrong:

Same-origin policy — your custom vx6:// scheme needs a defined origin model. Two different VX6 services should not be able to read each other's cookies/localStorage. Define vx6://alice.ssh and vx6://bob.api as separate origins. Mixed content — if a vx6:// page loads resources from http:// (not HTTPS), decide your policy upfront. Block it, warn, or allow. Don't leave it undefined. Content Security Policy — decide whether you enforce CSP on VX6-served pages. Probably yes, since you're proxying through localhost. Localhost trust boundary — your forwarded ports on 127.0.0.1 are accessible to any process on the machine. This means a malicious local process could try to intercept your forwarded VX6 sessions. Mitigate by binding to random high ports, not predictable ones, and rotating them per session. Protocol handler injection — if a regular web page can trigger vx6:// navigations, that's a security hole. Your custom protocol should only be triggerable from within the VX6 browser itself, not from arbitrary web content. Certificate handling — when forwarding a VX6 service over http://127.0.0.1, the webview will not show HTTPS. That's fine for VX6 services (VX6 handles transport encryption at the layer below), but you need to make sure the user understands this and you don't accidentally display a "not secure" warning that confuses them. Hidden service security — hidden service connections involve invite secrets. These must never be stored in plaintext, never appear in browser history URLs, and never be logged.

## 4. Process Model

Real browsers use multi-process architecture for isolation and stability. You need to decide your model:

Single process — simplest to build. If the webview crashes, the whole app crashes. Acceptable for a v1 VX6 browser. One process per tab — much more work, but a tab crash doesn't kill other tabs. Required if you want "production grade." VX6 runtime as separate process — this is non-negotiable. The vx6 node Go process must run independently of the UI process. If the UI crashes, the node keeps running. If the node crashes, the UI should detect it and show a reconnect state, not crash itself. Crash recovery — what happens when the VX6 node process dies unexpectedly? The browser needs to detect this (watch the process, or ping the control socket), show an error state, and offer a restart button.

## 5. Tab Management

Each tab needs its own webview instance (or a shared webview with navigation swapping — simpler but worse UX). Each tab needs its own navigation history. Each tab that connects to a VX6 service holds an active forwarded port — when the tab closes, you must release that port and tell the VX6 runtime to stop forwarding. Resource leaks — if you don't clean up forwarded ports on tab close, you'll exhaust available ports over time.

## 6. Permissions Model

Browsers gate access to camera, microphone, location, notifications, etc. You need to decide:

Do VX6 services get access to device APIs? Probably needs explicit user permission per service, stored per VX6 service identity (not per localhost port, since ports rotate). File system access — if a VX6 service requests file upload/download, the webview will trigger the OS file picker. That's fine, but make sure the downloaded file destination is sane. Clipboard — decide if VX6 services can read the clipboard. Most browsers ask. You should too.

## 7. Storage & State

Cookies — scope cookies to the VX6 service identity (alice.ssh), not the localhost port (which changes). If you scope to port, cookies break every reconnect. localStorage / IndexedDB — same issue. Must be scoped to service identity. Cache — HTTP cache for resources served by VX6 services. Decide max size and eviction policy. Bookmarks — store vx6://service-name, not http://127.0.0.1:PORT. History — same. Store the VX6 URL, not the forwarded localhost URL. Never store invite secrets or hidden service resolution details in history. Profile isolation — if multiple users share a machine, their VX6 identities and browser profiles should be fully separate.

## 8. Networking

Proxy model — regular http:// and https:// URLs should go through the OS/system network stack as normal. Only vx6:// URLs go through your runtime bridge. Don't accidentally route all traffic through VX6. DNS for regular URLs — the webview handles this normally. Don't interfere with it. IPv6 first — VX6 is IPv6-native. Your forwarded ports on localhost are 127.0.0.1 (IPv4 loopback). That's fine — localhost forwarding is local, IPv6 is used for the actual peer transport. But make sure you're not accidentally forcing IPv4 for the VX6 peer connections. Timeout handling — what if a VX6 service never becomes reachable? Set a connection timeout (e.g., 15 seconds), then show an error page, not a forever-spinning tab. Offline state — if the VX6 node loses all peers, what does the browser show? Detect this and surface it clearly.

## 9. UI/UX Requirements

These are easy to underestimate:

Address bar behavior — pressing Enter on a bare service name like alice.ssh should resolve as vx6://alice.ssh. Pressing Enter on https://example.com should load normally. Entering garbage should offer a search or show an error. Loading states — two-phase loading: first the VX6 resolution (DHT lookup + port forwarding setup), then the actual page load. Show a distinct state for each so the user knows if the delay is VX6 or the service itself. Error pages — you need custom error pages for: service not found in DHT, service found but unreachable, connection timed out, hidden service invite required, signature verification failed. Node status always visible — the user must always be able to see whether the VX6 node is running, how many peers are connected, and whether DHT is healthy. Don't hide this in a buried settings screen. Tray icon on all platforms — since the VX6 node runs as a background process, a system tray icon (Linux, macOS, Windows) lets the user know it's running and gives a quick way to open the browser or stop the node.

## 10. Cross-Platform Packaging Requirements

Code signing — macOS requires app signing and notarization or users get a "cannot be opened" Gatekeeper warning. You need an Apple Developer account ($99/year). Windows SmartScreen will flag unsigned executables. You need a code signing certificate. Auto-update — users won't manually update. Build in an auto-update mechanism from day one. Tauri has tauri-plugin-updater for this. Define your update server endpoint early. Bundled binary per platform — the vx6 Go binary must be compiled for each target: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 (Apple Silicon), windows/amd64. That's 5 separate binaries to ship and keep in sync with the browser version. Installer behavior — on Windows, your installer must handle: first install, upgrade, uninstall, and the case where WebView2 runtime is missing. On macOS, drag-to-Applications must work. On Linux, both .deb and .AppImage are expected. Startup on login — users will want the VX6 node to start automatically. This means: systemd user unit on Linux, launchd plist on macOS, registry run key on Windows. Each has its own quirks.
