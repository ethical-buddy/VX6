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

func TestRefreshAdvertiseAddressKeepsConfiguredLocalIPv6(t *testing.T) {
	t.Parallel()

	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("2401:db8::10")},
	}

	got, changed, err := RefreshAdvertiseAddressWithAddrs("[2401:db8::10]:4242", "[::]:4242", addrs)
	if err != nil {
		t.Fatalf("refresh advertise address: %v", err)
	}
	if changed {
		t.Fatal("expected configured advertise address to stay unchanged")
	}
	if got != "[2401:db8::10]:4242" {
		t.Fatalf("unexpected advertise address %q", got)
	}
}

func TestRefreshAdvertiseAddressReplacesStaleIPv6(t *testing.T) {
	t.Parallel()

	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("2401:db8::20")},
	}

	got, changed, err := RefreshAdvertiseAddressWithAddrs("[2401:db8::10]:4242", "[::]:4242", addrs)
	if err != nil {
		t.Fatalf("refresh advertise address: %v", err)
	}
	if !changed {
		t.Fatal("expected stale advertise address to be replaced")
	}
	if got != "[2401:db8::20]:4242" {
		t.Fatalf("unexpected advertise address %q", got)
	}
}

func TestRefreshAdvertiseAddressKeepsExplicitLoopback(t *testing.T) {
	t.Parallel()

	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("2401:db8::20")},
	}

	got, changed, err := RefreshAdvertiseAddressWithAddrs("[::1]:4242", "[::]:4242", addrs)
	if err != nil {
		t.Fatalf("refresh advertise address: %v", err)
	}
	if changed {
		t.Fatal("expected explicit loopback advertise address to stay unchanged")
	}
	if got != "[::1]:4242" {
		t.Fatalf("unexpected advertise address %q", got)
	}
}

func TestRefreshAdvertiseAddressUsesRouteSelectedIPv6ForAutoMode(t *testing.T) {
	originalProbe := routeIPv6Probe
	routeIPv6Probe = func(targets []string) (net.IP, bool) {
		if len(targets) != 1 || targets[0] != "[2401:db8::1]:4242" {
			t.Fatalf("unexpected route targets %#v", targets)
		}
		return net.ParseIP("2401:db8::30"), true
	}
	t.Cleanup(func() {
		routeIPv6Probe = originalProbe
	})

	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("2401:db8::20")},
	}

	got, changed, err := RefreshAdvertiseAddressWithAddrsAndTargets("[2401:db8::20]:4242", "[::]:4242", addrs, []string{"[2401:db8::1]:4242"}, false)
	if err != nil {
		t.Fatalf("refresh advertise address: %v", err)
	}
	if !changed {
		t.Fatal("expected route-aware auto detection to update the advertise address")
	}
	if got != "[2401:db8::30]:4242" {
		t.Fatalf("unexpected advertise address %q", got)
	}
}

func TestRefreshAdvertiseAddressDoesNotAutoPublishLoopbackListener(t *testing.T) {
	originalProbe := routeIPv6Probe
	routeIPv6Probe = func(targets []string) (net.IP, bool) {
		t.Fatal("route probe should not run for loopback listeners")
		return nil, false
	}
	t.Cleanup(func() {
		routeIPv6Probe = originalProbe
	})

	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("2401:db8::20")},
	}

	got, changed, err := RefreshAdvertiseAddressWithAddrsAndTargets("", "[::1]:4242", addrs, []string{"[2401:db8::1]:4242"}, false)
	if err == nil {
		t.Fatal("expected loopback listener to have no auto advertise address")
	}
	if changed {
		t.Fatal("did not expect advertise address change for loopback listener")
	}
	if got != "" {
		t.Fatalf("expected empty advertise address, got %q", got)
	}
}
