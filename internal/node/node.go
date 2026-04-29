package node

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/vx6/vx6/internal/config"
	"github.com/vx6/vx6/internal/dht"
	"github.com/vx6/vx6/internal/discovery"
	"github.com/vx6/vx6/internal/hidden"
	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/netutil"
	"github.com/vx6/vx6/internal/onion"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/secure"
	"github.com/vx6/vx6/internal/serviceproxy"
	"github.com/vx6/vx6/internal/transfer"
)

const (
	syncCycleInterval   = 30 * time.Second
	syncTargetTimeout   = 2 * time.Second
	syncProbeTimeout    = 1 * time.Second
	syncMaxRounds       = 3
	syncParallelTargets = 6
)

type ServiceRefresher func() map[string]string

type Config struct {
	Name            string
	NodeID          string
	ListenAddr      string
	AdvertiseAddr   string
	HideEndpoint    bool
	DataDir         string
	ReceiveDir      string
	ConfigPath      string
	RefreshServices ServiceRefresher
	BootstrapAddrs  []string
	Services        map[string]string
	Identity        identity.Identity
	Registry        *discovery.Registry
	DHT             *dht.Server
	Reload          <-chan struct{}
}

const SeedBootstrapDomain = "bootstrap.vx6.dev"

func Run(ctx context.Context, log io.Writer, cfg Config) error {
	if cfg.Name == "" {
		return errors.New("node name cannot be empty")
	}
	if cfg.NodeID == "" {
		return errors.New("node id cannot be empty")
	}
	if cfg.Registry == nil {
		return errors.New("registry cannot be nil")
	}
	cfg = refreshAdvertiseAddress(log, cfg)
	if err := transfer.ValidateIPv6Address(cfg.ListenAddr); err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if cfg.ReceiveDir == "" {
		cfg.ReceiveDir = defaultReceiveDir(cfg.DataDir)
	}
	if err := os.MkdirAll(cfg.ReceiveDir, 0o755); err != nil {
		return fmt.Errorf("create receive directory: %w", err)
	}
	if len(cfg.Services) == 0 && cfg.RefreshServices != nil {
		cfg.Services = cfg.RefreshServices()
	}
	if cfg.Services == nil {
		cfg.Services = map[string]string{}
	}

	listener, err := net.Listen("tcp6", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}
	defer listener.Close()

	fmt.Fprintf(log, "vx6 node %q (%s) listening on %s\n", cfg.Name, cfg.NodeID, listener.Addr().String())

	if cfg.AdvertiseAddr != "" {
		go runBootstrapTasks(ctx, log, cfg)
		go runLocalDiscovery(ctx, log, cfg)
	}

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() {
				continue
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept connection: %w", err)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer conn.Close()
			reader := bufio.NewReader(conn)
			kind, err := proto.ReadHeader(reader)
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				fmt.Fprintf(log, "session error from %s: %v\n", conn.RemoteAddr().String(), err)
				return
			}

			switch kind {
			case proto.KindFileTransfer:
				secureConn, err := secure.Server(&bufferedConn{Conn: conn, reader: reader}, proto.KindFileTransfer, cfg.Identity)
				if err != nil {
					fmt.Fprintf(log, "secure receive error from %s: %v\n", conn.RemoteAddr().String(), err)
					return
				}
				res, err := transfer.ReceiveFile(secureConn, cfg.ReceiveDir)
				if err != nil {
					fmt.Fprintf(log, "receive error from %s: %v\n", conn.RemoteAddr().String(), err)
					return
				}
				absPath, pathErr := filepath.Abs(res.StoredPath)
				if pathErr != nil {
					absPath = res.StoredPath
				}
				fmt.Fprintf(log, "received %q (%d bytes) from node %q into %s\n", res.FileName, res.BytesReceived, res.SenderNode, absPath)
			case proto.KindDiscoveryReq:
				if err := cfg.Registry.HandleConn(&bufferedConn{Conn: conn, reader: reader}); err != nil {
					fmt.Fprintf(log, "discovery error from %s: %v\n", conn.RemoteAddr().String(), err)
					return
				}
				fmt.Fprintf(log, "processed discovery request from %s\n", conn.RemoteAddr().String())
			case proto.KindDHT:
				payload, err := proto.ReadLengthPrefixed(reader, 1024*1024)
				if err != nil {
					fmt.Fprintf(log, "dht read error from %s: %v\n", conn.RemoteAddr().String(), err)
					return
				}
				var dr proto.DHTRequest
				if err := json.Unmarshal(payload, &dr); err != nil {
					fmt.Fprintf(log, "dht decode error from %s: %v\n", conn.RemoteAddr().String(), err)
					return
				}
				if cfg.DHT != nil {
					if err := cfg.DHT.HandleDHT(ctx, conn, dr); err != nil {
						fmt.Fprintf(log, "dht error from %s: %v\n", conn.RemoteAddr().String(), err)
					}
				}
			case proto.KindExtend:
				payload, err := proto.ReadLengthPrefixed(reader, 1024*1024)
				if err != nil {
					fmt.Fprintf(log, "extend read error from %s: %v\n", conn.RemoteAddr().String(), err)
					return
				}
				var er proto.ExtendRequest
				if err := json.Unmarshal(payload, &er); err != nil {
					fmt.Fprintf(log, "extend decode error from %s: %v\n", conn.RemoteAddr().String(), err)
					return
				}
				if err := onion.HandleExtend(ctx, conn, er); err != nil {
					fmt.Fprintf(log, "extend error from %s: %v\n", conn.RemoteAddr().String(), err)
				}
			case proto.KindRendezvous:
				liveServices := runtimeServices(cfg)
				if err := hidden.HandleConn(ctx, &bufferedConn{Conn: conn, reader: reader}, hidden.HandlerConfig{
					Identity:      cfg.Identity,
					Services:      liveServices,
					HiddenAliases: hiddenAliasMap(cfg.ConfigPath),
					Registry:      cfg.Registry,
				}); err != nil {
					fmt.Fprintf(log, "hidden service error from %s: %v\n", conn.RemoteAddr().String(), err)
				}
			case proto.KindServiceConn:
				if err := serviceproxy.HandleInbound(&bufferedConn{Conn: conn, reader: reader}, cfg.Identity, runtimeServices(cfg)); err != nil {
					fmt.Fprintf(log, "service proxy error from %s: %v\n", conn.RemoteAddr().String(), err)
				}
			default:
				fmt.Fprintf(log, "session error from %s: unsupported kind %d\n", conn.RemoteAddr().String(), kind)
			}
		}()
	}
}

