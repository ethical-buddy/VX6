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
	"sync"
	"time"

	"github.com/vx6/vx6/internal/config"
	"github.com/vx6/vx6/internal/dht"
	"github.com/vx6/vx6/internal/discovery"
	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/netutil"
	"github.com/vx6/vx6/internal/onion"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/secure"
	"github.com/vx6/vx6/internal/serviceproxy"
	"github.com/vx6/vx6/internal/transfer"
)

type ServiceRefresher func() map[string]string

type Config struct {
	Name            string
	NodeID          string
	ListenAddr      string
	AdvertiseAddr   string
	DataDir         string
	ConfigPath      string
	RefreshServices ServiceRefresher
	BootstrapAddrs  []string
	Services        map[string]string
	Identity        identity.Identity
	Registry        *discovery.Registry
	DHT             *dht.Server
}

const SeedBootstrapDomain = "bootstrap.vx6.dev"

func Run(ctx context.Context, log io.Writer, cfg Config) error {
	if cfg.Name == "" { return errors.New("node name cannot be empty") }
	if cfg.NodeID == "" { return errors.New("node id cannot be empty") }
	if cfg.Registry == nil { return errors.New("registry cannot be nil") }
	if err := transfer.ValidateIPv6Address(cfg.ListenAddr); err != nil { return fmt.Errorf("invalid listen address: %w", err) }
	_ = os.MkdirAll(cfg.DataDir, 0o755)

	if cfg.AdvertiseAddr == "" {
		_, port, _ := net.SplitHostPort(cfg.ListenAddr)
		addr, detectErr := netutil.DetectAdvertiseAddress(port)
		if detectErr == nil { cfg.AdvertiseAddr = addr }
	}

	listener, err := net.Listen("tcp6", cfg.ListenAddr)
	if err != nil { return fmt.Errorf("listen fail: %w", err) }
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
			if ctx.Err() != nil { return nil }
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer conn.Close()
			reader := bufio.NewReader(conn)
			kind, err := proto.ReadHeader(reader)
			if err != nil { return }

			switch kind {
			case proto.KindFileTransfer:
				secureConn, _ := secure.Server(&bufferedConn{Conn: conn, reader: reader}, proto.KindFileTransfer, cfg.Identity)
				res, _ := transfer.ReceiveFile(secureConn, cfg.DataDir)
				fmt.Fprintf(log, "received %q from %q\n", res.FileName, res.SenderNode)
			case proto.KindDiscoveryReq:
				_ = cfg.Registry.HandleConn(&bufferedConn{Conn: conn, reader: reader})
			case proto.KindDHT:
				payload, _ := proto.ReadLengthPrefixed(reader, 1024*1024)
				var dr proto.DHTRequest
				_ = json.Unmarshal(payload, &dr)
				if cfg.DHT != nil { _ = cfg.DHT.HandleDHT(ctx, conn, dr) }
			case proto.KindExtend:
				payload, _ := proto.ReadLengthPrefixed(reader, 1024*1024)
				var er proto.ExtendRequest
				_ = json.Unmarshal(payload, &er)
				_ = onion.HandleExtend(ctx, conn, er)
			case proto.KindServiceConn:
				_ = serviceproxy.HandleInbound(&bufferedConn{Conn: conn, reader: reader}, cfg.Identity, cfg.Services)
			}
		}()
	}
}

