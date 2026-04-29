package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vx6/vx6/internal/config"
	"github.com/vx6/vx6/internal/dht"
	"github.com/vx6/vx6/internal/discovery"
	"github.com/vx6/vx6/internal/hidden"
	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/node"
	"github.com/vx6/vx6/internal/onion"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/serviceproxy"
)

func TestSixteenNodeSwarmServiceAndProxy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node integration test in short mode")
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	baseDir := t.TempDir()
	bootstrap := startVX6Node(t, rootCtx, filepath.Join(baseDir, "bootstrap"), "bootstrap", nil, nil)

	echoAddr := reserveTCPAddr(t, "127.0.0.1:0")
	startEchoServer(t, rootCtx, echoAddr)

	owner := startVX6Node(
		t,
		rootCtx,
		filepath.Join(baseDir, "owner"),
		"owner",
		[]string{bootstrap.listenAddr},
		map[string]string{"echo": echoAddr},
	)
	client := startVX6Node(t, rootCtx, filepath.Join(baseDir, "client"), "client", []string{bootstrap.listenAddr}, nil)

	for i := 0; i < 13; i++ {
		startVX6Node(
			t,
			rootCtx,
			filepath.Join(baseDir, fmt.Sprintf("relay-%02d", i+1)),
			fmt.Sprintf("relay-%02d", i+1),
			[]string{bootstrap.listenAddr},
			nil,
		)
	}

	waitForCondition(t, 10*time.Second, func() bool {
		nodes, services := bootstrap.registry.Snapshot()
		return len(nodes) >= 16 && len(services) >= 1
	}, "bootstrap registry to converge with 16 nodes")

	records, services, err := discovery.Snapshot(rootCtx, bootstrap.listenAddr)
	if err != nil {
		t.Fatalf("snapshot bootstrap registry: %v", err)
	}
	if got := len(records); got < 16 {
		t.Fatalf("expected at least 16 discovered nodes, got %d", got)
	}
	if err := client.registry.Import(records, services); err != nil {
		t.Fatalf("import client registry: %v", err)
	}

	serviceName := record.FullServiceName(owner.name, "echo")
	serviceRec, err := client.registry.ResolveServiceLocal(serviceName)
	if err != nil {
		t.Fatalf("resolve service from client registry: %v", err)
	}

	dhtClient := dht.NewServer("observer")
	dhtClient.RT.AddNode(proto.NodeInfo{ID: "seed:" + bootstrap.listenAddr, Addr: bootstrap.listenAddr})

	waitForCondition(t, 5*time.Second, func() bool {
		value, err := dhtClient.RecursiveFindValue(rootCtx, dht.ServiceKey(serviceName))
		if err != nil || value == "" {
			return false
		}
		var got record.ServiceRecord
		return json.Unmarshal([]byte(value), &got) == nil && got.ServiceName == "echo" && got.NodeName == owner.name
	}, "service record in DHT")

	endpointValue, err := dhtClient.RecursiveFindValue(rootCtx, dht.NodeNameKey(owner.name))
	if err != nil {
		t.Fatalf("resolve endpoint from DHT: %v", err)
	}
	var endpointRec record.EndpointRecord
	if err := json.Unmarshal([]byte(endpointValue), &endpointRec); err != nil {
		t.Fatalf("decode endpoint DHT value: %v", err)
	}
	if endpointRec.NodeName != owner.name || endpointRec.Address != owner.listenAddr {
		t.Fatalf("unexpected endpoint record from DHT: %+v", endpointRec)
	}

	directAddr := reserveTCPAddr(t, "127.0.0.1:0")
	directCtx, directCancel := context.WithCancel(rootCtx)
	directDone := make(chan error, 1)
	go func() {
		directDone <- serviceproxy.ServeLocalForward(directCtx, directAddr, serviceRec, client.id, func(ctx context.Context) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp6", serviceRec.Address)
		})
	}()
	assertEchoEventually(t, directAddr, "direct service path")
	directCancel()
	waitForShutdown(t, directDone)

	onionPeers := filterPeers(records, func(rec record.EndpointRecord) bool {
		return rec.NodeID != owner.id.NodeID && rec.NodeID != client.id.NodeID
	})
	if len(onionPeers) < 5 {
		t.Fatalf("expected at least 5 relay candidates, got %d", len(onionPeers))
	}

	onionAddr := reserveTCPAddr(t, "127.0.0.1:0")
	onionCtx, onionCancel := context.WithCancel(rootCtx)
	onionDone := make(chan error, 1)
	go func() {
		onionDone <- serviceproxy.ServeLocalForward(onionCtx, onionAddr, serviceRec, client.id, func(ctx context.Context) (net.Conn, error) {
			return onion.BuildAutomatedCircuit(ctx, serviceRec, onionPeers)
		})
	}()
	assertEchoEventually(t, onionAddr, "five-hop proxy path")
	onionCancel()
	waitForShutdown(t, onionDone)
}

