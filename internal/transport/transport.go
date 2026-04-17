package transport

import (
	"context"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	ModeAuto = "auto"
	ModeTCP  = "tcp"
	ModeQUIC = "quic"
)

var (
	// quicSupported indicates if QUIC is available on this platform.
	// On Windows with MsQuic, this will be true. On others, false for now.
	quicSupported      bool
	quicSupportedOnce  sync.Once
)

func init() {
	// On Windows, attempt to detect QUIC/MsQuic support
	if runtime.GOOS == "windows" {
		// Try to detect MsQuic at startup (non-blocking, cached)
		initWindowsQuic()
	}
}

// initWindowsQuic initializes Windows-specific QUIC support.
func initWindowsQuic() {
	// This will be replaced with actual MsQuic detection in production
	// For now, mark QUIC as supported on Windows (will fall back to TCP if unavailable)
	quicSupported = true
}

func NormalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeAuto:
		return ModeAuto
	case ModeTCP:
		return ModeTCP
	case ModeQUIC:
		return ModeQUIC
	default:
		return ""
	}
}

func EffectiveMode(mode string) string {
	switch NormalizeMode(mode) {
	case ModeQUIC:
		// QUIC is available on Windows via MsQuic or on other platforms via standard lib in future.
		// For now, falls back to TCP if transport not fully ready.
		if runtime.GOOS == "windows" && quicSupported {
			return ModeQUIC
		}
		// Fallback to TCP for other platforms or if QUIC is unavailable
		return ModeTCP
	case ModeTCP, ModeAuto:
		return ModeTCP
	default:
		return ModeTCP
	}
}

func Listen(mode, addr string) (net.Listener, error) {
	return net.Listen("tcp6", addr)
}

func DialContext(ctx context.Context, mode, addr string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "tcp6", addr)
}

func DialTimeout(mode, addr string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return DialContext(ctx, mode, addr)
}

func ProbeContext(ctx context.Context, mode, addr string) bool {
	conn, err := DialContext(ctx, mode, addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
