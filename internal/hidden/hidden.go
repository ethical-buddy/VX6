package hidden

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vx6/vx6/internal/discovery"
	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/onion"
	"github.com/vx6/vx6/internal/proto"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/serviceproxy"
)

const (
	activeIntroCount  = 3
	standbyIntroCount = 2
	guardCount        = 2

	IntroModeRandom = "random"
	IntroModeManual = "manual"
	IntroModeHybrid = "hybrid"
)

type Message struct {
	Action               string   `json:"action"`
	Service              string   `json:"service,omitempty"`
	OwnerAddr            string   `json:"owner_addr,omitempty"`
	NotifyAddrs          []string `json:"notify_addrs,omitempty"`
	RendezvousID         string   `json:"rendezvous_id,omitempty"`
	RendezvousCandidates []string `json:"rendezvous_candidates,omitempty"`
	HopCount             int      `json:"hop_count,omitempty"`
	RelayExcludes        []string `json:"relay_excludes,omitempty"`
}

type HandlerConfig struct {
	Identity      identity.Identity
	Services      map[string]string
	HiddenAliases map[string]string
	Registry      *discovery.Registry
}

type Topology struct {
	ActiveIntros  []string
	StandbyIntros []string
	Guards        []string
}

type DialOptions struct {
	SelfAddr string
}

type rendezvousWait struct {
	peerCh chan net.Conn
	doneCh chan struct{}
}

type introRegistration struct {
	NotifyAddrs []string
}

type guardRegistration struct {
	OwnerAddr string
}

type peerScore struct {
	Addr      string
	NodeName  string
	Prefix    string
	RTT       time.Duration
	Healthy   bool
	Failures  int
	Preferred bool
}

type healthEntry struct {
	Healthy     bool
	RTT         time.Duration
	LastChecked time.Time
	Failures    int
}

var (
	introMu       sync.RWMutex
	introServices = map[string]introRegistration{}

	guardMu       sync.RWMutex
	guardServices = map[string]guardRegistration{}

	rendezvousMu sync.Mutex
	rendezvouses = map[string]*rendezvousWait{}

	healthMu    sync.Mutex
	healthCache = map[string]healthEntry{}

	trackerMu    sync.Mutex
	trackersByIP = map[string]struct{}{}
)

func RegisterIntro(ctx context.Context, introAddr, lookupKey string, notifyAddrs []string) error {
	msg := Message{
		Action:      "intro_register",
		Service:     lookupKey,
		NotifyAddrs: append([]string(nil), notifyAddrs...),
	}
	return sendControl(ctx, introAddr, msg)
}

func RegisterGuard(ctx context.Context, guardAddr, lookupKey, ownerAddr string) error {
	msg := Message{
		Action:    "guard_register",
		Service:   lookupKey,
		OwnerAddr: ownerAddr,
	}
	return sendControl(ctx, guardAddr, msg)
}

func DialHiddenService(ctx context.Context, service record.ServiceRecord, registry *discovery.Registry) (net.Conn, error) {
	return DialHiddenServiceWithOptions(ctx, service, registry, DialOptions{})
}

