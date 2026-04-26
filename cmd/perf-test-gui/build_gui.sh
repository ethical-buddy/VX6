#!/bin/bash
# Build script for C GUI using MinGW64 on Windows

# This script should be run from within MSYS2/MinGW64 environment
# or from a system with MinGW64 installed

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
OUTPUT="$SCRIPT_DIR/perf-test-gui-windows.exe"

echo "Building GUI with MinGW64..."

# Try common MinGW64 installations
MINGW_PATH=""
if [ -d "C:/msys64/mingw64" ]; then
    MINGW_PATH="C:/msys64/mingw64"
elif [ -d "C:/MinGW64" ]; then
    MINGW_PATH="C:/MinGW64"
elif [ -d "/mingw64" ]; then
    MINGW_PATH="/mingw64"
fi

if [ -z "$MINGW_PATH" ]; then
    echo "Error: MinGW64 not found. Please install MinGW64 or set MINGW_PATH."
    exit 1
fi

# Try to find gcc
GCC="${MINGW_PATH}/bin/gcc.exe"
if [ ! -f "$GCC" ]; then
    # Fallback to system gcc
    GCC="gcc"
fi

echo "Using GCC: $GCC"

# Compile with optimization
"$GCC" \
    -Wall -Wextra \
    -O2 \
    -pedantic \
    -o "$OUTPUT" \
    "$SCRIPT_DIR/gui_windows.c" \
    -luser32 -lkernel32 -lcomctl32 -lshell32

if [ -f "$OUTPUT" ]; then
    echo "Build successful: $OUTPUT"
    ls -lh "$OUTPUT"
else
    echo "Build failed!"
    exit 1
fi
