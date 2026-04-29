package dht

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/vx6/vx6/internal/proto"
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
	charlie.Values["svc:surya.echo"] = `{"service":"echo"}`

	got, err := alice.RecursiveFindValue(ctx, "svc:surya.echo")
	if err != nil {
		t.Fatalf("recursive find value: %v", err)
	}
	if want := `{"service":"echo"}`; got != want {
		t.Fatalf("unexpected value %q, want %q", got, want)
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
