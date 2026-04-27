# Windows Baseline Test Log (Before Changes)

## Scope
- Repository: `vx6-fork`
- OS: Windows
- Command: `go test ./...`
- Purpose: capture all pre-change failures.

## Full Result (Before)
```text
PS C:\Users\ilker\Desktop\VX6\vx6-fork> go test ./...
# github.com/vx6/vx6/internal/cli
internal\cli\app.go:531:20: undefined: syscall.Kill
internal\cli\app.go:534:20: undefined: syscall.Kill
internal\cli\app.go:1632:20: undefined: syscall.Flock
internal\cli\app.go:1632:54: undefined: syscall.LOCK_EX
internal\cli\app.go:1632:70: undefined: syscall.LOCK_NB
internal\cli\app.go:1640:15: undefined: syscall.Flock
internal\cli\app.go:1640:49: undefined: syscall.LOCK_UN
internal\cli\app.go:1645:15: undefined: syscall.Flock
internal\cli\app.go:1645:49: undefined: syscall.LOCK_UN
internal\cli\app.go:1650:15: undefined: syscall.Flock
internal\cli\app.go:1650:15: too many errors
FAIL    github.com/vx6/vx6/cmd/cmd/vx6 [build failed]
FAIL    github.com/vx6/vx6/cmd/vx6 [build failed]
FAIL    github.com/vx6/vx6/internal/cli [build failed]
# github.com/vx6/vx6/internal/internal/cli [github.com/vx6/vx6/internal/internal/cli.test]
internal\internal\cli\app.go:531:20: undefined: syscall.Kill
internal\internal\cli\app.go:534:20: undefined: syscall.Kill
internal\internal\cli\app.go:1632:20: undefined: syscall.Flock
internal\internal\cli\app.go:1632:54: undefined: syscall.LOCK_EX
internal\internal\cli\app.go:1632:70: undefined: syscall.LOCK_NB
internal\internal\cli\app.go:1640:15: undefined: syscall.Flock
internal\internal\cli\app.go:1640:49: undefined: syscall.LOCK_UN
internal\internal\cli\app.go:1645:15: undefined: syscall.Flock
internal\internal\cli\app.go:1645:49: undefined: syscall.LOCK_UN
internal\internal\cli\app.go:1650:15: undefined: syscall.Flock
internal\internal\cli\app.go:1650:15: too many errors
--- FAIL: TestDefaultPathsUseHomeDirectory (0.00s)
    config_test.go:84: unexpected config path "C:\\Users\\ilker\\.config\\vx6\\config.json"
--- FAIL: TestRuntimeLockPathUsesConfigDirectory (0.00s)
    config_test.go:60: unexpected lock path "\\tmp\\vx6\\node.lock"
--- FAIL: TestRuntimeControlPathUsesConfigDirectory (0.00s)
    config_test.go:72: unexpected control path "\\tmp\\vx6\\node.control.json"
--- FAIL: TestRuntimePIDPathUsesConfigDirectory (0.00s)
    config_test.go:48: unexpected pid path "\\tmp\\vx6\\node.pid"
FAIL
FAIL    github.com/vx6/vx6/internal/config      0.019s
ok      github.com/vx6/vx6/internal/dht (cached)
ok      github.com/vx6/vx6/internal/discovery   (cached)
ok      github.com/vx6/vx6/internal/hidden      (cached)
--- FAIL: TestDefaultPathUsesHomeConfigDirectory (0.00s)
    store_test.go:84: unexpected default identity path "C:\\Users\\ilker\\.config\\vx6\\identity.json"
FAIL
FAIL    github.com/vx6/vx6/internal/identity    0.020s
ok      github.com/vx6/vx6/internal/integration (cached)
FAIL    github.com/vx6/vx6/internal/internal/cli [build failed]
--- FAIL: TestDefaultPathsUseHomeDirectory (0.00s)
    config_test.go:84: unexpected config path "C:\\Users\\ilker\\.config\\vx6\\config.json"
--- FAIL: TestRuntimeLockPathUsesConfigDirectory (0.00s)
    config_test.go:60: unexpected lock path "\\tmp\\vx6\\node.lock"
--- FAIL: TestRuntimeControlPathUsesConfigDirectory (0.00s)
    config_test.go:72: unexpected control path "\\tmp\\vx6\\node.control.json"
--- FAIL: TestRuntimePIDPathUsesConfigDirectory (0.00s)
    config_test.go:48: unexpected pid path "\\tmp\\vx6\\node.pid"
FAIL
FAIL    github.com/vx6/vx6/internal/internal/config     0.026s
ok      github.com/vx6/vx6/internal/internal/dht        (cached)
ok      github.com/vx6/vx6/internal/internal/discovery  (cached)
ok      github.com/vx6/vx6/internal/internal/hidden     (cached)
--- FAIL: TestDefaultPathUsesHomeConfigDirectory (0.00s)
    store_test.go:84: unexpected default identity path "C:\\Users\\ilker\\.config\\vx6\\identity.json"
FAIL
FAIL    github.com/vx6/vx6/internal/internal/identity   0.027s
... (remaining packages ok or no test files)
FAIL
```

## Root Causes Found
1. Unix-only syscalls in CLI (`syscall.Kill`, `syscall.Flock`, `LOCK_*`) break Windows compilation.
2. Tests hard-code Unix paths and rely only on `HOME`, causing path/home mismatches on Windows.
3. Failures exist in both primary and mirrored package trees (`internal/*` and `internal/internal/*`).
