# VX6 Performance Test GUI
## by Ilker Ozturk @dailker

A comprehensive performance testing framework for VX6 on Windows 11/Server 2022+ with both CLI and GUI interfaces.

## Overview

The performance test suite measures:
- **Transport Performance**: TCP latency, throughput, and connection success rates
- **System Metrics**: Memory usage, goroutine counts, allocation statistics
- **VX6-Specific Metrics**: Node startup time, service discovery time, relay path setup time
- **Platform Capabilities**: Available acceleration features (eBPF, XDP, MsQuic)

## Components

### 1. CLI Tool (Go)
- **File**: `main.go`
- **Executable**: `perf-test-cli` or `perf-test-cli-arm64.exe` (Windows)
- **Supports**: Multiple output formats (JSON, text, CSV)
- **Portable**: Works on Windows (AMD64/ARM64), Linux, and macOS

### 2. GUI Application (C with Win32 API)
- **File**: `gui_windows.c`
- **Executable**: `perf-test-gui-windows.exe` / `perf-test-gui-windows-arm64.exe`
- **Platform**: Windows 11/Server 2022+ only
- **Dependencies**: MinGW64, Windows SDK (included with MSYS2)

## Building

### Prerequisites

#### Windows (AMD64 and ARM64)
```powershell
# Install MinGW64 via MSYS2
# Download from: https://www.msys2.org/

# Verify MinGW64 is installed
C:\msys64\mingw64\bin\gcc.exe --version
```

#### Linux/macOS
```bash
# No additional requirements for CLI tool
go version  # Must be 1.22.0 or later
```

### Build Instructions

#### CLI Tool (All Platforms)

**On Windows (PowerShell):**
```powershell
cd cmd/perf-test-gui
.\build.ps1
```

**On Linux/macOS:**
```bash
cd cmd/perf-test-gui
chmod +x build.sh
./build.sh
```

**Manual build:**
```bash
cd cmd/perf-test-gui
go build -o perf-test-cli ./   # Linux/macOS
# or
set GOOS=windows&& set GOARCH=amd64&& go build -o perf-test-cli-amd64.exe ./
```

#### GUI Application (Windows Only)

**Using PowerShell:**
```powershell
cd cmd/perf-test-gui
.\build_gui.ps1
```

**Using MSYS2 Bash:**
```bash
cd cmd/perf-test-gui
bash build_gui.sh
```

**Manual build with MinGW64:**
```bash
C:\msys64\mingw64\bin\gcc.exe -Wall -Wextra -O2 `
  -o perf-test-gui-windows.exe gui_windows.c `
  -luser32 -lkernel32 -lcomctl32 -lshell32
```

## Usage

### CLI Tool

**Basic usage (JSON output):**
```bash
./perf-test-cli
```

**Text format:**
```bash
./perf-test-cli -format text
```

**CSV format with custom output:**
```bash
./perf-test-cli -format csv -output results.csv
```

**Verbose mode with custom target:**
```bash
./perf-test-cli -v -format json -target [::1]:9000 -duration 60s
```

**All available options:**
```
-v                      Verbose output
-format string          Output format: json, text, csv (default: "json")
-output string          Output file (default: stdout)
-target string          Target address for network tests (default: "[::1]:8080")
-duration duration      Test duration (default: 30s)
-tor                    Enable Tor relay benchmarks
-tor-proxy string       Tor SOCKS5 proxy address (default: "127.0.0.1:9050")
-tor-target string      Tor relay target host or URL for your own server/onion target
-temp-tor-automatic-test
                        Run a temporary private Tor-like test server automatically
```

**Private Tor-style local test:**
```powershell
.
perf-test-cli.exe -temp-tor-automatic-test -format json -v
```

This starts a temporary private in-process HTTP server, generates a short-lived token, and runs stress, upload, and download checks against it without using the public Tor network.

**User-owned server / onion target:**
```powershell
.
perf-test-cli.exe -tor -tor-target http://your-server.example.com -tor-proxy 127.0.0.1:9050
```

Use `-tor-target` when you want to point the benchmark at your own relay, server, or onion service.

### GUI Application

**Launch:**
```powershell
.\perf-test-gui-windows.exe

# or via Windows Explorer
# Double-click the .exe file
```

**Features:**
- Real-time test progress status
- Visual performance metrics display
- One-click performance testing
- Results displayed in formatted text
- Automatically collects system information

## Automation Hooks

The performance test suite supports automation through hooks and environment variables.

### Environmental Configuration

**Pre-test execution:**
```powershell
# Set test target
$env:VX6_PERF_TARGET = "[::1]:9000"

# Set test duration
$env:VX6_PERF_DURATION = "60s"

# Enable detailed metrics collection
$env:VX6_PERF_VERBOSE = "1"
```

### Batch Testing

