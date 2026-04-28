package dht

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
)

func TestRecursiveFindValueAcrossPeers(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	alice := NewServer("alice-node")
	bob := NewServer("bob-node")
	charlie := NewServer("charlie-node")

	bobAddr := startDHTListener(t, ctx, bob)
	charlieAddr := startDHTListener(t, ctx, charlie)

	alice.RT.AddNode(proto.NodeInfo{ID: "bob-node", Addr: bobAddr})
	bob.RT.AddNode(proto.NodeInfo{ID: "charlie-node", Addr: charlieAddr})

	rec := mustServiceRecord(t, "surya", "echo", "[2001:db8::10]:4242", false, "", time.Now())
	charlie.StoreLocal(ServiceKey("surya.echo"), mustJSON(t, rec))

	got, err := alice.RecursiveFindValue(ctx, ServiceKey("surya.echo"))
	if err != nil {
		t.Fatalf("recursive find value: %v", err)
	}

	var decoded record.ServiceRecord
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("decode returned record: %v", err)
	}
	if decoded.ServiceName != "echo" || decoded.NodeName != "surya" {
		t.Fatalf("unexpected resolved record: %+v", decoded)
	}
}

func TestStoreReplicatesAcrossPeer(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	alice := NewServer("alice-node")
	bob := NewServer("bob-node")

	bobAddr := startDHTListener(t, ctx, bob)
	alice.RT.AddNode(proto.NodeInfo{ID: "bob-node", Addr: bobAddr})

	rec := mustServiceRecord(t, "bob", "chat", "[2001:db8::20]:4242", false, "", time.Now())
	payload := mustJSON(t, rec)

	if err := alice.Store(ctx, ServiceKey("bob.chat"), payload); err != nil {
		t.Fatalf("store: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		bob.mu.RLock()
		got := bob.Values[ServiceKey("bob.chat")]
		bob.mu.RUnlock()
		if got == payload {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("replicated value not found, got %q", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRecursiveFindValueFiltersPoisonedValueAndUsesConfirmedRecord(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := NewServer("client-node")
	poison := NewServer("poison-node")
	goodA := NewServer("good-a")
	goodB := NewServer("good-b")

	poisonAddr := startDHTListener(t, ctx, poison)
	goodAAddr := startDHTListener(t, ctx, goodA)
	goodBAddr := startDHTListener(t, ctx, goodB)

	client.RT.AddNode(proto.NodeInfo{ID: "poison-node", Addr: poisonAddr})
	client.RT.AddNode(proto.NodeInfo{ID: "good-a", Addr: goodAAddr})
	client.RT.AddNode(proto.NodeInfo{ID: "good-b", Addr: goodBAddr})

	rec := mustEndpointRecord(t, "owner", "[2001:db8::42]:4242", time.Now())
	payload := mustJSON(t, rec)

	poison.StoreLocal(NodeNameKey("owner"), `{"node_name":"owner","address":"[2001:db8::666]:4242"}`)
	goodA.StoreLocal(NodeNameKey("owner"), payload)
	goodB.StoreLocal(NodeNameKey("owner"), payload)

	result, err := client.RecursiveFindValueDetailed(ctx, NodeNameKey("owner"))
	if err != nil {
		t.Fatalf("detailed find value: %v", err)
	}
	if result.Value != payload {
		t.Fatalf("unexpected resolved payload: %q", result.Value)
	}
	if !result.Verified {
		t.Fatal("expected verified DHT result")
	}
	if result.SourceCount < 2 {
		t.Fatalf("expected multi-source confirmation, got %d", result.SourceCount)
	}
	if result.RejectedValues == 0 {
		t.Fatal("expected poisoned value rejection to be recorded")
	}
}

func TestRecursiveFindValueRejectsConflictingVerifiedValues(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := NewServer("client-node")
	left := NewServer("left-node")
	right := NewServer("right-node")

	leftAddr := startDHTListener(t, ctx, left)
	rightAddr := startDHTListener(t, ctx, right)

	client.RT.AddNode(proto.NodeInfo{ID: "left-node", Addr: leftAddr})
	client.RT.AddNode(proto.NodeInfo{ID: "right-node", Addr: rightAddr})

	left.StoreLocal(NodeNameKey("shared"), mustJSON(t, mustEndpointRecord(t, "shared", "[2001:db8::51]:4242", time.Now())))
	right.StoreLocal(NodeNameKey("shared"), mustJSON(t, mustEndpointRecord(t, "shared", "[2001:db8::52]:4242", time.Now().Add(time.Second))))

	_, err := client.RecursiveFindValueDetailed(ctx, NodeNameKey("shared"))
	if err == nil || !errors.Is(err, ErrConflictingValues) {
		t.Fatalf("expected conflicting value error, got %v", err)
	}
}

func TestStoreKeepsNewestVerifiedRecord(t *testing.T) {
	t.Parallel()

	server := NewServer("server-node")
	now := time.Now()
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	older := mustServiceRecordForIdentity(t, id, "owner", "api", "[2001:db8::61]:4242", false, "", now)
	newer := mustServiceRecordForIdentity(t, id, "owner", "api", "[2001:db8::62]:4242", false, "", now.Add(time.Minute))

	if err := server.Store(context.Background(), ServiceKey("owner.api"), mustJSON(t, newer)); err != nil {
		t.Fatalf("store newer record: %v", err)
	}
	if err := server.Store(context.Background(), ServiceKey("owner.api"), mustJSON(t, older)); err != nil {
		t.Fatalf("store older record: %v", err)
	}

	server.mu.RLock()
	got := server.Values[ServiceKey("owner.api")]
	server.mu.RUnlock()
	if got != mustJSON(t, newer) {
		t.Fatalf("expected latest record to win, got %q", got)
	}
}

func TestStoreRejectsConflictingVerifiedFamily(t *testing.T) {
	t.Parallel()

	server := NewServer("server-node")
	now := time.Now()

	first := mustEndpointRecord(t, "owner", "[2001:db8::71]:4242", now)
	second := mustEndpointRecord(t, "owner", "[2001:db8::72]:4242", now.Add(time.Second))

	if _, _, err := server.storeValidated(NodeNameKey("owner"), mustJSON(t, first), now); err != nil {
		t.Fatalf("seed first record: %v", err)
	}
	if _, _, err := server.storeValidated(NodeNameKey("owner"), mustJSON(t, second), now.Add(time.Second)); err == nil || !errors.Is(err, ErrConflictingValues) {
		t.Fatalf("expected conflicting family rejection, got %v", err)
	}
}

func startDHTListener(t *testing.T, ctx context.Context, srv *Server) string {
	t.Helper()

	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatalf("listen dht: %v", err)
	}

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go func(conn net.Conn) {
				defer conn.Close()

				kind, err := proto.ReadHeader(conn)
				if err != nil || kind != proto.KindDHT {
					return
				}
				payload, err := proto.ReadLengthPrefixed(conn, 1024*1024)
				if err != nil {
					return
				}

				var req proto.DHTRequest
				if err := json.Unmarshal(payload, &req); err != nil {
					return
				}
				_ = srv.HandleDHT(ctx, conn, req)
			}(conn)
		}
	}()

	return ln.Addr().String()
}

func mustEndpointRecord(t *testing.T, nodeName, address string, now time.Time) record.EndpointRecord {
	t.Helper()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate endpoint identity: %v", err)
	}
	rec, err := record.NewEndpointRecord(id, nodeName, address, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("new endpoint record: %v", err)
	}
	return rec
}

func mustServiceRecord(t *testing.T, nodeName, serviceName, address string, hidden bool, alias string, now time.Time) record.ServiceRecord {
	t.Helper()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate service identity: %v", err)
	}
	rec, err := record.NewServiceRecord(id, nodeName, serviceName, address, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("new service record: %v", err)
	}
	return finalizeServiceRecord(t, id, rec, hidden, alias)
}

func mustServiceRecordForIdentity(t *testing.T, id identity.Identity, nodeName, serviceName, address string, hidden bool, alias string, now time.Time) record.ServiceRecord {
	t.Helper()

	rec, err := record.NewServiceRecord(id, nodeName, serviceName, address, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("new service record: %v", err)
	}
	return finalizeServiceRecord(t, id, rec, hidden, alias)
}

func finalizeServiceRecord(t *testing.T, id identity.Identity, rec record.ServiceRecord, hidden bool, alias string) record.ServiceRecord {
	t.Helper()

	rec.IsHidden = hidden
	rec.Alias = alias
	if hidden {
		rec.HiddenProfile = "fast"
		rec.Address = ""
		if err := record.SignServiceRecord(id, &rec); err != nil {
			t.Fatalf("sign hidden service record: %v", err)
		}
	}
	return rec
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(data)
}
