package secure

import (
	"io"
	"net"
	"testing"

	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/proto"
)

func TestSessionRoundTrip(t *testing.T) {
	t.Parallel()

	clientID, _ := identity.Generate()
	serverID, _ := identity.Generate()
	left, right := net.Pipe()

	errCh := make(chan error, 2)
	go func() {
		defer left.Close()
		if err := proto.WriteHeader(left, proto.KindFileTransfer); err != nil {
			errCh <- err
			return
		}
		c, err := Client(left, proto.KindFileTransfer, clientID)
		if err != nil {
			errCh <- err
			return
		}
		_, err = c.Write([]byte("hello"))
		errCh <- err
	}()
	go func() {
		defer right.Close()
		kind, err := proto.ReadHeader(right)
		if err != nil {
			errCh <- err
			return
		}
		c, err := Server(right, kind, serverID)
		if err != nil {
			errCh <- err
			return
		}
		buf := make([]byte, 5)
		_, err = io.ReadFull(c, buf)
		if err == nil && string(buf) != "hello" {
			t.Fatalf("unexpected payload %q", string(buf))
		}
		errCh <- err
	}()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
}

func TestSessionExposesPeerIdentity(t *testing.T) {
	t.Parallel()

	clientID, _ := identity.Generate()
	serverID, _ := identity.Generate()
	left, right := net.Pipe()

	clientCh := make(chan *Conn, 1)
	serverCh := make(chan *Conn, 1)
	errCh := make(chan error, 2)

	go func() {
		defer left.Close()
		c, err := Client(left, proto.KindRendezvous, clientID)
		if err != nil {
			errCh <- err
			return
		}
		clientCh <- c
		errCh <- nil
	}()
	go func() {
		defer right.Close()
		c, err := Server(right, proto.KindRendezvous, serverID)
		if err != nil {
			errCh <- err
			return
		}
		serverCh <- c
		errCh <- nil
	}()

	clientConn := <-clientCh
	serverConn := <-serverCh
	if clientConn.LocalNodeID() != clientID.NodeID || clientConn.PeerNodeID() != serverID.NodeID {
		t.Fatalf("unexpected client session identities: local=%s peer=%s", clientConn.LocalNodeID(), clientConn.PeerNodeID())
	}
	if serverConn.LocalNodeID() != serverID.NodeID || serverConn.PeerNodeID() != clientID.NodeID {
		t.Fatalf("unexpected server session identities: local=%s peer=%s", serverConn.LocalNodeID(), serverConn.PeerNodeID())
	}

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
}
