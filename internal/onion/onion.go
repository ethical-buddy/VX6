package onion

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/vx6/vx6/internal/proto"
)

// Forward handles an incoming onion packet and sends it to the next hop.
func Forward(ctx context.Context, header proto.OnionHeader) error {
	fmt.Printf("[ONION] Received Hop %d/5. Next: %s\n", header.HopCount+1, header.Hops[header.HopCount])

	if header.HopCount >= 4 {
		fmt.Printf("[ONION] Reached Exit Node. Delivering to: %s\n", header.FinalDst)
		return exit(ctx, header)
	}

	// Move to next hop
	header.HopCount++
	nextAddr := header.Hops[header.HopCount]

	// Use TCP6 to forward to the next relay
	conn, err := net.Dial("tcp6", nextAddr)
	if err != nil {
		return fmt.Errorf("onion relay to %s failed: %w", nextAddr, err)
	}
	defer conn.Close()

	if err := proto.WriteHeader(conn, proto.KindOnion); err != nil {
		return err
	}

	payload, err := json.Marshal(header)
	if err != nil {
		return err
	}

	return proto.WriteLengthPrefixed(conn, payload)
}

func exit(ctx context.Context, header proto.OnionHeader) error {
	// Final delivery to the destination (e.g., a local port or public IP)
	conn, err := net.Dial("tcp", header.FinalDst)
	if err != nil {
		return fmt.Errorf("onion exit to %s failed: %w", header.FinalDst, err)
	}
	defer conn.Close()

	_, err = conn.Write(header.Payload)
	return err
}