func runLocalDiscovery(ctx context.Context, log io.Writer, cfg Config) {
	const multicastAddr = "[ff02::1]:4243"
	addr, _ := net.ResolveUDPAddr("udp6", multicastAddr)
	conn, err := net.ListenMulticastUDP("udp6", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil || n == 0 {
				return
			}
			var info proto.NodeInfo
			if err := json.Unmarshal(buf[:n], &info); err == nil && info.ID != cfg.NodeID {
				rec := record.EndpointRecord{NodeID: info.ID, NodeName: info.Name, Address: info.Addr}
				_ = cfg.Registry.Import([]record.EndpointRecord{rec}, nil)
			}
		}
	}()

	ticker := time.NewTicker(15 * time.Second)
	data, _ := json.Marshal(proto.NodeInfo{ID: cfg.NodeID, Name: cfg.Name, Addr: cfg.AdvertiseAddr})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = conn.WriteToUDP(data, addr)
		}
	}
}

func runBootstrapTasks(ctx context.Context, log io.Writer, cfg Config) {
	ips, _ := net.LookupIP(SeedBootstrapDomain)
	dnsSeeds := []string{}
	for _, ip := range ips {
		if ip.To4() == nil {
			dnsSeeds = append(dnsSeeds, fmt.Sprintf("[%s]:4242", ip.String()))
		}
	}

	publishAndSync := func() {
		liveCfg := runtimeConfig(cfg)
		rec, err := record.NewEndpointRecord(liveCfg.Identity, liveCfg.Name, liveCfg.AdvertiseAddr, 20*time.Minute, time.Now())
		if err != nil {
			return
		}
		if !liveCfg.HideEndpoint {
			_ = liveCfg.Registry.Import([]record.EndpointRecord{rec}, nil)
		}

		nodes, _ := liveCfg.Registry.Snapshot()
		seedDHTRouting(liveCfg.DHT, liveCfg.BootstrapAddrs, nodes)
		targets := syncMesh(ctx, log, liveCfg, rec, dnsSeeds, nodes)

		nodes, _ = liveCfg.Registry.Snapshot()
		hidden.TrackAddresses(ctx, nodeAddresses(nodes), 30*time.Second)
		serviceRecords, hiddenTopologies := buildServiceRecords(ctx, liveCfg, nodes)
		publishServicesToTargets(ctx, liveCfg, log, targets, serviceRecords)

		for _, srec := range serviceRecords {
			if !srec.IsHidden {
				continue
			}
			topology := hiddenTopologies[record.ServiceLookupKey(srec)]
			notifyAddrs := append([]string(nil), topology.Guards...)
			if len(notifyAddrs) == 0 {
				notifyAddrs = []string{liveCfg.AdvertiseAddr}
			}
			hidden.TrackAddresses(ctx, append(append([]string(nil), topology.ActiveIntros...), append(topology.StandbyIntros, topology.Guards...)...), 20*time.Second)
			for _, guardAddr := range topology.Guards {
				_ = hidden.RegisterGuard(ctx, guardAddr, record.ServiceLookupKey(srec), liveCfg.AdvertiseAddr)
			}
			for _, introAddr := range append([]string(nil), srec.IntroPoints...) {
				_ = hidden.RegisterIntro(ctx, introAddr, record.ServiceLookupKey(srec), notifyAddrs)
			}
			for _, introAddr := range srec.StandbyIntroPoints {
				_ = hidden.RegisterIntro(ctx, introAddr, record.ServiceLookupKey(srec), notifyAddrs)
			}
		}

		publishRecordsToTargets(ctx, liveCfg, log, targets, rec, serviceRecords)

		publishDHTRecords(ctx, liveCfg.DHT, rec, serviceRecords, liveCfg.HideEndpoint)
	}

	publishAndSync()
	ticker := time.NewTicker(syncCycleInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-cfg.Reload:
			fmt.Fprintf(log, "[RELOAD] configuration refresh requested\n")
			publishAndSync()
		case <-ticker.C:
			publishAndSync()
		}
	}
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

