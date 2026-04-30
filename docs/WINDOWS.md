# Windows Guide

## What This Is

This guide is for people who want to use VX6 on:

- Windows 11
- Windows Server class builds

The Windows-oriented release branch is:

- `Windows-compatible`

That branch is intended to build:

- `vx6.exe`
- `vx6-gui.exe`

## What To Expect

Windows is meant to follow the same protocol and feature behavior as Linux:

- signed discovery
- DHT-backed lookup
- public services
- private service catalogs
- hidden services
- file transfer
- GUI

The main differences are platform runtime details, not the network protocol.

## What Is Still Linux-Only

- systemd workflows
- eBPF/XDP attach and live Linux kernel status

## Build

If you are building for Windows from Go:

```powershell
go build -o vx6.exe ./cmd/vx6
go build -o vx6-gui.exe ./cmd/vx6-gui
```

## Notes

- transport is TCP-only in the current release
- VX6 still expects IPv6-capable node endpoints
- the current Linux-first branch is `main`
