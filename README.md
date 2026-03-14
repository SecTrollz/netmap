## Network Map Toolkit

![Platform]()
![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)

**Network Map Toolkit** is a cross-platform solution designed to unveil your device’s true network path—from the kernel’s routing table to the public internet. It integrates low-level system introspection (using a C collector on Linux and native APIs on other operating systems) with egress tests and optional osquery enrichment. The result is a single, human-readable report: `networkmapreport.txt`.

Whether you are a sysadmin troubleshooting a peculiar connection, a privacy advocate checking your VPN, or a curious user aiming to uncover the traffic between your device and the cloud, this toolkit offers evidence-quality data without altering your system.

---

## Table of Contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Usage](#usage)
- [Understanding the Report](#understanding-the-report)
- [Troubleshooting](#troubleshooting)
- [Example Report Snippet](#example-report-snippet)
- [Why This Toolkit?](#why-this-toolkit)
- [License](#license)
- [Contributing](#contributing)

---

## Features

- **Cross-platform compatibility**: Operates on Linux, Android (utilizing the same Linux tools), macOS, and Windows (with fallback command-line tools).
- **Low-level Linux collector**: Utilizes netlink sockets and `/proc` to gather information on interfaces, routes, addresses, and DNS configuration. A small inline assembly example is included (optional).
- **Go orchestrator**: Correlates data from the C collector, executes egress tests (UDP source IP, public HTTP IP, traceroute, DNS resolution), and enhances process information if osquery is installed.
- **Graceful degradation**: If tools are missing, the report notes what could and could not be observed.
- **Tamper-evident logging**: Optionally submit a timestamped, hashed report to a server you manage for long-term transparency archives.
- **No system modifications**: The toolkit only reads data; it never alters your network configuration.
- **Clear, plain-language report**: Provides an understandable overview of the likely end-to-end network path, highlighting evidence from Android, VM, or hypervisor layers.

---

## Requirements

- **Linux / Android**: Requires gcc, go, and optionally traceroute and osquery.
- **macOS**: Requires go and standard command-line tools (ifconfig, netstat, scutil, lsof). Traceroute usually comes pre-installed.
- **Windows**: Requires go and PowerShell. Traceroute is accessible via `tracert`, which is built-in.
- **All platforms**: For the most comprehensive information (e.g., netlink on Linux, raw socket access), sudo (or administrator privileges) is recommended. The toolkit still runs without privileges but may present limited data.

---

## Installation

1. Clone or copy the three files into a directory on your machine:
   - `netmap_collect.c` – C collector (Linux only)
   - `netmap_report.go` – Go orchestrator (all platforms)
   - `netmap.sh` – Bootstrap script (recommended for convenience)

2. Make the bootstrap script executable (if you choose to use it):
   ```bash
   chmod +x netmap.sh
   ```

3. Compile the C collector (Linux/Android only):
   ```bash
   gcc -Wall -O2 -o netmapcollect netmap_collect.c
   ```
   The Go orchestrator compiles automatically when you execute it, or you may build it manually:
   ```bash
   go build -o netmapreport netmap_report.go
   ```

---

## Usage

### Quick Start (Linux / macOS)

```bash
sudo ./netmap.sh
```

This command will:
- Detect your operating system.
- Execute the appropriate collectors.
- Conduct egress tests.
- Write `networkmapreport.txt` in the current directory.
- Display a summary in the console.

### Running Individual Components

- **Linux only**: For raw low-level data, directly run the C collector:
  ```bash
  sudo ./netmap_collect
  ```
- **All platforms**: To execute the Go orchestrator without the bootstrap script:
  ```bash
  sudo ./netmap_report
  ```
- **Enable osquery enrichment**: Install osquery first; the toolkit will automatically recognize it.
- **Send evidence to a remote server (optional)**:
  ```bash
  sudo ./netmap_report -server https://your-server.com/endpoint
  ```
  *(The server endpoint should accept plain text POSTs.)*

---

## Understanding the Report (`networkmapreport.txt`)

The report is organized into sections:

1. **Header** – Contains a timestamp, hostname, and operating system.
2. **Low-level collector output** (Linux only) – Displays links, addresses, routes, listening sockets, and DNS servers (extracted from `/etc/resolv.conf`).
3. **Platform-specific fallback data** – If the C collector fails, output from standard tools (`ip`, `ss`, `netstat`, etc.) is presented.
4. **Egress tests**:
   - UDP source IP – The IP used when connecting to `8.8.8.8:53` (your apparent IP visible to your ISP, pre-NAT).
   - External IP – Your public IP as identified by `http://httpbin.org/ip` (after carrier-grade NAT or VPN).
   - Traceroute – Initial hops to `8.8.8.8` (helps pinpoint middleboxes, VPN endpoints, or hypervisor layers).
   - DNS resolution – Confirms that `google.com` resolves correctly.
5. **Osquery enrichment** (if available) – Correlates listening ports with running processes, alongside detailed interface addresses.
6. **Optional server log confirmation** – If the `-server` flag was used, the server’s response is included.

---

## Troubleshooting

| Problem                            | Likely Cause                                | Solution                                                               |
|-----------------------------------|---------------------------------------------|------------------------------------------------------------------------|
| `netmapcollect: command not found` | C collector not compiled.                   | Run `gcc -Wall -O2 -o netmapcollect netmap_collect.c`.                |
| `go: command not found`           | Go not installed.                           | Install Go from [golang.org](https://golang.org).                     |
| `sudo: netmap.sh: command not found` | Script not executable or not in PATH.     | Use `./netmap.sh` or run it with the full path.                       |
| Permission denied (netlink)       | Not running as root.                        | Execute with `sudo`.                                                  |
| Egress tests fail                 | Network offline or firewall blocking.      | Check your internet connection; tests have short timeouts and will not hang. |
| No osquery data                   | osquery not installed or not in PATH.      | Install osquery if you require process correlation; the toolkit still operates without it. |
| Android specific quirks           | Android may restrict some `/proc` access.  | The toolkit identifies Android and reads `getprop` DNS; run as root if possible. |

---

## Example Report Snippet

```plaintext
NETWORK MAP REPORT
Generated: 2025-03-14T10:15:30Z
Hostname: debian-android
OS: linux/arm64

--- Linux/Android Low-Level Collector ---
LINK: index=1 name=lo flags=0x49 type=772
LINK: index=2 name=wlan0 flags=0x11043 type=1
ADDR: index=2 family=2 prefixlen=24 addr=192.168.1.100 scope=0
ROUTE: family=2 table=254 dst=0.0.0.0 gateway=192.168.1.1 oif=2
LISTENER: proto=tcp local=0.0.0.0:22 state=10 uid=0 inode=12345
DNS: nameserver=192.168.1.1

--- Egress Path Tests ---
UDP source IP (to 8.8.8.8): 192.168.1.100
External IP (via httpbin): {"origin": "203.0.113.45"}
Traceroute to 8.8.8.8:
 1  192.168.1.1  2.3 ms
 2  10.0.0.1  5.1 ms
 3  203.0.113.1  10.2 ms
DNS resolved google.com to 142.250.185.78
```

---

## Why This Toolkit?

- **Transparency**: Reveals precisely what your device perceives, without conjectures.
- **Evidence**: Merges low-level kernel data with external tests to form a robust understanding.
- **Low-level control**: The C collector accesses netlink directly, bypassing userspace tools that might be affected by VPNs or proxies.
- **Portability**: A single Go binary runs on any platform; the C collector enriches functionality on Linux.

---

## License

This project is licensed under the [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0). You are free to use, modify, and distribute it under this license, per its terms.

---

## Contributing

If you discover a bug or wish to suggest a feature, open an issue or pull request on the project repository (if hosted). Contributions that maintain the toolkit’s focus on transparency, safety, and cross-platform support are most welcome.
