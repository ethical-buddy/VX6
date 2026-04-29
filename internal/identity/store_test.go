package identity

import (
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
