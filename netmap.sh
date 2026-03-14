#!/bin/bash
set -euo pipefail

rainbow_animation() {
    tput civis
    tput smcup

    WORD="netmap"
    AMPLITUDE=3
    SPEED=0.1
    CYCLES=10

    RAINBOW=(
        $'\e[91m'
        $'\e[93m'
        $'\e[92m'
        $'\e[96m'
        $'\e[94m'
        $'\e[95m'
    )
    RESET=$'\e[0m'

    rows=$(tput lines)
    cols=$(tput cols)
    base_row=$((rows / 2))
    start_col=$(( (cols - (${#WORD} * 3)) / 2 ))
    (( start_col < 1 )) && start_col=1

    trap 'tput cnorm; tput rmcup; echo' EXIT

    for ((frame=0; frame<CYCLES; frame++)); do
        printf '\e[2J'
        for ((i=0; i<${#WORD}; i++)); do
            ch="${WORD:i:1}"
            if command -v bc &>/dev/null; then
                offset=$(echo "scale=0; $AMPLITUDE * s($i * 0.8 + $frame * 0.5)" | bc -l 2>/dev/null)
                offset=$(printf "%.0f" "$offset" 2>/dev/null || echo 0)
            else
                offset=$(( ( (frame + i * 2) % (AMPLITUDE*2 + 1) ) - AMPLITUDE ))
            fi
            [[ ! "$offset" =~ ^-?[0-9]+$ ]] && offset=0

            row=$((base_row + offset))
            col=$((start_col + i * 3))
            color_idx=$(( (i + frame) % ${#RAINBOW[@]} ))
            color="${RAINBOW[color_idx]}"

            printf "\033[%d;%dH%s%s%s" "$row" "$col" "$color" "$ch" "$RESET"
        done
        sleep "$SPEED"
    done

    tput cnorm
    tput rmcup
    trap - EXIT
}

if [[ -t 1 ]]; then
    rainbow_animation
fi

REPORT="networkmapreport.txt"
echo "=== Network Map Toolkit ===" > "$REPORT"
date >> "$REPORT"

OS="$(uname -s)"
case "$OS" in
    Linux)
        if [[ -f /system/bin/sh ]]; then
            echo "Android detected." >> "$REPORT"
        else
            echo "Linux detected." >> "$REPORT"
        fi
        if [[ ! -x ./netmap_collect ]]; then
            echo "Compiling C collector..." | tee -a "$REPORT"
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

if [[ -x ./netmap_report ]]; then
    ./netmap_report >> "$REPORT" 2>&1
else
    echo "Go orchestrator not found; building..." | tee -a "$REPORT"
    go build -o netmap_report netmap_report.go && ./netmap_report >> "$REPORT" 2>&1
fi

echo "Done. See $REPORT"
