package netutil

import (
	"fmt"
	"net"
	"strings"
)

var routeIPv6Probe = pickRouteIPv6

func DetectAdvertiseAddress(port string) (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("list interface addresses: %w", err)
	}

	ip, err := PickGlobalIPv6(addrs)
	if err != nil {
		return "", err
	}

	return net.JoinHostPort(ip.String(), port), nil
}

func RefreshAdvertiseAddress(configured, listenAddr string) (string, bool, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", false, fmt.Errorf("list interface addresses: %w", err)
	}
	return RefreshAdvertiseAddressWithAddrs(configured, listenAddr, addrs)
}

func RefreshAdvertiseAddressWithAddrs(configured, listenAddr string, addrs []net.Addr) (string, bool, error) {
	return RefreshAdvertiseAddressWithAddrsAndTargets(configured, listenAddr, addrs, nil, true)
}

func RefreshAdvertiseAddressWithAddrsAndTargets(configured, listenAddr string, addrs []net.Addr, targets []string, explicit bool) (string, bool, error) {
	port, err := advertisePort(configured, listenAddr)
	if err != nil {
		return "", false, err
	}

	if !explicit {
		if ip, ok := publishableListenIPv6(listenAddr); ok {
			refreshed := net.JoinHostPort(ip.String(), port)
			return refreshed, refreshed != configured, nil
		}
		if isSpecificUnpublishableListenHost(listenAddr) {
			return "", false, fmt.Errorf("listen address is not publishable")
		}
		if ip, ok := routeIPv6Probe(targets); ok {
			refreshed := net.JoinHostPort(ip.String(), port)
			return refreshed, refreshed != configured, nil
		}
		if configured != "" {
			host, _, err := net.SplitHostPort(configured)
			if err == nil && shouldKeepConfiguredIPv6(addrs, host) {
				return configured, false, nil
			}
		}
	} else if configured != "" {
		host, _, err := net.SplitHostPort(configured)
		if err != nil {
			return "", false, fmt.Errorf("parse configured advertise address: %w", err)
		}
		if shouldKeepConfiguredIPv6(addrs, host) {
			return configured, false, nil
		}
	}

	ip, err := PickGlobalIPv6(addrs)
	if err != nil {
		return "", false, err
	}

	refreshed := net.JoinHostPort(ip.String(), port)
	return refreshed, refreshed != configured, nil
}

func publishableListenIPv6(listenAddr string) (net.IP, bool) {
	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return nil, false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if !isValidGlobalIPv6(ip) {
		return nil, false
	}
	return ip, true
}

func isSpecificUnpublishableListenHost(listenAddr string) bool {
	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	return !ip.IsUnspecified() && !isValidGlobalIPv6(ip)
}

func pickRouteIPv6(targets []string) (net.IP, bool) {
	for _, target := range targets {
		host, port, err := net.SplitHostPort(strings.TrimSpace(target))
		if err != nil {
			continue
		}
		ip := net.ParseIP(strings.Trim(host, "[]"))
		if !isValidGlobalIPv6(ip) {
			continue
		}
		portNum, err := net.LookupPort("tcp", port)
		if err != nil {
			continue
		}
		conn, err := net.DialUDP("udp6", nil, &net.UDPAddr{IP: ip, Port: portNum})
		if err != nil {
			continue
		}
		localAddr, _ := conn.LocalAddr().(*net.UDPAddr)
		_ = conn.Close()
		if localAddr == nil || !isValidGlobalIPv6(localAddr.IP) {
			continue
		}
		return localAddr.IP, true
	}
	return nil, false
}

func PickGlobalIPv6(addrs []net.Addr) (net.IP, error) {
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if !isValidGlobalIPv6(ip) {
			continue
		}
		return ip, nil
	}

	return nil, fmt.Errorf("no global IPv6 address detected")
}

func advertisePort(configured, listenAddr string) (string, error) {
	for _, addr := range []string{configured, listenAddr} {
		if addr == "" {
			continue
		}
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return "", fmt.Errorf("parse address %q: %w", addr, err)
		}
		if port != "" {
			return port, nil
		}
	}
	return "", fmt.Errorf("no port available for advertise address")
}

func hasGlobalIPv6(addrs []net.Addr, host string) bool {
	ip := net.ParseIP(host)
	if !isValidGlobalIPv6(ip) {
		return false
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if !isValidGlobalIPv6(ipNet.IP) {
			continue
		}
		if ipNet.IP.Equal(ip) {
			return true
		}
	}
	return false
}

func shouldKeepConfiguredIPv6(addrs []net.Addr, host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if !ip.IsGlobalUnicast() || ip.IsLinkLocalUnicast() || isULA(ip) || ip.IsLoopback() {
		return true
	}
	return hasGlobalIPv6(addrs, host)
}

func isULA(ip net.IP) bool {
	return len(ip) > 0 && (ip[0]&0xfe) == 0xfc
}

func isValidGlobalIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil {
		return false
	}
	if !ip.IsGlobalUnicast() {
		return false
	}
	return !ip.IsLinkLocalUnicast() && !isULA(ip)
}
