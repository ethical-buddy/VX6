package onion

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/vx6/vx6/internal/proto"
)

// Circuit represents an active tunnel segment
type Circuit struct {
	ID       string
	Inbound  net.Conn
	Outbound net.Conn
}

var (
	circuits   = make(map[string]*Circuit)
	circuitsMu sync.RWMutex
)

// HandleExtend process a request from a previous hop to connect to a next hop
func HandleExtend(ctx context.Context, inbound net.Conn, req proto.ExtendRequest) error {
	fmt.Printf("[CIRCUIT] Extending circuit %s to %s\n", req.CircuitID, req.NextHop)

	// Dial the next hop in the chain
	outbound, err := net.Dial("tcp6", req.NextHop)
	if err != nil {
		return fmt.Errorf("failed to extend to %s: %w", req.NextHop, err)
	}

	c := &Circuit{
		ID:       req.CircuitID,
		Inbound:  inbound,
		Outbound: outbound,
	}

	circuitsMu.Lock()
	circuits[req.CircuitID] = c
	circuitsMu.Unlock()

	return relay(c)
}

func relay(c *Circuit) error {
	defer c.Inbound.Close()
	defer c.Outbound.Close()

	errCh := make(chan error, 2)

	copyPipe := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errCh <- err
	}

	go copyPipe(c.Inbound, c.Outbound)
	go copyPipe(c.Outbound, c.Inbound)

	// Wait for connection to close
	err := <-errCh

	circuitsMu.Lock()
	delete(circuits, c.ID)
	circuitsMu.Unlock()
	fmt.Printf("[CIRCUIT] Circuit %s closed\n", c.ID)

	if err != nil && err != io.EOF {
		return err
	}
	return nil
}
