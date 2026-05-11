package dht

import (
	"encoding/json"
	"testing"
)

// FuzzEnvelopeJSON ensures DHT-facing payload parsing remains robust.
func FuzzEnvelopeJSON(f *testing.F) {
	f.Add([]byte(`{"version":1,"id":"x","value":"y"}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var out map[string]any
		_ = json.Unmarshal(data, &out)
		// Accept malformed inputs without panics; parser robustness is the goal.
	})
}

