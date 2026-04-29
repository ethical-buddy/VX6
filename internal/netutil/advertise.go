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

func isULA(ip net.IP) bool {
	return len(ip) > 0 && (ip[0]&0xfe) == 0xfc
}
