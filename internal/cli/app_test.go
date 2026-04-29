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

func TestExtractLeadingConnectService(t *testing.T) {
	t.Parallel()

	service, rest := extractLeadingConnectService([]string{"bob.ssh", "--listen", "127.0.0.1:3333"})
	if service != "bob.ssh" {
		t.Fatalf("unexpected service %q", service)
	}
	if len(rest) != 2 || rest[0] != "--listen" || rest[1] != "127.0.0.1:3333" {
		t.Fatalf("unexpected remaining args: %#v", rest)
	}
}

func TestExtractLeadingConnectServiceKeepsFlagFirstForm(t *testing.T) {
	t.Parallel()

	service, rest := extractLeadingConnectService([]string{"--listen", "127.0.0.1:3333", "bob.ssh"})
	if service != "" {
		t.Fatalf("unexpected service %q", service)
	}
	if len(rest) != 3 || rest[0] != "--listen" || rest[2] != "bob.ssh" {
		t.Fatalf("unexpected remaining args: %#v", rest)
	}
}
