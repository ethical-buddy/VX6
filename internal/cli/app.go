package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vx6/vx6/internal/config"
	"github.com/vx6/vx6/internal/dht"
	"github.com/vx6/vx6/internal/discovery"
	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/node"
	"github.com/vx6/vx6/internal/onion"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/serviceproxy"
	"github.com/vx6/vx6/internal/transfer"
)

func Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "init":      return runInit(args[1:])
	case "list":      return runList(ctx, args[1:])
	case "send":      return runSend(ctx, args[1:])
	case "connect":   return runConnect(ctx, args[1:])
	case "status":    return runStatus(ctx, args[1:])
	case "node":      return runNode(ctx, args[1:])
	case "peer":      return runPeer(args[1:])
	case "bootstrap": return runBootstrap(args[1:])
	case "service":   return runService(args[1:])
	case "identity":  return runIdentity(args[1:])
	case "debug":     return runDebug(ctx, args[1:])
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "VX6 - The Ghost Fabric")
	fmt.Fprintln(w, "======================")
	fmt.Fprintln(w, "CORE COMMANDS:")
	fmt.Fprintln(w, "  init             Setup node name.")
	fmt.Fprintln(w, "  list             Show all peers and services.")
	fmt.Fprintln(w, "  node             Start the engine.")
	fmt.Fprintln(w, "  connect          Create a tunnel to a service.")
	fmt.Fprintln(w, "  service add      Expose a local port (add --hidden for proxy).")
}

