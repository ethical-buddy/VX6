package discovery

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
)

func TestRegistryPublishResolve(t *testing.T) {
	t.Parallel()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	reg, err := NewRegistry("")
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	rec, err := record.NewEndpointRecord(id, "alpha", "[2001:db8::1]:4242", 10*time.Minute, time.Now())
	if err != nil {
		t.Fatalf("new endpoint record: %v", err)
	}

	server, client := net.Pipe()
	done := make(chan error, 2)

	go func() {
		kind, err := proto.ReadHeader(server)
		if err != nil {
			done <- err
			return
		}
		if kind != proto.KindDiscoveryReq {
			done <- testErr("unexpected request kind")
			return
		}
		done <- reg.HandleConn(server)
	}()
	go func() {
		_, err := roundTripWithConn(client, request{Action: "publish", Record: rec})
		done <- err
	}()

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("publish flow failed: %v", err)
		}
	}

	server2, client2 := net.Pipe()
	done = make(chan error, 2)

	go func() {
		kind, err := proto.ReadHeader(server2)
		if err != nil {
			done <- err
			return
		}
		if kind != proto.KindDiscoveryReq {
			done <- testErr("unexpected request kind")
			return
		}
		done <- reg.HandleConn(server2)
	}()
	go func() {
		resp, err := roundTripWithConn(client2, request{Action: "resolve", Name: "alpha"})
		if err != nil {
			done <- err
			return
		}
		if resp.Record.NodeID != rec.NodeID {
			done <- testErr("resolved wrong record")
			return
		}
		done <- nil
	}()

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("resolve flow failed: %v", err)
		}
	}
}

func TestRegistryPublishResolveService(t *testing.T) {
	t.Parallel()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	reg, err := NewRegistry("")
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	rec, err := record.NewServiceRecord(id, "surya", "ssh", "[2001:db8::1]:4242", 10*time.Minute, time.Now())
	if err != nil {
		t.Fatalf("new service record: %v", err)
	}

	server, client := net.Pipe()
	done := make(chan error, 2)

	go func() {
		kind, err := proto.ReadHeader(server)
		if err != nil {
			done <- err
			return
		}
		if kind != proto.KindDiscoveryReq {
			done <- testErr("unexpected request kind")
			return
		}
		done <- reg.HandleConn(server)
	}()
	go func() {
		_, err := roundTripWithConn(client, request{Action: "publish_service", ServiceRecord: rec})
		done <- err
	}()

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("publish service flow failed: %v", err)
		}
	}

	server2, client2 := net.Pipe()
	done = make(chan error, 2)

	go func() {
		kind, err := proto.ReadHeader(server2)
		if err != nil {
			done <- err
			return
		}
		if kind != proto.KindDiscoveryReq {
			done <- testErr("unexpected request kind")
			return
		}
		done <- reg.HandleConn(server2)
	}()
	go func() {
		resp, err := roundTripWithConn(client2, request{Action: "resolve_service", Service: "surya.ssh"})
		if err != nil {
			done <- err
			return
		}
		if resp.ServiceRecord.ServiceName != "ssh" {
			done <- testErr("resolved wrong service")
			return
		}
		done <- nil
	}()

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("resolve service flow failed: %v", err)
		}
	}
}

func TestRegistryImportReplacesOlderEndpointAddress(t *testing.T) {
	t.Parallel()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	reg, err := NewRegistry("")
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	now := time.Now()
	oldRec, err := record.NewEndpointRecord(id, "alpha", "[2001:db8::1]:4242", 10*time.Minute, now)
	if err != nil {
		t.Fatalf("new old endpoint record: %v", err)
	}
	newRec, err := record.NewEndpointRecord(id, "alpha", "[2001:db8::2]:4242", 10*time.Minute, now.Add(time.Second))
	if err != nil {
		t.Fatalf("new updated endpoint record: %v", err)
	}

	if err := reg.Import([]record.EndpointRecord{oldRec}, nil); err != nil {
		t.Fatalf("import old record: %v", err)
	}
	if err := reg.Import([]record.EndpointRecord{newRec}, nil); err != nil {
		t.Fatalf("import new record: %v", err)
	}

	got, err := reg.ResolveLocal("alpha", "")
	if err != nil {
		t.Fatalf("resolve local: %v", err)
	}
	if got.Address != "[2001:db8::2]:4242" {
		t.Fatalf("unexpected resolved address %q", got.Address)
	}

	nodes, _ := reg.Snapshot()
	if len(nodes) != 1 {
		t.Fatalf("unexpected node count %d", len(nodes))
	}
	if nodes[0].Address != "[2001:db8::2]:4242" {
		t.Fatalf("unexpected snapshot address %q", nodes[0].Address)
	}
}

func roundTripWithConn(conn net.Conn, req request) (response, error) {
	defer conn.Close()

	if err := proto.WriteHeader(conn, proto.KindDiscoveryReq); err != nil {
		return response{}, err
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return response{}, err
	}
	if err := proto.WriteLengthPrefixed(conn, payload); err != nil {
		return response{}, err
	}

	kind, err := proto.ReadHeader(conn)
	if err != nil {
		return response{}, err
	}
	if kind != proto.KindDiscoveryRes {
		return response{}, testErr("unexpected response kind")
	}

	reply, err := proto.ReadLengthPrefixed(conn, maxMessageSize)
	if err != nil {
		return response{}, err
	}

	var resp response
	if err := json.Unmarshal(reply, &resp); err != nil {
		return response{}, err
	}

	return resp, nil
}

type testErr string

func (e testErr) Error() string { return string(e) }