**Run multiple tests with different configurations:**
```powershell
# save as test_suite.ps1
$configs = @(
    @{ target = "[::1]:8080"; duration = "30s"; name = "baseline" },
    @{ target = "[::1]:9000"; duration = "30s"; name = "relay" },
    @{ target = "[::1]:9001"; duration = "30s"; name = "hidden" }
)

foreach ($config in $configs) {
    Write-Host "Running test: $($config.name)"
    $env:VX6_PERF_TARGET = $config.target
    $env:VX6_PERF_DURATION = $config.duration
    
    $output = "results_$($config.name).json"
    & .\perf-test-cli -format json -output $output
}
```

### Dashboard Integration

**Export metrics for monitoring systems:**
```powershell
# Run CLI and capture JSON
$json = & .\perf-test-cli -format json | ConvertFrom-Json

# Send to monitoring system
$json | ConvertTo-Json | Invoke-WebRequest -Uri "https://metrics.example.com/api/metrics" `
    -Method POST -ContentType "application/json"
```

### CI/CD Pipeline Integration

**GitHub Actions example:**
```yaml
name: VX6 Performance Tests
on: [push, pull_request]

jobs:
  perf-test:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22.0'
      
      - name: Build CLI
        run: |
          cd cmd/perf-test-gui
          go build -o perf-test-cli.exe ./
      
      - name: Run Performance Tests
        run: |
          cd cmd/perf-test-gui
          .\perf-test-cli.exe -format json -output results.json
      
      - name: Upload Results
        uses: actions/upload-artifact@v3
        with:
          name: perf-results
          path: cmd/perf-test-gui/results.json
```

### JSON Output Schema

The CLI outputs JSON with the following structure:

```json
{
  "timestamp": "2024-04-29T15:30:45Z",
  "platform": {
    "os": "windows",
    "arch": "amd64",
    "go_version": "go1.22.0",
    "cpu_count": 8
  },
  "system": {
    "mem_heap_alloc_bytes": 10485760,
    "mem_heap_sys_bytes": 20971520,
    "goroutines_count": 25,
    "allocs_count": 1000
  },
  "transport": {
    "tcp_latency_ns": 15000000,
    "tcp_throughput_mbps": 950.5,
    "connection_attempts": 10,
    "successful_conns": 9,
    "failed_conns": 1
  },
  "vx6": {
    "node_startup_ns": 500000000,
    "service_add_ns": 100000000,
    "discovery_time_ns": 2000000000,
    "relay_setup_ns": 1000000000
  },
  "summary": {
    "total_duration_ns": 4500000000,
    "status": "success",
    "errors": []
  }
}
```

## Performance Benchmarks

### Windows 11 AMD64 Baseline (Reference)
- TCP Latency: ~15-20ms (IPv6 loopback)
- TCP Throughput: ~900-950 MB/s
- Average Success Rate: >99%
- Memory Usage: <50MB heap

### Windows 11 ARM64 Baseline (Reference)
- TCP Latency: ~20-25ms (IPv6 loopback)
- TCP Throughput: ~800-900 MB/s
- Average Success Rate: >98%
- Memory Usage: <50MB heap

## Troubleshooting

### Build Issues

**"gcc: command not found" or "gcc.exe not found"**
- Ensure MinGW64 is installed (via MSYS2)
- Add MinGW64 to PATH if using system GCC
- Use full path: `C:\msys64\mingw64\bin\gcc.exe`

**Linker errors with Windows libraries**
- Verify Windows SDK is installed with MSYS2
- Use flag: `-luser32 -lkernel32 -lcomctl32 -lshell32`

**"undefined reference to" errors**
- Check library order: place Windows libs at end of gcc command
- Use: `gcc ... source.c -luser32 -lkernel32 ...`

### Runtime Issues

**"Target address connection refused"**
- Verify VX6 node is running on the target address
- Check firewall rules allow IPv6 connections
- Use `vx6 status` to verify node is active

**GUI window not appearing**
- Run from Command Prompt or PowerShell
- Check event log for application crashes
- Try running as Administrator

**Unrealistic performance numbers**
- Ensure no other heavy processes are running
- Use system resource monitor to verify CPU/memory availability
- Run tests multiple times for consistency

## Architecture Support

### Tier A: Windows 11 / Server 2022+
**Full Support (AMD64 & ARM64)**
- TCP/IPv6 transport ✓
- QUIC/MsQuic support (when installed) ✓
- eBPF/XDP acceleration (when drivers installed) ✓
- Full GUI support ✓

### Future Expansion
- Windows 10 / Server 2016/2019 support planned
- Additional metrics collection (CPU, network, disk I/O)
- Real-time charting and graphing
- Network packet inspection tools

## Contributing

To contribute improvements:
1. Extend `PerformanceMetrics` struct in `main.go` for new metrics
2. Add corresponding test functions in `benchmarkTransport()`, `gatherSystemMetrics()`
3. Update output formatters: `formatText()`, `formatCSV()`, `formatJSON()`
4. Test on both AMD64 and ARM64 architectures
5. Add documentation for new metrics

## Notes

- All output formats are cross-platform compatible
- JSON output is recommended for programmatic use
- Text format is human-readable for manual inspection
- CSV format is optimal for spreadsheet and graphing tools
- The GUI requires Windows; CLI tools work on all platforms

## License

Same as VX6 project (see LICENSE file in project root)
