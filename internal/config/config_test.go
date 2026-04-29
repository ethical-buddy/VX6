package config

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	cfg.Node.Name = "alpha"
	cfg.Peers["beta"] = PeerEntry{Address: "[2001:db8::2]:4242"}

	if err := store.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Node.Name != "alpha" {
		t.Fatalf("unexpected node name %q", loaded.Node.Name)
	}
	if loaded.Peers["beta"].Address != "[2001:db8::2]:4242" {
		t.Fatalf("unexpected peer address %q", loaded.Peers["beta"].Address)
	}
}

func TestRuntimePIDPathUsesConfigDirectory(t *testing.T) {
	t.Parallel()

	path, err := RuntimePIDPath("/tmp/vx6/config.json")
	if err != nil {
		t.Fatalf("runtime pid path: %v", err)
	}
	if path != "/tmp/vx6/node.pid" {
		t.Fatalf("unexpected pid path %q", path)
	}
}

func TestDefaultPathsUseHomeDirectory(t *testing.T) {
	t.Setenv("HOME", "/tmp/vx6-home")

	configPath, err := DefaultPath()
	if err != nil {
		t.Fatalf("default config path: %v", err)
	}
	if configPath != "/tmp/vx6-home/.config/vx6/config.json" {
		t.Fatalf("unexpected config path %q", configPath)
	}

	dataDir, err := DefaultDataDir()
	if err != nil {
		t.Fatalf("default data dir: %v", err)
	}
	if dataDir != "/tmp/vx6-home/.local/share/vx6" {
		t.Fatalf("unexpected data dir %q", dataDir)
	}

	downloadDir, err := DefaultDownloadDir()
	if err != nil {
		t.Fatalf("default download dir: %v", err)
	}
	if downloadDir != "/tmp/vx6-home/Downloads" {
		t.Fatalf("unexpected download dir %q", downloadDir)
	}
}
