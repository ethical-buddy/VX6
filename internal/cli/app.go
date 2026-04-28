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
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
	"github.com/vx6/vx6/internal/runtimectl"
	"github.com/vx6/vx6/internal/serviceproxy"
	"github.com/vx6/vx6/internal/transfer"
	vxtransport "github.com/vx6/vx6/internal/transport"
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
	case "init":
		return runInit(args[1:])
	case "list":
		return runList(ctx, args[1:])
	case "send":
		return runSend(ctx, args[1:])
	case "receive":
		return runReceive(args[1:])
	case "connect":
		return runConnect(ctx, args[1:])
	case "status":
		return runStatus(ctx, args[1:])
	case "node":
		return runNode(ctx, args[1:])
	case "reload":
		return runReload(args[1:])
	case "peer":
		return runPeer(args[1:])
	case "bootstrap":
		return runBootstrap(args[1:])
	case "service":
		return runService(args[1:])
	case "identity":
		return runIdentity(args[1:])
	case "debug":
		return runDebug(ctx, args[1:])
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "VX6")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "IPv6-first overlay transport with signed discovery, encrypted sessions, direct service sharing,")
	fmt.Fprintln(w, "DHT-backed metadata lookup, and optional 5-hop proxy forwarding.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  vx6 init --name NAME [--listen [::]:4242] [--advertise [ipv6]:port] [--transport auto|tcp|quic] [--relay on|off] [--relay-percent N] [--bootstrap [ipv6]:port] [--hidden-node] [--data-dir DIR] [--downloads-dir DIR]")
	fmt.Fprintln(w, "  vx6 node")
	fmt.Fprintln(w, "  vx6 reload")
	fmt.Fprintln(w, "  vx6 service add --name NAME --target 127.0.0.1:22 [--hidden --alias NAME --profile fast|balanced --intro-mode random|manual|hybrid --intro NODE]")
	fmt.Fprintln(w, "  vx6 connect --service NAME [--listen 127.0.0.1:2222] [--proxy] [--addr [ipv6]:port]")
	fmt.Fprintln(w, "  vx6 send --file PATH (--to PEER | --addr [ipv6]:port) [--proxy]")
	fmt.Fprintln(w, "  vx6 receive status")
	fmt.Fprintln(w, "  vx6 receive allow --all | --node NAME")
	fmt.Fprintln(w, "  vx6 receive deny --node NAME")
	fmt.Fprintln(w, "  vx6 receive disable")
	fmt.Fprintln(w, "  vx6 service")
	fmt.Fprintln(w, "  vx6 peer")
	fmt.Fprintln(w, "  vx6 bootstrap")
	fmt.Fprintln(w, "  vx6 list [--user USER] [--hidden]")
	fmt.Fprintln(w, "  vx6 peer add --name NAME --addr [ipv6]:port")
	fmt.Fprintln(w, "  vx6 bootstrap add --addr [ipv6]:port")
	fmt.Fprintln(w, "  vx6 identity")
	fmt.Fprintln(w, "  vx6 status")
	fmt.Fprintln(w, "  vx6 debug registry")
	fmt.Fprintln(w, "  vx6 debug dht-get (--service NODE.SERVICE | --node NAME | --node-id ID | --key KEY)")
	fmt.Fprintln(w, "  vx6 debug ebpf-status")
	fmt.Fprintln(w, "  vx6 debug ebpf-attach --iface IFACE")
	fmt.Fprintln(w, "  vx6 debug ebpf-detach --iface IFACE")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Working features:")
	fmt.Fprintln(w, "  - Signed endpoint and service discovery via bootstrap registry")
	fmt.Fprintln(w, "  - DHT-backed endpoint/service key lookup")
	fmt.Fprintln(w, "  - Encrypted file transfer with local receive permissions")
	fmt.Fprintln(w, "  - Direct TCP service sharing")
	fmt.Fprintln(w, "  - 5-hop proxy forwarding for direct services and files")
	fmt.Fprintln(w, "  - Plain-TCP hidden services via 3 active intros, 2 standby intros, guards, and rendezvous relay")
	fmt.Fprintln(w, "  - Direct IPv6 service sharing without bootstrap using --addr")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Experimental / not complete:")
	fmt.Fprintln(w, "  - eBPF loader and attach path (embedded bytecode is present and tested)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  vx6 init --name alice --listen '[::]:4242' --bootstrap '[::1]:4242'")
	fmt.Fprintln(w, "  vx6 reload")
	fmt.Fprintln(w, "  vx6 init --name ghost --advertise '[2001:db8::10]:4242' --hidden-node")
	fmt.Fprintln(w, "  vx6 service add --name ssh --target 127.0.0.1:22")
	fmt.Fprintln(w, "  vx6 service add --name admin --target 127.0.0.1:22 --hidden --alias hs-admin --intro-mode random")
	fmt.Fprintln(w, "  vx6 connect --service alice.ssh --listen 127.0.0.1:2222")
	fmt.Fprintln(w, "  vx6 connect --service ssh --addr '[2001:db8::10]:4242' --listen 127.0.0.1:2222")
	fmt.Fprintln(w, "  vx6 connect --service alice.ssh --listen 127.0.0.1:2222 --proxy")
	fmt.Fprintln(w, "  vx6 debug dht-get --service alice.ssh")
	fmt.Fprintln(w, "  vx6 debug dht-get --service hs-admin")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Storage:")
	fmt.Fprintln(w, "  - Config: ~/.config/vx6/config.json")
	fmt.Fprintln(w, "  - Identity: ~/.config/vx6/identity.json")
	fmt.Fprintln(w, "  - Runtime state: ~/.local/share/vx6")
	fmt.Fprintln(w, "  - Received files: ~/Downloads")
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
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	name := fs.String("name", "", "local human-readable node name")
	listenAddr := fs.String("listen", "[::]:4242", "default IPv6 listen address in [addr]:port form")
	advertiseAddr := fs.String("advertise", "", "public IPv6 address in [addr]:port form for discovery records")
	transportMode := fs.String("transport", vxtransport.ModeAuto, "neighbor transport mode: auto, tcp, or quic")
	hiddenNode := fs.Bool("hidden-node", false, "do not publish the node endpoint record; publish services only")
	relayMode := fs.String("relay", config.RelayModeOn, "relay participation: on or off")
	relayPercent := fs.Int("relay-percent", 33, "maximum share of local relay capacity reserved for transit work")
	dataDir := fs.String("data-dir", defaultDataDirValue(), "directory for VX6 runtime state")
	downloadDir := fs.String("downloads-dir", defaultDownloadDirValue(), "directory for received files")
	var bootstraps stringListFlag
	fs.Var(&bootstraps, "bootstrap", "bootstrap IPv6 address in [addr]:port form; repeatable")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" && len(fs.Args()) > 0 {
		*name = fs.Args()[0]
	}
	if *name == "" {
		*name = prompt("Enter node name")
	}
	if *name == "" {
		return errors.New("init requires --name")
	}
	if err := record.ValidateNodeName(*name); err != nil {
		return err
	}
	if err := transfer.ValidateIPv6Address(*listenAddr); err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if *advertiseAddr != "" {
		if err := transfer.ValidateIPv6Address(*advertiseAddr); err != nil {
			return fmt.Errorf("invalid advertise address: %w", err)
		}
	}
	normalizedTransport := vxtransport.NormalizeMode(*transportMode)
	if normalizedTransport == "" {
		return fmt.Errorf("invalid transport mode %q", *transportMode)
	}
	normalizedRelayMode := config.NormalizeRelayMode(*relayMode)
	if normalizedRelayMode == "" {
		return fmt.Errorf("invalid relay mode %q", *relayMode)
	}
	for _, addr := range bootstraps {
		if err := transfer.ValidateIPv6Address(addr); err != nil {
			return fmt.Errorf("invalid bootstrap address %q: %w", addr, err)
		}
	}

	store, err := config.NewStore("")
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
	cfg.Node.TransportMode = normalizedTransport
	cfg.Node.HideEndpoint = *hiddenNode
	cfg.Node.RelayMode = normalizedRelayMode
	cfg.Node.RelayResourcePercent = config.NormalizeRelayResourcePercent(*relayPercent)
	cfg.Node.DataDir = *dataDir
	cfg.Node.DownloadDir = *downloadDir
	cfg.Node.FileReceiveMode = config.NormalizeFileReceiveMode(cfg.Node.FileReceiveMode)
	if len(bootstraps) > 0 {
		cfg.Node.BootstrapAddrs = append([]string(nil), bootstraps...)
	}
	if err := store.Save(cfg); err != nil {
		return err
	}

	idStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, _, err := idStore.Ensure()
	if err != nil {
		return err
	}
	fmt.Printf("node_initialized\t%s\t%s\n", *name, id.NodeID)
	fmt.Printf("config_path\t%s\n", store.Path())
	fmt.Printf("identity_path\t%s\n", idStore.Path())
	return nil
}

