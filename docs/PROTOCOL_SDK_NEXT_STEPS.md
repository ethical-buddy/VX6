# VX6 Protocol + SDK Next Steps

## Protocol

1. Multi-device identity/session model
2. SLA policy routing engine
3. NAT/ICE robustness matrix and TURN operations automation
4. Stronger anti-abuse controls for hidden/service signaling

## SDK

1. Stable public interfaces and semantic version guarantees
2. Explicit error taxonomy (`retryable`, `policy-denied`, `unreachable`)
3. Conformance vectors and language-agnostic compatibility tests
4. Reference app examples (desktop, Android, service-daemon)

## Testing and Quality

1. Fuzz surfaces:
   - DHT payloads
   - signaling envelopes
   - session state transitions
2. Property tests:
   - idempotent replay handling
   - monotonic counters
   - version compatibility checks
3. Cross-platform CI gates:
   - Linux/Windows/macOS build checks

