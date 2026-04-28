package dht

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
)

func BenchmarkValidateLookupValueServiceRecord(b *testing.B) {
	id, err := identity.Generate()
	if err != nil {
		b.Fatal(err)
	}
	now := time.Now()
	rec, err := record.NewServiceRecord(id, "bench", "api", "[2001:db8::80]:4242", 10*time.Minute, now)
	if err != nil {
		b.Fatal(err)
	}
	payload := string(mustJSONBytes(b, rec))
	key := ServiceKey("bench.api")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := validateLookupValue(key, payload, now); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStoreValidatedServiceRecord(b *testing.B) {
	id, err := identity.Generate()
	if err != nil {
		b.Fatal(err)
	}
	now := time.Now()
	rec, err := record.NewServiceRecord(id, "bench", "api", "[2001:db8::81]:4242", 10*time.Minute, now)
	if err != nil {
		b.Fatal(err)
	}
	payload := string(mustJSONBytes(b, rec))
	key := ServiceKey("bench.api")
	server := NewServer("bench-node")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, _, err := server.storeValidated(key, payload, now); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRecursiveFindValueDetailedConfirmed3Sources(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewServer("client-node")
	relayA := NewServer("relay-a")
	relayB := NewServer("relay-b")
	relayC := NewServer("relay-c")

	addrA := startDHTBenchmarkListener(b, ctx, relayA)
	addrB := startDHTBenchmarkListener(b, ctx, relayB)
	addrC := startDHTBenchmarkListener(b, ctx, relayC)

	client.RT.AddNode(nodeInfo("relay-a", addrA))
	client.RT.AddNode(nodeInfo("relay-b", addrB))
	client.RT.AddNode(nodeInfo("relay-c", addrC))

	rec := mustBenchmarkServiceRecord(b, "bench", "api", "[2001:db8::82]:4242", time.Now())
	payload := string(mustJSONBytes(b, rec))
	key := ServiceKey("bench.api")
	relayA.StoreLocal(key, payload)
	relayB.StoreLocal(key, payload)
	relayC.StoreLocal(key, payload)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := client.RecursiveFindValueDetailed(ctx, key)
		if err != nil {
			b.Fatal(err)
		}
		if result.SourceCount < 2 {
			b.Fatalf("expected confirmed result, got %+v", result)
		}
	}
}

func mustJSONBytes(b *testing.B, v any) []byte {
	b.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		b.Fatal(err)
	}
	return data
}

func nodeInfo(id, addr string) proto.NodeInfo {
	return proto.NodeInfo{ID: id, Addr: addr}
}

func mustBenchmarkServiceRecord(b *testing.B, nodeName, serviceName, address string, now time.Time) record.ServiceRecord {
	b.Helper()

	id, err := identity.Generate()
	if err != nil {
		b.Fatal(err)
	}
	rec, err := record.NewServiceRecord(id, nodeName, serviceName, address, 10*time.Minute, now)
	if err != nil {
		b.Fatal(err)
	}
	return rec
}

func startDHTBenchmarkListener(b *testing.B, ctx context.Context, srv *Server) string {
	b.Helper()

	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		b.Fatalf("listen dht benchmark: %v", err)
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
