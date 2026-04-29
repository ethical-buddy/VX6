package cli

import (
	"testing"

	"github.com/vx6/vx6/internal/config"
)

func TestStatusProbeAddrUsesAdvertiseForWildcardListen(t *testing.T) {
	t.Parallel()

	cfg := config.File{
		Node: config.NodeConfig{
			ListenAddr:    "[::]:4242",
			AdvertiseAddr: "[2001:db8::10]:4242",
		},
	}

	if got := statusProbeAddr(cfg); got != "[2001:db8::10]:4242" {
		t.Fatalf("unexpected probe address %q", got)
	}
}

func TestStatusProbeAddrFallsBackToLoopbackForWildcardListen(t *testing.T) {
	t.Parallel()

	cfg := config.File{
		Node: config.NodeConfig{
			ListenAddr: "[::]:4242",
		},
	}

	if got := statusProbeAddr(cfg); got != "[::1]:4242" {
		t.Fatalf("unexpected probe address %q", got)
	}
}

func TestStatusProbeAddrKeepsConcreteListenAddress(t *testing.T) {
	t.Parallel()

	cfg := config.File{
		Node: config.NodeConfig{
			ListenAddr:    "[2001:db8::20]:4242",
			AdvertiseAddr: "[2001:db8::10]:4242",
		},
	}

	if got := statusProbeAddr(cfg); got != "[2001:db8::20]:4242" {
		t.Fatalf("unexpected probe address %q", got)
	}
}
