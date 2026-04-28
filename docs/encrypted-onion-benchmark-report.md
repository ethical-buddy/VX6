# Encrypted Onion Routing Benchmark Report

Date:
- 2026-04-28

Revision:
- `ded4c82` was the documentation commit before this benchmark pass
- benchmark code and this report were added after the encrypted relay milestone

Machine:
- OS: `Linux 6.19.14-arch1-1 x86_64 GNU/Linux`
- Go: `go1.26.2-X:nodwarf5 linux/amd64`
- CPU: `AMD Ryzen 7 250 w/ Radeon 780M Graphics`
- Cores / threads: `8 cores / 16 threads`

## What Was Verified First

Before recording benchmark numbers, VX6 was rechecked with live multi-node tests and the full suite.

Passed:
- `go test ./internal/integration -run TestSixteenNodeSwarmServiceAndProxy -v`
- `go test ./internal/integration -run TestHiddenServiceRendezvousPlainTCP -v`
- `go test ./internal/integration -run TestOnionRelayInspectVisibility -v`
- `go test ./...`

Meaning:
- normal relay proxy still works
- hidden rendezvous still works
- the relay visibility boundary still works

The relay visibility test proved:
- first hop saw only its own next hop
- middle hop saw only its own next hop
- only the exit hop saw the final target

## What The Benchmarks Mean

`ns/op`:
- average time for one benchmark operation

`B/op`:
- average heap bytes allocated per operation

`allocs/op`:
- average heap allocation count per operation

Important:
- this is **operation memory cost**, not full process RAM or total resident memory
- real end-to-end latency also includes:
  - socket dial time
  - scheduler/goroutine overhead
  - network RTT
  - queueing

## Benchmark Commands

Identity:

```bash
go test -run '^$' -bench 'BenchmarkIdentityGenerate$' -benchmem ./internal/identity
```

Secure transport:

```bash
go test -run '^$' -bench 'BenchmarkSecureHandshake$' -benchmem ./internal/secure
go test -run '^$' -bench 'BenchmarkSecureChunk(RoundTrip|Seal|Open)4K$' -benchmem ./internal/secure
```

Onion relay layer:

```bash
go test -run '^$' -bench 'BenchmarkOnion' -benchmem ./internal/onion
```

## Results

### Identity

| Benchmark | What it measures | Time/op | B/op | allocs/op |
|---|---|---:|---:|---:|
| `BenchmarkIdentityGenerate` | Long-term Ed25519 identity generation | `14.833 us` | `152` | `4` |

### Secure Transport Layer

| Benchmark | What it measures | Time/op | Throughput | B/op | allocs/op |
|---|---|---:|---:|---:|---:|
| `BenchmarkSecureHandshake` | One VX6 neighbor handshake using X25519 + Ed25519 + AES-GCM session setup | `264.363 us` | `-` | `9467` | `102` |
| `BenchmarkSecureChunkRoundTrip4K` | 4 KB AES-GCM seal + open round trip | `2.880 us` | `1422.26 MB/s` | `8992` | `4` |
| `BenchmarkSecureChunkSeal4K` | 4 KB AES-GCM encrypt only | `1.350 us` | `3035.17 MB/s` | `4880` | `2` |
| `BenchmarkSecureChunkOpen4K` | 4 KB AES-GCM decrypt only | `1.384 us` | `2959.14 MB/s` | `4112` | `2` |

### Onion Relay Layer

| Benchmark | What it measures | Time/op | Throughput | B/op | allocs/op |
|---|---|---:|---:|---:|---:|
| `BenchmarkOnionCreateClientKey` | One onion hop ephemeral X25519 keypair generation | `34.926 us` | `-` | `224` | `5` |
| `BenchmarkOnionSingleHopHandshake` | One onion hop setup: create keys, sign `CREATED`, verify, derive per-hop keys | `200.768 us` | `-` | `6194` | `58` |
| `BenchmarkOnionLayerWrap3Hop1K` | Wrap a 1 KB relay payload through 3 forward layers | `1.343 us` | `682.77 MB/s` | `4144` | `7` |
| `BenchmarkOnionLayerUnwrap3Hop1K` | Unwrap a 1 KB relay payload through 3 forward layers | `2.616 us` | `350.50 MB/s` | `7264` | `13` |
| `BenchmarkOnionCellReadWrite` | Fixed-size onion cell encode + decode | `0.598 us` | `1711.71 MB/s` | `4144` | `5` |

## What These Numbers Say

### 1. The expensive part is setup, not payload crypto

The heavy work is:
- secure neighbor handshake: about `264 us`
- onion hop setup: about `201 us`

The light work is:
- 4 KB secure encrypt: about `1.35 us`
- 4 KB secure decrypt: about `1.38 us`
- 3-hop 1 KB wrap: about `1.34 us`

So the main latency cost is still:
- more hops
- more dials
- more network RTT

Not raw AES-GCM.

### 2. A rough local crypto estimate for a full circuit

This is only a CPU-side estimate, not network latency.

For a 3-hop circuit:

- outer secure handshakes: `3 x 264.363 us = 793.089 us`
- onion hop setup: `3 x 200.768 us = 602.304 us`

Estimated local crypto/setup total:

- about `1.395 ms`

For a 5-hop proxy circuit:

- outer secure handshakes: `5 x 264.363 us = 1.322 ms`
- onion hop setup: `5 x 200.768 us = 1.004 ms`

Estimated local crypto/setup total:

- about `2.326 ms`

This does **not** include:
- actual TCP dial time
- OS scheduling
- relay queueing
- network RTT

### 3. Hidden mode slowdown is mostly network path length

Since 1 KB wrap/unwrap costs are in low microseconds, the main reason hidden mode feels slower is:
- extra relays
- extra connection establishment
- rendezvous coordination

Not the symmetric payload encryption itself.

### 4. Memory cost per operation is moderate

The highest measured allocation cost in this set is:
- secure handshake: about `9.5 KB/op`
- 3-hop unwrap: about `7.3 KB/op`

That is acceptable for current development stage.

The most allocation-heavy paths are setup paths, not steady-state data movement.

## Practical Reading

If you are thinking about user experience:

- direct encrypted mode should stay fast
- relay mode should be fine unless RTT is bad
- hidden mode cost is dominated by hop count and path length

If you are thinking about optimization:

Best next wins are not “replace AES”.

Best next wins are:
- connection reuse
- faster active-circuit failover
- less repeated setup work
- relay admission and queue tuning

## Files That Implement What Was Measured

Identity:
- `internal/identity/store.go`
- `internal/identity/identity_bench_test.go`

Secure transport:
- `internal/secure/session.go`
- `internal/secure/session_bench_test.go`

Onion cell layer:
- `internal/onion/cell.go`
- `internal/onion/bench_test.go`

Client circuit build:
- `internal/onion/onion.go`

Relay processing:
- `internal/onion/circuit.go`

Live behavior checks:
- `internal/integration/swarm_test.go`

## Remaining Work After This Report

The main protocol/encryption pieces are in place.

What remains is mostly runtime hardening:

1. active hidden-circuit failover
2. hidden-mode abuse controls
3. later transport/session optimization

So from here, the work shifts from:
- building the encryption architecture

to:
- resilience
- tuning
- optimization
