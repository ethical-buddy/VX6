package main

import (
	"context"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// PerformanceMetrics holds the results of a performance test suite.
type PerformanceMetrics struct {
	// Timestamp when tests were run
	Timestamp time.Time `json:"timestamp"`

	// Platform information
	Platform struct {
		OS       string `json:"os"`
		Arch     string `json:"arch"`
		Runtime  string `json:"go_version"`
		CPUCount int    `json:"cpu_count"`
	} `json:"platform"`

	// System metrics at test time
	System struct {
		MemHeapAlloc uint64        `json:"mem_heap_alloc_bytes"`
		MemHeapSys   uint64        `json:"mem_heap_sys_bytes"`
		Goroutines   int           `json:"goroutines_count"`
		Allocs       uint64        `json:"allocs_count"`
	} `json:"system"`

	// Transport performance
	Transport struct {
		TCPLatency         time.Duration `json:"tcp_latency_ns"`
		TCPThroughput      float64       `json:"tcp_throughput_mbps"`
		ConnectionAttempts int           `json:"connection_attempts"`
		SuccessfulConns    int           `json:"successful_conns"`
		FailedConns        int           `json:"failed_conns"`
	} `json:"transport"`

	// VX6-specific metrics
	VX6 struct {
		NodeStartupTime    time.Duration `json:"node_startup_ns"`
		ServiceAddTime     time.Duration `json:"service_add_ns"`
		DiscoveryTime      time.Duration `json:"discovery_time_ns"`
		RelayPathSetupTime time.Duration `json:"relay_setup_ns"`
	} `json:"vx6"`

	// Tor-specific metrics for onion relay benchmarking
	Tor struct {
		Enabled            bool          `json:"enabled"`
		Proxy              string        `json:"proxy"`
		Target             string        `json:"target"`
		StressRequests     int           `json:"stress_requests"`
		StressSuccess      int           `json:"stress_success"`
		StressFailure      int           `json:"stress_failure"`
		StressLatency      time.Duration `json:"stress_latency_ns"`
		UploadBytes        uint64        `json:"upload_bytes"`
		UploadThroughput   float64       `json:"upload_throughput_mbps"`
		DownloadBytes      uint64        `json:"download_bytes"`
		DownloadThroughput float64       `json:"download_throughput_mbps"`
		UploadStatus       string        `json:"upload_status"`
		DownloadStatus     string        `json:"download_status"`
	} `json:"tor"`

	// Test summary
	Summary struct {
		TotalDuration time.Duration `json:"total_duration_ns"`
		Status        string        `json:"status"`
		Errors        []string      `json:"errors"`
	} `json:"summary"`
}

// PerformanceTest runs a suite of performance benchmarks.
type PerformanceTest struct {
	verbose      bool
	outputFormat string // "json", "text", "csv"
	outputFile   string
	address      string // Target address for network tests
	torMode      bool
	torProxy     string
	torTarget    string
	torStress    int
	torUpload    int
	torDownload  int
	torScheme    string
	torStressPath   string
	torUploadPath   string
	torDownloadPath string
	tempTorAutomaticTest bool
	torPrivateToken      string
	duration     time.Duration
}

// NewPerformanceTest creates a new performance test.
func NewPerformanceTest() *PerformanceTest {
	return &PerformanceTest{
		verbose:      false,
		outputFormat: "json",
		address:      "[::1]:8080",
		torProxy:     "127.0.0.1:9050",
		torScheme:    "http",
		torStress:    10,
		torUpload:    1 << 20,
		torDownload:  1 << 20,
		torStressPath:   "/",
		torUploadPath:   "/upload",
		torDownloadPath: "/download?bytes=%d",
		duration:     30 * time.Second,
	}
}

// Run executes the full performance test suite.
func (pt *PerformanceTest) Run(ctx context.Context) (*PerformanceMetrics, error) {
	metrics := &PerformanceMetrics{
		Timestamp: time.Now(),
	}

	start := time.Now()

	// Gather platform info
	metrics.Platform.OS = runtime.GOOS
	metrics.Platform.Arch = runtime.GOARCH
	metrics.Platform.Runtime = runtime.Version()
	metrics.Platform.CPUCount = runtime.NumCPU()

	// Run transport benchmarks
	if err := pt.benchmarkTransport(ctx, metrics); err != nil {
		metrics.Summary.Errors = append(metrics.Summary.Errors, fmt.Sprintf("transport benchmark failed: %v", err))
	}

	// Run Tor relay benchmarks when a Tor target is configured
	if pt.torMode || pt.torTarget != "" || pt.tempTorAutomaticTest {
		metrics.Tor.Enabled = true
		metrics.Tor.Proxy = pt.torProxy
		metrics.Tor.Target = pt.torTarget
		if err := pt.benchmarkTorRelay(ctx, metrics); err != nil {
			metrics.Summary.Errors = append(metrics.Summary.Errors, fmt.Sprintf("tor benchmark failed: %v", err))
		}
	}

	// Gather system metrics
	pt.gatherSystemMetrics(metrics)

	// Summary
	metrics.Summary.TotalDuration = time.Since(start)
	if len(metrics.Summary.Errors) == 0 {
		metrics.Summary.Status = "success"
	} else {
		metrics.Summary.Status = "completed_with_errors"
	}

	return metrics, nil
}

// benchmarkTransport measures TCP transport performance.
func (pt *PerformanceTest) benchmarkTransport(ctx context.Context, metrics *PerformanceMetrics) error {
	if pt.verbose {
		fmt.Fprintf(os.Stderr, "Benchmarking transport layer on %s...\n", pt.address)
	}

	connTest := &metrics.Transport
	connTest.ConnectionAttempts = 10
	connTest.SuccessfulConns = 0
	connTest.FailedConns = 0

	var totalLat time.Duration

	// Try to connect and measure latency
	for i := 0; i < connTest.ConnectionAttempts; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", pt.address, 2*time.Second)
		latency := time.Since(start)

		if err != nil {
			connTest.FailedConns++
			if pt.verbose {
				fmt.Fprintf(os.Stderr, "  Attempt %d: FAILED (%v)\n", i+1, err)
			}
			continue
		}
		defer conn.Close()

		connTest.SuccessfulConns++
		totalLat += latency
		if pt.verbose {
			fmt.Fprintf(os.Stderr, "  Attempt %d: %v\n", i+1, latency)
		}
	}

	if connTest.SuccessfulConns > 0 {
		connTest.TCPLatency = totalLat / time.Duration(connTest.SuccessfulConns)
		if pt.verbose {
			fmt.Fprintf(os.Stderr, "  Average Latency: %v\n", connTest.TCPLatency)
		}
	}

	// Simple throughput estimate (assuming success)
	if connTest.SuccessfulConns > 0 {
		// Rough estimate: successful connections per second * ~1KB per connection
		connPerSec := float64(connTest.SuccessfulConns*1000) / float64(totalLat.Milliseconds())
		connTest.TCPThroughput = (connPerSec * 1000) / 1024 // Convert to MB/s approx
	}

	return nil
}

