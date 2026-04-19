package transfer

import (
	"context"
	"fmt"
	"net"
	"os"
)

type SendResult struct {
	BytesSent  int64
	RemoteAddr string
}

func SendFile(ctx context.Context, filePath, address string) (SendResult, error) {
	if err := validateIPv6Address(address); err != nil {
		return SendResult{}, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return SendResult{}, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp6", address)
	if err != nil {
		return SendResult{}, fmt.Errorf("dial tcp6 %s: %w", address, err)
	}
	defer conn.Close()

	written, err := file.WriteTo(conn)
	if err != nil {
		return SendResult{}, fmt.Errorf("stream file to %s: %w", address, err)
	}

	return SendResult{
		BytesSent:  written,
		RemoteAddr: conn.RemoteAddr().String(),
	}, nil
}

func validateIPv6Address(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", address, err)
	}

	ip := net.ParseIP(host)
	if ip == nil || ip.To4() != nil {
		return fmt.Errorf("address %q is not an IPv6 endpoint", address)
	}

	return nil
}
