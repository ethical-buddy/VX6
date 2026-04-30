# Windows Guide

## What This Is

This guide is for running VX6 on:

- Windows 11
- Windows Server class builds

The Windows-compatible branch is intended to produce:

- `vx6.exe`
- `vx6-gui.exe`

## What Works On Windows

- node initialization
- node runtime
- signed discovery
- DHT lookup
- public services
- private service catalogs
- hidden services
- file transfer
- GUI
- runtime status and reload through the local control channel

## What Is Still Linux-Only

- systemd integration
- eBPF/XDP attach and live Linux kernel status

## Build

From source:

```powershell
go build -o vx6.exe ./cmd/vx6
go build -o vx6-gui.exe ./cmd/vx6-gui
```

If your environment has `make`:

```powershell
make build
```

## First Run

Initialize:

```powershell
.\vx6.exe init --name alice --listen "[::]:4242"
```

Run:

```powershell
.\vx6.exe node
```

Check status:

```powershell
.\vx6.exe status
```

Open the GUI:

```powershell
.\vx6-gui.exe
```

## Notes

- VX6 still expects IPv6-capable node endpoints
- the transport is TCP-only in the current release
- hidden services, DHT, and relay behavior are the same protocol features as Linux

## Current Limitation

Windows runtime compatibility is in place, but the following are still outside the current release polish:

- packaged installer
- service manager integration
- automatic firewall rule management
- deeper live Windows soak testing
