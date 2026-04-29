package transfer

import (
	"io"
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
		state, err := readResumeState(clientConn)
		if err != nil {
			t.Errorf("read resume state: %v", err)
			return
		}
		if state.Offset != 0 {
			t.Errorf("unexpected resume offset %d", state.Offset)
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

func TestReceiveFileResumesPartialDownload(t *testing.T) {
	t.Parallel()

	payload := []byte("vx6 resumable transfer payload")
	receiveDir := t.TempDir()
	filePath := filepath.Join(receiveDir, "resume.txt")
	if err := os.WriteFile(filePath, payload[:10], 0o644); err != nil {
		t.Fatalf("seed partial file: %v", err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		defer clientConn.Close()

		if err := writeMetadata(clientConn, metadata{
			NodeName: "alpha",
			FileName: "resume.txt",
			FileSize: int64(len(payload)),
		}); err != nil {
			t.Errorf("write metadata: %v", err)
			return
		}
		state, err := readResumeState(clientConn)
		if err != nil {
			t.Errorf("read resume state: %v", err)
			return
		}
		if state.Offset != 10 {
			t.Errorf("unexpected resume offset %d", state.Offset)
			return
		}
		if _, err := io.Copy(clientConn, bytesReader(payload[state.Offset:])); err != nil {
			t.Errorf("write resumed payload: %v", err)
			return
		}
	}()

	result, err := ReceiveFile(serverConn, receiveDir)
	if err != nil {
		t.Fatalf("receive file: %v", err)
	}
	if result.BytesReceived != int64(len(payload)) {
		t.Fatalf("unexpected bytes received %d", result.BytesReceived)
	}

	received, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read resumed file: %v", err)
	}
	if string(received) != string(payload) {
		t.Fatalf("unexpected resumed payload %q", string(received))
	}
}

func bytesReader(b []byte) io.Reader {
	return &sliceReader{data: b}
}

type sliceReader struct {
	data []byte
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
