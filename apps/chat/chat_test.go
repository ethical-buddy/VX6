package main

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/vx6/vx6/internal/config"
	"github.com/vx6/vx6/internal/dht"
	"github.com/vx6/vx6/internal/discovery"
	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/node"
)

func TestDMConversationIDStable(t *testing.T) {
	t.Parallel()

	if got, want := dmConversationID("bob", "alice"), dmConversationID("alice", "bob"); got != want {
		t.Fatalf("dm conversation id should be stable, got %q want %q", got, want)
	}
}

func TestStoreDeduplicatesMessages(t *testing.T) {
	t.Parallel()

	store, err := newChatStore(filepath.Join(t.TempDir(), "chat-state.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	conv, err := store.ensureConversation(&Conversation{
		ID:      "group_demo",
		Kind:    "group",
		Title:   "demo",
		Members: []string{"alice", "bob"},
	})
	if err != nil {
		t.Fatalf("ensure conversation: %v", err)
	}

	msg := &Message{
		ID:             "m1",
		ConversationID: conv.ID,
		Kind:           conv.Kind,
		Title:          conv.Title,
		Members:        conv.Members,
		From:           "alice",
		Body:           "hello",
		SentAt:         time.Now().UTC().Format(time.RFC3339),
	}
	if added, err := store.acceptRemoteMessage(msg); err != nil || !added {
		t.Fatalf("first accept: added=%v err=%v", added, err)
	}
	if added, err := store.acceptRemoteMessage(msg); err != nil || added {
		t.Fatalf("second accept should dedupe: added=%v err=%v", added, err)
	}

	state := store.snapshot("alice", "127.0.0.1:0", "127.0.0.1:0", nil)
	if len(state.Conversations) != 1 || len(state.Conversations[0].Messages) != 1 {
		t.Fatalf("unexpected store snapshot: %+v", state)
	}
}