func DialHiddenServiceWithOptions(ctx context.Context, service record.ServiceRecord, registry *discovery.Registry, opts DialOptions) (net.Conn, error) {
	if !service.IsHidden {
		return nil, fmt.Errorf("service %s is not hidden", record.FullServiceName(service.NodeName, service.ServiceName))
	}
	if registry == nil {
		return nil, fmt.Errorf("hidden service dialing requires a local registry")
	}

	nodes, _ := registry.Snapshot()
	introPool := append([]string(nil), service.IntroPoints...)
	introPool = append(introPool, service.StandbyIntroPoints...)
	introPool = rankAddresses(ctx, introPool)
	if len(introPool) == 0 {
		return nil, fmt.Errorf("hidden service has no reachable introduction points")
	}

	excluded := append([]string(nil), introPool...)
	if opts.SelfAddr != "" {
		excluded = append(excluded, opts.SelfAddr)
	}
	excluded = sanitizeAddressList(excluded)
	rendezvousCandidates := SelectRendezvousCandidates(ctx, nodes, excluded, 3)
	if len(rendezvousCandidates) == 0 {
		return nil, fmt.Errorf("no rendezvous candidates available")
	}
	primeHealth(ctx, append(append([]string(nil), introPool...), rendezvousCandidates...))

	hopCount := hopCountForProfile(service.HiddenProfile)
	lookupKey := record.ServiceLookupKey(service)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	var lastErr error
	for _, introAddr := range introPool {
		rendezvousID := fmt.Sprintf("rv_%d", rng.Int63())
		for _, rendezvousAddr := range rendezvousCandidates {
			plan, err := onion.PlanAutomatedCircuit(record.ServiceRecord{Address: rendezvousAddr}, nodes, hopCount, excluded)
			if err != nil {
				lastErr = err
				continue
			}
			candidateOrder := preferAddressFirst(rendezvousCandidates, rendezvousAddr)

			relayExcludes := append([]string(nil), excluded...)
			relayExcludes = append(relayExcludes, candidateOrder...)
			relayExcludes = append(relayExcludes, plan.RelayAddrs()...)
			relayExcludes = sanitizeAddressList(relayExcludes)

			if err := sendControl(ctx, introAddr, Message{
				Action:               "intro_request",
				Service:              lookupKey,
				RendezvousID:         rendezvousID,
				RendezvousCandidates: candidateOrder,
				HopCount:             hopCount,
				RelayExcludes:        relayExcludes,
			}); err != nil {
				lastErr = err
				continue
			}

			conn, err := onion.DialPlannedCircuit(ctx, plan)
			if err != nil {
				lastErr = err
				continue
			}
			if err := writeControl(conn, Message{
				Action:       "rv_join",
				RendezvousID: rendezvousID,
			}); err != nil {
				_ = conn.Close()
				lastErr = err
				continue
			}
			return conn, nil
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("failed to establish hidden-service circuit")
	}
	return nil, lastErr
}

func HandleConn(ctx context.Context, conn net.Conn, cfg HandlerConfig) error {
	msg, err := readControl(conn)
	if err != nil {
		return err
	}

	switch msg.Action {
	case "intro_register":
		introMu.Lock()
		introServices[msg.Service] = introRegistration{NotifyAddrs: sanitizeAddressList(msg.NotifyAddrs)}
		introMu.Unlock()
		return nil
	case "guard_register":
		guardMu.Lock()
		guardServices[msg.Service] = guardRegistration{OwnerAddr: msg.OwnerAddr}
		guardMu.Unlock()
		return nil
	case "intro_request":
		introMu.RLock()
		reg := introServices[msg.Service]
		introMu.RUnlock()
		if len(reg.NotifyAddrs) == 0 {
			return fmt.Errorf("hidden service %q is not registered on this intro point", msg.Service)
		}
		notifyAddrs := rankAddresses(ctx, reg.NotifyAddrs)
		for _, addr := range notifyAddrs {
			if err := sendControl(ctx, addr, Message{
				Action:               "guard_notify",
				Service:              msg.Service,
				RendezvousID:         msg.RendezvousID,
				RendezvousCandidates: append([]string(nil), msg.RendezvousCandidates...),
				HopCount:             msg.HopCount,
				RelayExcludes:        append([]string(nil), msg.RelayExcludes...),
			}); err == nil {
				return nil
			}
		}
		return fmt.Errorf("no reachable guard or owner for hidden service %q", msg.Service)
	case "guard_notify":
		guardMu.RLock()
		reg := guardServices[msg.Service]
		guardMu.RUnlock()
		if reg.OwnerAddr == "" {
			return handleIntroNotify(ctx, msg, cfg)
		}
		return sendControl(ctx, reg.OwnerAddr, Message{
			Action:               "intro_notify",
			Service:              msg.Service,
			RendezvousID:         msg.RendezvousID,
			RendezvousCandidates: append([]string(nil), msg.RendezvousCandidates...),
			HopCount:             msg.HopCount,
			RelayExcludes:        append([]string(nil), msg.RelayExcludes...),
		})
	case "intro_notify":
		return handleIntroNotify(ctx, msg, cfg)
	case "rv_join", "rv_register":
		return joinRendezvous(conn, msg.RendezvousID)
	default:
		return fmt.Errorf("unknown hidden action %q", msg.Action)
	}
}

func handleIntroNotify(ctx context.Context, msg Message, cfg HandlerConfig) error {
	if cfg.Registry == nil {
		return fmt.Errorf("hidden service owner requires a registry")
	}

	serviceName := resolveHostedService(msg.Service, cfg)
	if serviceName == "" {
		return fmt.Errorf("hidden service %q is not hosted on this node", msg.Service)
	}

	nodes, _ := cfg.Registry.Snapshot()
	hopCount := msg.HopCount
	if hopCount <= 0 {
		hopCount = 3
	}

	rankedCandidates := sanitizeAddressList(msg.RendezvousCandidates)
	var lastErr error
	for _, candidate := range rankedCandidates {
		plan, err := onion.PlanAutomatedCircuit(record.ServiceRecord{Address: candidate}, nodes, hopCount, msg.RelayExcludes)
		if err != nil {
			lastErr = err
			continue
		}

		conn, err := onion.DialPlannedCircuit(ctx, plan)
		if err != nil {
			lastErr = err
			continue
		}
		if err := writeControl(conn, Message{
			Action:       "rv_register",
			RendezvousID: msg.RendezvousID,
		}); err != nil {
			_ = conn.Close()
			lastErr = err
			continue
		}

		reader := bufio.NewReader(conn)
		kind, err := proto.ReadHeader(reader)
		if err != nil {
			_ = conn.Close()
			return err
		}
		if kind != proto.KindServiceConn {
			_ = conn.Close()
			return fmt.Errorf("unexpected hidden-service follow-up kind %d", kind)
		}
		return serviceproxy.HandleInbound(&bufferedConn{Conn: conn, reader: reader}, cfg.Identity, cfg.Services)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("failed to connect owner side to rendezvous")
	}
	return lastErr
}

func joinRendezvous(conn net.Conn, rendezvousID string) error {
	if rendezvousID == "" {
		return fmt.Errorf("missing rendezvous id")
	}

	rendezvousMu.Lock()
	wait, ok := rendezvouses[rendezvousID]
	if !ok {
		wait = &rendezvousWait{
			peerCh: make(chan net.Conn),
			doneCh: make(chan struct{}),
		}
		rendezvouses[rendezvousID] = wait
		rendezvousMu.Unlock()

		peer := <-wait.peerCh
		err := proxyDuplex(conn, peer)
		close(wait.doneCh)
		return err
	}
	delete(rendezvouses, rendezvousID)
	rendezvousMu.Unlock()

	wait.peerCh <- conn
	<-wait.doneCh
	return nil
}

func SelectTopology(ctx context.Context, selfAddr string, nodes []record.EndpointRecord, preferred []string, introMode, profile string) Topology {
	_ = profile // reserved for future profile-specific topology sizing.

	candidates := dedupeCandidates(nodes, map[string]struct{}{selfAddr: {}})
	if len(candidates) == 0 {
		return Topology{}
	}

	preferredAddrs := resolvePreferredAddressesOrdered(candidates, preferred)
	scored := scoreCandidates(ctx, candidates, nil, nil)
	scored = prioritizeScores(scored, preferredAddrs, introMode, activeIntroCount+standbyIntroCount)

	used := map[string]struct{}{}
	intros := pickAddresses(scored, activeIntroCount+standbyIntroCount, used)
	topology := Topology{}
	if len(intros) > activeIntroCount {
		topology.ActiveIntros = append([]string(nil), intros[:activeIntroCount]...)
		topology.StandbyIntros = append([]string(nil), intros[activeIntroCount:]...)
	} else {
		topology.ActiveIntros = append([]string(nil), intros...)
	}

	guardCandidates := scoreCandidates(ctx, candidates, nil, used)
	guardCandidates = randomizeTopScores(guardCandidates, guardCount)
	topology.Guards = pickAddresses(guardCandidates, guardCount, used)
	return topology
}

func SelectRendezvousCandidates(ctx context.Context, nodes []record.EndpointRecord, excludeAddrs []string, count int) []string {
	exclude := make(map[string]struct{}, len(excludeAddrs))
	for _, addr := range excludeAddrs {
		if addr != "" {
			exclude[addr] = struct{}{}
		}
	}
	candidates := dedupeCandidates(nodes, exclude)
	scored := scoreCandidates(ctx, candidates, nil, exclude)
	scored = randomizeTopScores(scored, count)
	return pickAddresses(scored, count, map[string]struct{}{})
}

func NormalizeIntroMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", IntroModeRandom:
		return IntroModeRandom
	case IntroModeManual:
		return IntroModeManual
	case IntroModeHybrid:
		return IntroModeHybrid
	default:
		return ""
	}
}