func hiddenAliasMap(configPath string) map[string]string {
	entries := loadServiceEntries(configPath)
	out := make(map[string]string, len(entries))
	for name, entry := range entries {
		if entry.IsHidden && entry.Alias != "" {
			out[entry.Alias] = name
		}
	}
	return out
}

func loadServiceEntries(configPath string) map[string]config.ServiceEntry {
	if configPath == "" {
		return nil
	}
	store, err := config.NewStore(configPath)
	if err != nil {
		return nil
	}
	cfgFile, err := store.Load()
	if err != nil {
		return nil
	}
	out := make(map[string]config.ServiceEntry, len(cfgFile.Services))
	for name, entry := range cfgFile.Services {
		out[name] = entry
	}
	return out
}

func seedDHTRouting(server *dht.Server, seedAddrs []string, records []record.EndpointRecord) {
	if server == nil {
		return
	}

	for _, addr := range seedAddrs {
		if addr == "" {
			continue
		}
		server.RT.AddNode(proto.NodeInfo{ID: "seed:" + addr, Addr: addr})
	}
	for _, rec := range records {
		if rec.NodeID == "" || rec.Address == "" {
			continue
		}
		server.RT.AddNode(proto.NodeInfo{ID: rec.NodeID, Addr: rec.Address})
	}
}

