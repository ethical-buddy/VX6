# VX6 Peer-to-Peer Chat

## Overview

VX6 supports secure peer-to-peer chat in addition to peer-to-peer file transfer.

Each chat session is established directly between two VX6 peers using:

- peer name
- peer ID
- IPv6 address
- encrypted VX6 transport

The chat feature reuses the same secure connection and identity system already used for file transfer.

---

## Goals

The chat system should:

- allow one peer to start a conversation with another peer
- identify both peers by name and peer ID
- use end-to-end encrypted communication
- work over IPv6 peer-to-peer connections
- remain separate from file transfer logic

---

## Command Design

Planned command:

vx6 chat --to <peer-name>
vx6 chat --addr [ipv6]:port
