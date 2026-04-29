package onion

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
)

type CircuitPlan struct {
	CircuitID  string
	Relays     []record.EndpointRecord
	TargetAddr string
}

type TraceEvent struct {
	CircuitID  string
	RelayNames []string
	RelayAddrs []string
	TargetAddr string
}

var (
	traceMu   sync.RWMutex
	traceHook func(TraceEvent)
)

func SetTraceHook(fn func(TraceEvent)) func() {
	traceMu.Lock()
	prev := traceHook
	traceHook = fn
	traceMu.Unlock()
	return func() {
		traceMu.Lock()
		traceHook = prev
		traceMu.Unlock()
	}
}

func (p CircuitPlan) RelayAddrs() []string {
	out := make([]string, 0, len(p.Relays))
	for _, relay := range p.Relays {
		out = append(out, relay.Address)
	}
	return out
}

// PlanAutomatedCircuit picks random peers and prepares a recursive tunnel plan.
func PlanAutomatedCircuit(finalTarget record.ServiceRecord, allPeers []record.EndpointRecord, hopCount int, excludeAddrs []string) (CircuitPlan, error) {
	if hopCount <= 0 {
		return CircuitPlan{}, fmt.Errorf("hop count must be greater than zero")
	}

	targetAddr := finalTarget.Address
	if finalTarget.IsHidden && len(finalTarget.IntroPoints) > 0 {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		targetAddr = finalTarget.IntroPoints[rng.Intn(len(finalTarget.IntroPoints))]
	}
	if targetAddr == "" {
		return CircuitPlan{}, fmt.Errorf("service does not expose a reachable address for proxy mode")
	}

	seen := map[string]struct{}{targetAddr: {}}
	for _, addr := range excludeAddrs {
		if addr != "" {
			seen[addr] = struct{}{}
		}
	}

	filtered := make([]record.EndpointRecord, 0, len(allPeers))
	for _, peer := range allPeers {
		if peer.Address == "" {
			continue
		}
		if _, ok := seen[peer.Address]; ok {
			continue
		}
		seen[peer.Address] = struct{}{}
		filtered = append(filtered, peer)
	}

	if len(filtered) < hopCount {
		return CircuitPlan{}, fmt.Errorf("not enough peers in registry to build a %d-hop chain (need %d, have %d)", hopCount, hopCount, len(filtered))
	}

	// Pick a unique relay set for the requested hop count.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(filtered), func(i, j int) { filtered[i], filtered[j] = filtered[j], filtered[i] })
	relays := append([]record.EndpointRecord(nil), filtered[:hopCount]...)

	return CircuitPlan{
		CircuitID:  fmt.Sprintf("auto_%d", rng.Intn(1000000)),
		Relays:     relays,
		TargetAddr: targetAddr,
	}, nil
}

// BuildAutomatedCircuit picks random peers and builds a 5-hop recursive tunnel.
func BuildAutomatedCircuit(ctx context.Context, finalTarget record.ServiceRecord, allPeers []record.EndpointRecord) (net.Conn, error) {
	plan, err := PlanAutomatedCircuit(finalTarget, allPeers, 5, nil)
	if err != nil {
		return nil, err
	}
	return DialPlannedCircuit(ctx, plan)
}

func BuildAutomatedCircuitWithExclude(ctx context.Context, finalTarget record.ServiceRecord, allPeers []record.EndpointRecord, excludeAddrs []string) (net.Conn, error) {
	plan, err := PlanAutomatedCircuit(finalTarget, allPeers, 5, excludeAddrs)
	if err != nil {
		return nil, err
	}
	return DialPlannedCircuit(ctx, plan)
}

func DialPlannedCircuit(ctx context.Context, plan CircuitPlan) (net.Conn, error) {
	if len(plan.Relays) == 0 {
		return nil, fmt.Errorf("circuit plan has no relays")
	}

	fmt.Printf("[CIRCUIT] Building automated circuit via: ")
	for _, r := range plan.Relays {
		fmt.Printf("%s -> ", r.NodeName)
	}
	fmt.Printf("TARGET\n")

	notifyTrace(plan)

	// 1. Connect to first hop.
	var dialer net.Dialer
	currConn, err := dialer.DialContext(ctx, "tcp6", plan.Relays[0].Address)
	if err != nil {
		return nil, fmt.Errorf("first hop connection failed: %w", err)
	}

	// 2. Recursively extend through the remaining relays.
	for i := 1; i < len(plan.Relays); i++ {
		req := proto.ExtendRequest{
			NextHop:   plan.Relays[i].Address,
			CircuitID: plan.CircuitID,
		}
		if err := sendExtend(currConn, req); err != nil {
			currConn.Close()
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 3. Final step: connect to the target IP or rendezvous node.
	if plan.TargetAddr != "" {
		req := proto.ExtendRequest{
			NextHop:   plan.TargetAddr,
			CircuitID: plan.CircuitID,
		}
		if err := sendExtend(currConn, req); err != nil {
			currConn.Close()
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}

	return currConn, nil
}

func notifyTrace(plan CircuitPlan) {
	traceMu.RLock()
	hook := traceHook
	traceMu.RUnlock()
	if hook == nil {
		return
	}

	event := TraceEvent{
		CircuitID:  plan.CircuitID,
		RelayNames: make([]string, 0, len(plan.Relays)),
		RelayAddrs: make([]string, 0, len(plan.Relays)),
		TargetAddr: plan.TargetAddr,
	}
	for _, relay := range plan.Relays {
		event.RelayNames = append(event.RelayNames, relay.NodeName)
		event.RelayAddrs = append(event.RelayAddrs, relay.Address)
	}
	hook(event)
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
