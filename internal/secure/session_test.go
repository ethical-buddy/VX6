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