func TrackAddresses(ctx context.Context, addrs []string, interval time.Duration) {
	if interval <= 0 {
		interval = 20 * time.Second
	}
	for _, addr := range sanitizeAddressList(addrs) {
		if addr == "" {
			continue
		}

		trackerMu.Lock()
		if _, ok := trackersByIP[addr]; ok {
			trackerMu.Unlock()
			continue
		}
		trackersByIP[addr] = struct{}{}
		trackerMu.Unlock()

		go func(addr string) {
			defer func() {
				trackerMu.Lock()
				delete(trackersByIP, addr)
				trackerMu.Unlock()
			}()

			primeHealth(context.Background(), []string{addr})
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					primeHealth(context.Background(), []string{addr})
				}
			}
		}(addr)
	}
}

func hopCountForProfile(profile string) int {
	if record.NormalizeHiddenProfile(profile) == "balanced" {
		return 5
	}
	return 3
}

func resolveHostedService(lookup string, cfg HandlerConfig) string {
	if name := cfg.HiddenAliases[lookup]; name != "" {
		return name
	}
	if _, ok := cfg.Services[lookup]; ok {
		return lookup
	}
	if strings.Contains(lookup, ".") {
		parts := strings.Split(lookup, ".")
		name := parts[len(parts)-1]
		if _, ok := cfg.Services[name]; ok {
			return name
		}
	}
	return ""
}

