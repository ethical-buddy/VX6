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
	if len(topology.StandbyIntros) != 1 {
		t.Fatalf("expected 1 standby intro after reserving guard capacity, got %d", len(topology.StandbyIntros))
	}
	if len(topology.Guards) != 2 {
		t.Fatalf("expected 2 guards to be reserved first, got %d", len(topology.Guards))
	}
}

func TestSelectTopologyKeepsGuardSetStable(t *testing.T) {
	t.Parallel()

	healthMu.Lock()
	for i := 0; i < 8; i++ {
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
		for i := 0; i < 8; i++ {
			delete(healthCache, "[2001:db8::"+string(rune('1'+i))+"]:4242")
		}
	})

	nodes := make([]record.EndpointRecord, 0, 8)
	for i := 0; i < 8; i++ {
		nodes = append(nodes, record.EndpointRecord{
			NodeName: "relay-" + string(rune('a'+i)),
			Address:  "[2001:db8::" + string(rune('1'+i)) + "]:4242",
		})
	}

	first := SelectTopology(context.Background(), "[2001:db8::ffff]:4242", nodes, nil, IntroModeRandom, "fast")
	second := SelectTopology(context.Background(), "[2001:db8::ffff]:4242", nodes, nil, IntroModeRandom, "fast")
	if len(first.Guards) == 0 {
		t.Fatal("expected guard set to be populated")
	}
	if len(first.Guards) != len(second.Guards) {
		t.Fatalf("expected stable guard count, got %d and %d", len(first.Guards), len(second.Guards))
	}
	for i := range first.Guards {
		if first.Guards[i] != second.Guards[i] {
			t.Fatalf("expected stable guard selection, got %#v and %#v", first.Guards, second.Guards)
		}
	}
}
