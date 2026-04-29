package serviceproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"

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
	secureConn, err := secure.Server(conn, proto.KindServiceConn, id)
	if err != nil {
		return err
	}
	reqPayload, err := proto.ReadLengthPrefixed(secureConn, maxRequestSize)
	if err != nil {
		return err
	}

	var req ConnectRequest
	if err := json.Unmarshal(reqPayload, &req); err != nil {
		return fmt.Errorf("decode service request: %w", err)
	}

	target, ok := services[req.ServiceName]
	if !ok {
		return fmt.Errorf("service %q not exposed on this node", req.ServiceName)
	}

	targetConn, err := net.Dial("tcp", target)
	if err != nil {
		return fmt.Errorf("dial service target %s: %w", target, err)
	}
	defer targetConn.Close()

	return proxyDuplex(secureConn, targetConn)
}

func ServeLocalForward(ctx context.Context, localListen string, service record.ServiceRecord, id identity.Identity, resolveRemote func(context.Context) (string, error)) error {
	listener, err := net.Listen("tcp", localListen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", localListen, err)
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		localConn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept local connection: %w", err)
		}

		go func() {
			defer localConn.Close()

			address, err := resolveRemote(ctx)
			if err != nil {
				return
			}

			var dialer net.Dialer
			remoteConn, err := dialer.DialContext(ctx, "tcp6", address)
			if err != nil {
				return
			}
			defer remoteConn.Close()

			if err := proto.WriteHeader(remoteConn, proto.KindServiceConn); err != nil {
				return
			}
			secureConn, err := secure.Client(remoteConn, proto.KindServiceConn, id)
			if err != nil {
				return
			}

			payload, err := json.Marshal(ConnectRequest{ServiceName: service.ServiceName})
			if err != nil {
				return
			}
			if err := proto.WriteLengthPrefixed(secureConn, payload); err != nil {
				return
			}

			_ = proxyDuplex(localConn, secureConn)
		}()
	}
}

func proxyDuplex(left io.ReadWriter, right io.ReadWriter) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	copyPipe := func(dst io.Writer, src io.Reader) {
		defer wg.Done()
		_, err := io.Copy(dst, src)
		errCh <- err
	}

	wg.Add(2)
	go copyPipe(right, left)
	go copyPipe(left, right)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil && err != io.EOF {
			return err
		}
	}
	return nil
}
