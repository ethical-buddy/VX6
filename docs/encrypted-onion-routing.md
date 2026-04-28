# Encrypted Onion Routing In VX6

This file explains the new relay encryption path in simple language.

It answers 4 questions:

1. What files contain the logic?
2. What encryption is being used?
3. How does a circuit get built?
4. What can each relay actually see?

## Quick Summary

VX6 now protects relay traffic in **two layers**:

1. **Hop-to-hop secure transport**
   Every direct TCP link between two VX6 nodes is protected by the existing secure session code.

2. **Circuit-layer onion encryption**
   Inside that secure link, VX6 sends fixed-size onion cells.
   Each relay can only open **its own layer**.

So the model is:

- your ISP sees you talking to your first hop
- your first hop sees your IP, because you connected to it directly
- middle relays do **not** see the final target
- only the last relay sees the final target
- the data inside the relay path is protected hop by hop

## Main Files To Check

### 1. Outer secure transport

File:
- `internal/secure/session.go`

What to read:
- `handshake`
- `Read`
- `Write`

What it does:
- uses `X25519` to create a shared secret
- uses `Ed25519` to prove node identity
- uses `AES-GCM` to encrypt the direct VX6-to-VX6 socket

Important point:
- this is the encryption for the **direct neighbor connection**
- this is **not** the whole onion circuit by itself

### 2. Cell format and per-hop circuit keys

File:
- `internal/onion/cell.go`

What to read:
- constants at the top
- `writeCell`
- `readCell`
- `buildCreatedPayload`
- `verifyCreatedPayload`
- `deriveCircuitKeys`
- `sealForward`
- `openForward`
- `sealBackward`
- `openBackward`
- `encodeRelayEnvelope`
- `decodeRelayEnvelope`

What it does:
- defines the fixed-size cell format
- defines the per-hop circuit crypto
- defines relay commands like `EXTEND`, `BEGIN`, `DATA`, `END`

### 3. Client-side circuit building

File:
- `internal/onion/onion.go`

What to read:
- `DialPlannedCircuit`
- `establishFirstHop`
- `extendCircuit`
- `beginCircuit`
- `sendRelayCommand`
- `readRelayResponse`
- `clientCircuitConn`

What it does:
- chooses relays
- builds the relay chain one hop at a time
- wraps commands in layered encryption
- exposes the finished circuit as a normal `net.Conn`

### 4. Relay-side hop processing

File:
- `internal/onion/circuit.go`

What to read:
- `HandleExtend`
- `serve`
- `handleRelayCell`
- `handleExtendCommand`
- `handleBeginCommand`
- `pumpOutbound`
- `pumpTarget`

What it does:
- accepts a new encrypted circuit
- decrypts only the current hop layer
- forwards inner opaque payload onward
- acts as the exit node when it receives `BEGIN`

### 5. Node entry point

File:
- `internal/node/node.go`

What to read:
- the `case proto.KindExtend` branch

What it does:
- upgrades incoming relay traffic into a secure VX6 session first
- then passes it into the onion relay handler

### 6. Real behavior checks

Files:
- `internal/onion/cell_test.go`
- `internal/integration/swarm_test.go`

What to read:
- `TestLayeredRelayWrapUnwrap`
- `TestSixteenNodeSwarmServiceAndProxy`
- `TestHiddenServiceRendezvousPlainTCP`
- `TestOnionRelayInspectVisibility`

What they prove:
- cell crypto unwraps in the right order
- multi-node relay traffic still works
- hidden rendezvous still works
- only the exit hop sees the final target

## Encryption Used

VX6 currently uses these pieces:

### Outer hop-to-hop session

Used in:
- `internal/secure/session.go`

Crypto:
- `X25519` for ephemeral key agreement
- `Ed25519` for node identity verification
- `AES-GCM` for encrypted transport
- `SHA-256` to derive the transport key from the shared secret

Purpose:
- protects the raw TCP socket between adjacent VX6 nodes

### Inner onion circuit layer

Used in:
- `internal/onion/cell.go`

Crypto:
- `X25519` again, but this time per circuit hop
- `Ed25519` signature on the `CREATED` reply so the client knows which relay answered
- `HKDF-SHA256` to derive:
  - forward key
  - backward key
- `AES-GCM` for the actual layer encryption

Purpose:
- hides deeper relay instructions from earlier relays

## The Two Layers In Plain Language

Think of it like this:

### Layer 1: Safe pipe between neighbors

If node A talks directly to node B, that pipe is encrypted.

That means:
- a random machine on the network cannot read the bytes
- the ISP cannot read the bytes
- but they can still see A is talking to B

### Layer 2: Wrapped message inside the pipe

Inside that pipe, VX6 sends a message that is wrapped for multiple relays.

That means:
- relay 1 removes only its own wrapper
- relay 2 removes only its own wrapper
- relay 3 removes only its own wrapper

Earlier relays do not get to see the deeper instructions.

## How A Circuit Is Built

### Step 1: Pick relays

The client creates a plan:

- relay 1
- relay 2
- relay 3
- final target

This happens in `DialPlannedCircuit`.

### Step 2: Open a secure connection to relay 1

The client first opens a direct secure VX6 connection to relay 1.

This uses the outer transport in `internal/secure/session.go`.

### Step 3: Create the first hop

