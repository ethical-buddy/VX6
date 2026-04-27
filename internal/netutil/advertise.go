package netutil

import (
	"fmt"
	"net"
)

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
	port, err := advertisePort(configured, listenAddr)
	if err != nil {
		return "", false, err
	}

	if configured != "" {
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

func PickGlobalIPv6(addrs []net.Addr) (net.IP, error) {
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip == nil || ip.To4() != nil {
			continue
		}
		if !ip.IsGlobalUnicast() {
			continue
		}
		if ip.IsLinkLocalUnicast() || isULA(ip) {
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
	if ip == nil {
		return false
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ipNet.IP == nil || ipNet.IP.To4() != nil {
			continue
		}
		if !ipNet.IP.IsGlobalUnicast() || ipNet.IP.IsLinkLocalUnicast() || isULA(ipNet.IP) {
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
