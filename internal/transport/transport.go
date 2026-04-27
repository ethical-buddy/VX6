package transport

import (
	"context"
	"net"
	"strings"
)

const (
	ModeAuto = "auto"
	ModeTCP  = "tcp"
	ModeQUIC = "quic"
)

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
		// The current standard-library VX6 build has no full QUIC transport yet.
		// We keep the config surface now so the neighbor-session transport can swap
		// in later without changing the higher layers.
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
