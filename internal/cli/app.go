package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/vx6/vx6/internal/config"
	"github.com/vx6/vx6/internal/discovery"
	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/netutil"
	"github.com/vx6/vx6/internal/node"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/serviceproxy"
	"github.com/vx6/vx6/internal/transfer"
)

type stringListFlag []string

func (s *stringListFlag) String() string { return fmt.Sprint([]string(*s)) }

func (s *stringListFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "bootstrap":
		return runBootstrap(args[1:])
	case "connect":
		return runConnect(ctx, args[1:])
	case "discover":
		return runDiscover(ctx, args[1:])
	case "identity":
		return runIdentity(args[1:])
	case "init":
		return runInit(args[1:])
	case "node":
		return runNode(ctx, args[1:])
	case "peer":
		return runPeer(args[1:])
	case "record":
		return runRecord(args[1:])
	case "service":
		return runService(args[1:])
	case "send":
		return runSend(ctx, args[1:])
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	name := fs.String("name", "", "local human-readable node name")
	listenAddr := fs.String("listen", "[::]:4242", "default IPv6 listen address in [addr]:port form")
	advertiseAddr := fs.String("advertise", "", "public IPv6 address in [addr]:port form for discovery records")
	dataDir := fs.String("data-dir", "./data/inbox", "default directory for received files")
	configPath := fs.String("config", "", "path to the VX6 config file")
	var bootstraps stringListFlag
	fs.Var(&bootstraps, "bootstrap", "bootstrap IPv6 address in [addr]:port form; repeatable")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("init requires --name")
	}
	if err := transfer.ValidateIPv6Address(*listenAddr); err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if *advertiseAddr != "" {
		if err := transfer.ValidateIPv6Address(*advertiseAddr); err != nil {
			return fmt.Errorf("invalid advertise address: %w", err)
		}
	}
	for _, addr := range bootstraps {
		if err := transfer.ValidateIPv6Address(addr); err != nil {
			return fmt.Errorf("invalid bootstrap address %q: %w", addr, err)
		}
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	identityStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}

	cfg, err := store.Load()
	if err != nil {
		return err
	}
	cfg.Node.Name = *name
	cfg.Node.ListenAddr = *listenAddr
	cfg.Node.AdvertiseAddr = *advertiseAddr
	cfg.Node.DataDir = *dataDir
	if len(bootstraps) > 0 {
		cfg.Node.BootstrapAddrs = append([]string(nil), bootstraps...)
	}

	if err := store.Save(cfg); err != nil {
		return err
	}

	id, created, err := identityStore.Ensure()
	if err != nil {
		return err
	}

	if created {
		fmt.Fprintf(os.Stdout, "initialized node %q in %s with identity %s\n", cfg.Node.Name, store.Path(), id.NodeID)
		return nil
	}

	fmt.Fprintf(os.Stdout, "initialized node %q in %s using existing identity %s\n", cfg.Node.Name, store.Path(), id.NodeID)
	return nil
}

