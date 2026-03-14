// netmap_report.go – Cross-platform orchestrator and reporter
// Build: go build -o netmap_report netmap_report.go
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

type options struct {
	quick       bool
	out         string
	jsonOut     bool
	verify      bool
	monitor     int
	noIntegrity bool
	redact      bool
	baseline    string
	diff        string
	server      string
}

type ProbeResult struct {
	Timestamp         string   `json:"timestamp"`
	PrimaryIface      string   `json:"primary_iface"`
	IfaceConfidence   string   `json:"iface_confidence"`
	Gateway           string   `json:"gateway"`
	GatewayConfidence string   `json:"gateway_confidence"`
	DNSSystem         string   `json:"dns_system"`
	DNSObserved       string   `json:"dns_observed"`
	UDPEgress         string   `json:"udp_egress"`
	TCPEgress         string   `json:"tcp_egress"`
	PublicIP          string   `json:"public_ip"`
	PublicIPLabel     string   `json:"public_ip_label"`
	Traceroute        string   `json:"traceroute"`
	Hostname          string   `json:"hostname"`
	OS                string   `json:"os"`
	Assertions        []string `json:"assertions"`
}

type ReportJSON struct {
	Generated  string                 `json:"generated"`
	Hostname   string                 `json:"hostname"`
	OS         string                 `json:"os"`
	Summary    map[string]string      `json:"summary"`
	Assertions []string               `json:"assertions"`
	Integrity  map[string]interface{} `json:"integrity,omitempty"`
}

func main() {
	opts := parseFlags()

	if opts.monitor > 0 {
		runMonitor(opts)
		return
	}

	run1, details1 := runFullCollection(opts)
	runs := 1
	verifyWarnings := []string{}

	if opts.verify {
		runs = 2
		run2, _ := runFullCollection(opts)
		verifyWarnings = compareRuns(run1, run2)
		run1.Assertions = append(run1.Assertions, verifyWarnings...)
	}

	diffSummary := ""
	if opts.diff != "" {
		ds, extraAssertions := diffAgainstBaseline(opts.diff, run1)
		diffSummary = ds
		run1.Assertions = append(run1.Assertions, extraAssertions...)
	}

	if opts.baseline != "" {
		_ = writeJSONFile(opts.baseline, run1)
	}

	reportText := buildReport(run1, details1, diffSummary, runs, opts.noIntegrity)
	jsonPayload := buildJSON(run1, runs, opts.noIntegrity, reportText)

	if opts.redact {
		reporter := newRedactor()
		reportText = reporter.Apply(reportText)
		if opts.jsonOut {
			j, _ := json.MarshalIndent(jsonPayload, "", "  ")
			redactedJSON := reporter.Apply(string(j))
			_ = os.WriteFile(jsonPathFor(opts.out), []byte(redactedJSON), 0644)
		}
	} else if opts.jsonOut {
		j, _ := json.MarshalIndent(jsonPayload, "", "  ")
		_ = os.WriteFile(jsonPathFor(opts.out), j, 0644)
	}

	if err := os.WriteFile(opts.out, []byte(reportText), 0644); err != nil {
		fmt.Println("Error writing report:", err)
		return
	}

	if opts.server != "" {
		sendToServer(opts.server, reportText)
	}

	fmt.Println("Report written to", opts.out)
}

func parseFlags() options {
	quick := flag.Bool("quick", false, "skip traceroute")
	out := flag.String("out", "networkmapreport.txt", "report output path")
	jsonOut := flag.Bool("json", false, "also write report.json beside text report")
	verify := flag.Bool("verify", false, "run probes twice and flag divergence")
	monitor := flag.Int("monitor", 0, "poll interval in seconds")
	noIntegrity := flag.Bool("no-integrity", false, "suppress integrity block")
	redact := flag.Bool("redact", false, "token-substitute all IPs and hostnames")
	baseline := flag.String("baseline", "", "path to save current run as JSON")
	diff := flag.String("diff", "", "path to load baseline JSON and compare")
	server := flag.String("server", "", "optional evidence server URL")
	flag.Parse()
	return options{*quick, *out, *jsonOut, *verify, *monitor, *noIntegrity, *redact, *baseline, *diff, *server}
}

