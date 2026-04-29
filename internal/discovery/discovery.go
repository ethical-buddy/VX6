package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/transfer"
)

const maxMessageSize = 64 * 1024

type Registry struct {
	mu     sync.RWMutex
	byName map[string]record.EndpointRecord
	byNode map[string]record.EndpointRecord
}

func NewRegistry() *Registry {
	return &Registry{
		byName: map[string]record.EndpointRecord{},
		byNode: map[string]record.EndpointRecord{},
	}
}

type request struct {
	Action string                `json:"action"`
	Record record.EndpointRecord `json:"record,omitempty"`
	Name   string                `json:"name,omitempty"`
	NodeID string                `json:"node_id,omitempty"`
}

type response struct {
	OK     bool                  `json:"ok"`
	Error  string                `json:"error,omitempty"`
	Record record.EndpointRecord `json:"record,omitempty"`
}

func (r *Registry) HandleConn(conn net.Conn) error {
	defer conn.Close()

	payload, err := proto.ReadLengthPrefixed(conn, maxMessageSize)
	if err != nil {
		return err
	}

	var req request
	if err := json.Unmarshal(payload, &req); err != nil {
		return writeResponse(conn, response{Error: fmt.Sprintf("decode request: %v", err)})
	}

	switch req.Action {
	case "publish":
		return r.handlePublish(conn, req.Record)
	case "resolve":
		return r.handleResolve(conn, req.Name, req.NodeID)
	default:
		return writeResponse(conn, response{Error: fmt.Sprintf("unknown action %q", req.Action)})
	}
}

func (r *Registry) handlePublish(conn net.Conn, rec record.EndpointRecord) error {
	if err := record.VerifyEndpointRecord(rec, time.Now()); err != nil {
		return writeResponse(conn, response{Error: err.Error()})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.byName[rec.NodeName]; ok && existing.NodeID != rec.NodeID {
		return writeResponse(conn, response{Error: fmt.Sprintf("node name %q already belongs to %s", rec.NodeName, existing.NodeID)})
	}

	if existing, ok := r.byNode[rec.NodeID]; ok {
		existingIssuedAt, _ := time.Parse(time.RFC3339, existing.IssuedAt)
		newIssuedAt, _ := time.Parse(time.RFC3339, rec.IssuedAt)
		if existingIssuedAt.After(newIssuedAt) {
			return writeResponse(conn, response{Error: "new record is older than stored record"})
		}
	}

	r.byName[rec.NodeName] = rec
	r.byNode[rec.NodeID] = rec
	return writeResponse(conn, response{OK: true, Record: rec})
}

func (r *Registry) handleResolve(conn net.Conn, name, nodeID string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var rec record.EndpointRecord
	var ok bool

	switch {
	case name != "":
		rec, ok = r.byName[name]
	case nodeID != "":
		rec, ok = r.byNode[nodeID]
	default:
		return writeResponse(conn, response{Error: "resolve requires name or node_id"})
	}

	if !ok {
		return writeResponse(conn, response{Error: "record not found"})
	}
	if err := record.VerifyEndpointRecord(rec, time.Now()); err != nil {
		return writeResponse(conn, response{Error: fmt.Sprintf("stored record invalid: %v", err)})
	}

	return writeResponse(conn, response{OK: true, Record: rec})
}

func Publish(ctx context.Context, address string, rec record.EndpointRecord) (record.EndpointRecord, error) {
	resp, err := roundTrip(ctx, address, request{
		Action: "publish",
		Record: rec,
	})
	if err != nil {
		return record.EndpointRecord{}, err
	}
	if !resp.OK {
		return record.EndpointRecord{}, fmt.Errorf(resp.Error)
	}
	return resp.Record, nil
}

func Resolve(ctx context.Context, address, name, nodeID string) (record.EndpointRecord, error) {
	resp, err := roundTrip(ctx, address, request{
		Action: "resolve",
		Name:   name,
		NodeID: nodeID,
	})
	if err != nil {
		return record.EndpointRecord{}, err
	}
	if !resp.OK {
		return record.EndpointRecord{}, fmt.Errorf(resp.Error)
	}
	if err := record.VerifyEndpointRecord(resp.Record, time.Now()); err != nil {
		return record.EndpointRecord{}, err
	}
	return resp.Record, nil
}

func roundTrip(ctx context.Context, address string, req request) (response, error) {
	if err := transfer.ValidateIPv6Address(address); err != nil {
		return response{}, err
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp6", address)
	if err != nil {
		return response{}, fmt.Errorf("dial discovery endpoint %s: %w", address, err)
	}
	defer conn.Close()

	if err := proto.WriteHeader(conn, proto.KindDiscoveryReq); err != nil {
		return response{}, err
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return response{}, fmt.Errorf("encode request: %w", err)
	}
	if err := proto.WriteLengthPrefixed(conn, payload); err != nil {
		return response{}, err
	}

	kind, err := proto.ReadHeader(conn)
	if err != nil {
		return response{}, err
	}
	if kind != proto.KindDiscoveryRes {
		return response{}, fmt.Errorf("unexpected response kind %d", kind)
	}

	reply, err := proto.ReadLengthPrefixed(conn, maxMessageSize)
	if err != nil {
		return response{}, err
	}

	var resp response
	if err := json.Unmarshal(reply, &resp); err != nil {
		return response{}, fmt.Errorf("decode response: %w", err)
	}

	return resp, nil
}

func writeResponse(conn net.Conn, resp response) error {
	if resp.Error == "" {
		resp.OK = true
	}
	if err := proto.WriteHeader(conn, proto.KindDiscoveryRes); err != nil {
		return err
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return proto.WriteLengthPrefixed(conn, payload)
}