func runSend(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	nodeName := fs.String("name", "", "local human-readable node name")
	filePath := fs.String("file", "", "path to the file to send")
	address := fs.String("addr", "", "remote IPv6 address in [addr]:port form")
	peerName := fs.String("to", "", "named peer from local VX6 config")
	configPath := fs.String("config", "", "path to the VX6 config file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *filePath == "" {
		return errors.New("send requires --file")
	}
	if *address == "" && *peerName == "" {
		return errors.New("send requires --addr or --to")
	}
	if *address != "" && *peerName != "" {
		return errors.New("send accepts only one of --addr or --to")
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	if *nodeName == "" {
		*nodeName = cfg.Node.Name
	}
	if *nodeName == "" {
		return errors.New("send requires --name or a configured node name via vx6 init")
	}
	identityStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, err := identityStore.Load()
	if err != nil {
		return err
	}

	if *peerName != "" {
		resolvedAddr, err := resolvePeerForSend(ctx, store, cfg, *peerName)
		if err != nil {
			return err
		}
		*address = resolvedAddr
	}

	req := transfer.SendRequest{
		NodeName: *nodeName,
		FilePath: *filePath,
		Address:  *address,
		Identity: id,
	}

	result, err := transfer.SendFile(ctx, req)
	if err != nil && *peerName != "" {
		resolvedAddr, resolveErr := refreshPeerFromNetwork(ctx, store, cfg, *peerName)
		if resolveErr == nil && resolvedAddr != req.Address {
			req.Address = resolvedAddr
			result, err = transfer.SendFile(ctx, req)
		}
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(
		os.Stdout,
		"sent %q (%d bytes) from node %q to %s\n",
		result.FileName,
		result.BytesSent,
		result.NodeName,
		result.RemoteAddr,
	)
	return nil
}

func runNode(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("node", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	nodeName := fs.String("name", "", "local human-readable node name")
	listenAddr := fs.String("listen", "", "IPv6 listen address in [addr]:port form")
	dataDir := fs.String("data-dir", "", "directory for received files")
	configPath := fs.String("config", "", "path to the VX6 config file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	identityStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	cfgFile, err := store.Load()
	if err != nil {
		return err
	}
	id, err := identityStore.Load()
	if err != nil {
		return fmt.Errorf("load node identity: %w", err)
	}

	if *nodeName == "" {
		*nodeName = cfgFile.Node.Name
	}
	if *listenAddr == "" {
		*listenAddr = cfgFile.Node.ListenAddr
	}
	if *dataDir == "" {
		*dataDir = cfgFile.Node.DataDir
	}
	if *nodeName == "" {
		return errors.New("node requires --name or a configured node name via vx6 init")
	}

	cfg := node.Config{
		Name:           *nodeName,
		NodeID:         id.NodeID,
		ListenAddr:     *listenAddr,
		AdvertiseAddr:  cfgFile.Node.AdvertiseAddr,
		DataDir:        *dataDir,
		BootstrapAddrs: append([]string(nil), cfgFile.Node.BootstrapAddrs...),
		Services:       make(map[string]string, len(cfgFile.Services)),
		Identity:       id,
	}
	for name, svc := range cfgFile.Services {
		cfg.Services[name] = svc.Target
	}
	registry, err := discovery.NewRegistry(filepath.Join(*dataDir, "registry.json"))
	if err != nil {
		return err
	}
	cfg.Registry = registry

	return node.Run(ctx, os.Stdout, cfg)
}

func runConnect(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	serviceName := fs.String("service", "", "full service name in node.service form")
	localListen := fs.String("listen", "127.0.0.1:2222", "local TCP listener address")
	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *serviceName == "" {
		return errors.New("connect requires --service")
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	serviceRec, err := resolveServiceDistributed(ctx, cfg, *serviceName)
	if err != nil {
		return err
	}
	identityStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, err := identityStore.Load()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "forwarding %s to %s on %s\n", *serviceName, serviceRec.Address, *localListen)

	return serviceproxy.ServeLocalForward(ctx, *localListen, serviceRec, id, func(resolveCtx context.Context) (string, error) {
		refreshed, err := resolveServiceDistributed(resolveCtx, cfg, *serviceName)
		if err != nil {
			return "", err
		}
		return refreshed.Address, nil
	})
}

func runDiscover(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing discover subcommand")
	}

	switch args[0] {
	case "publish":
		return runDiscoverPublish(ctx, args[1:])
	case "resolve":
		return runDiscoverResolve(ctx, args[1:])
	case "list":
		return runDiscoverList(args[1:])
	default:
		return fmt.Errorf("unknown discover subcommand %q", args[0])
	}
}

func runDiscoverPublish(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("discover publish", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	via := fs.String("via", "", "bootstrap node as peer name or [ipv6]:port")
	address := fs.String("addr", "", "public node address for the published record; defaults to configured listen address")
	ttl := fs.Duration("ttl", 15*time.Minute, "record time-to-live")
	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *via == "" {
		return errors.New("discover publish requires --via")
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	if cfg.Node.Name == "" {
		return errors.New("node name is not configured; run vx6 init first")
	}
	if *address == "" {
		*address = cfg.Node.AdvertiseAddr
	}
	if *address == "" {
		_, port, err := net.SplitHostPort(cfg.Node.ListenAddr)
		if err == nil {
			*address, _ = netutil.DetectAdvertiseAddress(port)
		}
	}
	if *address == "" {
		return errors.New("discover publish requires --addr, a configured advertise address, or a detectable global IPv6 address")
	}

	identityStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, err := identityStore.Load()
	if err != nil {
		return fmt.Errorf("load node identity: %w", err)
	}

	rec, err := record.NewEndpointRecord(id, cfg.Node.Name, *address, *ttl, time.Now())
	if err != nil {
		return err
	}

	bootstrapAddr, err := resolveAddress(store, *via)
	if err != nil {
		return err
	}

	stored, err := discovery.Publish(ctx, bootstrapAddr, rec)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "published %s for %q at %s via %s\n", stored.NodeID, stored.NodeName, stored.Address, bootstrapAddr)
	return nil
}

func runDiscoverResolve(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("discover resolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	via := fs.String("via", "", "bootstrap node as peer name or [ipv6]:port")
	name := fs.String("name", "", "node name to resolve")
	nodeID := fs.String("node-id", "", "node id to resolve")
	savePeer := fs.Bool("save-peer", false, "save the resolved address in the local peer list")
	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*name == "" && *nodeID == "") || (*name != "" && *nodeID != "") {
		return errors.New("discover resolve requires exactly one of --name or --node-id")
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}

	var rec record.EndpointRecord
	if *via != "" {
		bootstrapAddr, err := resolveAddress(store, *via)
		if err != nil {
			return err
		}
		rec, err = discovery.Resolve(ctx, bootstrapAddr, *name, *nodeID)
		if err != nil {
			return err
		}
	} else {
		// Resolve from local registry cache
		cfg, err := store.Load()
		if err != nil {
			return err
		}
		registry, err := loadLocalRegistry(cfg.Node.DataDir)
		if err != nil {
			return err
		}
		rec, err = registry.ResolveLocal(*name, *nodeID)
		if err != nil {
			return fmt.Errorf("node not found in local registry; use --via to resolve from network: %w", err)
		}
	}

	data, err := record.JSON(rec)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "%s", data)
	fmt.Fprintf(os.Stdout, "fingerprint\t%s\n", record.Fingerprint(rec))

	if *savePeer {
		if err := store.AddPeer(rec.NodeName, rec.Address); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "saved peer %q -> %s\n", rec.NodeName, rec.Address)
	}

	return nil
}

func runIdentity(args []string) error {
	if len(args) == 0 {
		return errors.New("missing identity subcommand")
	}

	switch args[0] {
	case "show":
		return runIdentityShow(args[1:])
	default:
		return fmt.Errorf("unknown identity subcommand %q", args[0])
	}
}

func runIdentityShow(args []string) error {
	fs := flag.NewFlagSet("identity show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	identityStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, err := identityStore.Load()
	if err != nil {
		return fmt.Errorf("load node identity: %w", err)
	}

	fmt.Fprintf(os.Stdout, "node_name\t%s\n", cfg.Node.Name)
	fmt.Fprintf(os.Stdout, "node_id\t%s\n", id.NodeID)
	fmt.Fprintf(os.Stdout, "listen_addr\t%s\n", cfg.Node.ListenAddr)
	fmt.Fprintf(os.Stdout, "advertise_addr\t%s\n", cfg.Node.AdvertiseAddr)
	fmt.Fprintf(os.Stdout, "identity_file\t%s\n", identityStore.Path())
	return nil
}

func runPeer(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return errors.New("missing peer subcommand")
	}

	switch args[0] {
	case "add":
		return runPeerAdd(args[1:])
	case "list":
		return runPeerList(args[1:])
	default:
		return fmt.Errorf("unknown peer subcommand %q", args[0])
	}
}

func runPeerAdd(args []string) error {
	fs := flag.NewFlagSet("peer add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	name := fs.String("name", "", "peer name")
	address := fs.String("addr", "", "peer IPv6 address in [addr]:port form")
	configPath := fs.String("config", "", "path to the VX6 config file")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("peer add requires --name")
	}
	if *address == "" {
		return errors.New("peer add requires --addr")
	}
	if err := transfer.ValidateIPv6Address(*address); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	if err := store.AddPeer(*name, *address); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "saved peer %q -> %s\n", *name, *address)
	return nil
}

func runPeerList(args []string) error {
	fs := flag.NewFlagSet("peer list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}

	names, peers, err := store.ListPeers()
	if err != nil {
		return err
	}

	for _, name := range names {
		fmt.Fprintf(os.Stdout, "%s\t%s\n", name, peers[name].Address)
	}
	return nil
}

func runBootstrap(args []string) error {
	if len(args) == 0 {
		return errors.New("missing bootstrap subcommand")
	}

	switch args[0] {
	case "add":
		return runBootstrapAdd(args[1:])
	case "list":
		return runBootstrapList(args[1:])
	default:
		return fmt.Errorf("unknown bootstrap subcommand %q", args[0])
	}
}

func runBootstrapAdd(args []string) error {
	fs := flag.NewFlagSet("bootstrap add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	address := fs.String("addr", "", "bootstrap IPv6 address in [addr]:port form")
	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *address == "" {
		return errors.New("bootstrap add requires --addr")
	}
	if err := transfer.ValidateIPv6Address(*address); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	if err := store.AddBootstrap(*address); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "saved bootstrap %s\n", *address)
	return nil
}

func runBootstrapList(args []string) error {
	fs := flag.NewFlagSet("bootstrap list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	addresses, err := store.ListBootstraps()
	if err != nil {
		return err
	}
	for _, addr := range addresses {
		fmt.Fprintln(os.Stdout, addr)
	}
	return nil
}

func runRecord(args []string) error {
	if len(args) == 0 {
		return errors.New("missing record subcommand")
	}

	switch args[0] {
	case "print":
		return runRecordPrint(args[1:])
	default:
		return fmt.Errorf("unknown record subcommand %q", args[0])
	}
}

func runService(args []string) error {
	if len(args) == 0 {
		return errors.New("missing service subcommand")
	}

	switch args[0] {
	case "add":
		return runServiceAdd(args[1:])
	case "list":
		return runServiceList(args[1:])
	default:
		return fmt.Errorf("unknown service subcommand %q", args[0])
	}
}

func runServiceAdd(args []string) error {
	fs := flag.NewFlagSet("service add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	name := fs.String("name", "", "local service name, for example ssh")
	target := fs.String("target", "", "local TCP target such as 127.0.0.1:22")
	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("service add requires --name")
	}
	if *target == "" {
		return errors.New("service add requires --target")
	}
	if err := record.ValidateServiceName(*name); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	if err := store.AddService(*name, *target); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "saved service %q -> %s\n", *name, *target)
	return nil
}

func runServiceList(args []string) error {
	fs := flag.NewFlagSet("service list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	names, services, err := store.ListServices()
	if err != nil {
		return err
	}
	for _, name := range names {
		fmt.Fprintf(os.Stdout, "%s\t%s\n", name, services[name].Target)
	}
	return nil
}

func runRecordPrint(args []string) error {
	fs := flag.NewFlagSet("record print", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	address := fs.String("addr", "", "IPv6 address for the endpoint record; defaults to configured listen address")
	ttl := fs.Duration("ttl", 15*time.Minute, "record time-to-live")
	configPath := fs.String("config", "", "path to the VX6 config file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	if cfg.Node.Name == "" {
		return errors.New("node name is not configured; run vx6 init first")
	}
	if *address == "" {
		*address = cfg.Node.ListenAddr
	}

	identityStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, err := identityStore.Load()
	if err != nil {
		return fmt.Errorf("load node identity: %w", err)
	}

	rec, err := record.NewEndpointRecord(id, cfg.Node.Name, *address, *ttl, time.Now())
	if err != nil {
		return err
	}
	data, err := record.JSON(rec)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "%s", data)
	fmt.Fprintf(os.Stdout, "fingerprint\t%s\n", record.Fingerprint(rec))
	return nil
}

func resolveAddress(store *config.Store, value string) (string, error) {
	if err := transfer.ValidateIPv6Address(value); err == nil {
		return value, nil
	}

	peer, err := store.ResolvePeer(value)
	if err != nil {
		return "", fmt.Errorf("resolve %q as bootstrap peer or address: %w", value, err)
	}

	return peer.Address, nil
}

func resolveNodeDistributed(ctx context.Context, cfgFile config.File, name string) (record.EndpointRecord, error) {
	candidates := discoveryCandidates(cfgFile)
	visited := map[string]struct{}{}

	for _, addr := range candidates {
		if _, ok := visited[addr]; ok {
			continue
		}
		visited[addr] = struct{}{}

		rec, err := discovery.Resolve(ctx, addr, name, "")
		if err == nil {
			return rec, nil
		}
	}

	registry, err := loadLocalRegistry(cfgFile.Node.DataDir)
	if err == nil {
		if rec, err := registry.ResolveLocal(name, ""); err == nil {
			return rec, nil
		}
	}

	return record.EndpointRecord{}, fmt.Errorf("node %q could not be resolved from configured network candidates", name)
}

func resolveServiceDistributed(ctx context.Context, cfgFile config.File, service string) (record.ServiceRecord, error) {
	candidates := discoveryCandidates(cfgFile)
	visited := map[string]struct{}{}

	for _, addr := range candidates {
		if _, ok := visited[addr]; ok {
			continue
		}
		visited[addr] = struct{}{}

		rec, err := discovery.ResolveService(ctx, addr, service)
		if err == nil {
			return rec, nil
		}
	}

	registry, err := loadLocalRegistry(cfgFile.Node.DataDir)
	if err == nil {
		if rec, err := registry.ResolveServiceLocal(service); err == nil {
			return rec, nil
		}
	}

	return record.ServiceRecord{}, fmt.Errorf("service %q could not be resolved from configured network candidates", service)
}

func discoveryCandidates(cfgFile config.File) []string {
	seen := map[string]struct{}{}
	var out []string

	add := func(addr string) {
		if addr == "" {
			return
		}
		if _, ok := seen[addr]; ok {
			return
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}

	for _, addr := range cfgFile.Node.BootstrapAddrs {
		add(addr)
	}
	for _, peer := range cfgFile.Peers {
		add(peer.Address)
	}
	if registry, err := loadLocalRegistry(cfgFile.Node.DataDir); err == nil {
		nodes, _ := registry.Snapshot()
		for _, rec := range nodes {
			add(rec.Address)
		}
	}
	return out
}

func loadLocalRegistry(dataDir string) (*discovery.Registry, error) {
	if dataDir == "" {
		dataDir = "./data/inbox"
	}
	return discovery.NewRegistry(filepath.Join(dataDir, "registry.json"))
}

func resolvePeerForSend(ctx context.Context, store *config.Store, cfgFile config.File, name string) (string, error) {
	peer, err := store.ResolvePeer(name)
	if err == nil {
		return peer.Address, nil
	}

	return refreshPeerFromNetwork(ctx, store, cfgFile, name)
}

func refreshPeerFromNetwork(ctx context.Context, store *config.Store, cfgFile config.File, name string) (string, error) {
	rec, err := resolveNodeDistributed(ctx, cfgFile, name)
	if err != nil {
		return "", err
	}
	if err := store.AddPeer(rec.NodeName, rec.Address); err != nil {
		return "", err
	}
	return rec.Address, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "VX6")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  vx6 bootstrap add --addr [ipv6]:port")
	fmt.Fprintln(w, "  vx6 bootstrap list")
	fmt.Fprintln(w, "  vx6 connect --service <node.service> [--listen 127.0.0.1:2222]")
	fmt.Fprintln(w, "  vx6 discover list")
	fmt.Fprintln(w, "  vx6 discover publish --via <peer-name|[ipv6]:port> [--addr [ipv6]:port]")
	fmt.Fprintln(w, "  vx6 discover resolve [--via <peer-name|[ipv6]:port>] (--name <node-name> | --node-id <node-id>) [--save-peer]")
	fmt.Fprintln(w, "  vx6 init --name <node-name> [--listen [::]:4242] [--advertise [ipv6]:port] [--bootstrap [ipv6]:port]")
	fmt.Fprintln(w, "  vx6 identity show")
	fmt.Fprintln(w, "  vx6 node [--name <node-name>] [--listen [::]:4242] [--data-dir ./data/inbox]")
	fmt.Fprintln(w, "  vx6 peer add --name <peer-name> --addr [ipv6]:port")
	fmt.Fprintln(w, "  vx6 peer list")
	fmt.Fprintln(w, "  vx6 record print [--addr [ipv6]:port] [--ttl 15m]")
	fmt.Fprintln(w, "  vx6 service add --name <service> --target 127.0.0.1:22")
	fmt.Fprintln(w, "  vx6 service list")
	fmt.Fprintln(w, "  vx6 send [--name <node-name>] --file <path> (--addr [ipv6]:port | --to <peer-name>)")
}

func runDiscoverList(args []string) error {
	fs := flag.NewFlagSet("discover list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", "path to the VX6 config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore(*configPath)
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	registry, err := loadLocalRegistry(cfg.Node.DataDir)
	if err != nil {
		return err
	}

	records, services := registry.Snapshot()
	if len(records) > 0 {
		fmt.Fprintln(os.Stdout, "Nodes in Registry Cache:")
		for _, rec := range records {
			fmt.Fprintf(os.Stdout, "  %s\t%s\t%s\n", rec.NodeName, rec.Address, rec.NodeID)
		}
	} else {
		fmt.Fprintln(os.Stdout, "No nodes in registry cache.")
	}

	if len(services) > 0 {
		fmt.Fprintln(os.Stdout, "\nServices in Registry Cache:")
		for _, rec := range services {
			fmt.Fprintf(os.Stdout, "  %s.%s\t%s\n", rec.NodeName, rec.ServiceName, rec.Address)
		}
	}
	return nil
}