func runFullCollection(opts options) (ProbeResult, string) {
	res := ProbeResult{Timestamp: time.Now().Format(time.RFC3339), Hostname: getHostname(), OS: runtime.GOOS}
	res.PrimaryIface, res.IfaceConfidence = detectPrimaryInterface()
	res.Gateway, res.GatewayConfidence = detectDefaultGateway()
	res.DNSSystem = detectSystemResolver()
	res.DNSObserved = detectObservedResolver()

	res.UDPEgress = detectUDPEgress()
	res.TCPEgress = detectTCPEgress()
	if res.UDPEgress != "unknown" && res.TCPEgress != "unknown" {
		if res.UDPEgress != res.TCPEgress {
			res.Assertions = append(res.Assertions, "WARN: TCP/UDP egress mismatch — possible transparent proxy")
		} else {
			res.TCPEgress += " [consistent]"
		}
	}

	if res.DNSSystem != "unknown" && res.DNSObserved != "unknown" {
		if res.DNSSystem != res.DNSObserved {
			res.Assertions = append(res.Assertions, fmt.Sprintf("ASSERT: DNS path hijacked — system resolver (%s) != observed upstream (%s)", res.DNSSystem, res.DNSObserved))
		} else {
			res.Assertions = append(res.Assertions, "DNS path: consistent")
		}
	}

	res.PublicIP, res.PublicIPLabel = detectPublicIP()

	details := collectPlatformDetails()
	tr, trRaw := runTraceroute(opts.quick)
	res.Traceroute = tr
	if trRaw != "" {
		details += "\n" + trRaw
	}
	details += runOsquery()
	return res, details
}

func buildReport(res ProbeResult, details string, diffSummary string, runs int, noIntegrity bool) string {
	var b strings.Builder
	b.WriteString("NETWORK MAP REPORT\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", res.Timestamp))
	b.WriteString(fmt.Sprintf("Hostname: %s\n", res.Hostname))
	b.WriteString(fmt.Sprintf("OS: %s\n\n", res.OS))

	dnsStatus := "[consistent]"
	if res.DNSSystem != "unknown" && res.DNSObserved != "unknown" && res.DNSSystem != res.DNSObserved {
		dnsStatus = "[WARNING: MISMATCH]"
	}
	tcpStatus := ""
	if strings.Contains(res.TCPEgress, "[consistent]") {
		tcpStatus = ""
	} else if res.UDPEgress != "unknown" && res.TCPEgress != "unknown" && res.UDPEgress != res.TCPEgress {
		tcpStatus = " [WARNING: MISMATCH]"
	}
	publicLabel := "Public IP (HTTPS)"
	if res.PublicIPLabel != "HTTPS" {
		publicLabel = "Public IP (UNVERIFIED - HTTP only)"
	}
	if strings.HasPrefix(res.PublicIP, "ERROR") {
		publicLabel = "Public IP"
	}

	b.WriteString("SUMMARY\n")
	b.WriteString(fmt.Sprintf("  Primary Interface : %s [confidence: %s]\n", res.PrimaryIface, res.IfaceConfidence))
	b.WriteString(fmt.Sprintf("  Default Gateway   : %s [confidence: %s]\n", res.Gateway, res.GatewayConfidence))
	b.WriteString(fmt.Sprintf("  DNS (system)      : %s\n", res.DNSSystem))
	b.WriteString(fmt.Sprintf("  DNS (observed)    : %s %s\n", res.DNSObserved, dnsStatus))
	b.WriteString(fmt.Sprintf("  UDP Egress IP     : %s\n", res.UDPEgress))
	b.WriteString(fmt.Sprintf("  TCP Egress IP     : %s%s\n", res.TCPEgress, tcpStatus))
	b.WriteString(fmt.Sprintf("  %s : %s\n", publicLabel, res.PublicIP))
	b.WriteString(fmt.Sprintf("  Traceroute        : %s\n", res.Traceroute))

	if len(res.Assertions) > 0 {
		b.WriteString("\nASSERTIONS\n")
		for _, a := range res.Assertions {
			b.WriteString("  " + a + "\n")
		}
	}

	if diffSummary != "" {
		b.WriteString("\n" + diffSummary + "\n")
	}

	b.WriteString("\nDETAILS\n")
	b.WriteString(details)

	if !noIntegrity {
		base := b.String()
		sum := sha256.Sum256([]byte(base))
		b.WriteString("\nINTEGRITY\n")
		b.WriteString(fmt.Sprintf("  SHA256 : %s\n", hex.EncodeToString(sum[:])))
		b.WriteString("  Signed : false\n")
		b.WriteString(fmt.Sprintf("  Runs   : %d\n", runs))
	}
	return b.String()
}