func TestDirectIPv6ServiceWithoutBootstrap(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	baseDir := t.TempDir()
	echoAddr := reserveTCPAddr(t, "127.0.0.1:0")
	startEchoServer(t, rootCtx, echoAddr)

	owner := startVX6Node(t, rootCtx, filepath.Join(baseDir, "owner"), "owner", nil, map[string]string{"echo": echoAddr})
	client := startVX6Node(t, rootCtx, filepath.Join(baseDir, "client"), "client", nil, nil)

	directRec := record.ServiceRecord{
		NodeName:    "direct",
		ServiceName: "echo",
		Address:     owner.listenAddr,
	}

	directAddr := reserveTCPAddr(t, "127.0.0.1:0")
	directCtx, directCancel := context.WithCancel(rootCtx)
	directDone := make(chan error, 1)
	go func() {
		directDone <- serviceproxy.ServeLocalForward(directCtx, directAddr, directRec, client.id, func(ctx context.Context) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp6", owner.listenAddr)
		})
	}()

	assertEchoEventually(t, directAddr, "direct service without bootstrap")
	directCancel()
	waitForShutdown(t, directDone)
}

func TestBootstrapLossStillAllowsRepublish(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bootstrap-loss integration test in short mode")
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	baseDir := t.TempDir()
	bootstrap := startVX6Node(t, rootCtx, filepath.Join(baseDir, "bootstrap"), "bootstrap", nil, nil)

	echoAddr1 := reserveTCPAddr(t, "127.0.0.1:0")
	startEchoServer(t, rootCtx, echoAddr1)
	client := startVX6Node(t, rootCtx, filepath.Join(baseDir, "client"), "client", []string{bootstrap.listenAddr}, nil)
	for i := 0; i < 6; i++ {
		startVX6Node(t, rootCtx, filepath.Join(baseDir, fmt.Sprintf("mesh-%02d", i+1)), fmt.Sprintf("mesh-%02d", i+1), []string{bootstrap.listenAddr}, nil)
	}
	owner := startVX6Node(t, rootCtx, filepath.Join(baseDir, "owner"), "owner", []string{bootstrap.listenAddr}, map[string]string{"echo": echoAddr1})

	waitForCondition(t, 10*time.Second, func() bool {
		nodes, services := bootstrap.registry.Snapshot()
		return len(nodes) >= 9 && len(services) >= 1
	}, "bootstrap mesh convergence")

	records, services, err := discovery.Snapshot(rootCtx, bootstrap.listenAddr)
	if err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}
	if err := client.registry.Import(records, services); err != nil {
		t.Fatalf("import client registry: %v", err)
	}

	bootstrap.cancel()
	time.Sleep(200 * time.Millisecond)

	echoAddr2 := reserveTCPAddr(t, "127.0.0.1:0")
	startEchoServer(t, rootCtx, echoAddr2)
	owner.cancel()
	time.Sleep(200 * time.Millisecond)

	ownerRestarted := startVX6NodeWithOptions(t, rootCtx, nodeOptions{
		dir:           filepath.Join(baseDir, "owner"),
		name:          "owner",
		bootstraps:    []string{bootstrap.listenAddr},
		services:      map[string]string{"echo": echoAddr2},
		identity:      owner.id,
		reuseIdentity: true,
	})

	waitForCondition(t, 10*time.Second, func() bool {
		nodes, _ := ownerRestarted.registry.Snapshot()
		return len(nodes) >= 2
	}, "owner restart peer cache")

	var cachedPeer record.EndpointRecord
	waitForCondition(t, 10*time.Second, func() bool {
		nodes, _ := ownerRestarted.registry.Snapshot()
		for _, node := range nodes {
			if node.NodeName != "owner" && node.NodeName != "bootstrap" && node.Address != "" {
				cachedPeer = node
				return true
			}
		}
		return false
	}, "owner restart cached peer after bootstrap shutdown")

	waitForCondition(t, 10*time.Second, func() bool {
		rec, err := discovery.Resolve(rootCtx, cachedPeer.Address, "owner", "")
		return err == nil && rec.Address == ownerRestarted.listenAddr
	}, "peer-to-peer owner record update after bootstrap loss")

	dhtClient := dht.NewServer("post-bootstrap-client")
	dhtClient.RT.AddNode(proto.NodeInfo{ID: "peer:" + cachedPeer.Address, Addr: cachedPeer.Address})
	waitForCondition(t, 10*time.Second, func() bool {
		value, err := dhtClient.RecursiveFindValue(rootCtx, dht.ServiceKey("owner.echo"))
		if err != nil || value == "" {
			return false
		}
		var rec record.ServiceRecord
		return json.Unmarshal([]byte(value), &rec) == nil && rec.Address == ownerRestarted.listenAddr
	}, "DHT service update after bootstrap loss")
}