func runList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", "path to the VX6 config file")
	userFilter := fs.String("user", "", "show direct services for a single user")
	hiddenOnly := fs.Bool("hidden", false, "show hidden aliases from the local registry")
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
	names, _, err := store.ListPeers()
	if err != nil {
		return err
	}
	printSectionHeader("PEERS", len(names))
	for _, n := range names {
		fmt.Printf("  %-15s configured\n", n)
	}
	if len(names) == 0 {
		fmt.Println("  (none)")
	}

	serviceNames, localServices, err := store.ListServices()
	if err != nil {
		return err
	}
	localPublicCount := 0
	localHiddenCount := 0
	for _, name := range serviceNames {
		if localServices[name].IsHidden {
			localHiddenCount++
		} else {
			localPublicCount++
		}
	}
	if !*hiddenOnly {
		printSectionHeader("LOCAL PUBLIC SERVICES", localPublicCount)
		printed := 0
		for _, name := range serviceNames {
			svc := localServices[name]
			if svc.IsHidden {
				continue
			}
			fmt.Printf("  %-15s target=%s\n", name, svc.Target)
			printed++
		}
		if printed == 0 {
			fmt.Println("  (none)")
		}
	}

	printSectionHeader("LOCAL HIDDEN SERVICES", localHiddenCount)
	printedHidden := 0
	for _, name := range serviceNames {
		svc := localServices[name]
		if !svc.IsHidden {
			continue
		}
		label := svc.Alias
		if label == "" {
			label = name
		}
		fmt.Printf("  %-15s alias=%s profile=%s\n", name, label, record.NormalizeHiddenProfile(svc.HiddenProfile))
		printedHidden++
	}
	if printedHidden == 0 {
		fmt.Println("  (none)")
	}

	reg, err := loadLocalRegistry(cfg.Node.DataDir)
	if err != nil {
		return err
	}
	recs, svcs := reg.Snapshot()
	printSectionHeader("DISCOVERED NODES", len(recs))
	for _, r := range recs {
		fmt.Printf("  %-15s discovered\n", r.NodeName)
	}
	if len(recs) == 0 {
		fmt.Println("  (none)")
	}

	publicPrinted := 0
	hiddenPrinted := 0
	if !*hiddenOnly {
		for _, s := range svcs {
			if s.IsHidden {
				continue
			}
			if *userFilter != "" && s.NodeName != *userFilter {
				continue
			}
			publicPrinted++
		}
		printSectionHeader("DISCOVERED PUBLIC SERVICES", publicPrinted)
		for _, s := range svcs {
			if s.IsHidden {
				continue
			}
			if *userFilter != "" && s.NodeName != *userFilter {
				continue
			}
			fmt.Printf("  %-15s node=%s\n", record.FullServiceName(s.NodeName, s.ServiceName), s.NodeName)
		}
		if publicPrinted == 0 {
			fmt.Println("  (none)")
		}
	}

	for _, s := range svcs {
		if !s.IsHidden {
			continue
		}
		hiddenPrinted++
	}
	printSectionHeader("DISCOVERED HIDDEN ALIASES", hiddenPrinted)
	for _, s := range svcs {
		if !s.IsHidden {
			continue
		}
		fmt.Printf("  %-15s profile=%s\n", record.ServiceLookupKey(s), record.NormalizeHiddenProfile(s.HiddenProfile))
	}
	if hiddenPrinted == 0 {
		fmt.Println("  (none)")
	}
	return nil
}

