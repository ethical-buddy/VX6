# Build script for C GUI using MinGW64 on Windows (PowerShell)
# Usage: .\build_gui.ps1

param(
    [string]$MinGW64Path = "C:\msys64\mingw64"
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$outputFile = Join-Path $scriptDir "perf-test-gui-windows-gui.exe"

Write-Host "Building Windows GUI with MinGW64..."

# Check if MinGW64 exists
if (!(Test-Path $MinGW64Path)) {
    Write-Host "Error: MinGW64 not found at $MinGW64Path" -ForegroundColor Red
    Write-Host "Please install MinGW64 or provide the correct path with -MinGW64Path" -ForegroundColor Red
    exit 1
}

$gccPath = Join-Path $MinGW64Path "bin\gcc.exe"
if (!(Test-Path $gccPath)) {
    Write-Host "Error: GCC not found at $gccPath" -ForegroundColor Red
    exit 1
}

Write-Host "Using GCC: $gccPath" -ForegroundColor Green

# Build for both AMD64 and ARM64
$architectures = @("amd64", "arm64")

foreach ($arch in $architectures) {
    Write-Host "`nBuilding GUI for $arch architecture..." -ForegroundColor Cyan
    
    $exeName = "perf-test-gui-windows-$arch.exe"
    $outputPath = Join-Path $scriptDir $exeName
    
    # Set architecture-specific flags
    $archFlags = @()
    if ($arch -eq "arm64") {
        $archFlags = @("-march=armv8", "-mtune=cortex-a72")
    }
    
    # Compile
    $gccArgs = @(
        "-Wall", "-Wextra",
        "-O2",
        "-pedantic",
        "-o", $outputPath,
        (Join-Path $scriptDir "gui_windows.c"),
        "-luser32", "-lkernel32", "-lcomctl32", "-lshell32"
    )
    
    if ($archFlags.Count -gt 0) {
        $gccArgs += $archFlags
    }
    
    & $gccPath $gccArgs 2>&1 | ForEach-Object { Write-Host $_ }
    
    if ($LASTEXITCODE -eq 0 -and (Test-Path $outputPath)) {
        $size = (Get-Item $outputPath).Length
        Write-Host "Built successfully: $exeName ($size bytes)" -ForegroundColor Green
    } else {
        Write-Host "Build failed for $arch" -ForegroundColor Red
    }
}

Write-Host "`nBuild complete!" -ForegroundColor Green