func sanitizeAddressList(addrs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	return out
}

func resolvePreferredAddressesOrdered(nodes []record.EndpointRecord, selectors []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		for _, node := range nodes {
			if node.Address == "" {
				continue
			}
			if selector == node.Address || selector == node.NodeName {
				if _, ok := seen[node.Address]; ok {
					continue
				}
				seen[node.Address] = struct{}{}
				out = append(out, node.Address)
			}
		}
	}
	return out
}

func dedupeCandidates(nodes []record.EndpointRecord, exclude map[string]struct{}) []record.EndpointRecord {
	seen := map[string]struct{}{}
	out := make([]record.EndpointRecord, 0, len(nodes))
	for _, node := range nodes {
		if node.Address == "" {
			continue
		}
		if _, ok := exclude[node.Address]; ok {
			continue
		}
		if _, ok := seen[node.Address]; ok {
			continue
		}
		seen[node.Address] = struct{}{}
		out = append(out, node)
	}
	return out
}

func scoreCandidates(ctx context.Context, nodes []record.EndpointRecord, preferred map[string]bool, exclude map[string]struct{}) []peerScore {
	scored := make([]peerScore, 0, len(nodes))
	for _, node := range nodes {
		if node.Address == "" {
			continue
		}
		if exclude != nil {
			if _, ok := exclude[node.Address]; ok {
				continue
			}
		}
		healthy, rtt, failures := measureHealth(ctx, node.Address)
		scored = append(scored, peerScore{
			Addr:      node.Address,
			NodeName:  node.NodeName,
			Prefix:    addrPrefix(node.Address),
			RTT:       rtt,
			Healthy:   healthy,
			Failures:  failures,
			Preferred: preferred[node.Address],
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Preferred != scored[j].Preferred {
			return scored[i].Preferred
		}
		if scored[i].Healthy != scored[j].Healthy {
			return scored[i].Healthy
		}
		if scored[i].Failures != scored[j].Failures {
			return scored[i].Failures < scored[j].Failures
		}
		if scored[i].RTT != scored[j].RTT {
			return scored[i].RTT < scored[j].RTT
		}
		if scored[i].NodeName != scored[j].NodeName {
			return scored[i].NodeName < scored[j].NodeName
		}
		return scored[i].Addr < scored[j].Addr
	})
	return scored
}

func prioritizeScores(scored []peerScore, preferredAddrs []string, introMode string, count int) []peerScore {
	introMode = NormalizeIntroMode(introMode)
	if introMode == "" {
		introMode = IntroModeRandom
	}
	if introMode == IntroModeRandom {
		return randomizeTopScores(scored, count)
	}

	preferred := make([]peerScore, 0, len(preferredAddrs))
	remaining := make([]peerScore, 0, len(scored))
	byAddr := make(map[string]peerScore, len(scored))
	for _, candidate := range scored {
		byAddr[candidate.Addr] = candidate
	}
	selected := map[string]struct{}{}
	for _, addr := range preferredAddrs {
		candidate, ok := byAddr[addr]
		if !ok {
			continue
		}
		preferred = append(preferred, candidate)
		selected[addr] = struct{}{}
	}
	for _, candidate := range scored {
		if _, ok := selected[candidate.Addr]; ok {
			continue
		}
		remaining = append(remaining, candidate)
	}
	if introMode == IntroModeHybrid {
		remaining = randomizeTopScores(remaining, count-len(preferred))
	}
	return append(preferred, remaining...)
}

func randomizeTopScores(scored []peerScore, count int) []peerScore {
	if len(scored) <= 1 {
		return scored
	}
	limit := count * 4
	if limit <= 0 || limit > len(scored) {
		limit = len(scored)
	}
	head := append([]peerScore(nil), scored[:limit]...)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(head), func(i, j int) {
		head[i], head[j] = head[j], head[i]
	})
	out := append([]peerScore(nil), head...)
	out = append(out, scored[limit:]...)
	return out
}