func TestVX6ChatDirectAndGroup(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	baseDir := t.TempDir()
	bootstrap := startTestVX6Node(t, rootCtx, filepath.Join(baseDir, "bootstrap"), "bootstrap", nil)
	aliceNode := startTestVX6Node(t, rootCtx, filepath.Join(baseDir, "alice"), "alice", []string{bootstrap.listenAddr})
	bobNode := startTestVX6Node(t, rootCtx, filepath.Join(baseDir, "bob"), "bob", []string{bootstrap.listenAddr})
	carolNode := startTestVX6Node(t, rootCtx, filepath.Join(baseDir, "carol"), "carol", []string{bootstrap.listenAddr})

	aliceChat := startTestChatApp(t, rootCtx, aliceNode.configPath)
	bobChat := startTestChatApp(t, rootCtx, bobNode.configPath)
	carolChat := startTestChatApp(t, rootCtx, carolNode.configPath)

	waitForCondition(t, 8*time.Second, func() bool {
		_, services := bootstrap.registry.Snapshot()
		found := map[string]bool{}
		for _, svc := range services {
			if svc.ServiceName == chatServiceName {
				found[svc.NodeName] = true
			}
		}
		return found["alice"] && found["bob"] && found["carol"]
	}, "chat service records to converge")

	dm, err := aliceChat.store.ensureDM("alice", "bob")
	if err != nil {
		t.Fatalf("ensure dm: %v", err)
	}
	if err := aliceChat.sendConversationMessage(rootCtx, dm.ID, "hello bob from alice"); err != nil {
		t.Fatalf("send dm: %v", err)
	}
	waitForCondition(t, 8*time.Second, func() bool {
		return conversationHasMessage(bobChat, dm.ID, "hello bob from alice", "alice")
	}, "direct message delivery")

	group, err := aliceChat.createGroup("ops", []string{"bob", "carol"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := aliceChat.sendConversationMessage(rootCtx, group.ID, "group hello"); err != nil {
		t.Fatalf("send group message: %v", err)
	}
	waitForCondition(t, 8*time.Second, func() bool {
		return conversationHasMessage(bobChat, group.ID, "group hello", "alice") &&
			conversationHasMessage(carolChat, group.ID, "group hello", "alice")
	}, "group message delivery")
}

type testNode struct {
	cancel     context.CancelFunc
	configPath string
	listenAddr string
	registry   *discovery.Registry
}

func startTestVX6Node(t *testing.T, parent context.Context, dir, name string, bootstraps []string) testNode {
	t.Helper()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	dataDir := filepath.Join(dir, "data")
	registry, err := discovery.NewRegistry(filepath.Join(dataDir, "registry.json"))
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	listenAddr := reserveTCPAddr(t, "[::1]:0")
	configPath := filepath.Join(dir, "config.json")
	store, err := config.NewStore(configPath)
	if err != nil {
		t.Fatalf("new config store: %v", err)
	}
	cfgFile := config.File{
		Node: config.NodeConfig{
			Name:           name,
			ListenAddr:     listenAddr,
			AdvertiseAddr:  listenAddr,
			DataDir:        dataDir,
			BootstrapAddrs: append([]string(nil), bootstraps...),
		},
		Peers:    map[string]config.PeerEntry{},
		Services: map[string]config.ServiceEntry{},
	}
	if err := store.Save(cfgFile); err != nil {
		t.Fatalf("save config: %v", err)
	}
	idStore, err := identity.NewStoreForConfig(configPath)
	if err != nil {
		t.Fatalf("new identity store: %v", err)
	}
	if err := idStore.Save(id); err != nil {
		t.Fatalf("save identity: %v", err)
	}

	nodeCtx, cancel := context.WithCancel(parent)
	go func() {
		_ = node.Run(nodeCtx, io.Discard, node.Config{
			Name:           name,
			NodeID:         id.NodeID,
			ListenAddr:     listenAddr,
			AdvertiseAddr:  listenAddr,
			DataDir:        dataDir,
			ConfigPath:     configPath,
			BootstrapAddrs: append([]string(nil), bootstraps...),
			Services:       map[string]string{},
			Identity:       id,
			Registry:       registry,
			DHT:            dht.NewServerWithIdentity(id),
		})
	}()

	waitForCondition(t, 3*time.Second, func() bool {
		conn, err := net.DialTimeout("tcp6", listenAddr, 50*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, name+" listener")

	return testNode{
		cancel:     cancel,
		configPath: configPath,
		listenAddr: listenAddr,
		registry:   registry,
	}
}

type runningChat struct {
	*App
	cancel context.CancelFunc
	done   chan error
}

func startTestChatApp(t *testing.T, parent context.Context, configPath string) runningChat {
	t.Helper()

	ctx, cancel := context.WithCancel(parent)
	app, err := New(io.Discard, Options{
		ConfigPath:      configPath,
		HTTPAddr:        "127.0.0.1:0",
		TransportAddr:   "127.0.0.1:0",
		PublishInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("new chat app: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	waitForCondition(t, 3*time.Second, func() bool {
		return app.httpAddr != "" && app.transportAddr != ""
	}, "chat app listeners")

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("chat app shutdown timed out")
		}
	})

	return runningChat{App: app, cancel: cancel, done: done}
}

func conversationHasMessage(app runningChat, conversationID, body, from string) bool {
	state := app.currentState()
	for _, conv := range state.Conversations {
		if conv.ID != conversationID {
			continue
		}
		for _, msg := range conv.Messages {
			if msg.Body == body && msg.From == from {
				return true
			}
		}
	}
	return false
}

func reserveTCPAddr(t *testing.T, networkAddr string) string {
	t.Helper()

	network := "tcp"
	if networkAddr != "" && networkAddr[0] == '[' {
		network = "tcp6"
	}

	ln, err := net.Listen(network, networkAddr)
	if err != nil {
		t.Fatalf("reserve address %s: %v", networkAddr, err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool, label string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", label)
}
