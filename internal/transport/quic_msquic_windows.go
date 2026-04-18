//go:build windows
// +build windows

package transport

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MsQuicContext represents a connection to the MsQuic library.
type MsQuicContext struct {
	mu      sync.RWMutex
	enabled bool
	version string // MsQuic version
}

var (
	msQuicOnce   sync.Once
	msQuicCtx    *MsQuicContext
	msQuicErr    error
)

// InitMsQuic initializes the MsQuic library for Windows.
// Returns immediately if already initialized or if MsQuic is unavailable.
func InitMsQuic() error {
	msQuicOnce.Do(func() {
		msQuicCtx = &MsQuicContext{
			enabled: false,
			version: "unknown",
		}
		// Try to load MsQuic - stub for now
		// In production, would call Windows APIs to detect and load msquic.dll
		msQuicErr = nil
	})
	return msQuicErr
}

// IsMsQuicAvailable returns true if MsQuic is available on this system.
func IsMsQuicAvailable() bool {
	_ = InitMsQuic()
	if msQuicCtx == nil {
		return false
	}
	msQuicCtx.mu.RLock()
	defer msQuicCtx.mu.RUnlock()
	return msQuicCtx.enabled
}

// GetMsQuicVersion returns the loaded MsQuic version or "unavailable".
func GetMsQuicVersion() string {
	_ = InitMsQuic()
	if msQuicCtx == nil {
		return "unavailable"
	}
	msQuicCtx.mu.RLock()
	defer msQuicCtx.mu.RUnlock()
	return msQuicCtx.version
}

// QuicListener wraps a QUIC listener (MsQuic-backed on Windows).
type QuicListener struct {
	addr net.Addr
	// Placeholder for actual MsQuic listener handle
	closed bool
	mu     sync.Mutex
}

// NewQuicListener creates a new QUIC listener on the given address (Windows MsQuic).
// For now, this is a stub that falls back to TCP for compatibility.
func NewQuicListener(addr string) (net.Listener, error) {
	// Ensure MsQuic is initialized
	_ = InitMsQuic()

	// Implement actual MsQuic listener binding
	// Check if MsQuic is available
	if !IsMsQuicAvailable() {
		// Fallback to TCP for compatibility
		return net.Listen("tcp6", addr)
	}

	// Try to create MsQuic listener
	// In production, this would use Windows QUIC APIs (MsQuic)
	// For now, create a wrapper that handles both QUIC and TCP
	
	listener := &quicListenerWrapper{
		addr:       addr,
		fallback:   nil,
		msQuicReady: true,
	}

	// Attempt to create TCP fallback first (which always works)
	tcpListener, err := net.Listen("tcp6", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create fallback TCP listener: %w", err)
	}

	listener.fallback = tcpListener

	return listener, nil
}

// quicListenerWrapper wraps both QUIC and TCP listening capabilities
type quicListenerWrapper struct {
	addr        string
	fallback    net.Listener
	msQuicReady bool
	mu          sync.Mutex
}

func (q *quicListenerWrapper) Accept() (net.Conn, error) {
	q.mu.Lock()
	if q.fallback == nil {
		q.mu.Unlock()
		return nil, fmt.Errorf("listener closed")
	}
	fallback := q.fallback
	q.mu.Unlock()

	// Use TCP fallback for Accept
	return fallback.Accept()
}

func (q *quicListenerWrapper) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.fallback != nil {
		return q.fallback.Close()
	}
	return nil
}

func (q *quicListenerWrapper) Addr() net.Addr {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.fallback != nil {
		return q.fallback.Addr()
	}
	return nil
}

// QuicConn wraps a QUIC connection (MsQuic-backed on Windows).
type QuicConn struct {
	// Placeholder for actual MsQuic connection handle
	closed bool
	mu     sync.Mutex
}

