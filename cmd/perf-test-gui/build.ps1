# Build script for perf-test-gui on Windows (PowerShell)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir

Write-Host "Building perf-test-gui for Windows..."

Push-Location $scriptDir

# Support both AMD64 and ARM64 architectures
$architectures = @("amd64", "arm64")

foreach ($arch in $architectures) {
    Write-Host "Building for $arch architecture..."
    $env:GOOS = "windows"
    $env:GOARCH = $arch
    $exeName = "perf-test-gui-$arch.exe"
    
    & go build -o $exeName ./
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Build failed for $arch" -ForegroundColor Red
        Pop-Location
        exit 1
    }
    Write-Host "Built: $exeName" -ForegroundColor Green
}

Pop-Location

Write-Host "Build complete. Binaries are in $scriptDir" -ForegroundColor Green
