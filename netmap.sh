#!/bin/bash
# netmap.sh – Detects environment and runs the appropriate tools
set -euo pipefail

REPORT="networkmapreport.txt"
echo "=== Network Map Toolkit ===" > "$REPORT"
date >> "$REPORT"

# Detect OS
OS="$(uname -s)"
case "$OS" in
    Linux)
        if [[ -f /system/bin/sh ]]; then
            echo "Android detected." >> "$REPORT"
        else
            echo "Linux detected." >> "$REPORT"
        fi
        # Compile C collector if not present
        if [[ ! -x ./netmap_collect ]]; then
            echo "Compiling C collector..."
            gcc -Wall -O2 -o netmap_collect netmap_collect.c
        fi
        ./netmap_collect >> "$REPORT" 2>&1 || echo "C collector failed; using fallbacks." >> "$REPORT"
        ;;
    Darwin)
        echo "macOS detected." >> "$REPORT"
        ;;
    MINGW*|CYGWIN*|MSYS*)
        echo "Windows detected." >> "$REPORT"
        ;;
    *)
        echo "Unknown OS: $OS" >> "$REPORT"
        ;;
esac

# Run Go orchestrator if available
if [[ -x ./netmap_report ]]; then
    ./netmap_report >> "$REPORT" 2>&1
else
    echo "Go orchestrator not found; building..." >> "$REPORT"
    go build -o netmap_report netmap_report.go && ./netmap_report >> "$REPORT" 2>&1
fi

echo "Done. See $REPORT"