func pickAddresses(scored []peerScore, count int, used map[string]struct{}) []string {
	if count <= 0 {
		return nil
	}

	out := make([]string, 0, count)
	usedPrefixes := map[string]struct{}{}

	pick := func(requireFreshPrefix bool) {
		for _, candidate := range scored {
			if len(out) >= count {
				return
			}
			if _, ok := used[candidate.Addr]; ok {
				continue
			}
			if requireFreshPrefix && candidate.Prefix != "" {
				if _, ok := usedPrefixes[candidate.Prefix]; ok {
					continue
				}
			}
			used[candidate.Addr] = struct{}{}
			if candidate.Prefix != "" {
				usedPrefixes[candidate.Prefix] = struct{}{}
			}
			out = append(out, candidate.Addr)
		}
	}

	pick(true)
	pick(false)
	return out
}

func rankAddresses(ctx context.Context, addrs []string) []string {
	nodes := make([]record.EndpointRecord, 0, len(addrs))
	for _, addr := range sanitizeAddressList(addrs) {
		nodes = append(nodes, record.EndpointRecord{NodeName: addr, Address: addr})
	}
	scored := scoreCandidates(ctx, nodes, nil, nil)
	out := make([]string, 0, len(scored))
	for _, candidate := range scored {
		out = append(out, candidate.Addr)
	}
	return out
}

func preferAddressFirst(addrs []string, first string) []string {
	out := make([]string, 0, len(addrs))
	if first != "" {
		out = append(out, first)
	}
	for _, addr := range sanitizeAddressList(addrs) {
		if addr == first {
			continue
		}
		out = append(out, addr)
	}
	return out
}

func addrPrefix(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	ip = ip.To16()
	if ip == nil {
		return ""
	}
	return fmt.Sprintf("%x:%x:%x:%x", ip[0:2], ip[2:4], ip[4:6], ip[6:8])
}

func primeHealth(ctx context.Context, addrs []string) {
	for _, addr := range sanitizeAddressList(addrs) {
		if addr == "" {
			continue
		}
		_, _, _ = measureHealth(ctx, addr)
	}
}

func measureHealth(ctx context.Context, addr string) (bool, time.Duration, int) {
	healthMu.Lock()
	entry, ok := healthCache[addr]
	if ok && time.Since(entry.LastChecked) < 30*time.Second {
		healthMu.Unlock()
		return entry.Healthy, entry.RTT, entry.Failures
	}
	healthMu.Unlock()

	timeout := 300 * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline) / 2; remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp6", addr)
	healthy := err == nil
	rtt := timeout
	failures := entry.Failures
	if healthy {
		rtt = time.Since(start)
		_ = conn.Close()
		failures = 0
	} else {
		failures++
	}

	healthMu.Lock()
	healthCache[addr] = healthEntry{
		Healthy:     healthy,
		RTT:         rtt,
		LastChecked: time.Now(),
		Failures:    failures,
	}
	healthMu.Unlock()
	return healthy, rtt, failures
}

func sendControl(ctx context.Context, addr string, msg Message) error {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp6", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	return writeControl(conn, msg)
}

func writeControl(conn net.Conn, msg Message) error {
	if err := proto.WriteHeader(conn, proto.KindRendezvous); err != nil {
		return err
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return proto.WriteLengthPrefixed(conn, payload)
}

func readControl(conn net.Conn) (Message, error) {
	payload, err := proto.ReadLengthPrefixed(conn, 1024*1024)
	if err != nil {
		return Message{}, err
	}
	var msg Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func proxyDuplex(a, b net.Conn) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	copyPipe := func(dst io.Writer, src io.Reader) {
		defer wg.Done()
		_, err := io.Copy(dst, src)
		errCh <- err
	}

	wg.Add(2)
	go copyPipe(a, b)
	go copyPipe(b, a)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil && err != io.EOF {
			return err
		}
	}
	return nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