func TestDeadBootstrapIsSkippedWhileHealthyBootstrapStillSyncs(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	baseDir := t.TempDir()
	bootstrap := startVX6Node(t, rootCtx, filepath.Join(baseDir, "bootstrap"), "bootstrap", nil, nil)
	deadBootstrap := startBlackholeServer(t, rootCtx)

	owner := startVX6Node(
		t,
		rootCtx,
		filepath.Join(baseDir, "owner"),
		"owner",
		[]string{deadBootstrap, bootstrap.listenAddr},
		nil,
	)

	waitForCondition(t, 5*time.Second, func() bool {
		rec, err := discovery.Resolve(rootCtx, bootstrap.listenAddr, owner.name, "")
		return err == nil && rec.Address == owner.listenAddr
	}, "owner sync through healthy bootstrap while dead bootstrap is skipped")
}

func TestHiddenServiceRendezvousPlainTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping hidden-service integration test in short mode")
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	baseDir := t.TempDir()
	bootstrap := startVX6Node(t, rootCtx, filepath.Join(baseDir, "bootstrap"), "bootstrap", nil, nil)

	echoAddr := reserveTCPAddr(t, "127.0.0.1:0")
	startEchoServer(t, rootCtx, echoAddr)

	var (
		traceMu sync.Mutex
		traces  []onion.TraceEvent
	)
	restoreTrace := onion.SetTraceHook(func(ev onion.TraceEvent) {
		traceMu.Lock()
		traces = append(traces, ev)
		traceMu.Unlock()
	})
	defer restoreTrace()

	client := startVX6Node(t, rootCtx, filepath.Join(baseDir, "client"), "client", []string{bootstrap.listenAddr}, nil)
	for i := 0; i < 14; i++ {
		startVX6Node(t, rootCtx, filepath.Join(baseDir, fmt.Sprintf("relay-%02d", i+1)), fmt.Sprintf("relay-%02d", i+1), []string{bootstrap.listenAddr}, nil)
	}
	owner := startVX6NodeWithOptions(t, rootCtx, nodeOptions{
		dir:            filepath.Join(baseDir, "hidden-owner"),
		name:           "hidden-owner",
		bootstraps:     []string{bootstrap.listenAddr},
		services:       map[string]string{"ghost": echoAddr},
		hiddenServices: map[string]bool{"ghost": true},
		hideEndpoint:   true,
	})

	waitForCondition(t, 10*time.Second, func() bool {
		nodes, services := bootstrap.registry.Snapshot()
		return len(nodes) >= 16 && len(services) >= 1
	}, "hidden-service mesh convergence")

	records, services, err := discovery.Snapshot(rootCtx, bootstrap.listenAddr)
	if err != nil {
		t.Fatalf("snapshot hidden-service bootstrap registry: %v", err)
	}
	if err := client.registry.Import(records, services); err != nil {
		t.Fatalf("import client hidden registry: %v", err)
	}

	serviceName := "ghost"
	waitForCondition(t, 10*time.Second, func() bool {
		rec, err := client.registry.ResolveServiceLocal(serviceName)
		return err == nil && rec.IsHidden && rec.Address == "" && len(rec.IntroPoints) == 3 && len(rec.StandbyIntroPoints) == 2
	}, "hidden service descriptor publication")

	if _, err := discovery.Resolve(rootCtx, bootstrap.listenAddr, owner.name, ""); err == nil {
		t.Fatalf("hidden owner endpoint should not be published publicly")
	}

	serviceRec, err := client.registry.ResolveServiceLocal(serviceName)
	if err != nil {
		t.Fatalf("resolve hidden service: %v", err)
	}

	dhtClient := dht.NewServer("hidden-observer")
	dhtClient.RT.AddNode(proto.NodeInfo{ID: "seed:" + bootstrap.listenAddr, Addr: bootstrap.listenAddr})
	waitForCondition(t, 10*time.Second, func() bool {
		value, err := dhtClient.RecursiveFindValue(rootCtx, dht.HiddenServiceKey(serviceName))
		if err != nil || value == "" {
			return false
		}
		var rec record.ServiceRecord
		return json.Unmarshal([]byte(value), &rec) == nil && rec.IsHidden && len(rec.IntroPoints) == 3 && len(rec.StandbyIntroPoints) == 2 && rec.Address == ""
	}, "hidden service descriptor in DHT")

	hiddenAddr := reserveTCPAddr(t, "127.0.0.1:0")
	hiddenCtx, hiddenCancel := context.WithCancel(rootCtx)
	hiddenDone := make(chan error, 1)
	go func() {
		hiddenDone <- serviceproxy.ServeLocalForward(hiddenCtx, hiddenAddr, serviceRec, client.id, func(ctx context.Context) (net.Conn, error) {
			return hidden.DialHiddenServiceWithOptions(ctx, serviceRec, client.registry, hidden.DialOptions{SelfAddr: client.listenAddr})
		})
	}()
	assertEchoEventually(t, hiddenAddr, "hidden rendezvous path")

	waitForCondition(t, 5*time.Second, func() bool {
		traceMu.Lock()
		defer traceMu.Unlock()
		return len(traces) >= 2
	}, "hidden-service circuit traces")

	hiddenCancel()
	waitForShutdown(t, hiddenDone)

	traceMu.Lock()
	gotTraces := append([]onion.TraceEvent(nil), traces...)
	traceMu.Unlock()
	if len(gotTraces) < 2 {
		t.Fatalf("expected at least two hidden-service circuit traces, got %d", len(gotTraces))
	}
	gotTraces = gotTraces[len(gotTraces)-2:]
	if gotTraces[0].TargetAddr == "" || gotTraces[0].TargetAddr != gotTraces[1].TargetAddr {
		t.Fatalf("expected both hidden-service circuits to share one rendezvous target, got %q and %q", gotTraces[0].TargetAddr, gotTraces[1].TargetAddr)
	}
	if len(gotTraces[0].RelayAddrs) != 3 || len(gotTraces[1].RelayAddrs) != 3 {
		t.Fatalf("expected 3 relay hops on both sides, got %d and %d", len(gotTraces[0].RelayAddrs), len(gotTraces[1].RelayAddrs))
	}

	seenRelays := map[string]struct{}{}
	for _, trace := range gotTraces {
		for _, addr := range trace.RelayAddrs {
			if _, ok := seenRelays[addr]; ok {
				t.Fatalf("expected disjoint hidden-service relay sets, duplicate %s detected", addr)
			}
			if addr == client.listenAddr {
				t.Fatalf("requester node should not be reused as a hidden-service relay: %s", addr)
			}
			seenRelays[addr] = struct{}{}
		}
	}
	if len(seenRelays) != 6 {
		t.Fatalf("expected 6 unique hidden-service relays, got %d", len(seenRelays))
	}
}