func prompt(label string) string {
	fmt.Printf("%s: ", label)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func runInit(args []string) error {
	name := ""; if len(args) > 0 { name = args[0] }
	if name == "" { name = prompt("Enter node name") }
	store, _ := config.NewStore(""); cfg, _ := store.Load()
	cfg.Node.Name = name; _ = store.Save(cfg)
	idStore, _ := identity.NewStoreForConfig(store.Path())
	id, _, _ := idStore.Ensure()
	fmt.Printf("✔ Node initialized: %s (%s)\n", name, id.NodeID)
	return nil
}

func runList(ctx context.Context, args []string) error {
	store, _ := config.NewStore(""); cfg, _ := store.Load()
	fmt.Println("\n[ PEERS ]")
	names, peers, _ := store.ListPeers()
	for _, n := range names { fmt.Printf("  %-15s %s\n", n, peers[n].Address) }
	fmt.Println("\n[ DISCOVERY ]")
	reg, _ := loadLocalRegistry(cfg.Node.DataDir)
	recs, svcs := reg.Snapshot()
	for _, r := range recs { fmt.Printf("  %-15s %s\n", r.NodeName, r.Address) }
	for _, s := range svcs { 
		mode := "DIRECT"; if s.IsHidden { mode = "GHOST" }
		fmt.Printf("  %-25s %-15s %s\n", s.NodeName+"."+s.ServiceName, s.Address, mode) 
	}
	return nil
}

func runNode(ctx context.Context, args []string) error {
	store, _ := config.NewStore(""); cfgFile, _ := store.Load()
	idStore, _ := identity.NewStoreForConfig(store.Path()); id, _ := idStore.Load()
	cfg := node.Config{
		Name: cfgFile.Node.Name, NodeID: id.NodeID, ListenAddr: cfgFile.Node.ListenAddr,
		DataDir: cfgFile.Node.DataDir, ConfigPath: store.Path(), Identity: id,
		DHT: dht.NewServer(id.NodeID), BootstrapAddrs: cfgFile.Node.BootstrapAddrs,
		Registry: func() *discovery.Registry { r, _ := discovery.NewRegistry(filepath.Join(cfgFile.Node.DataDir, "registry.json")); return r }(),
		RefreshServices: func() map[string]string {
			c, _ := store.Load(); m := make(map[string]string)
			for k, v := range c.Services { m[k] = v.Target }; return m
		},
	}
	return node.Run(ctx, os.Stdout, cfg)
}

func runConnect(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	svc := fs.String("service", "", "service")
	proxy := fs.Bool("proxy", false, "force proxy")
	_ = fs.Parse(args)
	finalSvc := *svc; if finalSvc == "" && len(args) > 0 && args[0][0] != '-' { finalSvc = args[0] }
	if finalSvc == "" { finalSvc = prompt("Enter service name") }

	store, _ := config.NewStore(""); cfg, _ := store.Load()
	idStore, _ := identity.NewStoreForConfig(store.Path()); id, _ := idStore.Load()
	
	// FIX: Check local cache first, then DHT
	serviceRec, err := resolveServiceDistributed(ctx, cfg, finalSvc)
	if err != nil { return fmt.Errorf("service %q not found. try running 'vx6 list' to verify", finalSvc) }

	dialer := func(rctx context.Context) (net.Conn, error) {
		if *proxy || serviceRec.IsHidden {
			fmt.Printf("[GHOST] Building 5-hop circuit to %s...\n", finalSvc)
			reg, _ := loadLocalRegistry(cfg.Node.DataDir); peers, _ := reg.Snapshot()
			return onion.BuildAutomatedCircuit(rctx, serviceRec, peers)
		}
		var d net.Dialer
		return d.DialContext(rctx, "tcp6", serviceRec.Address)
	}
	fmt.Printf("✔ Tunnel Active: localhost:2222 -> %s\n", finalSvc)
	return serviceproxy.ServeLocalForward(ctx, "127.0.0.1:2222", serviceRec, id, dialer)
}

func runService(args []string) error {
	store, _ := config.NewStore("")
	if len(args) >= 1 && args[0] == "add" {
		fs := flag.NewFlagSet("service add", flag.ContinueOnError)
		h := fs.Bool("hidden", false, "hidden")
		_ = fs.Parse(args[1:])
		n := prompt("Service Name"); t := prompt("Target (e.g. :8000)")
		return store.AddService(n, t, *h)
	}
	c, _ := store.Load()
	for n, s := range c.Services { fmt.Printf("%s\t%s\tHidden:%v\n", n, s.Target, s.IsHidden) }
	return nil
}

func runPeer(args []string) error {
	store, _ := config.NewStore("")
	if len(args) >= 1 && args[0] == "add" {
		n := prompt("Peer Name"); a := prompt("Peer Address")
		return store.AddPeer(n, a)
	}
	names, peers, _ := store.ListPeers()
	for _, n := range names { fmt.Printf("%s\t%s\n", n, peers[n].Address) }
	return nil
}

func runBootstrap(args []string) error {
	store, _ := config.NewStore("")
	if len(args) >= 1 && args[0] == "add" {
		a := prompt("Bootstrap Address")
		return store.AddBootstrap(a)
	}
	list, _ := store.ListBootstraps()
	for _, b := range list { fmt.Println(b) }
	return nil
}

func runSend(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	proxy := fs.Bool("proxy", false, "proxy")
	_ = fs.Parse(args)
	file := prompt("File Path"); to := prompt("Receiver Name")
	store, _ := config.NewStore(""); cfg, _ := store.Load()
	idStore, _ := identity.NewStoreForConfig(store.Path()); id, _ := idStore.Load()
	addr, err := resolvePeerForSend(ctx, store, cfg, to)
	if err != nil { return err }

	dialer := func(rctx context.Context) (net.Conn, error) {
		if *proxy {
			reg, _ := loadLocalRegistry(cfg.Node.DataDir); peers, _ := reg.Snapshot()
			return onion.BuildAutomatedCircuit(rctx, record.ServiceRecord{Address: addr}, peers)
		}
		var d net.Dialer; return d.DialContext(rctx, "tcp6", addr)
	}
	conn, _ := dialer(ctx); defer conn.Close()
	res, err := transfer.SendFileWithConn(ctx, conn, transfer.SendRequest{NodeName: cfg.Node.Name, FilePath: file, Address: addr, Identity: id})
	if err != nil { return err }
	fmt.Printf("✔ Sent %q\n", res.FileName); return nil
}

func runStatus(ctx context.Context, args []string) error {
	store, _ := config.NewStore(""); cfg, _ := store.Load()
	conn, err := net.DialTimeout("tcp6", cfg.Node.ListenAddr, 500*time.Millisecond)
	if err != nil { fmt.Println("OFFLINE"); return nil }
	conn.Close(); fmt.Println("ONLINE"); return nil
}

func runIdentity(args []string) error {
	idStore, _ := identity.NewStoreForConfig(""); id, _ := idStore.Load()
	fmt.Printf("NodeID: %s\n", id.NodeID); return nil
}

func runDebug(ctx context.Context, args []string) error { fmt.Println("Debug active."); return nil }

func loadLocalRegistry(dataDir string) (*discovery.Registry, error) {
	if dataDir == "" { dataDir = "./data/inbox" }
	return discovery.NewRegistry(filepath.Join(dataDir, "registry.json"))
}

func resolvePeerForSend(ctx context.Context, store *config.Store, cfg config.File, name string) (string, error) {
	p, err := store.ResolvePeer(name)
	if err == nil { return p.Address, nil }
	rec, err := resolveNodeDistributed(ctx, cfg, name)
	if err != nil { return "", err }
	_ = store.AddPeer(rec.NodeName, rec.Address)
	return rec.Address, nil
}

func resolveNodeDistributed(ctx context.Context, cfg config.File, name string) (record.EndpointRecord, error) {
	// 1. Check local cache first
	reg, _ := loadLocalRegistry(cfg.Node.DataDir)
	if reg != nil {
		nodes, _ := reg.Snapshot()
		for _, n := range nodes { if n.NodeName == name { return n, nil } }
	}
	// 2. Fallback to DHT
	d := dht.NewServer(cfg.Node.Name)
	for _, b := range cfg.Node.BootstrapAddrs { d.RT.AddNode(proto.NodeInfo{ID: "seed", Addr: b}) }
	nodes, err := d.RecursiveFindNode(ctx, name)
	if err == nil && len(nodes) > 0 { return discovery.Resolve(ctx, nodes[0].Addr, name, "") }
	return record.EndpointRecord{}, errors.New("not found")
}

func resolveServiceDistributed(ctx context.Context, cfg config.File, service string) (record.ServiceRecord, error) {
	// 1. Check local cache first
	reg, _ := loadLocalRegistry(cfg.Node.DataDir)
	if reg != nil {
		_, svcs := reg.Snapshot()
		for _, s := range svcs { if s.NodeName+"."+s.ServiceName == service { return s, nil } }
	}
	// 2. Fallback to DHT
	d := dht.NewServer(cfg.Node.Name)
	for _, b := range cfg.Node.BootstrapAddrs { d.RT.AddNode(proto.NodeInfo{ID: "seed", Addr: b}) }
	val, err := d.RecursiveFindValue(ctx, service)
	if err == nil && val != "" {
		var r record.ServiceRecord
		_ = json.Unmarshal([]byte(val), &r); return r, nil
	}
	return record.ServiceRecord{}, errors.New("not found")
}