func publishDHTRecords(ctx context.Context, server *dht.Server, endpoint record.EndpointRecord, services []record.ServiceRecord, hideEndpoint bool) {
	if server == nil {
		return
	}

	if !hideEndpoint {
		if data, err := json.Marshal(endpoint); err == nil {
			payload := string(data)
			_ = server.Store(ctx, dht.NodeNameKey(endpoint.NodeName), payload)
			_ = server.Store(ctx, dht.NodeIDKey(endpoint.NodeID), payload)
		}
	}

	for _, svc := range services {
		if data, err := json.Marshal(svc); err == nil {
			payload := string(data)
			if svc.IsHidden && svc.Alias != "" {
				_ = server.Store(ctx, dht.HiddenServiceKey(svc.Alias), payload)
			}
			_ = server.Store(ctx, dht.ServiceKey(record.FullServiceName(svc.NodeName, svc.ServiceName)), payload)
		}
	}
}

func buildServiceRecords(ctx context.Context, cfg Config, nodes []record.EndpointRecord) ([]record.ServiceRecord, map[string]hidden.Topology) {
	serviceRecords := make([]record.ServiceRecord, 0, len(cfg.Services))
	topologies := make(map[string]hidden.Topology)
	entries := loadServiceEntries(cfg.ConfigPath)

	for name := range cfg.Services {
		entry := entries[name]
		isHidden := entry.IsHidden
		svcAddr := cfg.AdvertiseAddr
		if isHidden {
			svcAddr = ""
		}
		srec, err := record.NewServiceRecord(cfg.Identity, cfg.Name, name, svcAddr, 20*time.Minute, time.Now())
		if err != nil {
			continue
		}

		srec.IsHidden = isHidden
		if isHidden {
			topology := hidden.SelectTopology(ctx, cfg.AdvertiseAddr, nodes, entry.IntroNodes, entry.IntroMode, entry.HiddenProfile)
			srec.Alias = entry.Alias
			if srec.Alias == "" {
				srec.Alias = name
			}
			srec.HiddenProfile = record.NormalizeHiddenProfile(entry.HiddenProfile)
			srec.IntroPoints = append([]string(nil), topology.ActiveIntros...)
			srec.StandbyIntroPoints = append([]string(nil), topology.StandbyIntros...)
			topologies[record.ServiceLookupKey(srec)] = topology
		}
		_ = record.SignServiceRecord(cfg.Identity, &srec)
		_ = cfg.Registry.Import(nil, []record.ServiceRecord{srec})
		serviceRecords = append(serviceRecords, srec)
	}

	return serviceRecords, topologies
}

func nodeAddresses(nodes []record.EndpointRecord) []string {
	out := make([]string, 0, len(nodes))
	for _, nodeRec := range nodes {
		if nodeRec.Address == "" {
			continue
		}
		out = append(out, nodeRec.Address)
	}
	return out
}

func syncMesh(ctx context.Context, log io.Writer, cfg Config, rec record.EndpointRecord, dnsSeeds []string, initialNodes []record.EndpointRecord) map[string]struct{} {
	targets := map[string]struct{}{}
	reachable := map[string]struct{}{}
	for _, addr := range dnsSeeds {
		addSyncTarget(targets, cfg.AdvertiseAddr, addr)
	}
	for _, addr := range cfg.BootstrapAddrs {
		addSyncTarget(targets, cfg.AdvertiseAddr, addr)
	}
	for _, nodeRec := range initialNodes {
		if nodeRec.NodeID == cfg.NodeID {
			continue
		}
		addSyncTarget(targets, cfg.AdvertiseAddr, nodeRec.Address)
	}

	synced := map[string]struct{}{}
	for round := 0; round < syncMaxRounds; round++ {
		pending := pendingSyncTargets(targets, synced)
		if len(pending) == 0 {
			break
		}

		results := syncTargetBatch(ctx, log, cfg, rec, pending)
		for _, result := range results {
			synced[result.addr] = struct{}{}
			if result.err != nil {
				continue
			}
			reachable[result.addr] = struct{}{}
			_ = cfg.Registry.Import(result.records, result.services)
			seedDHTRouting(cfg.DHT, nil, result.records)
			for _, nodeRec := range result.records {
				if nodeRec.NodeID == cfg.NodeID {
					continue
				}
				addSyncTarget(targets, cfg.AdvertiseAddr, nodeRec.Address)
			}
		}

		nodes, _ := cfg.Registry.Snapshot()
		for _, nodeRec := range nodes {
			if nodeRec.NodeID == cfg.NodeID {
				continue
			}
			addSyncTarget(targets, cfg.AdvertiseAddr, nodeRec.Address)
		}
	}

	return reachable
}

