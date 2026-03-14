# Netmap Enhancement Plan (Capability-First)

This plan prioritizes **diagnostic capability** while keeping UX simple. The default command should remain easy to run, but output should be complete enough to detect anomalies that matter in real environments.

## Problems in the Previous Plan (What We Correct)

1. **"Simple by default" reduced coverage** by pushing high-value checks too far out.
2. **Daemon/monitor mode was removed**, which weakens detection of route, gateway, and DNS path changes over time.
3. **Baseline/diff and integrity were delayed**, even though they are core to change detection and evidence quality.
4. **Integrity was optional**; forensic metadata should be default-on.
5. **`--redact` design leaked structure** (`192.168.x.x`) instead of true safe sharing.
6. **Correlation output was overly hedged**; conclusive evidence should produce assertions.
7. **Definition of Done lacked adversarial validation**.
8. **Public IP over HTTP was allowed**, which is interception-prone.

---

## Revised Goals

- Default output is **complete enough to detect anomalies**, not just readable.
- Advanced capability is built into defaults where security-relevant, with opt-outs where needed.
- Security-critical features (integrity, diff, DNS consistency/leak detection) start in Phase 1.
- Linux-native depth (netlink/raw socket/eBPF-adjacent inputs) is first-class while retaining cross-platform behavior where practical.

---

## Phase 1: Diagnostic Completeness First

### 1) Executive Summary with confidence and anomaly flags
- Include primary interface, default gateway, DNS (system + observed), UDP/TCP egress, public IP source/protocol, traceroute status.
- Public IP probe must use **HTTPS**. If only HTTP is possible, label it `UNVERIFIED`.
- Compare TCP vs UDP egress and report divergence explicitly.
- Flag DNS system-vs-observed mismatch as `WARNING`.

**Example output**
```text
SUMMARY
  Primary Interface : wlan0                    [confidence: HIGH]
  Default Gateway   : 192.168.1.1              [confidence: HIGH]
  DNS (system)      : 192.168.1.1
  DNS (observed)    : 10.0.0.53                [WARNING: MISMATCH]
  UDP Egress IP     : 192.168.1.44
  TCP Egress IP     : 192.168.1.44             [consistent]
  Public IP (HTTPS) : 198.51.100.21
  Traceroute        : skipped (--quick)
```

### 2) Flags (core + capability)
- `--quick`: skip slow probes.
- `--out <file>`: explicit report path.
- `--json`: write structured output for automation.
- `--monitor <interval>`: poll continuously and emit diffs on change.
- `--verify`: run probes twice and flag divergence.
- `--no-integrity`: opt out of default integrity block.

**Example commands**
```bash
./netmap_report --quick
./netmap_report --monitor 30s --json --out ./reports/live.json
./netmap_report --verify --out /tmp/netmap.txt
```

### 3) Standardized probe status and error codes
Keep explicit per-probe status and unify failure labels.

**Example**
```text
PROBE dns_lookup: ERROR_TIMEOUT (2s)
PROBE traceroute: SKIPPED_QUICK_MODE
PROBE public_ip_https: ERROR_TLS_HANDSHAKE
```

### 4) Baseline + diff (moved to Phase 1)
Change detection is foundational, not optional power.

**Example command**
```bash
./netmap_report --baseline baseline.json --diff current.json
```

**Example diff snippet**
```text
DIFF SUMMARY
  Default gateway changed: 192.168.1.1 -> 10.0.0.1
  DNS changed: [192.168.1.1] -> [10.0.0.53, 1.1.1.1]
  Public IP changed: 198.51.100.21 -> 203.0.113.45
```

### 5) Integrity block (moved to Phase 1, default-on)

**Example**
```text
INTEGRITY
  SHA256 : a3f1...c9d2
  Signed : false
  Runs   : 1  (use --verify for dual-run consistency check)
```

---

## Phase 2: Hardened Output + Leak Detection

### 6) Real redaction via token substitution
`--redact` should fully replace identifiers, with deterministic per-run tokens so diffs remain useful.

**Example**
```text
ADDR     : <ADDR_A>
ADDR     : <ADDR_B>
HOSTNAME : <HOST_A>
```

### 7) DNS leak detection
Detect whether DNS traffic exits through unexpected interfaces, not just whether resolver IPs differ.

### 8) Correlation assertions (not just hints)
Use assertive language when evidence is conclusive; reserve hints for ambiguity.

**Example**
```text
ASSERT: VPN tunnel active — utun0 present, default route via tunnel interface
ASSERT: DNS path hijacked — system resolver (192.168.1.1) != observed upstream (10.0.0.53)
WARN:   TCP/UDP egress mismatch — possible transparent proxy on TCP port 80/443
```

---

## Phase 3: Profiles + Structured Export

### 9) Profiles
- `--profile safe`: minimum collection.
- `--profile standard`: default depth + summary.
- `--profile forensic`: maximum detail and slower checks.

### 10) Structured exports
- JSON (single document)
- JSONL (event stream)
- Markdown summary

---

## Revised Definition of Done (per feature)
A feature is complete only if:
1. It is optional **or** preserves/improves existing defaults.
2. It includes one documented command example.
3. It includes one sample output snippet with a **failure/anomaly case**.
4. It is tested against at least one known-bad configuration (hijacked DNS, substituted gateway, leaking VPN, transparent proxy).
5. Failure behavior is explicit in report output.

## Revised Implementation Order
1. Flags: `--quick`, `--out`, `--json`, `--monitor`, `--verify`, `--no-integrity`.
2. Executive summary with DNS consistency + TCP/UDP egress comparison.
3. Standardized probe errors/status.
4. Baseline/diff mode.
5. Integrity block default-on.
6. Real redaction + DNS leak detection.
7. Correlation assertions.
8. Profiles + JSONL/Markdown export.