func buildJSON(res ProbeResult, runs int, noIntegrity bool, reportText string) ReportJSON {
	summary := map[string]string{
		"primary_interface": res.PrimaryIface,
		"default_gateway":   res.Gateway,
		"dns_system":        res.DNSSystem,
		"dns_observed":      res.DNSObserved,
		"udp_egress_ip":     res.UDPEgress,
		"tcp_egress_ip":     res.TCPEgress,
		"public_ip":         res.PublicIP,
		"traceroute":        res.Traceroute,
	}
	payload := ReportJSON{Generated: res.Timestamp, Hostname: res.Hostname, OS: res.OS, Summary: summary, Assertions: res.Assertions}
	if !noIntegrity {
		h := sha256.Sum256([]byte(reportText))
		payload.Integrity = map[string]interface{}{"sha256": hex.EncodeToString(h[:]), "signed": false, "runs": runs}
	}
	return payload
}

func compareRuns(a, b ProbeResult) []string {
	warns := []string{}
	check := func(field, v1, v2 string) {
		if v1 != v2 {
			warns = append(warns, fmt.Sprintf("WARN: %s changed between runs: %s -> %s", field, v1, v2))
		}
	}
	check("primary_iface", a.PrimaryIface, b.PrimaryIface)
	check("gateway", a.Gateway, b.Gateway)
	check("dns_system", a.DNSSystem, b.DNSSystem)
	check("dns_observed", a.DNSObserved, b.DNSObserved)
	check("udp_egress", a.UDPEgress, b.UDPEgress)
	check("tcp_egress", a.TCPEgress, b.TCPEgress)
	check("public_ip", a.PublicIP, b.PublicIP)
	return warns
}

func diffAgainstBaseline(path string, cur ProbeResult) (string, []string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "DIFF SUMMARY\n  ERROR: unable to read baseline", nil
	}
	var base ProbeResult
	if err := json.Unmarshal(b, &base); err != nil {
		return "DIFF SUMMARY\n  ERROR: baseline JSON invalid", nil
	}
	var s strings.Builder
	assertions := []string{}
	s.WriteString("DIFF SUMMARY\n")
	line := func(label, old, nw string) {
		state := "[unchanged]"
		if old != nw {
			state = "[CHANGED]"
		}
		s.WriteString(fmt.Sprintf("  %-16s: %s -> %s %s\n", label, old, nw, state))
	}
	line("Default Gateway", base.Gateway, cur.Gateway)
	line("DNS (observed)", base.DNSObserved, cur.DNSObserved)
	line("Public IP", base.PublicIP, cur.PublicIP)
	line("UDP Egress", base.UDPEgress, cur.UDPEgress)
	if base.DNSObserved != cur.DNSObserved && cur.DNSSystem != cur.DNSObserved {
		assertions = append(assertions, fmt.Sprintf("ASSERT: DNS path hijacked — system resolver (%s) != observed upstream (%s)", cur.DNSSystem, cur.DNSObserved))
	}
	if base.UDPEgress != cur.UDPEgress && cur.TCPEgress != "unknown" && !strings.Contains(cur.TCPEgress, cur.UDPEgress) {
		assertions = append(assertions, "WARN: TCP/UDP egress mismatch — possible transparent proxy")
	}
	return s.String(), assertions
}

func writeJSONFile(path string, v interface{}) error {
	b, _ := json.MarshalIndent(v, "", "  ")
	return os.WriteFile(path, b, 0644)
}

func jsonPathFor(out string) string {
	dir := filepath.Dir(out)
	if dir == "" || dir == "." {
		return "report.json"
	}
	return filepath.Join(dir, "report.json")
}

func getHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func detectPrimaryInterface() (string, string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "unknown", "LOW"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			addrs, _ := iface.Addrs()
			if len(addrs) > 0 {
				return iface.Name, "HIGH"
			}
		}
	}
	return "unknown", "LOW"
}

