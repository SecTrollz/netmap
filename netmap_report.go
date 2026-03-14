// netmap_report.go – Collects data from all sources and writes networkmapreport.txt
// Build: go build -o netmap_report netmap_report.go
package main

import (
    "bufio"
    "bytes"
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

    ioutil.WriteFile("networkmapreport.txt", []byte(report.String()), 0644)
    fmt.Println("Report written to networkmapreport.txt")
}

func runLinuxCollector() {
    cmd := exec.Command("./netmap_collect")
    out, err := cmd.Output()
    if err != nil {
        report.WriteString("C collector failed; falling back to command-line tools.\n")
        fallbackLinux()
        return
    }
    report.WriteString(string(out))
}

// ... other platform collectors, egress tests, osquery integration, server logging