// benchmarkTorRelay measures Tor relay performance over a SOCKS5 proxy.
func (pt *PerformanceTest) benchmarkTorRelay(ctx context.Context, metrics *PerformanceMetrics) error {
	if pt.verbose {
		mode := "Tor relay"
		if pt.tempTorAutomaticTest {
			mode = "temporary private Tor-like test server"
		}
		fmt.Fprintf(os.Stderr, "Benchmarking %s...\n", mode)
	}

	var (
		client  *http.Client
		baseURL string
		err     error
		cleanup func()
	)

	if pt.tempTorAutomaticTest {
		client, baseURL, cleanup, err = pt.newTemporaryPrivateTorClient(ctx)
		if err != nil {
			return err
		}
		metrics.Tor.Target = baseURL
		defer cleanup()
	} else {
		if pt.torTarget == "" {
			return fmt.Errorf("tor target is required")
		}
		client, baseURL, err = pt.newTorHTTPClient(pt.torProxy, pt.torScheme, pt.torTarget)
		if err != nil {
			return err
		}
	}

	metrics.Tor.StressRequests = pt.torStress
	metrics.Tor.UploadBytes = uint64(pt.torUpload)
	metrics.Tor.DownloadBytes = uint64(pt.torDownload)

	// Stress test: repeated requests over the same Tor relay
	var stressTotal time.Duration
	for i := 0; i < pt.torStress; i++ {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+pt.torStressPath, nil)
		if err != nil {
			metrics.Tor.StressFailure++
			continue
		}
		pt.applyTorAuth(req)

		resp, err := client.Do(req)
		if err != nil {
			metrics.Tor.StressFailure++
			if pt.verbose {
				fmt.Fprintf(os.Stderr, "  Tor stress attempt %d failed: %v\n", i+1, err)
			}
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		metrics.Tor.StressSuccess++
		stressTotal += time.Since(start)
	}
	if metrics.Tor.StressSuccess > 0 {
		metrics.Tor.StressLatency = stressTotal / time.Duration(metrics.Tor.StressSuccess)
	}

	// Upload test: POST a payload to the relay
	if err := pt.runTorUpload(ctx, client, baseURL, metrics); err != nil {
		metrics.Tor.UploadStatus = "failed"
		if pt.verbose {
			fmt.Fprintf(os.Stderr, "  Tor upload failed: %v\n", err)
		}
	} else {
		metrics.Tor.UploadStatus = "success"
	}

	// Download test: GET a stream from the relay
	if err := pt.runTorDownload(ctx, client, baseURL, metrics); err != nil {
		metrics.Tor.DownloadStatus = "failed"
		if pt.verbose {
			fmt.Fprintf(os.Stderr, "  Tor download failed: %v\n", err)
		}
	} else {
		metrics.Tor.DownloadStatus = "success"
	}

	if metrics.Tor.UploadStatus == "" {
		metrics.Tor.UploadStatus = "skipped"
	}
	if metrics.Tor.DownloadStatus == "" {
		metrics.Tor.DownloadStatus = "skipped"
	}

	return nil
}