func detectDefaultGateway() (string, string) {
	if runtime.GOOS == "linux" || runtime.GOOS == "android" {
		out, err := exec.Command("sh", "-c", "ip route show default 2>/dev/null | awk '{print $3; exit}'").Output()
		if err == nil {
			gw := strings.TrimSpace(string(out))
			if gw != "" {
				return gw, "HIGH"
			}
		}
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sh", "-c", "route -n get default 2>/dev/null | awk '/gateway:/{print $2; exit}'").Output()
		if err == nil {
			gw := strings.TrimSpace(string(out))
			if gw != "" {
				return gw, "HIGH"
			}
		}
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("powershell", "-Command", "(Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1).NextHop").Output()
		if err == nil {
			gw := strings.TrimSpace(string(out))
			if gw != "" {
				return gw, "MEDIUM"
			}
		}
	}
	return "unknown", "LOW"
}

func detectSystemResolver() string {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("powershell", "-Command", "(Get-DnsClientServerAddress -AddressFamily IPv4 | Select-Object -First 1).ServerAddresses[0]").Output()
		if err == nil {
			v := strings.TrimSpace(string(out))
			if v != "" {
				return v
			}
		}
		return "unknown"
	}
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return "unknown"
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "nameserver") {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				return parts[1]
			}
		}
	}
	return "unknown"
}

func detectObservedResolver() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:53", 5*time.Second)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "permission") {
			return "REQUIRES_ROOT"
		}
		return "unknown"
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	q := buildDNSQuery("google.com")
	if _, err := conn.Write(q); err != nil {
		return "unknown"
	}
	buf := make([]byte, 512)
	if _, err := conn.Read(buf); err != nil {
		return "unknown"
	}
	if addr, ok := conn.RemoteAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return "unknown"
}

func buildDNSQuery(host string) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:2], uint16(time.Now().UnixNano()))
	binary.BigEndian.PutUint16(buf[2:4], 0x0100)
	binary.BigEndian.PutUint16(buf[4:6], 1)
	parts := strings.Split(host, ".")
	for _, p := range parts {
		buf = append(buf, byte(len(p)))
		buf = append(buf, []byte(p)...)
	}
	buf = append(buf, 0x00)
	buf = append(buf, 0x00, 0x01, 0x00, 0x01)
	return buf
}

func detectUDPEgress() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 5*time.Second)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "permission") {
			return "REQUIRES_ROOT"
		}
		return "unknown"
	}
	defer conn.Close()
	if local, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return local.IP.String()
	}
	return "unknown"
}

func detectTCPEgress() string {
	conn, err := net.DialTimeout("tcp", "8.8.8.8:443", 5*time.Second)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "permission") {
			return "REQUIRES_ROOT"
		}
		return "unknown"
	}
	defer conn.Close()
	if local, ok := conn.LocalAddr().(*net.TCPAddr); ok {
		return local.IP.String()
	}
	return "unknown"
}

func detectPublicIP() (string, string) {
	client := http.Client{Timeout: 5 * time.Second}
	httpsEndpoints := []string{"https://api.ipify.org", "https://ifconfig.me/ip"}
	for _, ep := range httpsEndpoints {
		if ip, err := fetchIP(client, ep); err == nil {
			return ip, "HTTPS"
		}
	}
	if ip, err := fetchIP(client, "http://httpbin.org/ip"); err == nil {
		return ip, "UNVERIFIED"
	}
	return "ERROR_TIMEOUT", "ERROR"
}

func fetchIP(client http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http_%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	text := strings.TrimSpace(string(body))
	if strings.Contains(text, "origin") {
		var x map[string]string
		if json.Unmarshal(body, &x) == nil && x["origin"] != "" {
			text = strings.TrimSpace(strings.Split(x["origin"], ",")[0])
		}
	}
	if net.ParseIP(text) == nil {
		for _, f := range strings.FieldsFunc(text, func(r rune) bool {
			return r == '{' || r == '}' || r == '"' || r == ':' || r == ',' || r == ' ' || r == '\n' || r == '\t'
		}) {
			if net.ParseIP(f) != nil {
				return f, nil
			}
		}
		return "", fmt.Errorf("invalid_ip")
	}
	return text, nil
}