type runningNode struct {
	cancel     context.CancelFunc
	configPath string
	id         identity.Identity
	dht        *dht.Server
	name       string
	listenAddr string
	registry   *discovery.Registry
}

func startVX6Node(t *testing.T, parent context.Context, dir, name string, bootstraps []string, services map[string]string) runningNode {
	return startVX6NodeWithOptions(t, parent, nodeOptions{
		dir:        dir,
		name:       name,
		bootstraps: bootstraps,
		services:   services,
	})
}

type nodeOptions struct {
	dir            string
	name           string
	bootstraps     []string
	services       map[string]string
	hiddenServices map[string]bool
	hideEndpoint   bool
	identity       identity.Identity
	reuseIdentity  bool
}

func startVX6NodeWithOptions(t *testing.T, parent context.Context, opts nodeOptions) runningNode {
	t.Helper()

	id := opts.identity
	var err error
	if !opts.reuseIdentity {
		id, err = identity.Generate()
		if err != nil {
			t.Fatalf("generate identity for %s: %v", opts.name, err)
		}
	}

	dataDir := filepath.Join(opts.dir, "data")
	registry, err := discovery.NewRegistry(filepath.Join(dataDir, "registry.json"))
	if err != nil {
		t.Fatalf("new registry for %s: %v", opts.name, err)
	}

	listenAddr := reserveTCPAddr(t, "[::1]:0")
	configPath := filepath.Join(opts.dir, "config.json")
	store, err := config.NewStore(configPath)
	if err != nil {
		t.Fatalf("new config store for %s: %v", opts.name, err)
	}
	cfgFile := config.File{
		Node: config.NodeConfig{
			Name:           opts.name,
			ListenAddr:     listenAddr,
			AdvertiseAddr:  listenAddr,
			HideEndpoint:   opts.hideEndpoint,
			DataDir:        dataDir,
			BootstrapAddrs: append([]string(nil), opts.bootstraps...),
		},
		Peers:    map[string]config.PeerEntry{},
		Services: map[string]config.ServiceEntry{},
	}
	for serviceName, target := range opts.services {
		cfgFile.Services[serviceName] = config.ServiceEntry{
			Target:   target,
			IsHidden: opts.hiddenServices[serviceName],
		}
	}
	if err := store.Save(cfgFile); err != nil {
		t.Fatalf("save config for %s: %v", opts.name, err)
	}

	nodeCtx, cancel := context.WithCancel(parent)
	cfg := node.Config{
		Name:           opts.name,
		NodeID:         id.NodeID,
		ListenAddr:     listenAddr,
		AdvertiseAddr:  listenAddr,
		HideEndpoint:   opts.hideEndpoint,
		DataDir:        dataDir,
		ConfigPath:     configPath,
		BootstrapAddrs: append([]string(nil), opts.bootstraps...),
		Services:       cloneServices(opts.services),
		DHT:            dht.NewServer(id.NodeID),
		Identity:       id,
		Registry:       registry,
		RefreshServices: func() map[string]string {
			c, err := store.Load()
			if err != nil {
				return cloneServices(opts.services)
			}
			out := make(map[string]string, len(c.Services))
			for name, svc := range c.Services {
				out[name] = svc.Target
			}
			return out
		},
	}

	go func() {
		_ = node.Run(nodeCtx, io.Discard, cfg)
	}()

	waitForCondition(t, 3*time.Second, func() bool {
		conn, err := net.DialTimeout("tcp6", listenAddr, 50*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, fmt.Sprintf("%s listener", opts.name))

	return runningNode{
		cancel:     cancel,
		configPath: configPath,
		id:         id,
		dht:        cfg.DHT,
		name:       opts.name,
		listenAddr: listenAddr,
		registry:   registry,
	}
}

func startEchoServer(t *testing.T, ctx context.Context, addr string) {
	t.Helper()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen echo server: %v", err)
	}

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(conn)
		}
	}()
}

