package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type vectorFile struct {
	Vectors []struct {
		Name    string         `json:"name"`
		Payload map[string]any `json:"payload"`
	} `json:"vectors"`
}

func TestWireVectorsAreValidJSONObjects(t *testing.T) {
	path := filepath.Join("testdata", "wire_vectors.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var vf vectorFile
	if err := json.Unmarshal(raw, &vf); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}
	if len(vf.Vectors) == 0 {
		t.Fatal("no vectors found")
	}
	for _, v := range vf.Vectors {
		if v.Name == "" {
			t.Fatal("vector with empty name")
		}
		if len(v.Payload) == 0 {
			t.Fatalf("vector %q has empty payload", v.Name)
		}
		if ver, ok := v.Payload["version"]; !ok || ver == nil {
			t.Fatalf("vector %q missing version field", v.Name)
		}
	}
}