func runTraceroute(quick bool) (string, string) {
	if quick {
		return "skipped (-quick)", ""
	}
	cmdName := "traceroute"
	args := []string{"-m", "3", "8.8.8.8"}
	if runtime.GOOS == "windows" {
		cmdName = "tracert"
		args = []string{"-h", "3", "8.8.8.8"}
	}
	if _, err := exec.LookPath(cmdName); err != nil {
		return "unavailable", "traceroute not available\n"
	}
	out, err := exec.Command(cmdName, args...).CombinedOutput()
	if err != nil {
		if isPermissionErr(err.Error()) {
			return "REQUIRES_ROOT", "Traceroute: REQUIRES_ROOT\n"
		}
		return "failed", "Traceroute failed\n"
	}
	return "run", "Traceroute to 8.8.8.8 (first 3 hops):\n" + string(out) + "\n"
}

func collectPlatformDetails() string {
	var b strings.Builder
	switch runtime.GOOS {
	case "linux", "android":
		b.WriteString("\n--- Linux/Android Low-Level Collector ---\n")
		out, err := exec.Command("./netmap_collect").Output()
		if err == nil {
			b.Write(out)
		} else {
			b.WriteString("C collector failed; falling back to command-line tools.\n")
			b.WriteString(commandOutput("ip", "addr", "show"))
			b.WriteString(commandOutput("ip", "route", "show", "table", "all"))
			b.WriteString(commandOutput("ss", "-tulpn"))
			if _, err := os.Stat("/etc/resolv.conf"); err == nil {
				if data, e := os.ReadFile("/etc/resolv.conf"); e == nil {
					b.WriteString("## /etc/resolv.conf\n" + string(data) + "\n")
				}
			}
		}
	case "darwin":
		b.WriteString("\n--- macOS Collector ---\n")
		b.WriteString(commandOutput("ifconfig"))
		b.WriteString(commandOutput("netstat", "-rn"))
		b.WriteString(commandOutput("scutil", "--dns"))
		b.WriteString(commandOutput("lsof", "-i", "-P", "-n"))
	case "windows":
		b.WriteString("\n--- Windows Collector ---\n")
		b.WriteString(commandOutput("ipconfig", "/all"))
		b.WriteString(commandOutput("route", "print"))
		b.WriteString(commandOutput("netstat", "-an"))
		b.WriteString(commandOutput("powershell", "-Command", "Get-DnsClientServerAddress | Format-List"))
	default:
		b.WriteString("\n--- Generic System Information ---\n")
		b.WriteString(commandOutput("ifconfig"))
		b.WriteString(commandOutput("netstat", "-rn"))
	}
	return b.String()
}

func commandOutput(name string, args ...string) string {
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}
	out, err := exec.Command(name, args...).CombinedOutput()
	header := "## " + name
	if len(args) > 0 {
		header += " " + strings.Join(args, " ")
	}
	if err != nil {
		if isPermissionErr(string(out) + err.Error()) {
			return header + "\nREQUIRES_ROOT\n"
		}
		return header + "\n" + string(out) + "\n"
	}
	return header + "\n" + string(out) + "\n"
}

