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

	"github.com/vx6/vx6/internal/discovery"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/transfer"
)

type Config struct {
	Name       string
	NodeID     string
	ListenAddr string
	DataDir    string
	Registry   *discovery.Registry
}

func Run(ctx context.Context, log io.Writer, cfg Config) error {
	if cfg.Name == "" {
		return errors.New("node name cannot be empty")
	}
	if cfg.NodeID == "" {
		return errors.New("node id cannot be empty")
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
			default:
				fmt.Fprintf(log, "session error from %s: unsupported kind %d\n", conn.RemoteAddr().String(), kind)
			}
		}()
	}
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
