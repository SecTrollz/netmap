// netmap_report.go – Cross‑platform orchestrator and reporter
// Build: go build -o netmap_report netmap_report.go
package main

import (
    "bufio"
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net"
    "net/http"
    "os"
    "os/exec"
    "runtime"
    "strings"
    "time"
)

var report strings.Builder

func main() {
    report.WriteString("NETWORK MAP REPORT\n")
    report.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format(time.RFC3339)))
    report.WriteString(fmt.Sprintf("Hostname: %s\n", getHostname()))
    report.WriteString(fmt.Sprintf("OS: %s\n", runtime.GOOS))

    switch runtime.GOOS {
    case "linux", "android":
        runLinuxCollector()
    case "darwin":
        runMacCollector()
    case "windows":
        runWindowsCollector()
    default:
        report.WriteString("Unsupported OS; using generic tools.\n")
        runGeneric()
    }

    runEgressTests()
    runOsquery()

    if len(os.Args) > 1 && os.Args[1] == "-server" && len(os.Args) > 2 {
        sendToServer(os.Args[2])
    }

    err := ioutil.WriteFile("networkmapreport.txt", []byte(report.String()), 0644)
    if err != nil {
        fmt.Println("Error writing report:", err)
    } else {
        fmt.Println("Report written to networkmapreport.txt")
    }
}

func getHostname() string {
    h, err := os.Hostname()
    if err != nil {
        return "unknown"
    }
    return h
}

// Linux collector: run C helper, fallback to commands
func runLinuxCollector() {
    report.WriteString("\n--- Linux/Android Low-Level Collector ---\n")
    cmd := exec.Command("./netmap_collect")
    out, err := cmd.Output()
    if err != nil {
        report.WriteString("C collector failed; falling back to command-line tools.\n")
        fallbackLinux()
        return
    }
    report.WriteString(string(out))
}

func fallbackLinux() {
    // interfaces
    if out, err := exec.Command("ip", "addr", "show").Output(); err == nil {
        report.WriteString("## ip addr show\n" + string(out) + "\n")
    }
    // routes
    if out, err := exec.Command("ip", "route", "show", "table", "all").Output(); err == nil {
        report.WriteString("## ip route\n" + string(out) + "\n")
    }
    // listeners
    if out, err := exec.Command("ss", "-tulpn").Output(); err == nil {
        report.WriteString("## ss -tulpn\n" + string(out) + "\n")
    } else if out, err := exec.Command("netstat", "-tulpn").Output(); err == nil {
        report.WriteString("## netstat -tulpn\n" + string(out) + "\n")
    }
    // DNS
    if out, err := ioutil.ReadFile("/etc/resolv.conf"); err == nil {
        report.WriteString("## /etc/resolv.conf\n" + string(out) + "\n")
    }
}

// macOS collector
func runMacCollector() {
    report.WriteString("\n--- macOS Collector ---\n")
    // interfaces
    if out, err := exec.Command("ifconfig").Output(); err == nil {
        report.WriteString("## ifconfig\n" + string(out) + "\n")
    }
    // routes
    if out, err := exec.Command("netstat", "-rn").Output(); err == nil {
        report.WriteString("## netstat -rn\n" + string(out) + "\n")
    }
    // DNS
    if out, err := exec.Command("scutil", "--dns").Output(); err == nil {
        report.WriteString("## scutil --dns\n" + string(out) + "\n")
    }
    // listeners
    if out, err := exec.Command("lsof", "-i", "-P", "-n").Output(); err == nil {
        report.WriteString("## lsof -i -P -n\n" + string(out) + "\n")
    }
}

// Windows collector
func runWindowsCollector() {
    report.WriteString("\n--- Windows Collector ---\n")
    // interfaces
    if out, err := exec.Command("ipconfig", "/all").Output(); err == nil {
        report.WriteString("## ipconfig /all\n" + string(out) + "\n")
    }
    // routes
    if out, err := exec.Command("route", "print").Output(); err == nil {
        report.WriteString("## route print\n" + string(out) + "\n")
    }
    // listeners
    if out, err := exec.Command("netstat", "-an").Output(); err == nil {
        report.WriteString("## netstat -an\n" + string(out) + "\n")
    }
    // DNS via PowerShell
    cmd := exec.Command("powershell", "-Command", "Get-DnsClientServerAddress | Format-List")
    if out, err := cmd.Output(); err == nil {
        report.WriteString("## Get-DnsClientServerAddress\n" + string(out) + "\n")
    }
}

