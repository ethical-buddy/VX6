package runtimectl

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestServerStatusAndReload(t *testing.T) {
	t.Parallel()

	infoPath := filepath.Join(t.TempDir(), "node.control.json")
	reloads := 0
	server, err := Start(infoPath, 4242, func() error {
		reloads++
		return nil
	}, func() Status {
		return Status{
			NodeName:         "alpha",
			EndpointPublish:  "published",
			TransportConfig:  "auto",
			TransportActive:  "tcp",
			RelayMode:        "on",
			RelayPercent:     33,
			RegistryNodes:    7,
			RegistryServices: 3,
		}
	})
	if err != nil {
		t.Fatalf("start runtime control: %v", err)
	}
	defer server.Close()

	status, err := RequestStatus(context.Background(), infoPath)
	if err != nil {
		t.Fatalf("request status: %v", err)
	}
	if status.NodeName != "alpha" {
		t.Fatalf("unexpected node name %q", status.NodeName)
	}
	if status.EndpointPublish != "published" {
		t.Fatalf("unexpected endpoint publish mode %q", status.EndpointPublish)
	}
	if status.RegistryNodes != 7 || status.RegistryServices != 3 {
		t.Fatalf("unexpected registry counters %+v", status)
	}

	if err := RequestReload(context.Background(), infoPath); err != nil {
		t.Fatalf("request reload: %v", err)
	}
	if reloads != 1 {
		t.Fatalf("expected one reload request, got %d", reloads)
	}
}

func TestRequestFailsWithBadToken(t *testing.T) {
	t.Parallel()

	infoPath := filepath.Join(t.TempDir(), "node.control.json")
	server, err := Start(infoPath, 1111, nil, func() Status { return Status{} })
	if err != nil {
		t.Fatalf("start runtime control: %v", err)
	}
	defer server.Close()

	info, err := LoadInfo(infoPath)
	if err != nil {
		t.Fatalf("load info: %v", err)
	}
	info.Token = "bad-token"
	if err := writeBadInfo(infoPath, info); err != nil {
		t.Fatalf("write bad info: %v", err)
	}

	if _, err := RequestStatus(context.Background(), infoPath); err == nil {
		t.Fatal("expected bad token to fail")
	}
}

func writeBadInfo(path string, info Info) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