func runLocalDiscovery(ctx context.Context, log io.Writer, cfg Config) {
	const multicastAddr = "[ff02::1]:4243"
	addr, _ := net.ResolveUDPAddr("udp6", multicastAddr)
	conn, err := net.ListenMulticastUDP("udp6", nil, addr)
	if err != nil { return }
	defer conn.Close()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil || n == 0 { return }
			var info proto.NodeInfo
			if err := json.Unmarshal(buf[:n], &info); err == nil && info.ID != cfg.NodeID {
				rec := record.EndpointRecord{NodeID: info.ID, NodeName: "local-neighbor", Address: info.Addr}
				_ = cfg.Registry.Import([]record.EndpointRecord{rec}, nil)
			}
		}
	}()

	ticker := time.NewTicker(15 * time.Second)
	data, _ := json.Marshal(proto.NodeInfo{ID: cfg.NodeID, Addr: cfg.AdvertiseAddr})
	for {
		select {
		case <-ctx.Done(): return
		case <-ticker.C: _, _ = conn.WriteToUDP(data, addr)
		}
	}
}

func runBootstrapTasks(ctx context.Context, log io.Writer, cfg Config) {
	ips, _ := net.LookupIP(SeedBootstrapDomain)
	dnsSeeds := []string{}
	for _, ip := range ips { if ip.To4() == nil { dnsSeeds = append(dnsSeeds, fmt.Sprintf("[%s]:4242", ip.String())) } }

	publishAndSync := func() {
		if cfg.RefreshServices != nil { cfg.Services = cfg.RefreshServices() }
		rec, err := record.NewEndpointRecord(cfg.Identity, cfg.Name, cfg.AdvertiseAddr, 20*time.Minute, time.Now())
		if err != nil { return }
		_ = cfg.Registry.Import([]record.EndpointRecord{rec}, nil)

		targets := map[string]struct{}{}
		for _, a := range dnsSeeds { targets[a] = struct{}{} }
		for _, a := range cfg.BootstrapAddrs { targets[a] = struct{}{} }
		nodes, _ := cfg.Registry.Snapshot()
		for i := 0; i < len(nodes) && i < 5; i++ { if nodes[i].Address != "" { targets[nodes[i].Address] = struct{}{} } }

		for addr := range targets {
			fmt.Fprintf(log, "[SYNC] Connecting to target: %s\n", addr)
			_, err := discovery.Publish(ctx, addr, rec)
			if err != nil {
				fmt.Fprintf(log, "[SYNC] Publish to %s failed: %v\n", addr, err)
				continue
			}

			for name := range cfg.Services {
				isHidden := false; var introPoints []string
				if cfg.ConfigPath != "" {
					if store, _ := config.NewStore(cfg.ConfigPath); store != nil {
						if c, _ := store.Load(); c.Node.Name != "" { if s, ok := c.Services[name]; ok { isHidden = s.IsHidden } }
					}
				}
				svcAddr := cfg.AdvertiseAddr; if isHidden { svcAddr = "" }
				srec, err := record.NewServiceRecord(cfg.Identity, cfg.Name, name, svcAddr, 20*time.Minute, time.Now())
				if err == nil {
					srec.IsHidden = isHidden
					if isHidden {
						for i := 0; i < len(nodes) && len(introPoints) < 3; i++ { if nodes[i].NodeID != "" { introPoints = append(introPoints, nodes[i].NodeID) } }
						srec.IntroPoints = introPoints
					}
					_ = record.SignServiceRecord(cfg.Identity, &srec)
					_ = cfg.Registry.Import(nil, []record.ServiceRecord{srec})
					_, _ = discovery.PublishService(ctx, addr, srec)
				}
			}

			// PULL: Download the phonebook from the bootstrap
			recs, svcs, err := discovery.Snapshot(ctx, addr)
			if err != nil {
				fmt.Fprintf(log, "[SYNC] Snapshot from %s failed: %v\n", addr, err)
				continue
			}
			_ = cfg.Registry.Import(recs, svcs)
			fmt.Fprintf(log, "[SYNC] Successfully linked with %s. Received %d records.\n", addr, len(recs)+len(svcs))
		}
	}

	publishAndSync()
	ticker := time.NewTicker(2 * time.Minute)
	for { select { case <-ctx.Done(): return; case <-ticker.C: publishAndSync() } }
}

type bufferedConn struct { net.Conn; reader *bufio.Reader }
func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