type syncResult struct {
	addr     string
	records  []record.EndpointRecord
	services []record.ServiceRecord
	err      error
}

func syncTargetBatch(ctx context.Context, log io.Writer, cfg Config, rec record.EndpointRecord, targets []string) []syncResult {
	results := make([]syncResult, 0, len(targets))
	if len(targets) == 0 {
		return results
	}

	sem := make(chan struct{}, syncParallelTargets)
	resultsCh := make(chan syncResult, len(targets))
	var wg sync.WaitGroup

	for _, addr := range targets {
		addr := addr
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				resultsCh <- syncResult{addr: addr, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			resultsCh <- syncTarget(ctx, log, cfg, rec, addr)
		}()
	}

	wg.Wait()
	close(resultsCh)

	for result := range resultsCh {
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].addr < results[j].addr
	})
	return results
}

func syncTarget(ctx context.Context, log io.Writer, cfg Config, rec record.EndpointRecord, addr string) syncResult {
	result := syncResult{addr: addr}
	if addr == "" {
		return result
	}

	if !probeSyncTarget(ctx, addr) {
		fmt.Fprintf(log, "[SYNC] Skipping unreachable target: %s\n", addr)
		result.err = fmt.Errorf("unreachable")
		return result
	}

	fmt.Fprintf(log, "[SYNC] Connecting to target: %s\n", addr)
	if !cfg.HideEndpoint {
		publishCtx, cancel := withSyncTimeout(ctx)
		_, err := discovery.Publish(publishCtx, addr, rec)
		cancel()
		if err != nil {
			fmt.Fprintf(log, "[SYNC] Publish to %s failed: %v\n", addr, err)
		}
	}

	snapshotCtx, cancel := withSyncTimeout(ctx)
	recs, svcs, err := discovery.Snapshot(snapshotCtx, addr)
	cancel()
	if err != nil {
		fmt.Fprintf(log, "[SYNC] Snapshot from %s failed: %v\n", addr, err)
		result.err = err
		return result
	}

	result.records = recs
	result.services = svcs
	fmt.Fprintf(log, "[SYNC] Successfully linked with %s. Received %d records.\n", addr, len(recs)+len(svcs))
	return result
}

func publishServicesToTargets(ctx context.Context, cfg Config, log io.Writer, targets map[string]struct{}, serviceRecords []record.ServiceRecord) {
	for _, addr := range pendingSyncTargets(targets, nil) {
		for _, srec := range serviceRecords {
			publishCtx, cancel := withSyncTimeout(ctx)
			_, err := discovery.PublishService(publishCtx, addr, srec)
			cancel()
			if err != nil {
				fmt.Fprintf(log, "[SYNC] Service publish to %s failed: %v\n", addr, err)
			}
		}
	}
}

