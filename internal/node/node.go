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
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
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

	listener, err := net.Listen("tcp6", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}
	defer listener.Close()

	fmt.Fprintf(log, "vx6 node %q (%s) listening on %s\n", cfg.Name, cfg.NodeID, listener.Addr().String())

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
		go func() {
			defer wg.Done()
			defer conn.Close()

			reader := bufio.NewReader(conn)

			kind, err := proto.ReadHeader(reader)
			if err != nil {
				fmt.Fprintf(log, "session error from %s: %v\n", conn.RemoteAddr().String(), err)
				return
			}

			switch kind {
			case proto.KindFileTransfer:
				result, err := transfer.ReceiveFile(reader, cfg.DataDir)
				if err != nil {
					fmt.Fprintf(log, "receive error from %s: %v\n", conn.RemoteAddr().String(), err)
					return
				}

				absPath, pathErr := filepath.Abs(result.StoredPath)
				if pathErr != nil {
					absPath = result.StoredPath
				}

				fmt.Fprintf(
					log,
					"received %q (%d bytes) from node %q into %s\n",
					result.FileName,
					result.BytesReceived,
					result.SenderNode,
					absPath,
				)
			case proto.KindDiscoveryReq:
				if cfg.Registry == nil {
					fmt.Fprintf(log, "discovery request from %s rejected: registry disabled\n", conn.RemoteAddr().String())
					return
				}
				if err := cfg.Registry.HandleConn(&bufferedConn{Conn: conn, reader: reader}); err != nil {
					fmt.Fprintf(log, "discovery error from %s: %v\n", conn.RemoteAddr().String(), err)
					return
				}
				fmt.Fprintf(log, "processed discovery request from %s\n", conn.RemoteAddr().String())
			case proto.KindServiceConn:
				if err := serviceproxy.HandleInbound(&bufferedConn{Conn: conn, reader: reader}, cfg.Services); err != nil {
					fmt.Fprintf(log, "service proxy error from %s: %v\n", conn.RemoteAddr().String(), err)
					return
				}
			default:
				fmt.Fprintf(log, "session error from %s: unsupported kind %d\n", conn.RemoteAddr().String(), kind)
			}
		}()
	}
}

func runBootstrapTasks(ctx context.Context, log io.Writer, cfg Config) {
	publishAndSync := func() {
		rec, err := record.NewEndpointRecord(cfg.Identity, cfg.Name, cfg.AdvertiseAddr, 20*time.Minute, time.Now())
		if err != nil {
			fmt.Fprintf(log, "bootstrap publish skipped: %v\n", err)
			return
		}

		for _, addr := range cfg.BootstrapAddrs {
			if _, err := discovery.Publish(ctx, addr, rec); err != nil {
				fmt.Fprintf(log, "bootstrap publish to %s failed: %v\n", addr, err)
				continue
			}
			fmt.Fprintf(log, "published endpoint record to bootstrap %s\n", addr)

			for serviceName := range cfg.Services {
				serviceRec, err := record.NewServiceRecord(cfg.Identity, cfg.Name, serviceName, cfg.AdvertiseAddr, 20*time.Minute, time.Now())
				if err != nil {
					fmt.Fprintf(log, "service publish skipped for %s: %v\n", serviceName, err)
					continue
				}
				if _, err := discovery.PublishService(ctx, addr, serviceRec); err != nil {
					fmt.Fprintf(log, "service publish to %s for %s failed: %v\n", addr, serviceName, err)
				}
			}

			records, services, err := discovery.Snapshot(ctx, addr)
			if err != nil {
				fmt.Fprintf(log, "bootstrap snapshot from %s failed: %v\n", addr, err)
				continue
			}
			if err := cfg.Registry.Import(records, services); err != nil {
				fmt.Fprintf(log, "bootstrap import from %s failed: %v\n", addr, err)
				continue
			}
			fmt.Fprintf(log, "synced %d node records and %d service records from bootstrap %s\n", len(records), len(services), addr)
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
