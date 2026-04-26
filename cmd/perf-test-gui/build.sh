#!/bin/bash
# Build script for perf-test-gui on Windows/Linux/macOS

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/../.." && pwd )"

echo "Building perf-test-gui..."

# Build for current platform
cd "$SCRIPT_DIR"
go build -o perf-test-gui ./

echo "Build complete. Binary: $SCRIPT_DIR/perf-test-gui"
