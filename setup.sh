#!/bin/bash
# setup.sh – Automated dependency installer for Network Map Toolkit
# Detects OS, checks for required tools, and installs missing ones.
# Usage: ./setup.sh [--force] [--help]

set -euo pipefail

# Configuration
LOG_FILE="setup.log"
REQUIRED_TOOLS=("gcc" "go" "traceroute" "osquery" "bc")
# Optional: osquery might not be in default repos; we'll attempt but not fail if missing.

# Color output for terminal
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Help message
show_help() {
    cat <<EOF
Usage: $0 [OPTIONS]

Options:
  --force    Reinstall even if tools are already present (upgrade).
  --help     Show this help message.

This script installs dependencies for the Network Map Toolkit:
  - gcc (C compiler)
  - go (Go programming language)
  - traceroute (network path tracing)
  - osquery (optional, for process enrichment)
  - bc (command-line calculator, for animations)

It supports Linux (apt, yum, dnf, pacman), macOS (Homebrew), and Windows (WSL/Cygwin/MSYS2 with appropriate package managers).
EOF
}

# Parse arguments
FORCE=0
while [[ $# -gt 0 ]]; do
    case $1 in
        --force)
            FORCE=1
            shift
            ;;
        --help)
            show_help
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Initialize log
echo "=== Network Map Toolkit Setup ===" | tee "$LOG_FILE"
echo "Started at: $(date)" | tee -a "$LOG_FILE"
echo "Force mode: $FORCE" | tee -a "$LOG_FILE"
echo "" | tee -a "$LOG_FILE"

# Detect OS and package manager
OS="$(uname -s)"
PACKAGE_MANAGER=""
INSTALL_CMD=""
UPDATE_CMD=""

case "$OS" in
    Linux)
        if [[ -f /etc/os-release ]]; then
            . /etc/os-release
            case "$ID" in
                debian|ubuntu)
                    PACKAGE_MANAGER="apt"
                    UPDATE_CMD="sudo apt update"
                    INSTALL_CMD="sudo apt install -y"
                    ;;
                fedora)
                    PACKAGE_MANAGER="dnf"
                    UPDATE_CMD="sudo dnf check-update || true"  # dnf check-update returns non-zero if updates available
                    INSTALL_CMD="sudo dnf install -y"
                    ;;
                centos|rhel)
                    PACKAGE_MANAGER="yum"
                    UPDATE_CMD="sudo yum check-update || true"
                    INSTALL_CMD="sudo yum install -y"
                    ;;
                arch)
                    PACKAGE_MANAGER="pacman"
                    UPDATE_CMD="sudo pacman -Sy"
                    INSTALL_CMD="sudo pacman -S --noconfirm"
                    ;;
                opensuse*)
                    PACKAGE_MANAGER="zypper"
                    UPDATE_CMD="sudo zypper refresh"
                    INSTALL_CMD="sudo zypper install -y"
                    ;;
                *)
                    echo -e "${YELLOW}Unsupported Linux distribution: $ID${NC}" | tee -a "$LOG_FILE"
                    ;;
            esac
        fi
        ;;
    Darwin)
        PACKAGE_MANAGER="brew"
        if ! command -v brew &>/dev/null; then
            echo -e "${YELLOW}Homebrew not found. Please install from https://brew.sh${NC}" | tee -a "$LOG_FILE"
        else
            UPDATE_CMD="brew update"
            INSTALL_CMD="brew install"
        fi
        ;;
    MINGW*|CYGWIN*|MSYS*)
        # Windows environment – assume Chocolatey or pacman (MSYS2)
        if command -v choco &>/dev/null; then
            PACKAGE_MANAGER="choco"
            UPDATE_CMD="choco upgrade chocolatey"  # not really needed
            INSTALL_CMD="choco install -y"
        elif command -v pacman &>/dev/null; then
            # MSYS2
            PACKAGE_MANAGER="pacman"
            UPDATE_CMD="pacman -Sy"
            INSTALL_CMD="pacman -S --noconfirm"
        else
            echo -e "${YELLOW}No supported package manager found (choco or pacman).${NC}" | tee -a "$LOG_FILE"
        fi
        ;;
    *)
        echo -e "${RED}Unsupported OS: $OS${NC}" | tee -a "$LOG_FILE"
        exit 1
        ;;