// newTorHTTPClient creates an HTTP client that routes traffic through Tor SOCKS5.
func (pt *PerformanceTest) newTorHTTPClient(proxyAddr, scheme, target string) (*http.Client, string, error) {
	if proxyAddr == "" {
		proxyAddr = "127.0.0.1:9050"
	}
	if scheme == "" {
		scheme = "http"
	}

	baseURL := target
	if !strings.Contains(target, "://") {
		baseURL = scheme + "://" + target
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, "", err
	}
	if parsed.Host == "" {
		return nil, "", fmt.Errorf("tor target must include a host")
	}
	baseRoot := parsed.Scheme + "://" + parsed.Host

	dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialSOCKS5(ctx, proxyAddr, addr)
	}

	transport := &http.Transport{
		DialContext:           dialer,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   pt.duration,
	}

	return client, baseRoot, nil
}

// runTorUpload posts a payload to the Tor relay and measures throughput.
func (pt *PerformanceTest) runTorUpload(ctx context.Context, client *http.Client, baseURL string, metrics *PerformanceMetrics) error {
	payload := bytes.Repeat([]byte("VX6-TOR-UPLOAD-"), (pt.torUpload/16)+1)
	if len(payload) > pt.torUpload {
		payload = payload[:pt.torUpload]
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+pt.torUploadPath, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	pt.applyTorAuth(req)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	duration := time.Since(start)
	if duration > 0 {
		metrics.Tor.UploadThroughput = (float64(len(payload)) * 8) / duration.Seconds() / 1_000_000
	}
	return nil
}

// runTorDownload downloads a payload from the Tor relay and measures throughput.
func (pt *PerformanceTest) runTorDownload(ctx context.Context, client *http.Client, baseURL string, metrics *PerformanceMetrics) error {
	requestURL := baseURL + pt.torDownloadPath
	if strings.Contains(pt.torDownloadPath, "%d") {
		requestURL = fmt.Sprintf(baseURL+pt.torDownloadPath, pt.torDownload)
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	pt.applyTorAuth(req)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(pt.torDownload)))
	if err != nil {
		return err
	}
	metrics.Tor.DownloadBytes = uint64(len(data))

	duration := time.Since(start)
	if duration > 0 && len(data) > 0 {
		metrics.Tor.DownloadThroughput = (float64(len(data)) * 8) / duration.Seconds() / 1_000_000
	}
	return nil
}

// dialSOCKS5 connects to a target using a SOCKS5 proxy.
func dialSOCKS5(ctx context.Context, proxyAddr, targetAddr string) (net.Conn, error) {
	d := &net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}

	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	defer func() {
		_ = conn.SetDeadline(time.Time{})
	}()

	// SOCKS5 greeting: version 5, 1 auth method (no-auth)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		conn.Close()
		return nil, err
	}
	if len(resp) != 2 || resp[0] != 0x05 || resp[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 auth negotiation failed")
	}

	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		conn.Close()
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		conn.Close()
		return nil, err
	}

	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			req = append(req, 0x01)
			req = append(req, v4...)
		} else {
			v6 := ip.To16()
			req = append(req, 0x04)
			req = append(req, v6...)
		}
	} else {
		if len(host) > 255 {
			conn.Close()
			return nil, fmt.Errorf("tor host too long")
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, host...)
	}
	req = append(req, byte(port>>8), byte(port))

	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}

	// Read SOCKS5 reply
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		conn.Close()
		return nil, err
	}
	if hdr[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 connect failed: %d", hdr[1])
	}

	// Skip bind address in the reply
	switch hdr[3] {
	case 0x01:
		_, _ = io.CopyN(io.Discard, conn, 4+2)
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			conn.Close()
			return nil, err
		}
		_, _ = io.CopyN(io.Discard, conn, int64(lenBuf[0])+2)
	case 0x04:
		_, _ = io.CopyN(io.Discard, conn, 16+2)
	default:
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 returned unknown address type")
	}

	return conn, nil
}