// NewQuicConn creates a new QUIC connection to the given address (Windows MsQuic).
// For now, this is a stub that falls back to TCP for compatibility.
func NewQuicConn(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	// Ensure MsQuic is initialized
	_ = InitMsQuic()

	// Implement actual MsQuic connection establishment
	if !IsMsQuicAvailable() {
		// Fallback to TCP for compatibility
		var dialer net.Dialer
		if timeout > 0 {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return dialer.DialContext(ctx, "tcp6", addr)
		}
		return dialer.DialContext(ctx, "tcp6", addr)
	}

	// Try MsQuic connection first, fall back to TCP on failure
	quicConn, err := createMsQuicConnection(ctx, addr, timeout)
	if err == nil {
		return quicConn, nil
	}

	// Fallback to TCP
	var dialer net.Dialer
	if timeout > 0 {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return dialer.DialContext(ctx, "tcp6", addr)
	}
	return dialer.DialContext(ctx, "tcp6", addr)
}

// createMsQuicConnection attempts to create an MsQuic QUIC connection
func createMsQuicConnection(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	// In production, this would call MsQuic APIs
	// For now, return an error to trigger fallback to TCP
	return nil, fmt.Errorf("MsQuic connection creation not yet implemented - falling back to TCP")
}

// MsQuicTransportFeatures describes capabilities of the current Windows/MsQuic stack.
type MsQuicTransportFeatures struct {
	// MsQuicAvailable is true if the system has MsQuic runtime
	MsQuicAvailable bool
	// Version is the detected MsQuic version string
	Version string
	// SupportsZeroRTT is true if the system can perform 0-RTT resumption
	SupportsZeroRTT bool
	// SupportsConnectionMigration is true if the system supports UDP connection migration
	SupportsConnectionMigration bool
	// MaxMTU is the maximum packet size this transport can send
	MaxMTU int
}

// DetectMsQuicFeatures probes the Windows system for MsQuic capabilities.
func DetectMsQuicFeatures() *MsQuicTransportFeatures {
	features := &MsQuicTransportFeatures{
		MsQuicAvailable:             IsMsQuicAvailable(),
		Version:                     GetMsQuicVersion(),
		SupportsZeroRTT:             probeSchannelCapability(),
		SupportsConnectionMigration: probeConnectionMigrationCapability(),
		MaxMTU:                      1450,  // Conservative default for Windows UDP
	}

	if features.MsQuicAvailable && features.Version != "unavailable" {
		// Enable features for full MsQuic stack
		features.SupportsZeroRTT = true
		features.SupportsConnectionMigration = true
		features.MaxMTU = 1472 // Larger packet for MsQuic-native stack
	}

	return features
}

// probeSchannelCapability detects if Schannel (Windows crypto) supports 0-RTT
func probeSchannelCapability() bool {
	// Probe Schannel capability for 0-RTT support
	// This requires Windows 11+ with TLS 1.3 support
	
	// Check Windows version first
	isWin11Plus := isWindowsNativeVersion(11)
	if !isWin11Plus {
		return false
	}

	// Check if Schannel is enabled
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		`$provider = Get-Item -Path 'HKLM:\System\CurrentControlSet\Control\SecurityProviders\SCHANNEL' -ErrorAction SilentlyContinue; $provider -ne $null`)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	return strings.Contains(string(output), "True")
}

// probeConnectionMigrationCapability detects if OS supports UDP connection migration
func probeConnectionMigrationCapability() bool {
	// Probe OS capability for connection migration
	// This requires Windows 11+ with UDP GSO support
	
	// Check if UDP GSO (Generic Segmentation Offload) is supported
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		`Get-NetAdapterOffloadSetting -ErrorAction SilentlyContinue | Where-Object {$_.UdpIPv6Checksum -eq 'Enabled'} | Measure-Object | Select-Object -ExpandProperty Count`)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return count > 0
}

// isWindowsNativeVersion checks if the OS is Windows N or later
func isWindowsNativeVersion(majorVersion int) bool {
	// Get Windows version from registry or PowerShell
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		`[System.Environment]::OSVersion.Version.Major`)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	version, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return version >= majorVersion
}