func printSectionHeader(title string, count int) {
	fmt.Printf("\n== %s (%d) ==\n", title, count)
}

func runNode(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("node", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	nodeName := fs.String("name", "", "local human-readable node name")
	listenAddr := fs.String("listen", "", "IPv6 listen address in [addr]:port form")
	dataDir := fs.String("data-dir", "", "directory for VX6 runtime state")
	downloadDir := fs.String("downloads-dir", "", "directory for received files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	cfgFile, err := store.Load()
	if err != nil {
		return err
	}
	idStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, err := idStore.Load()
	if err != nil {
		return err
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
	if *downloadDir == "" {
		*downloadDir = cfgFile.Node.DownloadDir
	}
	lock, err := acquireNodeLock(store.Path())
	if err != nil {
		return err
	}
	defer lock.Close()

	reloadCh := make(chan struct{}, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)
	defer signal.Stop(sigCh)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigCh:
				select {
				case reloadCh <- struct{}{}:
				default:
				}
			}
		}
	}()

	services := make(map[string]string, len(cfgFile.Services))
	for name, svc := range cfgFile.Services {
		services[name] = svc.Target
	}
	registry, err := discovery.NewRegistry(filepath.Join(*dataDir, "registry.json"))
	if err != nil {
		return err
	}
	controlInfoPath, err := config.RuntimeControlPath(store.Path())
	if err != nil {
		return err
	}

	cfg := node.Config{
		Name:                 *nodeName,
		NodeID:               id.NodeID,
		ListenAddr:           *listenAddr,
		AdvertiseAddr:        cfgFile.Node.AdvertiseAddr,
		TransportMode:        cfgFile.Node.TransportMode,
		HideEndpoint:         cfgFile.Node.HideEndpoint,
		RelayMode:            cfgFile.Node.RelayMode,
		RelayResourcePercent: cfgFile.Node.RelayResourcePercent,
		DataDir:              *dataDir,
		ReceiveDir:           *downloadDir,
		ConfigPath:           store.Path(),
		ControlInfoPath:      controlInfoPath,
		Identity:             id,
		FileReceiveMode:      cfgFile.Node.FileReceiveMode,
		AllowedFileSenders:   append([]string(nil), cfgFile.Node.AllowedFileSenders...),
		DHT:                  dht.NewServer(id.NodeID),
		BootstrapAddrs:       cfgFile.Node.BootstrapAddrs,
		Services:             services,
		Registry:             registry,
		Reload:               reloadCh,
		RefreshServices: func() map[string]string {
			c, err := store.Load()
			if err != nil {
				return nil
			}
			m := make(map[string]string, len(c.Services))
			for k, v := range c.Services {
				m[k] = v.Target
			}
			return m
		},
	}
	return node.Run(ctx, os.Stdout, cfg)
}

func runReload(args []string) error {
	fs := flag.NewFlagSet("reload", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	controlPath, err := config.RuntimeControlPath(store.Path())
	if err != nil {
		return err
	}
	if err := runtimectl.RequestReload(context.Background(), controlPath); err == nil {
		fmt.Println("reload_sent\tmode=control")
		return nil
	}

	pidPath, err := config.RuntimePIDPath(store.Path())
	if err != nil {
		return err
	}
	pid, err := readNodePID(pidPath)
	if err != nil {
		return fmt.Errorf("read node pid file: %w", err)
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return fmt.Errorf("check node process: %w", err)
	}
	if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
		return fmt.Errorf("signal node reload: %w", err)
	}
	fmt.Printf("reload_sent\tmode=signal\tpid=%d\n", pid)
	return nil
}

