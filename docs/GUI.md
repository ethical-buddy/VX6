# GUI

VX6 now includes `vx6-gui`.

## What It Is

It is a local web UI that runs on your machine and calls the `vx6` binary underneath.

That means:

- the GUI stays aligned with the CLI
- the protocol logic remains in one place
- Windows and Linux can use the same feature surface

## What It Exposes

- node initialization
- node start
- reload
- status
- identity
- service publishing
- connect tunnels
- file send
- receive policy
- DHT lookups
- eBPF status
- custom CLI argument execution

## Why It Was Built This Way

This release is still stabilizing. A thin GUI over the CLI is safer than duplicating VX6 logic in a separate desktop app.

## What Comes Next

A fuller browser-wrapper style experience is planned, but it is not part of the current release yet.
