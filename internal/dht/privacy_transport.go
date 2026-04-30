package dht

import (
	"context"
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/vx6/vx6/internal/onion"
	"github.com/vx6/vx6/internal/record"
)

const hiddenDescriptorRelayHopCount = 3

type HiddenDescriptorPrivacyConfig struct {
	TransportMode   string
	RelayHopCount   int
	RelayCandidates func() []record.EndpointRecord
	ExcludeAddrs    func() []string
}

func (s *Server) SetHiddenDescriptorPrivacy(cfg HiddenDescriptorPrivacyConfig) {
	if cfg.RelayHopCount <= 0 {
		cfg.RelayHopCount = hiddenDescriptorRelayHopCount
	}
	s.mu.Lock()
	s.hidden = cfg
	s.mu.Unlock()
}

func (s *Server) dialDHTConn(ctx context.Context, addr, key, action string) (net.Conn, error) {
	if conn, handled, err := s.dialHiddenDescriptorConn(ctx, addr, key, action); handled {
		return conn, err
	}

	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var dialer net.Dialer
	return dialer.DialContext(dialCtx, "tcp6", addr)
}

func (s *Server) dialHiddenDescriptorConn(ctx context.Context, targetAddr, key, action string) (net.Conn, bool, error) {
	if !stringsHasHiddenDescriptorKey(key) {
		return nil, false, nil
	}

	s.mu.RLock()
	cfg := s.hidden
	s.mu.RUnlock()
	if cfg.RelayCandidates == nil {
		return nil, false, nil
	}

	hopCount := cfg.RelayHopCount
	if hopCount <= 0 {
		hopCount = hiddenDescriptorRelayHopCount
	}

	exclude := []string{targetAddr}
	if cfg.ExcludeAddrs != nil {
		exclude = append(exclude, cfg.ExcludeAddrs()...)
	}
	relays := filterRelayCandidates(cfg.RelayCandidates(), exclude)
	if len(relays) < hopCount {
		return nil, true, fmt.Errorf("not enough relay candidates to anonymize hidden descriptor %s", action)
	}

	plan, err := onion.PlanAutomatedCircuit(record.ServiceRecord{Address: targetAddr}, relays, hopCount, exclude)
	if err != nil {
		return nil, true, err
	}
	plan.Purpose = "dht-hidden-desc-" + action

	opts := onion.ClientOptions{TransportMode: cfg.TransportMode}
	conn, err := onion.DialPlannedCircuit(ctx, plan, opts)
	return conn, true, err
}

func filterRelayCandidates(nodes []record.EndpointRecord, excludeAddrs []string) []record.EndpointRecord {
	seenAddrs := make(map[string]struct{}, len(excludeAddrs))
	for _, addr := range excludeAddrs {
		if addr == "" {
			continue
		}
		seenAddrs[addr] = struct{}{}
	}

	filtered := make([]record.EndpointRecord, 0, len(nodes))
	seenNodeIDs := make(map[string]struct{}, len(nodes))
	for _, rec := range nodes {
		if rec.NodeID == "" || rec.Address == "" || rec.PublicKey == "" {
			continue
		}
		if _, ok := seenAddrs[rec.Address]; ok {
			continue
		}
		if _, ok := seenNodeIDs[rec.NodeID]; ok {
			continue
		}
		seenNodeIDs[rec.NodeID] = struct{}{}
		filtered = append(filtered, rec)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].NodeID < filtered[j].NodeID
	})
	return filtered
}
