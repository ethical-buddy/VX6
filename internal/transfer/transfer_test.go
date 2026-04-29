package transfer

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateIPv6Address(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{name: "valid ipv6", address: "[2001:db8::1]:4242"},
		{name: "ipv4 rejected", address: "127.0.0.1:4242", wantErr: true},
		{name: "missing brackets rejected", address: "2001:db8::1:4242", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateIPv6Address(tc.address)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestReceiveFile(t *testing.T) {
	t.Parallel()

	payload := []byte("vx6 transport bootstrap")
	receiveDir := t.TempDir()
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		defer clientConn.Close()

		if err := writeMetadata(clientConn, metadata{
			NodeName: "alpha",
			FileName: "hello.txt",
			FileSize: int64(len(payload)),
		}); err != nil {
			t.Errorf("write metadata: %v", err)
			return
		}
		if _, err := clientConn.Write(payload); err != nil {
			t.Errorf("write payload: %v", err)
			return
		}
	}()

	result, err := ReceiveFile(serverConn, receiveDir)
	if err != nil {
		t.Fatalf("receive file: %v", err)
	}
	if result.SenderNode != "alpha" {
		t.Fatalf("unexpected sender node %q", result.SenderNode)
	}
	if result.FileName != "hello.txt" {
		t.Fatalf("unexpected file name %q", result.FileName)
	}
	if result.BytesReceived != int64(len(payload)) {
		t.Fatalf("unexpected bytes received: %d", result.BytesReceived)
	}

	receivedPath := filepath.Join(receiveDir, "hello.txt")
	received, err := os.ReadFile(receivedPath)
	if err != nil {
		t.Fatalf("read received file: %v", err)
	}
	if string(received) != string(payload) {
		t.Fatalf("unexpected payload %q", string(received))
	}
}
