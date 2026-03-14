```markdown
# Network Map Toolkit

![Platform]()
![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)

A cross‑platform network transparency toolkit that reveals your device’s true network path – from the kernel’s routing table to the public internet. It combines low‑level system introspection (C collector on Linux, native APIs on other OSes) with egress tests and optional osquery enrichment to produce a single, human‑readable report: `networkmapreport.txt`.

Whether you’re a sysadmin debugging a strange connection, a privacy advocate verifying your VPN, or a curious user who wants to see what’s really between your device and the cloud, this toolkit gives you evidence‑quality data without modifying your system.

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

- **Cross‑platform** – Works on Linux, Android (via same Linux tools), macOS, and Windows (with fallback command‑line tools).
- **Low‑level Linux collector** – Uses netlink sockets and `/proc` to read interfaces, routes, addresses, listening sockets, and DNS configuration. Includes a tiny inline assembly example (optional) for demonstration.
- **Go orchestrator** – Correlates data from the C collector, runs egress tests (UDP source IP, HTTP public IP, traceroute, DNS resolution), and integrates with osquery (if installed) to enrich process information.
- **Graceful degradation** – Missing tools are noted; the report always states what could and could not be observed.
- **Tamper‑evident logging** – Optionally send a timestamped, hashed report to a server you control for long‑term transparency archives.
- **No system modifications** – Reads only; never changes network configuration.
- **Clear, plain‑language report** – Explains the likely end‑to‑end network path, including evidence of Android/VM/hypervisor layers.

---

## Requirements

- **Linux / Android**: gcc, go, traceroute (optional), osquery (optional).
- **macOS**: go, standard command‑line tools (ifconfig, netstat, scutil, lsof). traceroute is usually present.
- **Windows**: go, PowerShell. For traceroute, `tracert` is built‑in.
- **All platforms**: sudo (or administrator privileges) is recommended to get the most complete information (e.g., netlink on Linux, raw socket access). Without privileges, the toolkit will still run but may show limited data.

---

## Installation

1. Clone or copy the three files into a directory on your machine:
   - `netmap_collect.c` – C collector (Linux only)
   - `netmap_report.go` – Go orchestrator (all platforms)
   - `netmap.sh` – Bootstrap script (optional but convenient)

2. Make the bootstrap script executable (if you use it):
   ```bash
   chmod +x netmap.sh
   ```

3. Compile the C collector (Linux/Android only):
   ```bash
   gcc -Wall -O2 -o netmapcollect netmap_collect.c
   ```
   The Go orchestrator will be compiled automatically when you run it, or you can build it manually:
   ```bash
   go build -o netmapreport netmap_report.go
   ```

---

## Usage

### Quick Start (Linux / macOS)

```bash
sudo ./netmap.sh
```

This will:
- Detect your OS.
- Run the appropriate collectors.
- Perform egress tests.
- Write `networkmapreport.txt` in the current directory.
- Print a summary to the console.

### Running Individual Components

- **Linux only**: If you just want raw low‑level data, run the C collector directly:
  ```bash
  sudo ./netmap_collect
  ```
- **All platforms**: To run the Go orchestrator without the bootstrap script:
  ```bash
  sudo ./netmap_report
  ```
- **Enable osquery enrichment**: Install osquery first; the toolkit will automatically detect it.
- **Send evidence to a remote server (optional)**:
  ```bash
  sudo ./netmap_report -server https://your-server.com/endpoint
  ```
  *(The server endpoint must accept plain text POSTs.)*

---

## Understanding the Report (`networkmapreport.txt`)

The report is divided into sections:

1. **Header** – Timestamp, hostname, OS.
2. **Low‑level collector output** (Linux only) – Links, addresses, routes, listening sockets, DNS servers (from `/etc/resolv.conf`).
3. **Platform‑specific fallback data** – If the C collector fails, the report shows output from standard tools (`ip`, `ss`, `netstat`, etc.).
4. **Egress tests**:
   - UDP source IP – The IP used when talking to `8.8.8.8:53` (your apparent IP from your ISP’s perspective, before any NAT).
   - External IP – Your public IP as seen by `http://httpbin.org/ip` (after any carrier‑grade NAT or VPN).
   - Traceroute – First few hops to `8.8.8.8` (helps identify middleboxes, VPN endpoints, or hypervisor layers).
   - DNS resolution – Tests that `google.com` resolves.
5. **Osquery enrichment** (if available) – Listening ports correlated with running processes, plus detailed interface addresses.
6. **Optional server log confirmation** – If you used the `-server` flag, the server’s response is shown.

---

## Troubleshooting

| Problem                            | Likely Cause                                | Solution                                                               |
|-----------------------------------|----------------------------------------------|------------------------------------------------------------------------|
| `netmapcollect: command not found` | C collector not compiled.                    | Run `gcc -Wall -O2 -o netmapcollect netmap_collect.c`.                |
| `go: command not found`             | Go not installed.                          | Install Go from [golang.org](https://golang.org).                     |
| `sudo: netmap.sh: command not found` | Script not executable or not in PATH.      | Use `./netmap.sh` or run with full path.                               |
| Permission denied (netlink)        | Not running as root.                         | Run with `sudo`.                                                       |
| Egress tests fail                  | Network offline or firewall blocking.       | Check your internet connection; the tests have short timeouts and will not hang. |
| No osquery data                   | osquery not installed or not in PATH.       | Install osquery if you need process correlation. The toolkit still works without it. |
| Android specific quirks           | Android may restrict some `/proc` access.   | The toolkit detects Android and also reads `getprop` DNS; run as root if possible. |

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

- **Transparency** – See exactly what your device sees, without assumptions.
- **Evidence** – Combine low‑level kernel data with external tests to build a reliable picture.
- **Low‑level control** – The C collector uses netlink directly, bypassing userspace tools that might be influenced by VPNs or proxies.
- **Portability** – One Go binary runs everywhere; the C collector adds depth on Linux.

---

## License

This project is licensed under the [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0). You are free to use, modify, and distribute it under the terms of this license.

---

## Contributing

Found a bug? Want to add a feature? Open an issue or pull request on the project repository (if hosted). Contributions that maintain the toolkit’s focus on transparency, safety, and cross‑platform support are welcome. (but unlikely, just make your ai hostage write it muhahah)
```
