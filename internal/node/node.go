package node

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vx6/vx6/internal/discovery"
	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/netutil"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/secure"
	"github.com/vx6/vx6/internal/serviceproxy"
	"github.com/vx6/vx6/internal/transfer"
)

type Config struct {
	Name           string
	NodeID         string
	ListenAddr     string
	AdvertiseAddr  string
	DataDir        string
	BootstrapAddrs []string
	Services       map[string]string
	Identity       identity.Identity
	Registry       *discovery.Registry
	Verbose        bool
}

func Run(ctx context.Context, log io.Writer, cfg Config) error {
	if cfg.Name == "" {
		return errors.New("node name cannot be empty")
	}
	if cfg.NodeID == "" {
		return errors.New("node id cannot be empty")
	}
	if cfg.Registry == nil {
		return errors.New("registry cannot be nil")
	}
	if err := transfer.ValidateIPv6Address(cfg.ListenAddr); err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	if cfg.AdvertiseAddr == "" {
		_, port, err := net.SplitHostPort(cfg.ListenAddr)
		if err == nil {
			addr, detectErr := netutil.DetectAdvertiseAddress(port)
			if detectErr == nil {
				cfg.AdvertiseAddr = addr
				fmt.Fprintf(log, "auto-detected advertise address %s", cfg.AdvertiseAddr)
			}
		}
	}

	listener, err := net.Listen("tcp6", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}
	defer listener.Close()

	fmt.Fprintf(log, "vx6 node %q (%s) listening on %s", cfg.Name, cfg.NodeID, listener.Addr().String())

	if cfg.AdvertiseAddr != "" {
		go runBootstrapTasks(ctx, log, cfg)
	}

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() {
				continue
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}

			return fmt.Errorf("accept connection: %w", err)
		}

		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			defer conn.Close()

			reader := bufio.NewReader(conn)

			kind, err := proto.ReadHeader(reader)
			if err != nil {
				fmt.Fprintf(log, "session error from %s: %v", conn.RemoteAddr().String(), err)
				return
			}

			switch kind {
			case proto.KindFileTransfer:
				secureConn, err := secure.Server(
					&bufferedConn{Conn: conn, reader: reader},
					proto.KindFileTransfer,
					cfg.Identity,
				)
				if err != nil {
					fmt.Fprintf(log, "secure receive error from %s: %v", conn.RemoteAddr().String(), err)
					return
				}

				result, err := transfer.ReceiveFile(
					secureConn,
					cfg.DataDir,
					func(received, total int64) {
						if cfg.Verbose {
							percent := float64(received) * 100 / float64(total)
							fmt.Fprintf(os.Stderr, "[RECV] %5.1f%% | %d / %d bytes\n", percent, received, total)
							if received == total {
								fmt.Fprintln(os.Stderr)
							}
						}
					},
				)
				if err != nil {
					fmt.Fprintf(log, "receive error from %s: %v", conn.RemoteAddr().String(), err)
					return
				}

				absPath, pathErr := filepath.Abs(result.StoredPath)
				if pathErr != nil {
					absPath = result.StoredPath
				}

				fmt.Fprintf(log, "received %q (%d bytes) from node %q into %s", result.FileName, result.BytesReceived, result.SenderNode, absPath)

			case proto.KindDiscoveryReq:
				if cfg.Registry == nil {
					fmt.Fprintf(log, "discovery request from %s rejected: registry disabled", conn.RemoteAddr().String())
					return
				}
				if err := cfg.Registry.HandleConn(&bufferedConn{Conn: conn, reader: reader}); err != nil {
					fmt.Fprintf(log, "discovery error from %s: %v", conn.RemoteAddr().String(), err)
					return
				}
				fmt.Fprintf(log, "processed discovery request from %s", conn.RemoteAddr().String())

			case proto.KindServiceConn:
				if err := serviceproxy.HandleInbound(&bufferedConn{Conn: conn, reader: reader}, cfg.Identity, cfg.Services); err != nil {
					fmt.Fprintf(log, "service proxy error from %s: %v", conn.RemoteAddr().String(), err)
					return
				}

			default:
				fmt.Fprintf(log, "session error from %s: unsupported kind %d", conn.RemoteAddr().String(), kind)
			}
		}(conn)
	}
}

func runBootstrapTasks(ctx context.Context, log io.Writer, cfg Config) {
	publishAndSync := func() {
		rec, err := record.NewEndpointRecord(cfg.Identity, cfg.Name, cfg.AdvertiseAddr, 20*time.Minute, time.Now())
		if err != nil {
			fmt.Fprintf(log, "bootstrap publish skipped: %v", err)
			return
		}

		if err := cfg.Registry.Import([]record.EndpointRecord{rec}, nil); err != nil {
			fmt.Fprintf(log, "local registry update failed: %v", err)
		}

		targets := map[string]struct{}{}
		for _, addr := range cfg.BootstrapAddrs {
			targets[addr] = struct{}{}
		}

		for addr := range targets {
			if _, err := discovery.Publish(ctx, addr, rec); err != nil {
				fmt.Fprintf(log, "discovery publish to %s failed: %v", addr, err)
				continue
			}

			fmt.Fprintf(log, "published endpoint record to %s", addr)
		}
	}

	publishAndSync()

	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			publishAndSync()
		}
	}
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