The client sends a `CREATE` cell with a fresh `X25519` public key.

Relay 1:
- generates its own fresh `X25519` key
- signs its reply with its long-term `Ed25519` key
- returns a `CREATED` cell

Now both sides derive:
- forward AES-GCM key
- backward AES-GCM key

### Step 4: Extend to relay 2

The client creates an `EXTEND` command saying:

- connect to relay 2
- expect relay 2 identity
- use this new client ephemeral key for that next hop

Important:
- that `EXTEND` command is encrypted with relay 1's forward key

So:
- relay 1 can read it
- nobody before relay 1 exists
- nobody after relay 1 is involved yet

Relay 1 then:
- opens a secure link to relay 2
- forwards a new `CREATE` cell using the next hop's client ephemeral key
- gets relay 2's `CREATED` reply
- sends that back to the client through the backward path

The client then learns the keys for relay 2.

### Step 5: Extend to relay 3

Now the client sends another `EXTEND`.

But this time the command is wrapped like this:

- encrypt for relay 3 layer
- then encrypt that result for relay 2 layer
- then encrypt that result for relay 1 layer

When relay 1 opens its layer, it only sees:
- forward this onward

When relay 2 opens its layer, it sees:
- connect to relay 3

Relay 1 does not see the relay 3 details.

### Step 6: Begin the final connection

Once all relays are set up, the client sends `BEGIN`.

Only the last relay can open that final command.

That last relay then dials the real target.

### Step 7: Send data

After that:

- client writes bytes to the circuit
- VX6 turns them into `DATA` relay cells
- each hop peels one layer
- exit hop gets the plaintext payload for local delivery to the target socket

Replies come back in reverse:

- exit hop wraps with its backward key
- previous hop wraps with its backward key
- previous hop wraps again
- client unwraps layer by layer

## What Each Hop Can See

Assume this path:

`Client -> Relay1 -> Relay2 -> Relay3 -> Target`

### Relay 1 can see

- client IP, because client connected directly to it
- its own direct next hop, usually relay 2
- that traffic exists

Relay 1 cannot see:
- the final target
- the deeper relay commands once more layers exist

### Relay 2 can see

- relay 1 as previous hop
- its own direct next hop, usually relay 3

Relay 2 cannot see:
- client IP directly
- final target, unless it is also the exit hop

### Relay 3, the exit hop, can see

- relay 2 as previous hop
- the final target address
- plaintext application bytes that it must deliver to the target socket

Relay 3 still does not see:
- client IP directly

## What Is Hidden And What Is Not

### Hidden

- middle relays do not know the final target
- earlier relays do not know deeper routing details
- raw relay bytes on the wire are encrypted between adjacent nodes
- circuit commands are layered

### Not hidden

- the first hop still knows who connected to it
- the exit hop still knows the final target
- the exit hop sees plaintext app bytes because it must talk to the real target socket

So this is stronger than the old plaintext extend path, but it does **not** make the first hop blind to the sender.

## Why The Exit Hop Sees More

The exit hop must:

- know where to dial
- forward the real bytes into the target socket

Because of that, the exit hop can see:

- target address
- application payload at that final step

This is normal for an onion-style exit node unless you add another end-to-end application encryption layer on top.

For example:
- SSH over VX6
- HTTPS over VX6

In those cases, the exit hop still carries bytes, but the application protocol itself also encrypts the content.

## Fixed-Size Cells

VX6 now uses a fixed cell size of `1024` bytes.

Why this helps:
- relay traffic looks more regular
- routing metadata is no longer sent as easy-to-read JSON messages on the wire
- it is easier to reason about layer-by-layer forwarding

Note:
- the used payload length is still stored in the cell header
- so this is a strong cleanup and a better base, not full traffic-shaping anonymity

## Simple Example

If the client wants to send:

`hello`

and the path is:

`R1 -> R2 -> R3`

then VX6 conceptually does:

1. make a `DATA(hello)` message
2. encrypt it for `R3`
3. encrypt that result for `R2`
4. encrypt that result for `R1`
5. send it

Then:

- `R1` removes its layer and forwards
- `R2` removes its layer and forwards
- `R3` removes its layer and gets `DATA(hello)`

## What Changed Compared To The Old Design

Before this change:

- relays received plaintext `ExtendRequest`
- relays could directly read `next_hop`
- the protocol was basically `read JSON -> dial next hop -> raw io.Copy`

Now:

- `KindExtend` first becomes a secure VX6 session
- cells are fixed-size
- each relay has its own per-hop keys
- relay commands are layered
- only the correct hop can open the command meant for it

## What Still Remains To Build

This new design completes the main relay encryption and layered hop processing.

The main hidden-mode items still left are:

1. active circuit failover
   If a relay dies in the middle of an open session, VX6 should rebuild faster without needing a full user retry.

2. abuse controls
   Examples:
   - intro flood limits
   - rendezvous limits
   - setup throttling

## Best Code Reading Order

If you want to understand the implementation quickly, read in this order:

1. `internal/secure/session.go`
2. `internal/onion/cell.go`
3. `internal/onion/onion.go`
4. `internal/onion/circuit.go`
5. `internal/node/node.go`
6. `internal/integration/swarm_test.go`

That order matches the real runtime path:

- secure link
- cell format
- client build
- relay behavior
- node dispatch
- integration proof
