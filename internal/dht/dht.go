package dht

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/vx6/vx6/internal/proto"
)

type Server struct {
	RT     *RoutingTable
	Values map[string]string // The decentralized database
	mu     sync.RWMutex
}

func NodeNameKey(name string) string {
	return "node/name/" + name
}

func NodeIDKey(nodeID string) string {
	return "node/id/" + nodeID
}

func ServiceKey(fullName string) string {
	return "service/" + fullName
}

func HiddenServiceKey(alias string) string {
	return "hidden/" + alias
}

func NewServer(selfID string) *Server {
	return &Server{
		RT:     NewRoutingTable(selfID),
		Values: make(map[string]string),
	}
}

// HandleDHT processes an incoming DHT request from a peer
func (s *Server) HandleDHT(ctx context.Context, conn net.Conn, req proto.DHTRequest) error {
	resp := proto.DHTResponse{}

	switch req.Action {
	case "find_node":
		resp.Nodes = s.RT.ClosestNodes(req.Target, K)
	case "find_value":
		s.mu.RLock()
		val, ok := s.Values[req.Target]
		s.mu.RUnlock()
		if ok {
			resp.Value = val
		} else {
			resp.Nodes = s.RT.ClosestNodes(req.Target, K)
		}
	case "store":
		s.mu.Lock()
		s.Values[req.Target] = req.Data
		s.mu.Unlock()
	}

	payload, _ := json.Marshal(resp)
	if err := proto.WriteHeader(conn, proto.KindDHT); err != nil {
		return err
	}
	return proto.WriteLengthPrefixed(conn, payload)
}

func (s *Server) StoreLocal(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Values[key] = value
}

// RecursiveFindNode searches the network for a specific NodeID
func (s *Server) RecursiveFindNode(ctx context.Context, targetID string) ([]proto.NodeInfo, error) {
	visited := make(map[string]bool)
	candidates := s.RT.ClosestNodes(targetID, K)

	for {
		foundNew := false
		newCandidates := []proto.NodeInfo{}
		for _, node := range candidates {
			if visited[node.ID] {
				continue
			}
			visited[node.ID] = true

			// Ask this node for its closest nodes to the target
			newNodes, err := s.QueryNode(ctx, node.Addr, targetID)
			if err == nil {
				for _, n := range newNodes {
					if !visited[n.ID] {
						s.RT.AddNode(n)
						newCandidates = append(newCandidates, n)
						foundNew = true
					}
				}
			}
		}
		candidates = append(candidates, newCandidates...)

		if !foundNew {
			break
		}
	}

	return s.RT.ClosestNodes(targetID, K), nil
}

// Store saves a value on the K closest nodes to the targetID
func (s *Server) Store(ctx context.Context, targetID, value string) error {
	s.StoreLocal(targetID, value)
	nodes := s.RT.ClosestNodes(targetID, K)
	for _, n := range nodes {
		_ = s.sendStore(ctx, n.Addr, targetID, value)
	}
	return nil
}

func (s *Server) sendStore(ctx context.Context, addr, key, value string) error {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp6", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	req := proto.DHTRequest{Action: "store", Target: key, Data: value}
	if err := proto.WriteHeader(conn, proto.KindDHT); err != nil {
		return err
	}
	payload, _ := json.Marshal(req)
	if err := proto.WriteLengthPrefixed(conn, payload); err != nil {
		return err
	}

	kind, err := proto.ReadHeader(conn)
	if err != nil {
		return err
	}
	if kind != proto.KindDHT {
		return fmt.Errorf("invalid response")
	}
	_, err = proto.ReadLengthPrefixed(conn, 1024*1024)
	return err
}

// RecursiveFindValue searches for a value in the network
func (s *Server) RecursiveFindValue(ctx context.Context, key string) (string, error) {
	visited := make(map[string]bool)
	candidates := s.RT.ClosestNodes(key, K)

	for len(candidates) > 0 {
		node := candidates[0]
		candidates = candidates[1:]

		if visited[node.ID] {
			continue
		}
		visited[node.ID] = true

		val, nextNodes, err := s.QueryValue(ctx, node.Addr, key)
		if err == nil {
			if val != "" {
				return val, nil
			}
			for _, n := range nextNodes {
				if !visited[n.ID] {
					candidates = append(candidates, n)
				}
			}
		}
	}
	return "", fmt.Errorf("value not found in DHT")
}

func (s *Server) QueryNode(ctx context.Context, addr, targetID string) ([]proto.NodeInfo, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp6", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := proto.DHTRequest{Action: "find_node", Target: targetID}
	_ = proto.WriteHeader(conn, proto.KindDHT)
	payload, _ := json.Marshal(req)
	_ = proto.WriteLengthPrefixed(conn, payload)

	kind, err := proto.ReadHeader(conn)
	if err != nil || kind != proto.KindDHT {
		return nil, fmt.Errorf("invalid response")
	}

	resPayload, _ := proto.ReadLengthPrefixed(conn, 1024*1024)
	var resp proto.DHTResponse
	_ = json.Unmarshal(resPayload, &resp)

	return resp.Nodes, nil
}

func (s *Server) QueryValue(ctx context.Context, addr, key string) (string, []proto.NodeInfo, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp6", addr)
	if err != nil {
		return "", nil, err
	}
	defer conn.Close()

	req := proto.DHTRequest{Action: "find_value", Target: key}
	_ = proto.WriteHeader(conn, proto.KindDHT)
	payload, _ := json.Marshal(req)
	_ = proto.WriteLengthPrefixed(conn, payload)

	kind, err := proto.ReadHeader(conn)
	if err != nil || kind != proto.KindDHT {
		return "", nil, fmt.Errorf("invalid response")
	}

	resPayload, _ := proto.ReadLengthPrefixed(conn, 1024*1024)
	var resp proto.DHTResponse
	_ = json.Unmarshal(resPayload, &resp)

	return resp.Value, resp.Nodes, nil
}