func runConnect(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	svc := fs.String("service", "", "service")
	localListen := fs.String("listen", "127.0.0.1:2222", "local TCP listener address")
	addrFlag := fs.String("addr", "", "direct VX6 node IPv6 address")
	proxy := fs.Bool("proxy", false, "force proxy")
	finalSvc, parseArgs := extractLeadingConnectService(args)
	if err := fs.Parse(parseArgs); err != nil {
		return err
	}
	if *svc != "" {
		finalSvc = *svc
	}
	if finalSvc == "" && len(fs.Args()) > 0 {
		finalSvc = fs.Args()[0]
	}
	if finalSvc == "" {
		finalSvc = prompt("Enter service name")
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	idStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, err := idStore.Load()
	if err != nil {
		return err
	}

	requestServiceName := requestedServiceName(finalSvc)
	serviceRec := record.ServiceRecord{}
	if *addrFlag != "" {
		if err := transfer.ValidateIPv6Address(*addrFlag); err != nil {
			return err
		}
		serviceRec = record.ServiceRecord{
			NodeName:    "direct",
			ServiceName: requestServiceName,
			Address:     *addrFlag,
		}
	} else {
		var err error
		serviceRec, err = resolveServiceDistributed(ctx, cfg, finalSvc)
		if err != nil {
			return fmt.Errorf("service %q not found. try running 'vx6 list --user NAME' or 'vx6 list --hidden' to verify", finalSvc)
		}
	}

	dialer := func(rctx context.Context) (net.Conn, error) {
		if serviceRec.IsHidden {
			reg, err := loadLocalRegistry(cfg.Node.DataDir)
			if err != nil {
				return nil, err
			}
			conn, err := hidden.DialHiddenServiceWithOptions(rctx, serviceRec, reg, hidden.DialOptions{
				SelfAddr:      cfg.Node.AdvertiseAddr,
				Identity:      id,
				TransportMode: cfg.Node.TransportMode,
			})
			if err != nil {
				return nil, friendlyRelayPathError(err, "hidden-service mode")
			}
			return conn, nil
		}
		if *proxy {
			fmt.Printf("[CIRCUIT] Building 5-hop circuit to %s\n", finalSvc)
			reg, err := loadLocalRegistry(cfg.Node.DataDir)
			if err != nil {
				return nil, err
			}
			peers, _ := reg.Snapshot()
			conn, err := onion.BuildAutomatedCircuit(rctx, serviceRec, peers, onion.ClientOptions{
				Identity:      id,
				TransportMode: cfg.Node.TransportMode,
			})
			if err != nil {
				return nil, friendlyRelayPathError(err, "proxy mode")
			}
			return conn, nil
		}
		return vxtransport.DialContext(rctx, cfg.Node.TransportMode, serviceRec.Address)
	}
	fmt.Printf("tunnel_active\t%s\t%s\n", *localListen, finalSvc)
	return serviceproxy.ServeLocalForward(ctx, *localListen, serviceRec, id, dialer)
}

func extractLeadingConnectService(args []string) (string, []string) {
	if len(args) == 0 {
		return "", args
	}
	if strings.HasPrefix(args[0], "-") {
		return "", args
	}
	return args[0], args[1:]
}

func runService(args []string) error {
	if len(args) >= 1 && args[0] == "add" {
		fs := flag.NewFlagSet("service add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		name := fs.String("name", "", "local service name")
		target := fs.String("target", "", "local TCP target")
		h := fs.Bool("hidden", false, "hidden")
		alias := fs.String("alias", "", "hidden alias; defaults to the local service name")
		profile := fs.String("profile", "fast", "hidden routing profile: fast or balanced")
		introMode := fs.String("intro-mode", "", "hidden intro selection mode: random, manual, or hybrid")
		var intros stringListFlag
		fs.Var(&intros, "intro", "preferred intro node name or IPv6 address; repeatable")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *name == "" {
			*name = prompt("Service Name")
		}
		if *target == "" {
			*target = prompt("Target (e.g. :8000)")
		}
		store, err := config.NewStore("")
		if err != nil {
			return err
		}
		entry := config.ServiceEntry{
			Target:        *target,
			IsHidden:      *h,
			Alias:         *alias,
			HiddenProfile: record.NormalizeHiddenProfile(*profile),
			IntroMode:     "",
			IntroNodes:    append([]string(nil), intros...),
		}
		if entry.IsHidden {
			if entry.Alias == "" {
				entry.Alias = *name
			}
			if entry.HiddenProfile == "" {
				return fmt.Errorf("invalid hidden profile %q", *profile)
			}
			if *introMode != "" {
				entry.IntroMode = hidden.NormalizeIntroMode(*introMode)
				if entry.IntroMode == "" {
					return fmt.Errorf("invalid intro mode %q", *introMode)
				}
			}
			if entry.IntroMode == "" {
				if len(entry.IntroNodes) > 0 {
					entry.IntroMode = hidden.IntroModeManual
				} else {
					entry.IntroMode = hidden.IntroModeRandom
				}
			}
		} else {
			entry.IntroMode = ""
			entry.HiddenProfile = ""
			entry.Alias = ""
		}
		if err := store.SetService(*name, entry); err != nil {
			return err
		}
		if entry.IsHidden {
			fmt.Printf("hidden_alias\t%s\nhidden_profile\t%s\nintro_mode\t%s\n", entry.Alias, entry.HiddenProfile, entry.IntroMode)
		}
		return nil
	}
	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	c, err := store.Load()
	if err != nil {
		return err
	}
	for n, s := range c.Services {
		mode := "DIRECT"
		label := n
		if s.IsHidden {
			mode = "HIDDEN"
			if s.Alias != "" {
				label = s.Alias
			}
		}
		fmt.Printf("%s\t%s\t%s\n", label, s.Target, mode)
	}
	return nil
}

func runPeer(args []string) error {
	if len(args) >= 1 && args[0] == "add" {
		fs := flag.NewFlagSet("peer add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		name := fs.String("name", "", "peer name")
		addr := fs.String("addr", "", "peer address")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *name == "" {
			*name = prompt("Peer Name")
		}
		if err := record.ValidateNodeName(*name); err != nil {
			return err
		}
		if *addr == "" {
			*addr = prompt("Peer Address")
		}
		store, err := config.NewStore("")
		if err != nil {
			return err
		}
		return store.AddPeer(*name, *addr)
	}
	fs := flag.NewFlagSet("peer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	names, peers, err := store.ListPeers()
	if err != nil {
		return err
	}
	printSectionHeader("PEER DIRECTORY", len(names))
	for _, n := range names {
		fmt.Printf("  %-15s %s\n", n, peerDirectoryState(peers[n].Address))
	}
	if len(names) == 0 {
		fmt.Println("  (none)")
	}
	return nil
}

func runBootstrap(args []string) error {
	if len(args) >= 1 && args[0] == "add" {
		fs := flag.NewFlagSet("bootstrap add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		addr := fs.String("addr", "", "bootstrap address")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *addr == "" {
			*addr = prompt("Bootstrap Address")
		}
		store, err := config.NewStore("")
		if err != nil {
			return err
		}
		return store.AddBootstrap(*addr)
	}
	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	list, err := store.ListBootstraps()
	if err != nil {
		return err
	}
	for _, b := range list {
		fmt.Println(b)
	}
	return nil
}

func runSend(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("file", "", "path to file")
	to := fs.String("to", "", "peer name")
	addrFlag := fs.String("addr", "", "peer IPv6 address")
	nodeName := fs.String("name", "", "local node name")
	proxy := fs.Bool("proxy", false, "proxy")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		*file = prompt("File Path")
	}
	if *to == "" && *addrFlag == "" {
		*to = prompt("Receiver Name")
	}
	if *file == "" {
		return errors.New("send requires --file")
	}
	if *to == "" && *addrFlag == "" {
		return errors.New("send requires --to or --addr")
	}
	if *to != "" && *addrFlag != "" {
		return errors.New("send accepts only one of --to or --addr")
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	idStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, err := idStore.Load()
	if err != nil {
		return err
	}
	if *nodeName == "" {
		*nodeName = cfg.Node.Name
	}
	if err := record.ValidateNodeName(*nodeName); err != nil {
		return err
	}

	addr := *addrFlag
	if addr == "" {
		addr, err = resolvePeerForSend(ctx, store, cfg, *to)
		if err != nil {
			return err
		}
	}

	dialer := func(rctx context.Context) (net.Conn, error) {
		if *proxy {
			reg, err := loadLocalRegistry(cfg.Node.DataDir)
			if err != nil {
				return nil, err
			}
			peers, _ := reg.Snapshot()
			conn, err := onion.BuildAutomatedCircuit(rctx, record.ServiceRecord{Address: addr}, peers, onion.ClientOptions{
				Identity:      id,
				TransportMode: cfg.Node.TransportMode,
			})
			if err != nil {
				return nil, friendlyRelayPathError(err, "proxy mode")
			}
			return conn, nil
		}
		return vxtransport.DialContext(rctx, cfg.Node.TransportMode, addr)
	}
	conn, err := dialer(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	res, err := transfer.SendFileWithConn(ctx, conn, transfer.SendRequest{NodeName: *nodeName, FilePath: *file, Address: addr, Identity: id})
	if err != nil {
		return err
	}
	fmt.Printf("sent\t%s\n", res.FileName)
	return nil
}

func runReceive(args []string) error {
	if len(args) == 0 {
		return runReceiveStatus(nil)
	}
	if args[0] == "status" {
		return runReceiveStatus(args[1:])
	}

	switch args[0] {
	case "allow":
		return runReceiveAllow(args[1:])
	case "deny":
		return runReceiveDeny(args[1:])
	case "disable":
		return runReceiveDisable(args[1:])
	default:
		return fmt.Errorf("unknown receive subcommand %q", args[0])
	}
}

func runReceiveStatus(args []string) error {
	fs := flag.NewFlagSet("receive status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	fmt.Printf("file_receive_mode\t%s\n", strings.ToUpper(cfg.Node.FileReceiveMode))
	fmt.Printf("allowed_senders\t%d\n", len(cfg.Node.AllowedFileSenders))
	for _, sender := range cfg.Node.AllowedFileSenders {
		fmt.Printf("allow\t%s\n", sender)
	}
	return nil
}

func runReceiveAllow(args []string) error {
	fs := flag.NewFlagSet("receive allow", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	all := fs.Bool("all", false, "allow files from any sender")
	nodeName := fs.String("node", "", "allow files from one node name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *all == (*nodeName != "") {
		return errors.New("receive allow requires exactly one of --all or --node")
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	if *all {
		cfg.Node.FileReceiveMode = config.FileReceiveOpen
		cfg.Node.AllowedFileSenders = nil
		if err := store.Save(cfg); err != nil {
			return err
		}
		fmt.Println("file_receive\tOPEN")
		return nil
	}

	if err := record.ValidateNodeName(*nodeName); err != nil {
		return err
	}
	cfg.Node.FileReceiveMode = config.FileReceiveTrusted
	cfg.Node.AllowedFileSenders = append(cfg.Node.AllowedFileSenders, *nodeName)
	if err := store.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("file_receive\tTRUSTED\nallow\t%s\n", *nodeName)
	return nil
}

func runReceiveDeny(args []string) error {
	fs := flag.NewFlagSet("receive deny", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	nodeName := fs.String("node", "", "deny files from one node name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *nodeName == "" {
		return errors.New("receive deny requires --node")
	}
	if err := record.ValidateNodeName(*nodeName); err != nil {
		return err
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	filtered := cfg.Node.AllowedFileSenders[:0]
	for _, sender := range cfg.Node.AllowedFileSenders {
		if sender == *nodeName {
			continue
		}
		filtered = append(filtered, sender)
	}
	cfg.Node.AllowedFileSenders = filtered
	if len(cfg.Node.AllowedFileSenders) == 0 {
		cfg.Node.FileReceiveMode = config.FileReceiveOff
	}
	if err := store.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("file_receive\t%s\ndeny\t%s\n", strings.ToUpper(cfg.Node.FileReceiveMode), *nodeName)
	return nil
}

func runReceiveDisable(args []string) error {
	fs := flag.NewFlagSet("receive disable", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	cfg.Node.FileReceiveMode = config.FileReceiveOff
	cfg.Node.AllowedFileSenders = nil
	if err := store.Save(cfg); err != nil {
		return err
	}
	fmt.Println("file_receive\tOFF")
	return nil
}

func runStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	controlPath, err := config.RuntimeControlPath(store.Path())
	if err != nil {
		return err
	}
	if status, err := runtimectl.RequestStatus(ctx, controlPath); err == nil {
		printRuntimeStatus("ONLINE", status)
		return nil
	}

	probeAddr := statusProbeAddr(cfg)
	conn, err := vxtransport.DialTimeout(cfg.Node.TransportMode, probeAddr, 500*time.Millisecond)
	if err != nil {
		printRuntimeStatus("OFFLINE", runtimectl.Status{
			NodeName:        cfg.Node.Name,
			EndpointPublish: endpointPublishMode(cfg.Node.HideEndpoint),
			TransportConfig: cfg.Node.TransportMode,
			TransportActive: vxtransport.EffectiveMode(cfg.Node.TransportMode),
			RelayMode:       cfg.Node.RelayMode,
			RelayPercent:    cfg.Node.RelayResourcePercent,
		})
		return nil
	}
	_ = conn.Close()

	registry, regErr := loadLocalRegistry(cfg.Node.DataDir)
	nodeCount := 0
	serviceCount := 0
	if regErr == nil {
		nodes, services := registry.Snapshot()
		nodeCount = len(nodes)
		serviceCount = len(services)
	}
	printRuntimeStatus("ONLINE", runtimectl.Status{
		NodeName:         cfg.Node.Name,
		EndpointPublish:  endpointPublishMode(cfg.Node.HideEndpoint),
		TransportConfig:  cfg.Node.TransportMode,
		TransportActive:  vxtransport.EffectiveMode(cfg.Node.TransportMode),
		RelayMode:        cfg.Node.RelayMode,
		RelayPercent:     cfg.Node.RelayResourcePercent,
		RegistryNodes:    nodeCount,
		RegistryServices: serviceCount,
	})
	return nil
}

func printRuntimeStatus(label string, status runtimectl.Status) {
	fmt.Printf("status\t%s\n", label)
	if status.NodeName != "" {
		fmt.Printf("node_name\t%s\n", status.NodeName)
	}
	if status.EndpointPublish != "" {
		fmt.Printf("endpoint_publish\t%s\n", status.EndpointPublish)
	}
	fmt.Printf("transport_config\t%s\n", status.TransportConfig)
	fmt.Printf("transport_active\t%s\n", status.TransportActive)
	fmt.Printf("relay_mode\t%s\n", status.RelayMode)
	fmt.Printf("relay_percent\t%d\n", status.RelayPercent)
	if status.PID > 0 {
		fmt.Printf("pid\t%d\n", status.PID)
	}
	if status.UptimeSeconds > 0 {
		fmt.Printf("uptime_seconds\t%d\n", status.UptimeSeconds)
	}
	fmt.Printf("registry_nodes\t%d\n", status.RegistryNodes)
	fmt.Printf("registry_services\t%d\n", status.RegistryServices)
}

func statusProbeAddr(cfg config.File) string {
	probe := cfg.Node.ListenAddr
	host, port, err := net.SplitHostPort(probe)
	if err != nil {
		return probe
	}

	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		if cfg.Node.AdvertiseAddr != "" {
			return cfg.Node.AdvertiseAddr
		}
		return net.JoinHostPort("::1", port)
	}

	return probe
}

func runIdentity(args []string) error {
	fs := flag.NewFlagSet("identity", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	idStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, err := idStore.Load()
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	fmt.Printf("node_name\t%s\n", cfg.Node.Name)
	fmt.Printf("node_id\t%s\n", id.NodeID)
	fmt.Printf("endpoint_publish\t%s\n", endpointPublishMode(cfg.Node.HideEndpoint))
	fmt.Printf("transport_config\t%s\n", cfg.Node.TransportMode)
	fmt.Printf("transport_active\t%s\n", vxtransport.EffectiveMode(cfg.Node.TransportMode))
	fmt.Printf("relay_mode\t%s\n", cfg.Node.RelayMode)
	fmt.Printf("relay_percent\t%d\n", cfg.Node.RelayResourcePercent)
	fmt.Printf("config_path\t%s\n", store.Path())
	return nil
}

func runDebug(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printDebugUsage(os.Stderr)
		return errors.New("missing debug subcommand")
	}

	switch args[0] {
	case "registry":
		return runDebugRegistry(args[1:])
	case "dht-get":
		return runDebugDHTGet(ctx, args[1:])
	case "ebpf-status":
		return runDebugEBPFStatus(args[1:]...)
	case "ebpf-attach":
		return runDebugEBPFAttach(ctx, args[1:])
	case "ebpf-detach":
		return runDebugEBPFDetach(ctx, args[1:])
	default:
		printDebugUsage(os.Stderr)
		return fmt.Errorf("unknown debug subcommand %q", args[0])
	}
}

func printDebugUsage(w io.Writer) {
	fmt.Fprintln(w, "Debug commands:")
	fmt.Fprintln(w, "  vx6 debug registry")
	fmt.Fprintln(w, "  vx6 debug dht-get (--service NODE.SERVICE | --node NAME | --node-id ID | --key KEY)")
	fmt.Fprintln(w, "  vx6 debug ebpf-status [--iface IFACE]")
	fmt.Fprintln(w, "  vx6 debug ebpf-attach --iface IFACE")
	fmt.Fprintln(w, "  vx6 debug ebpf-detach --iface IFACE")
}

func runDebugRegistry(args []string) error {
	fs := flag.NewFlagSet("debug registry", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	reg, err := loadLocalRegistry(cfg.Node.DataDir)
	if err != nil {
		return err
	}

	nodes, services := reg.Snapshot()
	fmt.Printf("registry_path\t%s\n", filepath.Join(cfg.Node.DataDir, "registry.json"))
	fmt.Printf("node_records\t%d\n", len(nodes))
	fmt.Printf("service_records\t%d\n", len(services))
	for _, rec := range nodes {
		fmt.Printf("node\t%s\t%s\tendpoint=%s\n", rec.NodeName, rec.NodeID, endpointVisibilitySummary(rec.Address, false))
	}
	for _, svc := range services {
		fmt.Printf("service\tkey=%s\tnode=%s\tservice=%s\tendpoint=%s\thidden=%v\n", record.ServiceLookupKey(svc), svc.NodeName, svc.ServiceName, endpointVisibilitySummary(svc.Address, svc.IsHidden), svc.IsHidden)
	}
	return nil
}

func endpointPublishMode(hidden bool) string {
	if hidden {
		return "hidden"
	}
	return "published"
}

func peerDirectoryState(address string) string {
	if address == "" {
		return "missing"
	}
	return "configured"
}

func endpointVisibilitySummary(address string, isHidden bool) string {
	switch {
	case isHidden:
		return "hidden"
	case address != "":
		return "sealed"
	default:
		return "missing"
	}
}

func runDebugDHTGet(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("debug dht-get", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	key := fs.String("key", "", "raw DHT key")
	service := fs.String("service", "", "service name in node.service form")
	nodeName := fs.String("node", "", "node name")
	nodeID := fs.String("node-id", "", "node id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	chosen := 0
	for _, value := range []string{*key, *service, *nodeName, *nodeID} {
		if value != "" {
			chosen++
		}
	}
	if chosen != 1 {
		return errors.New("debug dht-get requires exactly one of --key, --service, --node, or --node-id")
	}

	store, err := config.NewStore("")
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	client := newDHTClient(cfg)
	switch {
	case *service != "":
		if strings.Contains(*service, ".") {
			*key = dht.ServiceKey(*service)
		} else {
			*key = dht.HiddenServiceKey(*service)
		}
	case *nodeName != "":
		*key = dht.NodeNameKey(*nodeName)
	case *nodeID != "":
		*key = dht.NodeIDKey(*nodeID)
	}

	value, err := client.RecursiveFindValue(ctx, *key)
	if err != nil {
		return err
	}

	var pretty any
	if err := json.Unmarshal([]byte(value), &pretty); err == nil {
		formatted, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Printf("%s\n", formatted)
		return nil
	}

	fmt.Println(value)
	return nil
}

func runDebugEBPFStatus(args ...string) error {
	fs := flag.NewFlagSet("debug ebpf-status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	iface := fs.String("iface", "", "network interface name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *iface == "" {
		printXDPStatus(onion.XDPStatus{
			EmbeddedBytecode:     onion.IsEBPFAvailable(),
			BytecodeSize:         len(onion.OnionRelayBytecode),
			CompatibilityWarning: "embedded XDP program targets the legacy VX6 onion header and is not yet the active fast path for the current encrypted relay data path",
		})
		fmt.Println("attach_state\tuse --iface IFACE for live kernel status")
		return nil
	}

	status, err := onion.NewXDPManager().Status(context.Background(), *iface)
	if err != nil {
		return err
	}
	printXDPStatus(status)
	return nil
}

func runDebugEBPFAttach(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("debug ebpf-attach", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	iface := fs.String("iface", "", "network interface name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *iface == "" {
		return errors.New("debug ebpf-attach requires --iface")
	}

	status, err := onion.NewXDPManager().Attach(ctx, *iface)
	if err != nil {
		return err
	}
	fmt.Println("ebpf_attach\tok")
	printXDPStatus(status)
	return nil
}

func runDebugEBPFDetach(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("debug ebpf-detach", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	iface := fs.String("iface", "", "network interface name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *iface == "" {
		return errors.New("debug ebpf-detach requires --iface")
	}

	status, err := onion.NewXDPManager().Detach(ctx, *iface)
	if err != nil {
		return err
	}
	fmt.Println("ebpf_detach\tok")
	printXDPStatus(status)
	return nil
}

func printXDPStatus(status onion.XDPStatus) {
	fmt.Printf("embedded_bytecode\t%v\n", status.EmbeddedBytecode)
	fmt.Printf("bytecode_size\t%d\n", status.BytecodeSize)
	if status.Interface != "" {
		fmt.Printf("iface\t%s\n", status.Interface)
	}
	fmt.Printf("xdp_attached\t%v\n", status.Attached)
	fmt.Printf("vx6_active\t%v\n", status.VX6Active)
	if status.Mode != "" {
		fmt.Printf("mode\t%s\n", status.Mode)
	}
	if status.ProgramName != "" {
		fmt.Printf("program_name\t%s\n", status.ProgramName)
	}
	if status.ProgramID > 0 {
		fmt.Printf("program_id\t%d\n", status.ProgramID)
	}
	if status.ProgramTag != "" {
		fmt.Printf("program_tag\t%s\n", status.ProgramTag)
	}
	if status.CompatibilityWarning != "" {
		fmt.Printf("compatibility_warning\t%s\n", status.CompatibilityWarning)
	}
}

func loadLocalRegistry(dataDir string) (*discovery.Registry, error) {
	if dataDir == "" {
		dataDir = defaultDataDirValue()
	}
	return discovery.NewRegistry(filepath.Join(dataDir, "registry.json"))
}

func defaultDataDirValue() string {
	path, err := config.DefaultDataDir()
	if err != nil {
		return filepath.Join(".", "vx6-data")
	}
	return path
}

func defaultDownloadDirValue() string {
	path, err := config.DefaultDownloadDir()
	if err != nil {
		return filepath.Join(".", "Downloads")
	}
	return path
}

func friendlyRelayPathError(err error, feature string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not enough peers in registry to build a"):
		return fmt.Errorf("%s requires more reachable VX6 nodes. your local registry does not have enough peers to build the relay path; keep the node running so it can sync more peers, then try again", feature)
	case strings.Contains(msg, "hidden service has no reachable introduction points"),
		strings.Contains(msg, "no rendezvous candidates available"),
		strings.Contains(msg, "failed to establish hidden-service circuit"),
		strings.Contains(msg, "no reachable guard or owner for hidden service"):
		return fmt.Errorf("%s requires more reachable VX6 nodes. your local registry does not currently have enough live intro, guard, or rendezvous peers; keep the node running so it can sync more peers, then try again", feature)
	default:
		return err
	}
}

func resolvePeerForSend(ctx context.Context, store *config.Store, cfg config.File, name string) (string, error) {
	p, err := store.ResolvePeer(name)
	if err == nil {
		return p.Address, nil
	}
	rec, err := resolveNodeDistributed(ctx, cfg, name)
	if err != nil {
		return "", err
	}
	_ = store.AddPeer(rec.NodeName, rec.Address)
	return rec.Address, nil
}

func resolveNodeDistributed(ctx context.Context, cfg config.File, name string) (record.EndpointRecord, error) {
	reg, _ := loadLocalRegistry(cfg.Node.DataDir)
	if reg != nil {
		nodes, _ := reg.Snapshot()
		for _, n := range nodes {
			if n.NodeName == name {
				return n, nil
			}
		}
	}

	if d := newDHTClient(cfg); d != nil {
		if value, err := d.RecursiveFindValue(ctx, dht.NodeNameKey(name)); err == nil && value != "" {
			var rec record.EndpointRecord
			if err := json.Unmarshal([]byte(value), &rec); err == nil {
				if verifyErr := record.VerifyEndpointRecord(rec, time.Now()); verifyErr == nil {
					return rec, nil
				}
			}
		}
	}

	for _, addr := range discoveryCandidates(cfg) {
		rec, err := discovery.Resolve(ctx, addr, name, "")
		if err == nil {
			return rec, nil
		}
	}
	return record.EndpointRecord{}, errors.New("not found")
}

func resolveServiceDistributed(ctx context.Context, cfg config.File, service string) (record.ServiceRecord, error) {
	reg, _ := loadLocalRegistry(cfg.Node.DataDir)
	if reg != nil {
		if rec, err := reg.ResolveServiceLocal(service); err == nil {
			return rec, nil
		}
	}

	if d := newDHTClient(cfg); d != nil {
		keys := serviceLookupKeys(service)
		for _, key := range keys {
			if val, err := d.RecursiveFindValue(ctx, key); err == nil && val != "" {
				var r record.ServiceRecord
				if err := json.Unmarshal([]byte(val), &r); err == nil {
					if verifyErr := record.VerifyServiceRecord(r, time.Now()); verifyErr == nil {
						return r, nil
					}
				}
			}
		}
	}

	for _, addr := range discoveryCandidates(cfg) {
		rec, err := discovery.ResolveService(ctx, addr, service)
		if err == nil {
			return rec, nil
		}
	}
	return record.ServiceRecord{}, errors.New("not found")
}

func requestedServiceName(input string) string {
	if !strings.Contains(input, ".") {
		return input
	}
	parts := strings.Split(input, ".")
	return parts[len(parts)-1]
}

func serviceLookupKeys(service string) []string {
	if strings.Contains(service, ".") {
		return []string{dht.ServiceKey(service)}
	}
	return []string{dht.HiddenServiceKey(service), dht.ServiceKey(service)}
}

func discoveryCandidates(cfg config.File) []string {
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

	for _, addr := range cfg.Node.BootstrapAddrs {
		add(addr)
	}
	for _, peer := range cfg.Peers {
		add(peer.Address)
	}
	if registry, err := loadLocalRegistry(cfg.Node.DataDir); err == nil {
		nodes, _ := registry.Snapshot()
		for _, rec := range nodes {
			add(rec.Address)
		}
	}
	return out
}

func newDHTClient(cfg config.File) *dht.Server {
	client := dht.NewServer("cli-observer")

	for _, addr := range cfg.Node.BootstrapAddrs {
		if addr != "" {
			client.RT.AddNode(proto.NodeInfo{ID: "seed:" + addr, Addr: addr})
		}
	}
	for name, peer := range cfg.Peers {
		if peer.Address == "" {
			continue
		}
		client.RT.AddNode(proto.NodeInfo{ID: "peer:" + name + ":" + peer.Address, Addr: peer.Address})
	}
	if registry, err := loadLocalRegistry(cfg.Node.DataDir); err == nil {
		nodes, _ := registry.Snapshot()
		for _, rec := range nodes {
			if rec.NodeID != "" && rec.Address != "" {
				client.RT.AddNode(proto.NodeInfo{ID: rec.NodeID, Addr: rec.Address})
			}
		}
	}
	return client
}

func writePIDFile(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", pid)), 0o644)
}

type nodeRuntimeLock struct {
	file        *os.File
	lockPath    string
	pidPath     string
	controlPath string
}

func acquireNodeLock(configPath string) (*nodeRuntimeLock, error) {
	lockPath, err := config.RuntimeLockPath(configPath)
	if err != nil {
		return nil, err
	}
	pidPath, err := config.RuntimePIDPath(configPath)
	if err != nil {
		return nil, err
	}
	controlPath, err := config.RuntimeControlPath(configPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create runtime directory: %w", err)
	}

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open runtime lock: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, runningNodeError(pidPath)
		}
		return nil, fmt.Errorf("lock runtime state: %w", err)
	}
	if err := lockFile.Truncate(0); err != nil {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
		return nil, fmt.Errorf("reset runtime lock: %w", err)
	}
	if _, err := lockFile.Seek(0, 0); err != nil {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
		return nil, fmt.Errorf("seek runtime lock: %w", err)
	}
	if _, err := fmt.Fprintf(lockFile, "%d\n", os.Getpid()); err != nil {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
		return nil, fmt.Errorf("write runtime lock: %w", err)
	}
	if err := lockFile.Sync(); err != nil {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
		return nil, fmt.Errorf("sync runtime lock: %w", err)
	}
	if err := writePIDFile(pidPath, os.Getpid()); err != nil {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
		return nil, err
	}

	return &nodeRuntimeLock{file: lockFile, lockPath: lockPath, pidPath: pidPath, controlPath: controlPath}, nil
}

func (l *nodeRuntimeLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	_ = os.Remove(l.pidPath)
	_ = os.Remove(l.controlPath)
	_ = l.file.Truncate(0)
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	err := l.file.Close()
	l.file = nil
	_ = os.Remove(l.lockPath)
	return err
}

func runningNodeError(pidPath string) error {
	pid, err := readNodePID(pidPath)
	if err == nil && pid > 0 {
		return fmt.Errorf("vx6 node is already running in the background (pid %d). use 'vx6 status', 'vx6 reload', or 'systemctl --user restart vx6'", pid)
	}
	return errors.New("vx6 node is already running in the background. use 'vx6 status', 'vx6 reload', or 'systemctl --user restart vx6'")
}

func readNodePID(pidPath string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return pid, nil
}
