package transfer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/secure"
)

const maxHeaderSize = 4 * 1024

type metadata struct {
	NodeName string `json:"node_name"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
}

type SendRequest struct {
	NodeName     string
	FilePath     string
	Address      string
	Identity     identity.Identity
	OnProgress   func(sent, total int64)
	OnRetransmit func(packet int)
}

type SendResult struct {
	NodeName   string
	FileName   string
	BytesSent  int64
	RemoteAddr string
}

type ReceiveResult struct {
	SenderNode    string
	FileName      string
	BytesReceived int64
	StoredPath    string
}

func SendFile(ctx context.Context, req SendRequest) (SendResult, error) {
	if req.NodeName == "" {
		return SendResult{}, fmt.Errorf("node name cannot be empty")
	}
	if err := req.Identity.Validate(); err != nil {
		return SendResult{}, err
	}
	if err := ValidateIPv6Address(req.Address); err != nil {
		return SendResult{}, err
	}

	file, err := os.Open(req.FilePath)
	if err != nil {
		return SendResult{}, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return SendResult{}, fmt.Errorf("stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return SendResult{}, fmt.Errorf("%q is not a regular file", req.FilePath)
	}

	meta := metadata{
		NodeName: req.NodeName,
		FileName: filepath.Base(req.FilePath),
		FileSize: info.Size(),
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp6", req.Address)
	if err != nil {
		return SendResult{}, fmt.Errorf("dial tcp6 %s: %w", req.Address, err)
	}
	defer conn.Close()

	if err := proto.WriteHeader(conn, proto.KindFileTransfer); err != nil {
		return SendResult{}, err
	}
	secureConn, err := secure.Client(conn, proto.KindFileTransfer, req.Identity)
	if err != nil {
		return SendResult{}, err
	}
	if err := writeMetadata(secureConn, meta); err != nil {
		return SendResult{}, err
	}

	buf := make([]byte, 1024) // 1 KB chunks
	var written int64

	for {
		n, err := file.Read(buf)
		if n > 0 {
			wn, writeErr := secureConn.Write(buf[:n])
			if writeErr != nil {
				return SendResult{}, fmt.Errorf("stream file to %s: %w", req.Address, writeErr)
			}

			written += int64(wn)

			if req.OnProgress != nil {
				req.OnProgress(written, info.Size())

			}
			time.Sleep(100 * time.Millisecond) // temporary, only for testing

		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return SendResult{}, fmt.Errorf("read file: %w", err)
		}
	}

	return SendResult{
		NodeName:   req.NodeName,
		FileName:   meta.FileName,
		BytesSent:  written,
		RemoteAddr: conn.RemoteAddr().String(),
	}, nil
}

func ReceiveFile(r io.Reader, dataDir string, onProgress func(received, total int64)) (ReceiveResult, error) {
	meta, err := readMetadata(r)
	if err != nil {
		return ReceiveResult{}, err
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return ReceiveResult{}, fmt.Errorf("create data directory: %w", err)
	}

	targetPath := filepath.Join(dataDir, filepath.Base(meta.FileName))
	outFile, err := os.Create(targetPath)
	if err != nil {
		return ReceiveResult{}, fmt.Errorf("create output file: %w", err)
	}
	defer outFile.Close()

	buf := make([]byte, 1024)
	var written int64

	for written < meta.FileSize {
		n, err := r.Read(buf)
		if n > 0 {
			wn, writeErr := outFile.Write(buf[:n])
			if writeErr != nil {
				return ReceiveResult{}, fmt.Errorf("write output file: %w", writeErr)
			}

			written += int64(wn)

			if onProgress != nil {
				onProgress(written, meta.FileSize)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return ReceiveResult{}, fmt.Errorf("read payload: %w", err)
		}
	}

	return ReceiveResult{
		SenderNode:    meta.NodeName,
		FileName:      meta.FileName,
		BytesReceived: written,
		StoredPath:    targetPath,
	}, nil
}

func ValidateIPv6Address(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", address, err)
	}

	ip := net.ParseIP(host)
	if ip == nil || ip.To4() != nil {
		return fmt.Errorf("address %q is not an IPv6 endpoint", address)
	}

	return nil
}

func writeMetadata(w io.Writer, meta metadata) error {
	header, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if len(header) > maxHeaderSize {
		return fmt.Errorf("metadata too large")
	}
	return proto.WriteLengthPrefixed(w, header)
}

func readMetadata(r io.Reader) (metadata, error) {
	header, err := proto.ReadLengthPrefixed(r, maxHeaderSize)
	if err != nil {
		return metadata{}, err
	}

	var meta metadata
	if err := json.Unmarshal(header, &meta); err != nil {
		return metadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	if meta.NodeName == "" {
		return metadata{}, fmt.Errorf("metadata missing node_name")
	}
	if meta.FileName == "" {
		return metadata{}, fmt.Errorf("metadata missing file_name")
	}
	if meta.FileSize < 0 {
		return metadata{}, fmt.Errorf("metadata contains invalid file_size")
	}

	return meta, nil
}
