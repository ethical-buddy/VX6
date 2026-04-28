# 12. Temporary Private Tor Automatic Test

## What was done

Added a new performance testing mode to `cmd/perf-test-gui/main.go`:

- `-temp-tor-automatic-test`
- Starts a temporary private in-process HTTP server
- Generates a short-lived private token for the test session
- Runs the Tor-style benchmark flow automatically:
  - stress test
  - upload test
  - download test
- Keeps the test isolated from the public Tor network
- Preserves the existing user-supplied `-tor-target` path for testing on a user-owned server, relay, or onion service

## Why it was added

This gives a safe local/private test path for automated validation while still allowing users to point the benchmark at their own server when they want real network testing.

## Validation

Verified successfully with:

- `go build ./cmd/perf-test-gui/`
- `go run ./cmd/perf-test-gui/ -temp-tor-automatic-test -format json`

## Notes

- The private mode is intentionally local-only.
- Public Tor relay stress-testing was not enabled.
- Existing Tor target options remain available for user-controlled testing.
