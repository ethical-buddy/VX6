package transport

import (
	"context"
	"testing"
	"time"
)

func TestDialTimeoutAndProbeContext(t *testing.T) {
	t.Parallel()

	ln, err := Listen(ModeTCP, "[::1]:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	accepted := make(chan struct{}, 2)
	go func() {
		for i := 0; i < 2; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
			accepted <- struct{}{}
		}
	}()

	conn, err := DialTimeout(ModeTCP, ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("dial timeout: %v", err)
	}
	_ = conn.Close()

	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("listener did not accept transport connection")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !ProbeContext(ctx, ModeTCP, ln.Addr().String()) {
		t.Fatal("expected probe to succeed")
	}
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("listener did not accept probe connection")
	}
}