func publishRecordsToTargets(ctx context.Context, cfg Config, log io.Writer, targets map[string]struct{}, rec record.EndpointRecord, serviceRecords []record.ServiceRecord) {
	for _, addr := range pendingSyncTargets(targets, nil) {
		if !cfg.HideEndpoint {
			publishCtx, cancel := withSyncTimeout(ctx)
			_, err := discovery.Publish(publishCtx, addr, rec)
			cancel()
			if err != nil {
				fmt.Fprintf(log, "[SYNC] Publish to %s failed: %v\n", addr, err)
			}
		}
		for _, srec := range serviceRecords {
			publishCtx, cancel := withSyncTimeout(ctx)
			_, err := discovery.PublishService(publishCtx, addr, srec)
			cancel()
			if err != nil {
				fmt.Fprintf(log, "[SYNC] Service publish to %s failed: %v\n", addr, err)
			}
		}
	}
}

func pendingSyncTargets(targets map[string]struct{}, synced map[string]struct{}) []string {
	out := make([]string, 0, len(targets))
	for addr := range targets {
		if addr == "" {
			continue
		}
		if synced != nil {
			if _, ok := synced[addr]; ok {
				continue
			}
		}
		out = append(out, addr)
	}
	sort.Strings(out)
	return out
}

func addSyncTarget(targets map[string]struct{}, selfAddr, addr string) {
	if addr == "" || addr == selfAddr {
		return
	}
	targets[addr] = struct{}{}
}

func probeSyncTarget(ctx context.Context, addr string) bool {
	dialCtx, cancel := context.WithTimeout(ctx, syncProbeTimeout)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp6", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func withSyncTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, syncTargetTimeout)
}

func runtimeConfig(base Config) Config {
	live := base
	if base.ConfigPath == "" {
		if len(live.Services) == 0 && base.RefreshServices != nil {
			live.Services = base.RefreshServices()
		}
		if updated, _, err := netutil.RefreshAdvertiseAddress(live.AdvertiseAddr, live.ListenAddr); err == nil {
			live.AdvertiseAddr = updated
		}
		return live
	}

	store, err := config.NewStore(base.ConfigPath)
	if err != nil {
		return live
	}
	cfgFile, err := store.Load()
	if err != nil {
		return live
	}

	if cfgFile.Node.Name != "" {
		live.Name = cfgFile.Node.Name
	}
	if cfgFile.Node.AdvertiseAddr != "" {
		live.AdvertiseAddr = cfgFile.Node.AdvertiseAddr
	}
	live.HideEndpoint = cfgFile.Node.HideEndpoint
	live.BootstrapAddrs = append([]string(nil), cfgFile.Node.BootstrapAddrs...)
	live.Services = serviceTargets(cfgFile.Services)
	if len(live.Services) == 0 && base.RefreshServices != nil {
		live.Services = base.RefreshServices()
	}
	live.ReceiveDir = cfgFile.Node.DownloadDir
	if updated, _, err := netutil.RefreshAdvertiseAddress(live.AdvertiseAddr, live.ListenAddr); err == nil {
		live.AdvertiseAddr = updated
	}
	return live
}

func runtimeServices(base Config) map[string]string {
	return runtimeConfig(base).Services
}

func serviceTargets(entries map[string]config.ServiceEntry) map[string]string {
	out := make(map[string]string, len(entries))
	for name, entry := range entries {
		out[name] = entry.Target
	}
	return out
}

func refreshAdvertiseAddress(log io.Writer, cfg Config) Config {
	updated, changed, err := netutil.RefreshAdvertiseAddress(cfg.AdvertiseAddr, cfg.ListenAddr)
	if err != nil || updated == "" {
		return cfg
	}
	if changed {
		if cfg.AdvertiseAddr == "" {
			fmt.Fprintf(log, "auto-detected advertise address %s\n", updated)
		} else {
			fmt.Fprintf(log, "advertise address updated from %s to %s\n", cfg.AdvertiseAddr, updated)
		}
	}
	cfg.AdvertiseAddr = updated
	return cfg
}

func defaultReceiveDir(dataDir string) string {
	if path, err := config.DefaultDownloadDir(); err == nil {
		return path
	}
	if dataDir != "" {
		return dataDir
	}
	return filepath.Join(".", "Downloads")
}
