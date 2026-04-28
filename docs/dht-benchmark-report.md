# VX6 DHT Benchmark Report

Environment:

- date: 2026-04-29
- os: linux
- arch: amd64
- cpu: AMD Ryzen 7 250 w/ Radeon 780M Graphics

Command:

```bash
GOCACHE=/tmp/vx6-go-build GOMODCACHE=/tmp/vx6-go-mod \
go test -run '^$' \
  -bench 'Benchmark(ValidateLookupValueSignedEnvelopeServiceRecord|StoreValidatedSignedEnvelopeServiceRecord|RecursiveFindValueDetailedConfirmed3Sources)$' \
  -benchmem ./internal/dht
```

Results:

- `BenchmarkValidateLookupValueSignedEnvelopeServiceRecord`
  - `186142 ns/op`
  - `4744 B/op`
  - `44 allocs/op`
- `BenchmarkStoreValidatedSignedEnvelopeServiceRecord`
  - `177302 ns/op`
  - `9487 B/op`
  - `87 allocs/op`
- `BenchmarkRecursiveFindValueDetailedConfirmed3Sources`
  - `449536 ns/op`
  - `40852 B/op`
  - `380 allocs/op`

Interpretation:

- signed envelope verification is well under `1 ms`
- validated local store is also well under `1 ms`
- a confirmed three-source verified lookup remains under `0.5 ms` locally

These numbers are local-process and local-loopback costs.

They do not include:

- WAN latency
- real Internet packet loss
- remote relay queueing
- bootstrap or peer churn

What they do show:

- the hardened DHT logic is still lightweight compared with normal network RTT
- the safety checks are adding measurable but acceptable local overhead
