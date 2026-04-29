package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vx6/vx6/internal/config"
	"github.com/vx6/vx6/internal/dht"
	"github.com/vx6/vx6/internal/discovery"
	"github.com/vx6/vx6/internal/hidden"
	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/netutil"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/secure"
	"github.com/vx6/vx6/internal/serviceproxy"
)

const (
	chatServiceName  = "chat"
	serviceRecordTTL = 20 * time.Minute
	deliveryTimeout  = 6 * time.Second
	ackTimeout       = 5 * time.Second
	maxChatFrameSize = 256 * 1024
)

type runtimeContext struct {
	store     *config.Store
	cfg       config.File
	id        identity.Identity
	registry  *discovery.Registry
	advertise string
	statePath string
}

type wireEnvelope struct {
	Type    string   `json:"type"`
	Message *Message `json:"message,omitempty"`
	AckID   string   `json:"ack_id,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func loadRuntime(configPath string) (runtimeContext, error) {
	store, err := config.NewStore(configPath)
	if err != nil {
		return runtimeContext{}, err
	}
	cfg, err := store.Load()
	if err != nil {
		return runtimeContext{}, err
	}
	if cfg.Node.Name == "" {
		return runtimeContext{}, errors.New("vx6 node is not initialized; run 'vx6 init' first")
	}

	idStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return runtimeContext{}, err
	}
	id, err := idStore.Load()
	if err != nil {
		return runtimeContext{}, err
	}

	registry, err := discovery.NewRegistry(filepath.Join(cfg.Node.DataDir, "registry.json"))
	if err != nil {
		return runtimeContext{}, err
	}

	advertise := cfg.Node.AdvertiseAddr
	if advertise == "" {
		advertise = detectAdvertise(cfg.Node.ListenAddr)
	}
	if advertise == "" {
		return runtimeContext{}, errors.New("vx6 advertise address is empty; set --advertise in vx6 init")
	}

	statePath := filepath.Join(filepath.Dir(store.Path()), "chat", "state.json")
	return runtimeContext{
		store:     store,
		cfg:       cfg,
		id:        id,
		registry:  registry,
		advertise: advertise,
		statePath: statePath,
	}, nil
}

func detectAdvertise(listenAddr string) string {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return ""
	}
	addr, err := netutil.DetectAdvertiseAddress(port)
	if err != nil {
		return ""
	}
	return addr
}

func ensureChatService(store *config.Store, transportAddr string) (bool, error) {
	cfg, err := store.Load()
	if err != nil {
		return false, err
	}
	entry := config.ServiceEntry{Target: transportAddr}
	current, ok := cfg.Services[chatServiceName]
	if ok && current.Target == transportAddr && !current.IsHidden {
		return false, nil
	}
	cfg.Services[chatServiceName] = entry
	if err := store.Save(cfg); err != nil {
		return false, err
	}
	return true, nil
}

func signalVX6Reload(store *config.Store) error {
	pidPath, err := config.RuntimePIDPath(store.Path())
	if err != nil {
		return err
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return err
	}
	pidText := strings.TrimSpace(string(data))
	if pidText == "" {
		return errors.New("empty vx6 pid file")
	}
	var pid int
	if _, err := fmt.Sscanf(pidText, "%d", &pid); err != nil {
		return err
	}
	return syscall.Kill(pid, syscall.SIGHUP)
}

func publishChatService(ctx context.Context, rt runtimeContext) error {
	rec, err := record.NewServiceRecord(rt.id, rt.cfg.Node.Name, chatServiceName, rt.advertise, serviceRecordTTL, time.Now())
	if err != nil {
		return err
	}
	if err := rt.registry.Import(nil, []record.ServiceRecord{rec}); err != nil {
		return err
	}

	for _, addr := range discoveryCandidates(rt.cfg, rt.registry) {
		publishCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err := discovery.PublishService(publishCtx, addr, rec)
		cancel()
		if err != nil {
			continue
		}
	}

	client := newDHTClient(rt.cfg, rt.registry)
	if client != nil {
		_ = client.SetPublisherIdentity(rt.id)
		data, err := json.Marshal(rec)
		if err == nil {
			storeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_ = client.Store(storeCtx, dht.ServiceKey(record.FullServiceName(rec.NodeName, rec.ServiceName)), string(data))
			cancel()
		}
	}
	return nil
}

func dialChatPeer(ctx context.Context, rt runtimeContext, peer string) (net.Conn, error) {
	serviceName := peer + "." + chatServiceName
	rec, err := resolveServiceDistributed(ctx, rt.cfg, rt.registry, serviceName)
	if err != nil {
		return nil, err
	}

	var baseConn net.Conn
	switch {
	case rec.IsHidden:
		baseConn, err = hidden.DialHiddenServiceWithOptions(ctx, rec, rt.registry, hidden.DialOptions{SelfAddr: rt.advertise})
	case rec.Address != "":
		var d net.Dialer
		baseConn, err = d.DialContext(ctx, "tcp6", rec.Address)
	default:
		err = errors.New("chat service record missing address")
	}
	if err != nil {
		return nil, err
	}

	if err := proto.WriteHeader(baseConn, proto.KindServiceConn); err != nil {
		_ = baseConn.Close()
		return nil, err
	}
	secureConn, err := secure.Client(baseConn, proto.KindServiceConn, rt.id)
	if err != nil {
		_ = baseConn.Close()
		return nil, err
	}

	payload, err := json.Marshal(serviceproxy.ConnectRequest{ServiceName: chatServiceName})
	if err != nil {
		_ = secureConn.Close()
		return nil, err
	}
	if err := proto.WriteLengthPrefixed(secureConn, payload); err != nil {
		_ = secureConn.Close()
		return nil, err
	}
	return secureConn, nil
}

func readEnvelope(r io.Reader) (wireEnvelope, error) {
	payload, err := proto.ReadLengthPrefixed(r, maxChatFrameSize)
	if err != nil {
		return wireEnvelope{}, err
	}
	var env wireEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return wireEnvelope{}, err
	}
	return env, nil
}

func writeEnvelope(w io.Writer, env wireEnvelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return proto.WriteLengthPrefixed(w, payload)
}

func resolveServiceDistributed(ctx context.Context, cfg config.File, registry *discovery.Registry, service string) (record.ServiceRecord, error) {
	if registry != nil {
		if rec, err := registry.ResolveServiceLocal(service); err == nil {
			return rec, nil
		}
	}

	if d := newDHTClient(cfg, registry); d != nil {
		if strings.Contains(service, ".") {
			if val, err := d.RecursiveFindValue(ctx, dht.ServiceKey(service)); err == nil && val != "" {
				var rec record.ServiceRecord
				if err := json.Unmarshal([]byte(val), &rec); err == nil {
					if verifyErr := record.VerifyServiceRecord(rec, time.Now()); verifyErr == nil {
						return rec, nil
					}
				}
			}
		} else {
			for _, key := range dht.HiddenServiceLookupKeys(service, time.Now()) {
				if val, err := d.RecursiveFindValue(ctx, key); err == nil && val != "" {
					if rec, err := dht.DecodeHiddenServiceRecord(key, val, service, time.Now()); err == nil {
						return rec, nil
					}
				}
			}
		}
	}

	for _, addr := range discoveryCandidates(cfg, registry) {
		rec, err := discovery.ResolveService(ctx, addr, service)
		if err == nil {
			return rec, nil
		}
	}
	return record.ServiceRecord{}, errors.New("service not found")
}

func discoveryCandidates(cfg config.File, registry *discovery.Registry) []string {
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
	if registry != nil {
		nodes, _ := registry.Snapshot()
		for _, node := range nodes {
			add(node.Address)
		}
	}
	return out
}

func serviceLookupKeys(service string) []string {
	if strings.Contains(service, ".") {
		return []string{dht.ServiceKey(service)}
	}
	return dht.HiddenServiceLookupKeys(service, time.Now())
}

func newDHTClient(cfg config.File, registry *discovery.Registry) *dht.Server {
	client := dht.NewServer("vx6-chat-client")
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
	if registry != nil {
		nodes, _ := registry.Snapshot()
		for _, rec := range nodes {
			if rec.NodeID != "" && rec.Address != "" {
				client.RT.AddNode(proto.NodeInfo{ID: rec.NodeID, Addr: rec.Address})
			}
		}
	}
	return client
}

func peerViews(self string, registry *discovery.Registry) []PeerView {
	if registry == nil {
		return nil
	}
	nodes, services := registry.Snapshot()
	chatByNode := map[string]bool{}
	for _, svc := range services {
		if svc.ServiceName == chatServiceName {
			chatByNode[svc.NodeName] = true
		}
	}

	seen := map[string]struct{}{}
	out := make([]PeerView, 0, len(nodes))
	for _, rec := range nodes {
		if rec.NodeName == "" || rec.NodeName == self {
			continue
		}
		if _, ok := seen[rec.NodeName]; ok {
			continue
		}
		seen[rec.NodeName] = struct{}{}
		out = append(out, PeerView{
			Name:    rec.NodeName,
			HasChat: chatByNode[rec.NodeName],
		})
	}
	return out
}
