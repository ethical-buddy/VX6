package hidden

import (
	"context"
	"testing"
	"time"

	"github.com/vx6/vx6/internal/record"
)

func TestSelectTopologyManualPrefersChosenIntros(t *testing.T) {
	t.Parallel()

	healthMu.Lock()
	for i := 0; i < 6; i++ {
		addr := "[2001:db8::" + string(rune('1'+i)) + "]:4242"
		healthCache[addr] = healthEntry{
			Healthy:     true,
			RTT:         time.Duration(i+1) * time.Millisecond,
			LastChecked: time.Now(),
		}
	}
	healthMu.Unlock()
	t.Cleanup(func() {
		healthMu.Lock()
		defer healthMu.Unlock()
		for i := 0; i < 6; i++ {
			delete(healthCache, "[2001:db8::"+string(rune('1'+i))+"]:4242")
		}
	})

	var nodes []record.EndpointRecord
	for i := 0; i < 6; i++ {
		addr := "[2001:db8::" + string(rune('1'+i)) + "]:4242"
		nodes = append(nodes, record.EndpointRecord{
			NodeName:  "relay-" + string(rune('a'+i)),
			Address:   addr,
			NodeID:    "relay-id",
			PublicKey: "unused",
		})
	}

	topology := SelectTopology(
		context.Background(),
		"",
		nodes,
		[]string{"relay-c", "relay-a", "relay-b"},
		IntroModeManual,
		"fast",
	)

	if len(topology.ActiveIntros) != 3 {
		t.Fatalf("expected 3 active intros, got %d", len(topology.ActiveIntros))
	}
	if topology.ActiveIntros[0] != nodes[2].Address || topology.ActiveIntros[1] != nodes[0].Address || topology.ActiveIntros[2] != nodes[1].Address {
		t.Fatalf("manual intro selection was not preserved: %#v", topology.ActiveIntros)
	}
	if len(topology.StandbyIntros) != 2 {
		t.Fatalf("expected 2 standby intros, got %d", len(topology.StandbyIntros))
	}
	if len(topology.Guards) != 1 {
		t.Fatalf("expected 1 remaining guard candidate after reserving 5 intros, got %d", len(topology.Guards))
	}
}