func startBlackholeServer(t *testing.T, ctx context.Context) string {
	t.Helper()

	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatalf("listen blackhole server: %v", err)
	}

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				<-ctx.Done()
			}(conn)
		}
	}()

	return ln.Addr().String()
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
	t.Fatalf("timed out waiting for %s", label)
}

func waitForShutdown(t *testing.T, done <-chan error) {
	t.Helper()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("forwarder shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for forwarder shutdown")
	}
}

func assertEchoRoundTrip(t *testing.T, addr, message string) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial forwarder %s: %v", addr, err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, message); err != nil {
		t.Fatalf("write echo payload: %v", err)
	}

	buf := make([]byte, len(message))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo payload: %v", err)
	}
	if got := string(buf); got != message {
		t.Fatalf("unexpected echo payload %q, want %q", got, message)
	}
}

func assertEchoEventually(t *testing.T, addr, message string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := func() error {
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err != nil {
				return err
			}
			defer conn.Close()

			if _, err := io.WriteString(conn, message); err != nil {
				return err
			}

			buf := make([]byte, len(message))
			if _, err := io.ReadFull(conn, buf); err != nil {
				return err
			}
			if got := string(buf); got != message {
				return fmt.Errorf("unexpected echo payload %q", got)
			}
			return nil
		}()
		if err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	assertEchoRoundTrip(t, addr, message)
}

func filterPeers(records []record.EndpointRecord, keep func(record.EndpointRecord) bool) []record.EndpointRecord {
	out := make([]record.EndpointRecord, 0, len(records))
	for _, rec := range records {
		if keep(rec) {
			out = append(out, rec)
		}
	}
	return out
}

func cloneServices(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
