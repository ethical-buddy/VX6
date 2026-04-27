package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreEnsureRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewStore(filepath.Join(t.TempDir(), "identity.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	first, created, err := store.Ensure()
	if err != nil {
		t.Fatalf("ensure first identity: %v", err)
	}
	if !created {
		t.Fatal("expected first ensure to create identity")
	}

	second, created, err := store.Ensure()
	if err != nil {
		t.Fatalf("ensure second identity: %v", err)
	}
	if created {
		t.Fatal("expected second ensure to reuse identity")
	}
	if first.NodeID != second.NodeID {
		t.Fatalf("unexpected node id mismatch %q != %q", first.NodeID, second.NodeID)
	}
}

func TestNewStoreForConfigUsesDistinctIdentityFilesForCustomConfigs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	firstStore, err := NewStoreForConfig(filepath.Join(dir, "bootstrap.json"))
	if err != nil {
		t.Fatalf("new first store: %v", err)
	}
	secondStore, err := NewStoreForConfig(filepath.Join(dir, "client.json"))
	if err != nil {
		t.Fatalf("new second store: %v", err)
	}

	if firstStore.Path() == secondStore.Path() {
		t.Fatalf("expected distinct identity paths, got %s", firstStore.Path())
	}

	first, _, err := firstStore.Ensure()
	if err != nil {
		t.Fatalf("ensure first identity: %v", err)
	}
	second, _, err := secondStore.Ensure()
	if err != nil {
		t.Fatalf("ensure second identity: %v", err)
	}

	if first.NodeID == second.NodeID {
		t.Fatalf("expected distinct node ids, both were %s", first.NodeID)
	}

	if _, err := os.Stat(firstStore.Path()); err != nil {
		t.Fatalf("stat first identity path: %v", err)
	}
	if _, err := os.Stat(secondStore.Path()); err != nil {
		t.Fatalf("stat second identity path: %v", err)
	}
}

func TestDefaultPathUsesHomeConfigDirectory(t *testing.T) {
	t.Setenv("HOME", "/tmp/vx6-home")

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("default path: %v", err)
	}
	if path != "/tmp/vx6-home/.config/vx6/identity.json" {
		t.Fatalf("unexpected default identity path %q", path)
	}
}
