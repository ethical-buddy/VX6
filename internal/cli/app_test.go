package cli

import (
	"errors"
	"strings"
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

func TestFriendlyRelayPathErrorForProxy(t *testing.T) {
	t.Parallel()

	err := friendlyRelayPathError(errors.New("not enough peers in registry to build a 5-hop chain (need 5, have 2)"), "proxy mode")
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !strings.Contains(err.Error(), "proxy mode requires more reachable VX6 nodes") {
		t.Fatalf("unexpected error %q", err.Error())
	}
}

func TestFriendlyRelayPathErrorForHiddenService(t *testing.T) {
	t.Parallel()

	err := friendlyRelayPathError(errors.New("no rendezvous candidates available"), "hidden-service mode")
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !strings.Contains(err.Error(), "hidden-service mode requires more reachable VX6 nodes") {
		t.Fatalf("unexpected error %q", err.Error())
	}
}
