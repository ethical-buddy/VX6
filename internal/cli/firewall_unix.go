//go:build !windows

package cli

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// FirewallConfig holds configuration for firewall rule setup
type FirewallConfig struct {
	Port     int
	Protocol string
	RuleName string
}

// SetupFirewallException is a stub for Unix/Linux systems
// On Linux, firewall rules are typically managed by the system administrator
func SetupFirewallException(port int, protocol string) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port number: %d", port)
	}

	// On Linux/Unix, firewall setup is optional and typically handled by ufw, firewalld, or iptables
	// We just notify the user
	fmt.Printf("note: firewall rule setup is not automatic on this platform\n")
	fmt.Printf("note: to allow VX6 traffic on port %d, use your system firewall tool:\n", port)
	fmt.Printf("  ufw allow %d/tcp\n", port)
	fmt.Printf("  or configure firewalld/iptables manually\n")
	return nil
}

// ExtractPortFromAddress extracts port number from [addr]:port format
func ExtractPortFromAddress(addr string) (int, error) {
	// Handle IPv6 format like [::]:4242
	if strings.HasPrefix(addr, "[") {
		// Find the closing bracket
		closeBracketIdx := strings.Index(addr, "]")
		if closeBracketIdx == -1 {
			return 0, fmt.Errorf("invalid address format: %s", addr)
		}

		// Check if there's a port after the bracket
		if closeBracketIdx+1 < len(addr) && addr[closeBracketIdx+1] == ':' {
			portStr := addr[closeBracketIdx+2:]
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return 0, fmt.Errorf("invalid port in address %s: %w", addr, err)
			}
			return port, nil
		}
		return 0, fmt.Errorf("no port found in address: %s", addr)
	}

	// Handle regular IPv4 format with port
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("invalid address format: %s", addr)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port in address %s: %w", addr, err)
	}
	return port, nil
}

// RemoveFirewallException is a stub for Unix/Linux systems
func RemoveFirewallException(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port number: %d", port)
	}

	// On Linux/Unix, firewall rule removal is optional and typically handled by the system admin
	fmt.Printf("note: to remove VX6 firewall rules on port %d, use your system firewall tool:\n", port)
	fmt.Printf("  ufw delete allow %d/tcp\n", port)
	fmt.Printf("  or reconfigure firewalld/iptables manually\n")
	return nil
}