esac

# Function to check if a tool is installed
is_installed() {
    if command -v "$1" &>/dev/null; then
        return 0
    else
        return 1
    fi
}

# Install a tool using the detected package manager
install_tool() {
    local tool=$1
    local pkg_name=$2   # optional override for package name
    local pkg=${pkg_name:-$tool}

    echo -n "Installing $tool... " | tee -a "$LOG_FILE"
    if [[ -z "$INSTALL_CMD" ]]; then
        echo -e "${RED}FAILED (no package manager)${NC}" | tee -a "$LOG_FILE"
        return 1
    fi

    # Special handling for osquery on Linux (may need repo)
    if [[ "$tool" == "osquery" && "$PACKAGE_MANAGER" == "apt" ]]; then
        # Add osquery repo (official)
        if ! is_installed osquery; then
            echo "Adding osquery repository..." | tee -a "$LOG_FILE"
            sudo apt update
            sudo apt install -y wget
            wget -qO - https://osquery-packages.s3.amazonaws.com/key.gpg | sudo apt-key add -
            echo "deb [arch=amd64] https://osquery-packages.s3.amazonaws.com/ xenial main" | sudo tee /etc/apt/sources.list.d/osquery.list
            sudo apt update
            $INSTALL_CMD osquery
        fi
    elif [[ "$tool" == "osquery" && "$PACKAGE_MANAGER" == "brew" ]]; then
        $INSTALL_CMD osquery
    else
        # Regular install
        $INSTALL_CMD "$pkg" >> "$LOG_FILE" 2>&1
    fi

    if is_installed "$tool"; then
        echo -e "${GREEN}OK${NC}" | tee -a "$LOG_FILE"
        return 0
    else
        echo -e "${RED}FAILED${NC}" | tee -a "$LOG_FILE"
        return 1
    fi
}

# Update package lists
if [[ -n "$UPDATE_CMD" ]]; then
    echo "Updating package lists..." | tee -a "$LOG_FILE"
    eval "$UPDATE_CMD" >> "$LOG_FILE" 2>&1 || echo -e "${YELLOW}Update failed, continuing anyway.${NC}" | tee -a "$LOG_FILE"
fi

# Install required tools
FAILED=()
for tool in "${REQUIRED_TOOLS[@]}"; do
    if is_installed "$tool" && [[ $FORCE -eq 0 ]]; then
        echo -e "$tool: ${GREEN}already installed${NC}" | tee -a "$LOG_FILE"
        continue
    fi

    # Map tool names to package names where different
    pkg_name="$tool"
    case "$tool" in
        go)
            # On some systems, Go is 'golang'
            if [[ "$PACKAGE_MANAGER" == "apt" ]]; then
                pkg_name="golang-go"
            elif [[ "$PACKAGE_MANAGER" == "yum" || "$PACKAGE_MANAGER" == "dnf" ]]; then
                pkg_name="golang"
            fi
            ;;
        traceroute)
            # On some distros, traceroute is in a separate package
            if [[ "$PACKAGE_MANAGER" == "apt" ]]; then
                pkg_name="traceroute"
            elif [[ "$PACKAGE_MANAGER" == "yum" || "$PACKAGE_MANAGER" == "dnf" ]]; then
                pkg_name="traceroute"
            fi
            ;;
        bc)
            # bc is usually bc
            ;;
        osquery)
            # osquery might need repo
            ;;
    esac

    if ! install_tool "$tool" "$pkg_name"; then
        FAILED+=("$tool")
    fi
done

# Summary
echo "" | tee -a "$LOG_FILE"
echo "=== Setup Summary ===" | tee -a "$LOG_FILE"
if [[ ${#FAILED[@]} -eq 0 ]]; then
    echo -e "${GREEN}All required tools installed successfully.${NC}" | tee -a "$LOG_FILE"
else
    echo -e "${YELLOW}Some tools could not be installed: ${FAILED[*]}${NC}" | tee -a "$LOG_FILE"
    echo "The toolkit will still run but with reduced functionality." | tee -a "$LOG_FILE"
    echo "You may need to install them manually." | tee -a "$LOG_FILE"
fi

echo "Log written to $LOG_FILE"
