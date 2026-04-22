package onion

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
)

// BuildAutomatedCircuit picks random peers and builds a 5-hop recursive tunnel.
func BuildAutomatedCircuit(ctx context.Context, finalTarget record.ServiceRecord, allPeers []record.EndpointRecord) (net.Conn, error) {
	if len(allPeers) < 5 {
		return nil, fmt.Errorf("not enough peers in registry to build a 5-hop chain (need 5, have %d)", len(allPeers))
	}

	// 1. Pick 5 random unique relays
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(allPeers), func(i, j int) { allPeers[i], allPeers[j] = allPeers[j], allPeers[i] })
	relays := allPeers[:5]

	fmt.Printf("[GHOST] Building automated circuit via: ")
	for _, r := range relays {
		fmt.Printf("%s -> ", r.NodeName)
	}
	fmt.Printf("TARGET\n")

	// 2. Connect to first hop
	currConn, err := net.Dial("tcp6", relays[0].Address)
	if err != nil {
		return nil, fmt.Errorf("first hop connection failed: %w", err)
	}

	circuitID := fmt.Sprintf("auto_%d", rand.Intn(1000000))

	// 3. Recursively extend to the rest
	for i := 1; i < 5; i++ {
		req := proto.ExtendRequest{
			NextHop:   relays[i].Address,
			CircuitID: circuitID,
		}
		if err := sendExtend(currConn, req); err != nil {
			currConn.Close()
			return nil, err
		}
	}

	// 4. Final step: Connect to the target IP (or IntroPoint)
	targetAddr := finalTarget.Address
	if finalTarget.IsHidden && len(finalTarget.IntroPoints) > 0 {
		// Pick one of the 3 intro points randomly
		targetAddr = finalTarget.IntroPoints[rand.Intn(len(finalTarget.IntroPoints))]
	}

	if targetAddr != "" {
		req := proto.ExtendRequest{
			NextHop:   targetAddr,
			CircuitID: circuitID,
		}
		_ = sendExtend(currConn, req)
	}

	return currConn, nil
}

func sendExtend(conn net.Conn, req proto.ExtendRequest) error {
	if err := proto.WriteHeader(conn, proto.KindExtend); err != nil {
		return err
	}
	payload, _ := json.Marshal(req)
	return proto.WriteLengthPrefixed(conn, payload)
}

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
