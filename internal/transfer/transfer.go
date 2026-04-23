package transfer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/secure"
)

type SendRequest struct {
	NodeName string
	FilePath string
	Address  string
	Identity identity.Identity
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
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp6", req.Address)
	if err != nil {
		return SendResult{}, fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	return SendFileWithConn(ctx, conn, req)
}

func SendFileWithConn(ctx context.Context, conn net.Conn, req SendRequest) (SendResult, error) {
	if req.NodeName == "" {
		return SendResult{}, fmt.Errorf("node name cannot be empty")
	}
	if err := req.Identity.Validate(); err != nil {
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

	meta := metadata{
		NodeName: req.NodeName,
		FileName: filepath.Base(req.FilePath),
		FileSize: info.Size(),
	}

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

	written, err := io.Copy(secureConn, file)
	if err != nil {
		return SendResult{}, fmt.Errorf("stream file: %w", err)
	}

	return SendResult{
		FileName:   meta.FileName,
		BytesSent:  written,
		RemoteAddr: conn.RemoteAddr().String(),
	}, nil
}

func ReceiveFile(conn net.Conn, dataDir string) (ReceiveResult, error) {
	meta, err := readMetadata(conn)
	if err != nil {
		return ReceiveResult{}, err
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return ReceiveResult{}, fmt.Errorf("create directory: %w", err)
	}

	filePath := filepath.Join(dataDir, meta.FileName)
	file, err := os.Create(filePath)
	if err != nil {
		return ReceiveResult{}, fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	n, err := io.Copy(file, conn)
	if err != nil {
		return ReceiveResult{}, fmt.Errorf("receive stream: %w", err)
	}

	return ReceiveResult{
		SenderNode:    meta.NodeName,
		FileName:      meta.FileName,
		BytesReceived: n,
		StoredPath:    filePath,
	}, nil
}

type metadata struct {
	NodeName string `json:"node_name"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
}

func writeMetadata(w io.Writer, meta metadata) error {
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return proto.WriteLengthPrefixed(w, payload)
}

func readMetadata(r io.Reader) (metadata, error) {
	payload, err := proto.ReadLengthPrefixed(r, 1024*1024)
	if err != nil {
		return metadata{}, err
	}
	var meta metadata
	if err := json.Unmarshal(payload, &meta); err != nil {
		return metadata{}, err
	}
	return meta, nil
}

func ValidateIPv6Address(address string) error {
	_, _, err := net.SplitHostPort(address)
	return err
}
