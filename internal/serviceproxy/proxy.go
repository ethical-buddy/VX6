package serviceproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/secure"
)

const maxRequestSize = 4 * 1024

type ConnectRequest struct {
	ServiceName string `json:"service_name"`
}

func HandleInbound(conn net.Conn, id identity.Identity, services map[string]string) error {
	fmt.Printf("[PROXY] Incoming request from %s\n", conn.RemoteAddr())

	secureConn, err := secure.Server(conn, proto.KindServiceConn, id)
	if err != nil {
		fmt.Printf("[PROXY] Secure handshake failed: %v\n", err)
		return err
	}
	
	reqPayload, err := proto.ReadLengthPrefixed(secureConn, maxRequestSize)
	if err != nil {
		fmt.Printf("[PROXY] Failed to read request: %v\n", err)
		return err
	}

	var req ConnectRequest
	if err := json.Unmarshal(reqPayload, &req); err != nil {
		return fmt.Errorf("decode service request: %w", err)
	}

	target, ok := services[req.ServiceName]
	if !ok {
		fmt.Printf("[PROXY] Reject: Service %q not found in local config\n", req.ServiceName)
		return fmt.Errorf("service %q not found", req.ServiceName)
	}

	// Ensure we dial explicitly on the local interface
	targetConn, err := net.Dial("tcp", target)
	if err != nil {
		fmt.Printf("[PROXY] Reject: Could not connect to local target %s: %v\n", target, err)
		return err
	}
	defer targetConn.Close()

	fmt.Printf("[PROXY] Linked %s <-> %s (%s)\n", conn.RemoteAddr(), target, req.ServiceName)
	return proxyDuplex(secureConn, targetConn)
}

func ServeLocalForward(ctx context.Context, localListen string, service record.ServiceRecord, id identity.Identity, dialer func(context.Context) (net.Conn, error)) error {
	listener, err := net.Listen("tcp", localListen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", localListen, err)
	}
	defer listener.Close()

	fmt.Printf("✔ Local Forwarder ready at %s. Use this address to access %s.\n", localListen, service.ServiceName)

	for {
		localConn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil { return nil }
			continue
		}

		go func() {
			defer localConn.Close()

			fmt.Printf("[TUNNEL] Dialing remote node for %s...\n", service.ServiceName)
			remoteConn, err := dialer(ctx)
			if err != nil {
				fmt.Printf("[TUNNEL] Connection failed: %v\n", err)
				return
			}
			defer remoteConn.Close()

			if err := proto.WriteHeader(remoteConn, proto.KindServiceConn); err != nil {
				return
			}
			secureConn, err := secure.Client(remoteConn, proto.KindServiceConn, id)
			if err != nil {
				fmt.Printf("[TUNNEL] Handshake failed: %v\n", err)
				return
			}

			payload, _ := json.Marshal(ConnectRequest{ServiceName: service.ServiceName})
			_ = proto.WriteLengthPrefixed(secureConn, payload)

			fmt.Printf("[TUNNEL] Active: Local:%s <-> Remote:%s\n", localListen, service.NodeName)
			_ = proxyDuplex(localConn, secureConn)
		}()
	}
}

func proxyDuplex(a, b net.Conn) error {
	errCh := make(chan error, 2)
	go func() { _, err := io.Copy(a, b); errCh <- err }()
	go func() { _, err := io.Copy(b, a); errCh <- err }()
	return <-errCh
}
