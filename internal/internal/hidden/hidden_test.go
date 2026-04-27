package hidden

import (
	"context"
	"net"
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

func TestGuardCallbackRegistrationAndForwarding(t *testing.T) {
	guardMu.Lock()
	original := guardServices
	guardServices = map[string]*guardRegistration{}
	guardMu.Unlock()
	defer func() {
		guardMu.Lock()
		guardServices = original
		guardMu.Unlock()
	}()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	scope := nodeScopedService("guard-a", "ghost")
	done := make(chan error, 1)
	go func() {
		done <- holdGuardRegistration(serverConn, scope, "cb-1")
	}()

	waitForGuardRegistration(t, scope)

	msgCh := make(chan Message, 1)
	errCh := make(chan error, 1)
	go func() {
		msg, err := readControl(clientConn)
		if err != nil {
			errCh <- err
			return
		}
		msgCh <- msg
	}()

	if err := sendGuardCallback(scope, Message{
		Action:       "intro_notify",
		Service:      "ghost",
		RendezvousID: "rv-1",
	}); err != nil {
		t.Fatalf("send guard callback: %v", err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("read callback message: %v", err)
	case msg := <-msgCh:
		if msg.Action != "intro_notify" || msg.RendezvousID != "rv-1" || msg.Service != "ghost" {
			t.Fatalf("unexpected callback message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for callback message")
	}

	_ = clientConn.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("guard registration did not shut down after connection close")
	}
}

func TestGuardCallbackScopesAreIsolatedPerNode(t *testing.T) {
	guardMu.Lock()
	original := guardServices
	guardServices = map[string]*guardRegistration{}
	guardMu.Unlock()
	defer func() {
		guardMu.Lock()
		guardServices = original
		guardMu.Unlock()
	}()

	scopeA := nodeScopedService("guard-a", "ghost")
	scopeB := nodeScopedService("guard-b", "ghost")
	leftA, rightA := net.Pipe()
	defer leftA.Close()
	defer rightA.Close()

	guardMu.Lock()
	guardServices[scopeA] = &guardRegistration{CallbackID: "cb-a", Conn: leftA}
	guardMu.Unlock()

	msgCh := make(chan Message, 1)
	go func() {
		msg, err := readControl(rightA)
		if err == nil {
			msgCh <- msg
		}
	}()

	if err := sendGuardCallback(scopeA, Message{Action: "intro_notify", Service: "ghost", RendezvousID: "rv-a"}); err != nil {
		t.Fatalf("send scoped callback: %v", err)
	}

	select {
	case msg := <-msgCh:
		if msg.RendezvousID != "rv-a" {
			t.Fatalf("unexpected scoped callback message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scoped callback message")
	}

	if err := sendGuardCallback(scopeB, Message{Action: "intro_notify", Service: "ghost"}); err == nil {
		t.Fatal("expected callback on missing guard scope to fail")
	}
}

func waitForGuardRegistration(t *testing.T, scope string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		guardMu.RLock()
		_, ok := guardServices[scope]
		guardMu.RUnlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for guard registration %q", scope)
}
