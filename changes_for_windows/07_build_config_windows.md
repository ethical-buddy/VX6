# VX6 Windows Build Configuration
# This file documents the MinGW64 toolchain configuration used for building VX6 on Windows

[Build Environment]
OS = Windows 11 / Server 2022+
MSYS2_Installation = C:\msys64
MinGW64_Path = C:\msys64\mingw64
Go_Version = 1.22.0+
Architecture = AMD64 / ARM64

[Go Build Settings]
GOOS = windows
GOARCH = amd64 (or arm64 for ARM64 builds)
CGO_ENABLED = 0 (default for Windows builds)
GO_BUILD_FLAGS = -ldflags "-X main.Version=VERSION"

[MinGW64 Configuration]
GCC_Path = C:\msys64\mingw64\bin\gcc.exe
GCC_Flags = -Wall -Wextra -O2 -pedantic -fno-strict-aliasing
Win32_Libraries = -luser32 -lkernel32 -lcomctl32 -lshell32
Optimization = -O2 (production builds)

[Architecture Support]
x86_64 (AMD64)
  - Primary development platform
  - Full VX6 core support
  - Performance test tools included
  - Recommended for servers

ARM64 (aarch64)
  - Full support via cross-compilation
  - Identical feature set to AMD64
  - Performance test tools included
  - Recommended for ARM servers

[Build Targets]

CLI Tools (Go)
  - cmd/vx6/main.go → vx6-{amd64,arm64}.exe
  - cmd/perf-test-gui/main.go → perf-test-cli-{amd64,arm64}.exe

GUI Application (C)
  - cmd/perf-test-gui/gui_windows.c → perf-test-gui-windows-{amd64,arm64}.exe
  - Requires: MinGW64, Windows SDK headers

[Compiler Invocation]

CLI Build (PowerShell):
  $env:GOOS="windows"
  $env:GOARCH="amd64"
  go build -o vx6-amd64.exe ./cmd/vx6

GUI Build (with gcc):
  C:\msys64\mingw64\bin\gcc.exe `
    -Wall -Wextra -O2 -pedantic `
    -o perf-test-gui-windows.exe `
    cmd/perf-test-gui/gui_windows.c `
    -luser32 -lkernel32 -lcomctl32 -lshell32

[Environment Variables]

Required:
  PATH = C:\msys64\mingw64\bin;C:\Program Files\Go\bin;%PATH%

Optional (for CI/CD):
  VX6_RELEASE_BUILD = 1
  VX6_OPTIMIZE = -O3
  VX6_STRIP = 1

[Testing Configuration]

Unit Tests:
  go test ./... -tags="windows"

Performance Baseline:
  .\perf-test-cli.exe -v -format json -output baseline.json

Integration Tests:
  (Requires running VX6 node)
  vx6 node
  vx6 debug windows-status

[Output Locations]

Executables:
  - vx6-amd64.exe
  - vx6-arm64.exe
  - perf-test-cli.exe / perf-test-cli-amd64.exe / perf-test-cli-arm64.exe
  - perf-test-gui-windows.exe (when compiled with MinGW64)

Performance Results:
  - JSON: results.json (structured data for tools)
  - Text: results.txt (human-readable)
  - CSV: metrics.csv (spreadsheet import)

[Known Build Issues]

Issue: "gcc: command not found"
  Solution: Add C:\msys64\mingw64\bin to PATH
  Or: Use full path to gcc.exe

Issue: Linker errors with Windows libraries
  Solution: Ensure library flags are at end of command
  Example: gcc ... source.c -luser32 -lkernel32 ...

Issue: Test binary incompatibility
  Note: Cgo tests may require proper cross-compilation setup
  Solution: Use "go test ./..." (native build)

[Continuous Integration Setup]

GitHub Actions:
  - runs-on: windows-latest
  - uses: actions/setup-go@v4
  - go build -o vx6-amd64.exe ./cmd/vx6

Local Development:
  - Install Go 1.22+
  - Install MinGW64 via MSYS2
  - Run build scripts: .\build.ps1

[Performance and Size Characteristics]

Binary Sizes (Release):
  vx6-amd64.exe ≈ 5-6 MB
  vx6-arm64.exe ≈ 5-6 MB
  perf-test-cli.exe ≈ 3-4 MB
  perf-test-gui-windows.exe ≈ 0.5 MB (C binary, no runtime)

Runtime Memory:
  VX6 Core ≈ 10-20 MB baseline
  Performance Test Tool ≈ 5-10 MB per test run

Build Time:
  vx6 (Go) ≈ 5-10 seconds
  GUI (C with MinGW64) ≈ 2-5 seconds

[References]

- MSYS2: https://www.msys2.org/
- Go on Windows: https://golang.org/doc/install
- MinGW64: https://www.mingw-w64.org/
- Windows SDK: Included with Visual Studio Build Tools

[Support and Troubleshooting]

See: cmd/perf-test-gui/README.md for detailed usage and troubleshooting
See: changes_for_windows/05_windows_support_complete_implementation.md for technical details