func runOsquery() string {
	var b strings.Builder
	b.WriteString("\n--- Osquery Enrichment ---\n")
	if _, err := exec.LookPath("osqueryi"); err != nil {
		b.WriteString("osqueryi not found; skipping.\n")
		return b.String()
	}
	for _, q := range []struct{ title, query string }{
		{"listening_ports with processes", "select * from listening_ports l join processes p on l.pid = p.pid;"},
		{"interface_addresses (osquery)", "select * from interface_addresses;"},
	} {
		out, err := exec.Command("osqueryi", "--json", q.query).Output()
		if err != nil || len(out) == 0 {
			b.WriteString(q.title + " query failed or returned no data.\n")
			continue
		}
		b.WriteString("## " + q.title + "\n")
		var data interface{}
		if json.Unmarshal(out, &data) == nil {
			pretty, _ := json.MarshalIndent(data, "", "  ")
			b.Write(pretty)
			b.WriteString("\n")
		} else {
			b.Write(out)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func sendToServer(serverURL, report string) {
	hash := sha256.Sum256([]byte(report))
	hashStr := hex.EncodeToString(hash[:])
	logEntry := fmt.Sprintf("Time: %s\nReport:\n%s\nHash: %s\n", time.Now().Format(time.RFC3339), report, hashStr)
	resp, err := http.Post(serverURL, "text/plain", strings.NewReader(logEntry))
	if err != nil {
		fmt.Println("Failed to send evidence:", err)
		return
	}
	defer resp.Body.Close()
	fmt.Println("Evidence sent to server, response:", resp.Status)
}

func isPermissionErr(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "operation not permitted") || strings.Contains(m, "permission denied") || strings.Contains(m, "requires root")
}

func runMonitor(opts options) {
	interval := time.Duration(opts.monitor) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	prev, _ := runFullCollection(opts)
	for {
		select {
		case <-sig:
			fmt.Println("Monitor stopped.")
			return
		case <-time.After(interval):
			cur, _ := runFullCollection(opts)
			d := monitorDiff(prev, cur)
			if d != "" {
				line := fmt.Sprintf("[%s]\n%s\n", time.Now().Format(time.RFC3339), d)
				fmt.Print(line)
				f, err := os.OpenFile(opts.out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
				if err == nil {
					_, _ = f.WriteString(line)
					_ = f.Close()
				}
			}
			prev = cur
		}
	}
}

func monitorDiff(a, b ProbeResult) string {
	type field struct{ n, x, y string }
	fields := []field{
		{"Default Gateway", a.Gateway, b.Gateway},
		{"DNS (observed)", a.DNSObserved, b.DNSObserved},
		{"Public IP", a.PublicIP, b.PublicIP},
		{"UDP Egress", a.UDPEgress, b.UDPEgress},
		{"TCP Egress", a.TCPEgress, b.TCPEgress},
	}
	var sb strings.Builder
	changed := 0
	sb.WriteString("DIFF SUMMARY\n")
	for _, f := range fields {
		state := "[unchanged]"
		if f.x != f.y {
			state = "[CHANGED]"
			changed++
		}
		sb.WriteString(fmt.Sprintf("  %-16s: %s -> %s %s\n", f.n, f.x, f.y, state))
	}
	if changed == 0 {
		return ""
	}
	return sb.String()
}

type redactor struct {
	addrMap map[string]string
	hostMap map[string]string
}

func newRedactor() *redactor {
	return &redactor{addrMap: map[string]string{}, hostMap: map[string]string{}}
}

func (r *redactor) Apply(input string) string {
	addrRegexes := []*regexp.Regexp{
		regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
		regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){2,}[0-9a-fA-F:]*\b`),
	}
	hostRe := regexp.MustCompile(`\b([a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}\b`)

	for _, re := range addrRegexes {
		input = r.replaceByFirstSeen(input, re, true)
	}
	input = r.replaceByFirstSeen(input, hostRe, false)
	return input
}

func (r *redactor) replaceByFirstSeen(input string, re *regexp.Regexp, isAddr bool) string {
	matches := re.FindAllStringIndex(input, -1)
	if len(matches) == 0 {
		return input
	}
	var out bytes.Buffer
	last := 0
	for _, m := range matches {
		out.WriteString(input[last:m[0]])
		key := input[m[0]:m[1]]
		tok := r.tokenFor(key, isAddr)
		out.WriteString(tok)
		last = m[1]
	}
	out.WriteString(input[last:])
	return out.String()
}

func (r *redactor) tokenFor(v string, isAddr bool) string {
	if isAddr {
		if t, ok := r.addrMap[v]; ok {
			return t
		}
		t := fmt.Sprintf("<ADDR_%s>", alphaIndex(len(r.addrMap)))
		r.addrMap[v] = t
		return t
	}
	if t, ok := r.hostMap[v]; ok {
		return t
	}
	t := fmt.Sprintf("<HOST_%s>", alphaIndex(len(r.hostMap)))
	r.hostMap[v] = t
	return t
}

func alphaIndex(i int) string {
	i++
	letters := ""
	for i > 0 {
		i--
		letters = string(rune('A'+(i%26))) + letters
		i /= 26
	}
	return letters
}

func _stableKeys(m map[string]string) []string {
	k := make([]string, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}
