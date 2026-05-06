# VX6 Proxy Launcher

`vx6-proxy` is a minimal launcher binary that opens a local browser UI and controls VX6 from one page.

## What It Does

- opens `http://127.0.0.1:17886`
- Start/Stop/Reload VX6 node
- live runtime stats (registry, DHT, hidden descriptor health)
- command panel to run VX6 CLI commands (except `node`, managed by Start/Stop)

## Build

### Linux

```bash
go build -o vx6-proxy ./cmd/vx6-proxy
```

### Windows

```bash
GOOS=windows GOARCH=amd64 go build -o vx6-proxy.exe ./cmd/vx6-proxy
```

### macOS

```bash
GOOS=darwin GOARCH=arm64 go build -o vx6-proxy ./cmd/vx6-proxy
```

## Run

```bash
./vx6-proxy
```

The binary auto-opens your default browser and keeps node control in the background process.

