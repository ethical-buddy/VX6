package netutil

import (
	"net"
	"testing"
)

func TestPickGlobalIPv6(t *testing.T) {
	t.Parallel()

	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("fe80::1")},
		&net.IPNet{IP: net.ParseIP("fc00::1")},
		&net.IPNet{IP: net.ParseIP("2401:db8::10")},
	}

	ip, err := PickGlobalIPv6(addrs)
	if err != nil {
		t.Fatalf("pick global ipv6: %v", err)
	}
	if got := ip.String(); got != "2401:db8::10" {
		t.Fatalf("unexpected ip %q", got)
	}
}