// Generic fallback for unknown OS
func runGeneric() {
    report.WriteString("\n--- Generic System Information ---\n")
    // try some common commands if they exist
    if out, err := exec.Command("ifconfig").Output(); err == nil {
        report.WriteString("## ifconfig\n" + string(out) + "\n")
    }
    if out, err := exec.Command("netstat", "-rn").Output(); err == nil {
        report.WriteString("## netstat -rn\n" + string(out) + "\n")
    }
}

// Egress tests (common to all platforms)
func runEgressTests() {
    report.WriteString("\n--- Egress Path Tests ---\n")

    // UDP source IP
    conn, err := net.DialTimeout("udp", "8.8.8.8:53", 5*time.Second)
    if err == nil {
        localAddr := conn.LocalAddr().(*net.UDPAddr)
        report.WriteString(fmt.Sprintf("UDP source IP (to 8.8.8.8): %s\n", localAddr.IP.String()))
        conn.Close()
    } else {
        report.WriteString("UDP test failed: " + err.Error() + "\n")
    }

    // HTTP external IP
    client := http.Client{Timeout: 5 * time.Second}
    resp, err := client.Get("http://httpbin.org/ip")
    if err == nil {
        defer resp.Body.Close()
        body, _ := ioutil.ReadAll(resp.Body)
        report.WriteString(fmt.Sprintf("External IP (via httpbin): %s\n", string(body)))
    } else {
        report.WriteString("HTTP external IP test failed: " + err.Error() + "\n")
    }

    // Traceroute (if available)
    traceroute := "traceroute"
    if runtime.GOOS == "windows" {
        traceroute = "tracert"
    }
    if _, err := exec.LookPath(traceroute); err == nil {
        report.WriteString("\nTraceroute to 8.8.8.8 (first 3 hops):\n")
        var cmd *exec.Cmd
        if runtime.GOOS == "windows" {
            cmd = exec.Command(traceroute, "-h", "3", "8.8.8.8")
        } else {
            cmd = exec.Command(traceroute, "-m", "3", "8.8.8.8")
        }
        out, _ := cmd.CombinedOutput()
        report.WriteString(string(out) + "\n")
    } else {
        report.WriteString("traceroute not available\n")
    }

    // DNS resolution
    ips, err := net.LookupIP("google.com")
    if err == nil && len(ips) > 0 {
        report.WriteString(fmt.Sprintf("DNS resolved google.com to: %v\n", ips[0]))
    } else {
        report.WriteString("DNS resolution failed\n")
    }
}

// Osquery enrichment (if installed)
func runOsquery() {
    report.WriteString("\n--- Osquery Enrichment ---\n")
    if _, err := exec.LookPath("osqueryi"); err != nil {
        report.WriteString("osqueryi not found; skipping.\n")
        return
    }

    // Listening ports with process info
    cmd := exec.Command("osqueryi", "--json", "select * from listening_ports l join processes p on l.pid = p.pid;")
    out, err := cmd.Output()
    if err == nil && len(out) > 2 {
        report.WriteString("## listening_ports with processes\n")
        var data interface{}
        if json.Unmarshal(out, &data) == nil {
            pretty, _ := json.MarshalIndent(data, "", "  ")
            report.WriteString(string(pretty) + "\n")
        } else {
            report.WriteString(string(out) + "\n")
        }
    } else {
        report.WriteString("osquery listening_ports query failed or returned no data.\n")
    }

    // Interface addresses
    cmd = exec.Command("osqueryi", "--json", "select * from interface_addresses;")
    out, err = cmd.Output()
    if err == nil && len(out) > 2 {
        report.WriteString("## interface_addresses (osquery)\n")
        report.WriteString(string(out) + "\n")
    }
}

// Send evidence to remote server (optional)
func sendToServer(serverURL string) {
    hash := sha256.Sum256([]byte(report.String()))
    hashStr := hex.EncodeToString(hash[:])
    logEntry := fmt.Sprintf("Time: %s\nReport:\n%s\nHash: %s\n",
        time.Now().Format(time.RFC3339), report.String(), hashStr)
    resp, err := http.Post(serverURL, "text/plain", strings.NewReader(logEntry))
    if err != nil {
        fmt.Println("Failed to send evidence:", err)
        return
    }
    defer resp.Body.Close()
    fmt.Println("Evidence sent to server, response:", resp.Status)
}
```

---

## `netmap.sh` (Complete with Rainbow Animation)

```bash
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