// newTemporaryPrivateTorClient creates a private local test server and client.
func (pt *PerformanceTest) newTemporaryPrivateTorClient(ctx context.Context) (*http.Client, string, func(), error) {
	pt.torPrivateToken = generatePrivateToken()

	mux := http.NewServeMux()
	mux.HandleFunc("/", pt.privateTorHandler)
	mux.HandleFunc("/upload", pt.privateTorHandler)
	mux.HandleFunc("/download", pt.privateTorHandler)

	server := httptest.NewServer(mux)
	cleanup := func() { server.Close() }

	client := &http.Client{Timeout: pt.duration}
	return client, server.URL, cleanup, nil
}

// privateTorHandler emulates a private relay for local testing.
func (pt *PerformanceTest) privateTorHandler(w http.ResponseWriter, r *http.Request) {
	if pt.torPrivateToken != "" && r.Header.Get("X-VX6-Private-Token") != pt.torPrivateToken {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	switch r.URL.Path {
	case "/upload":
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("upload-ok"))
	case "/download":
		bytesCount := pt.torDownload
		if v := r.URL.Query().Get("bytes"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				bytesCount = n
			}
		}
		payload := bytes.Repeat([]byte("VX6-TOR-DOWNLOAD-"), (bytesCount/17)+1)
		if len(payload) > bytesCount {
			payload = payload[:bytesCount]
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	default:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// applyTorAuth attaches the temporary private token when private testing is enabled.
func (pt *PerformanceTest) applyTorAuth(req *http.Request) {
	if pt.tempTorAutomaticTest && pt.torPrivateToken != "" {
		req.Header.Set("X-VX6-Private-Token", pt.torPrivateToken)
	}
}

// generatePrivateToken creates a temporary token for private testing.
func generatePrivateToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("vx6-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// gatherSystemMetrics collects Go runtime statistics.
func (pt *PerformanceTest) gatherSystemMetrics(metrics *PerformanceMetrics) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metrics.System.MemHeapAlloc = m.HeapAlloc
	metrics.System.MemHeapSys = m.HeapSys
	metrics.System.Goroutines = runtime.NumGoroutine()
	metrics.System.Allocs = m.Mallocs
}

// Output writes the metrics in the requested format.
func (pt *PerformanceTest) Output(metrics *PerformanceMetrics) error {
	var output []byte
	var err error

	switch pt.outputFormat {
	case "json":
		output, err = json.MarshalIndent(metrics, "", "  ")
	case "text":
		output = pt.formatText(metrics)
	case "csv":
		output = pt.formatCSV(metrics)
	default:
		return fmt.Errorf("unknown output format: %s", pt.outputFormat)
	}

	if err != nil {
		return err
	}

	if pt.outputFile != "" {
		return os.WriteFile(pt.outputFile, output, 0644)
	}

	fmt.Println(string(output))
	return nil
}

// formatText formats metrics as human-readable text.
func (pt *PerformanceTest) formatText(m *PerformanceMetrics) []byte {
	text := fmt.Sprintf(`
VX6 Performance Test Results
============================
Timestamp: %s

PLATFORM
--------
OS:              %s
Architecture:    %s
Go Version:      %s
CPU Count:       %d

SYSTEM METRICS
--------------
Heap Allocated:  %d bytes
Heap System:     %d bytes
Goroutines:      %d
Total Allocs:    %d

TRANSPORT PERFORMANCE
---------------------
TCP Latency:        %v (avg)
TCP Throughput:     %.2f MB/s
Connection Attempts: %d
Successful:          %d
Failed:              %d

TOR RELAY PERFORMANCE
---------------------
Enabled:             %t
Proxy:               %s
Target:              %s
Stress Requests:     %d
Stress Success:      %d
Stress Failure:      %d
Stress Latency:      %v
Upload Bytes:        %d
Upload Throughput:   %.2f Mbps
Download Bytes:      %d
Download Throughput: %.2f Mbps
Upload Status:       %s
Download Status:     %s

SUMMARY
-------
Total Test Duration: %v
Status:              %s
Errors:              %d

`, m.Timestamp.Format(time.RFC3339), m.Platform.OS, m.Platform.Arch, m.Platform.Runtime, m.Platform.CPUCount,
		m.System.MemHeapAlloc, m.System.MemHeapSys, m.System.Goroutines, m.System.Allocs,
		m.Transport.TCPLatency, m.Transport.TCPThroughput, m.Transport.ConnectionAttempts,
		m.Transport.SuccessfulConns, m.Transport.FailedConns,
		m.Tor.Enabled, m.Tor.Proxy, m.Tor.Target, m.Tor.StressRequests, m.Tor.StressSuccess, m.Tor.StressFailure,
		m.Tor.StressLatency, m.Tor.UploadBytes, m.Tor.UploadThroughput, m.Tor.DownloadBytes, m.Tor.DownloadThroughput,
		m.Tor.UploadStatus, m.Tor.DownloadStatus,
		m.Summary.TotalDuration, m.Summary.Status, len(m.Summary.Errors))
	return []byte(text)
}

// formatCSV formats metrics as comma-separated values.
func (pt *PerformanceTest) formatCSV(m *PerformanceMetrics) []byte {
	csv := fmt.Sprintf("timestamp,os,arch,cpu_count,tcp_latency_ns,tcp_throughput_mbps,conn_attempts,successful,failed,tor_enabled,tor_proxy,tor_target,tor_stress_requests,tor_stress_success,tor_stress_failure,tor_stress_latency_ns,tor_upload_bytes,tor_upload_throughput_mbps,tor_download_bytes,tor_download_throughput_mbps,total_duration_ns,status\n")
	csv += fmt.Sprintf("%s,%s,%s,%d,%d,%.2f,%d,%d,%d,%t,%q,%q,%d,%d,%d,%d,%d,%.2f,%d,%.2f,%d,%s\n",
		m.Timestamp.Format(time.RFC3339), m.Platform.OS, m.Platform.Arch, m.Platform.CPUCount,
		m.Transport.TCPLatency.Nanoseconds(), m.Transport.TCPThroughput, m.Transport.ConnectionAttempts,
		m.Transport.SuccessfulConns, m.Transport.FailedConns,
		m.Tor.Enabled, m.Tor.Proxy, m.Tor.Target, m.Tor.StressRequests, m.Tor.StressSuccess, m.Tor.StressFailure,
		m.Tor.StressLatency.Nanoseconds(), m.Tor.UploadBytes, m.Tor.UploadThroughput, m.Tor.DownloadBytes, m.Tor.DownloadThroughput,
		m.Summary.TotalDuration.Nanoseconds(), m.Summary.Status)
	return []byte(csv)
}

func main() {
	test := NewPerformanceTest()

	flag.BoolVar(&test.verbose, "v", false, "Verbose output")
	flag.StringVar(&test.outputFormat, "format", "json", "Output format: json, text, csv")
	flag.StringVar(&test.outputFile, "output", "", "Output file (default: stdout)")
	flag.StringVar(&test.address, "target", "[::1]:8080", "Target address for network tests")
	flag.DurationVar(&test.duration, "duration", 30*time.Second, "Test duration")
	flag.BoolVar(&test.torMode, "tor", false, "Enable Tor relay benchmarks")
	flag.StringVar(&test.torProxy, "tor-proxy", "127.0.0.1:9050", "Tor SOCKS5 proxy address")
	flag.StringVar(&test.torTarget, "tor-target", "", "Tor relay target host or URL (for example, example.onion or http://example.onion)")
	flag.IntVar(&test.torStress, "tor-stress", 10, "Number of Tor stress requests")
	flag.IntVar(&test.torUpload, "tor-upload-bytes", 1<<20, "Bytes to upload in Tor upload test")
	flag.IntVar(&test.torDownload, "tor-download-bytes", 1<<20, "Bytes to download in Tor download test")
	flag.StringVar(&test.torStressPath, "tor-stress-path", "/", "Path used for Tor stress requests")
	flag.StringVar(&test.torUploadPath, "tor-upload-path", "/upload", "Path used for Tor upload requests")
	flag.StringVar(&test.torDownloadPath, "tor-download-path", "/download?bytes=%d", "Path used for Tor download requests")
	flag.StringVar(&test.torScheme, "tor-scheme", "http", "Tor relay URL scheme")
	flag.BoolVar(&test.tempTorAutomaticTest, "temp-tor-automatic-test", false, "Run a temporary private Tor-like test server automatically")

	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), test.duration)
	defer cancel()

	metrics, err := test.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := test.Output(metrics); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}
}
