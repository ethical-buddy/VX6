package dht

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/proto"
)

type Server struct {
	RT        *RoutingTable
	Values    map[string]string // The decentralized database
	publisher identity.Identity
	versions  map[string]StoredValueState
	replicas  map[string]ReplicaObservation
	mu        sync.RWMutex
}

type lookupBranch struct {
	id         int
	rootNodeID string
	queue      []proto.NodeInfo
}

const (
	lookupAlpha              = 3
	lookupQueryBudget        = 12
	replicationFactor        = 5
	hiddenDescriptorRotation = time.Hour
)

type StoredVersion struct {
	Family          string
	Fingerprint     string
	PublisherNodeID string
	Version         uint64
	IssuedAt        string
	ExpiresAt       string
}

type StoredValueState struct {
	Current   StoredVersion
	Previous  []StoredVersion
	Conflicts []StoredVersion
}

type ReplicationReport struct {
	Key            string
	Desired        int
	Attempted      int
	StoredRemotely int
	LocalStored    bool
	Successful     []proto.NodeInfo
	Failed         []proto.NodeInfo
}

func NodeNameKey(name string) string {
	return "node/name/" + name
}

func NodeIDKey(nodeID string) string {
	return "node/id/" + nodeID
}

func ServiceKey(fullName string) string {
	return "service/" + fullName
}

func isTrustedLookupKey(key string) bool {
	return strings.HasPrefix(key, "node/name/") ||
		strings.HasPrefix(key, "node/id/") ||
		strings.HasPrefix(key, "service/") ||
		strings.HasPrefix(key, "hidden-desc/v1/") ||
		strings.HasPrefix(key, "private-catalog/")
}

func HiddenServiceKey(alias string) string {
	return HiddenServiceKeyAt(alias, time.Now())
}

func HiddenServiceKeyAt(alias string, now time.Time) string {
	return hiddenServiceKeyForEpoch(alias, hiddenDescriptorEpoch(now))
}

func HiddenServiceLookupKeys(alias string, now time.Time) []string {
	current := hiddenDescriptorEpoch(now)
	keys := []string{hiddenServiceKeyForEpoch(alias, current)}
	previous := current - 1
	if previous >= 0 {
		keys = append(keys, hiddenServiceKeyForEpoch(alias, previous))
	}
	return keys
}

func HiddenServicePublishKeys(alias string, now time.Time) []string {
	keys := HiddenServiceLookupKeys(alias, now)
	out := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func NewServer(selfID string) *Server {
	return &Server{
		RT:       NewRoutingTable(selfID),
		Values:   make(map[string]string),
		versions: make(map[string]StoredValueState),
		replicas: make(map[string]ReplicaObservation),
	}
}

func hiddenDescriptorEpoch(now time.Time) int64 {
	if now.IsZero() {
		now = time.Now()
	}
	return now.UTC().Unix() / int64(hiddenDescriptorRotation/time.Second)
}

func hiddenServiceKeyForEpoch(alias string, epoch int64) string {
	sum := sha256.Sum256([]byte("vx6-hidden-desc-v1\n" + alias + "\n" + strconv.FormatInt(epoch, 10)))
	return "hidden-desc/v1/" + strconv.FormatInt(epoch, 10) + "/" + base64.RawURLEncoding.EncodeToString(sum[:20])
}

func NewServerWithIdentity(id identity.Identity) *Server {
	server := NewServer(id.NodeID)
	_ = server.SetPublisherIdentity(id)
	return server
}

func (s *Server) SetPublisherIdentity(id identity.Identity) error {
	if err := id.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publisher = id
	return nil
}

func (s *Server) LookupState(key string) (StoredValueState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.versions[key]
	return state, ok
}

// HandleDHT processes an incoming DHT request from a peer.
func (s *Server) HandleDHT(ctx context.Context, conn net.Conn, req proto.DHTRequest) error {
	resp := proto.DHTResponse{}

	switch req.Action {
	case "find_node":
		resp.Nodes = s.RT.ClosestNodes(req.Target, K)
	case "find_value":
		resp.Nodes = s.RT.ClosestNodes(req.Target, K)
		s.mu.RLock()
		val, ok := s.Values[req.Target]
		s.mu.RUnlock()
		if ok {
			resp.Value = val
		}
	case "store":
		if err := s.admitStoreValue(req.Target, req.Data, time.Now()); err != nil {
			// Invalid or conflicting writes are ignored to keep the DHT conservative
			// under poisoning attempts.
			break
		}
		if _, _, err := s.storeValidated(req.Target, req.Data, time.Now()); err != nil {
			// Invalid or conflicting writes are ignored to keep the DHT conservative
			// under poisoning attempts.
		}
	}

	payload, _ := json.Marshal(resp)
	if err := proto.WriteHeader(conn, proto.KindDHT); err != nil {
		return err
	}
	return proto.WriteLengthPrefixed(conn, payload)
}

func (s *Server) StoreLocal(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Values[key] = value
	if validated, err := validateLookupValue(key, value, time.Now()); err == nil {
		s.versions[key] = StoredValueState{Current: storedVersionFromValidated(validated)}
	}
}

func (s *Server) admitStoreValue(key, value string, now time.Time) error {
	if !isTrustedLookupKey(key) {
		return nil
	}
	env, ok, err := maybeDecodeEnvelope(value)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("trusted key %q requires a signed DHT envelope", key)
	}
	validated, err := validateLookupValue(key, value, now)
	if err != nil {
		return err
	}
	if !validated.verified {
		return fmt.Errorf("trusted key %q requires a verified value", key)
	}
	if validated.originNodeID == "" || env.PublisherNodeID != validated.originNodeID {
		return fmt.Errorf("trusted key %q requires authoritative publisher %q", key, validated.originNodeID)
	}
	return nil
}

// RecursiveFindNode searches the network for a specific NodeID.
func (s *Server) RecursiveFindNode(ctx context.Context, targetID string) ([]proto.NodeInfo, error) {
	visited := make(map[string]bool)
	candidates := s.RT.ClosestNodes(targetID, K)

	for {
		foundNew := false
		newCandidates := []proto.NodeInfo{}
		for _, node := range candidates {
			if visited[node.ID] {
				continue
			}
			visited[node.ID] = true

			newNodes, err := s.QueryNode(ctx, node.Addr, targetID)
			if err != nil {
				s.RT.NoteFailure(node.ID)
				continue
			}
			s.RT.AddNode(node)
			for _, n := range newNodes {
				if !visited[n.ID] {
					s.RT.AddNode(n)
					newCandidates = append(newCandidates, n)
					foundNew = true
				}
			}
		}
		candidates = append(candidates, newCandidates...)

		if !foundNew {
			break
		}
	}

	return s.RT.ClosestNodes(targetID, K), nil
}

// Store saves a value on a bounded set of the closest nodes to the target key.
func (s *Server) Store(ctx context.Context, targetID, value string) error {
	_, err := s.MaintainReplicas(ctx, targetID, value)
	return err
}

// MaintainReplicas stores a validated value locally and then repairs the remote
// replica set by walking bounded backup candidates until the desired replica
// count is reached or no more candidates remain.
func (s *Server) MaintainReplicas(ctx context.Context, targetID, value string) (ReplicationReport, error) {
	report := ReplicationReport{Key: targetID}

	wrapped, err := s.prepareStoreValue(targetID, value, time.Now())
	if err != nil {
		return report, err
	}
	if _, _, err := s.storeValidated(targetID, wrapped, time.Now()); err != nil {
		return report, err
	}
	report.LocalStored = true

	candidates := selectReplicationNodes(s.RT.ClosestNodes(targetID, K), K)
	if len(candidates) < replicationFactor {
		report.Desired = len(candidates)
	} else {
		report.Desired = replicationFactor
	}

	for offset := 0; offset < len(candidates) && report.StoredRemotely < report.Desired; {
		remaining := report.Desired - report.StoredRemotely
		end := offset + remaining
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[offset:end]
		offset = end

		type batchResult struct {
			node proto.NodeInfo
			err  error
		}
		results := make(chan batchResult, len(batch))
		for _, node := range batch {
			node := node
			go func() {
				results <- batchResult{node: node, err: s.sendStore(ctx, node.Addr, targetID, wrapped)}
			}()
		}

		for range batch {
			result := <-results
			report.Attempted++
			if result.err != nil {
				s.RT.NoteFailure(result.node.ID)
				report.Failed = append(report.Failed, result.node)
				continue
			}
			s.RT.AddNode(result.node)
			report.StoredRemotely++
			report.Successful = append(report.Successful, result.node)
		}
	}

	return report, nil
}

func (s *Server) prepareStoreValue(key, value string, now time.Time) (string, error) {
	info, err := validateInnerLookupValue(key, value, now)
	if err != nil {
		return "", err
	}
	if !info.verified {
		return value, nil
	}

	s.mu.RLock()
	publisher := s.publisher
	s.mu.RUnlock()
	if err := publisher.Validate(); err != nil {
		return value, nil
	}
	return wrapSignedEnvelope(publisher, key, value, info, now)
}

func (s *Server) storeValidated(key, value string, now time.Time) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.Values[key]
	chosen, changed, previousValue, incomingValue, err := chooseStoredValue(key, current, value, now)
	if err != nil {
		if incomingValue.raw != "" {
			s.recordConflictLocked(key, incomingValue)
		}
		return current, false, err
	}
	if changed {
		s.Values[key] = chosen
	}
	s.recordVersionLocked(key, previousValue, incomingValue, changed)
	return chosen, changed, nil
}

func (s *Server) sendStore(ctx context.Context, addr, key, value string) error {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp6", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	req := proto.DHTRequest{Action: "store", Target: key, Data: value}
	if err := proto.WriteHeader(conn, proto.KindDHT); err != nil {
		return err
	}
	payload, _ := json.Marshal(req)
	if err := proto.WriteLengthPrefixed(conn, payload); err != nil {
		return err
	}

	kind, err := proto.ReadHeader(conn)
	if err != nil {
		return err
	}
	if kind != proto.KindDHT {
		return fmt.Errorf("invalid response")
	}
	_, err = proto.ReadLengthPrefixed(conn, 1024*1024)
	return err
}

// RecursiveFindValue searches for a value in the network.
func (s *Server) RecursiveFindValue(ctx context.Context, key string) (string, error) {
	result, err := s.RecursiveFindValueDetailed(ctx, key)
	if err != nil {
		return "", err
	}
	return result.Value, nil
}

func (s *Server) RecursiveFindValueDetailed(ctx context.Context, key string) (LookupResult, error) {
	visited := make(map[string]bool)
	candidates := s.RT.ClosestNodes(key, K)
	collector := newLookupCollector(key, time.Now())
	queried := 0

	s.mu.RLock()
	if local, ok := s.Values[key]; ok && local != "" {
		collector.Observe(sourceObservation{nodeID: "local:" + s.RT.SelfID, trust: 3, branch: 0}, local)
	}
	s.mu.RUnlock()

	branches, spares, nextBranchID := buildLookupBranches(candidates, lookupAlpha)
	for len(branches) > 0 && queried < lookupQueryBudget {
		type branchQuery struct {
			branchID int
			node     proto.NodeInfo
		}
		batch := make([]branchQuery, 0, len(branches))
		active := make([]lookupBranch, 0, len(branches))
		for _, branch := range branches {
			node, ok := nextBranchCandidate(branch.queue, visited)
			if !ok {
				if len(spares) > 0 {
					branch.id = nextBranchID
					nextBranchID++
					branch.rootNodeID = spares[0].ID
					branch.queue = []proto.NodeInfo{spares[0]}
					spares = spares[1:]
					node, ok = nextBranchCandidate(branch.queue, visited)
				}
			}
			if !ok {
				continue
			}
			visited[node.ID] = true
			batch = append(batch, branchQuery{branchID: branch.id, node: node})
			active = append(active, branch)
		}
		branches = active
		if len(batch) == 0 {
			break
		}

		type queryResult struct {
			branchID int
			node     proto.NodeInfo
			value    string
			next     []proto.NodeInfo
			err      error
		}

		resultsCh := make(chan queryResult, len(batch))
		for _, item := range batch {
			item := item
			go func() {
				val, nextNodes, err := s.QueryValue(ctx, item.node.Addr, key)
				resultsCh <- queryResult{branchID: item.branchID, node: item.node, value: val, next: nextNodes, err: err}
			}()
		}

		for i := 0; i < len(batch); i++ {
			result := <-resultsCh
			queried++
			if result.err != nil {
				s.RT.NoteFailure(result.node.ID)
				continue
			}
			s.RT.AddNode(result.node)
			if result.value != "" {
				collector.Observe(sourceObservation{nodeID: result.node.ID, addr: result.node.Addr, trust: 1, branch: result.branchID}, result.value)
			}
			for idx := range branches {
				if branches[idx].id != result.branchID {
					continue
				}
				branches[idx].queue = mergeBranchCandidates(branches[idx].queue, result.next, visited, key, s.RT)
				break
			}
		}

		if collector.IsConfirmed() {
			break
		}
	}

	return collector.Resolve(queried)
}

func (s *Server) QueryNode(ctx context.Context, addr, targetID string) ([]proto.NodeInfo, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp6", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	req := proto.DHTRequest{Action: "find_node", Target: targetID}
	if err := proto.WriteHeader(conn, proto.KindDHT); err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(req)
	if err := proto.WriteLengthPrefixed(conn, payload); err != nil {
		return nil, err
	}

	kind, err := proto.ReadHeader(conn)
	if err != nil || kind != proto.KindDHT {
		return nil, fmt.Errorf("invalid response")
	}

	resPayload, err := proto.ReadLengthPrefixed(conn, 1024*1024)
	if err != nil {
		return nil, err
	}
	var resp proto.DHTResponse
	if err := json.Unmarshal(resPayload, &resp); err != nil {
		return nil, err
	}
	return resp.Nodes, nil
}

func (s *Server) QueryValue(ctx context.Context, addr, key string) (string, []proto.NodeInfo, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp6", addr)
	if err != nil {
		return "", nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	req := proto.DHTRequest{Action: "find_value", Target: key}
	if err := proto.WriteHeader(conn, proto.KindDHT); err != nil {
		return "", nil, err
	}
	payload, _ := json.Marshal(req)
	if err := proto.WriteLengthPrefixed(conn, payload); err != nil {
		return "", nil, err
	}

	kind, err := proto.ReadHeader(conn)
	if err != nil || kind != proto.KindDHT {
		return "", nil, fmt.Errorf("invalid response")
	}

	resPayload, err := proto.ReadLengthPrefixed(conn, 1024*1024)
	if err != nil {
		return "", nil, err
	}
	var resp proto.DHTResponse
	if err := json.Unmarshal(resPayload, &resp); err != nil {
		return "", nil, err
	}
	return resp.Value, resp.Nodes, nil
}

func nextCandidateBatch(candidates []proto.NodeInfo, visited map[string]bool, max int) []proto.NodeInfo {
	out := make([]proto.NodeInfo, 0, max)
	for _, node := range candidates {
		if visited[node.ID] {
			continue
		}
		out = append(out, node)
		if len(out) == max {
			break
		}
	}
	return out
}

func buildLookupBranches(candidates []proto.NodeInfo, limit int) ([]lookupBranch, []proto.NodeInfo, int) {
	if limit <= 0 || len(candidates) == 0 {
		return nil, nil, 1
	}
	roots := selectReplicationNodes(candidates, limit)
	branches := make([]lookupBranch, 0, len(roots))
	used := make(map[string]struct{}, len(roots))
	for i, node := range roots {
		branches = append(branches, lookupBranch{id: i + 1, rootNodeID: node.ID, queue: []proto.NodeInfo{node}})
		used[node.ID] = struct{}{}
	}
	spares := make([]proto.NodeInfo, 0, len(candidates))
	for _, node := range candidates {
		if _, ok := used[node.ID]; ok {
			continue
		}
		spares = append(spares, node)
	}
	return branches, spares, len(roots) + 1
}

func nextBranchCandidate(queue []proto.NodeInfo, visited map[string]bool) (proto.NodeInfo, bool) {
	for _, node := range queue {
		if visited[node.ID] {
			continue
		}
		return node, true
	}
	return proto.NodeInfo{}, false
}

func mergeBranchCandidates(existing, incoming []proto.NodeInfo, visited map[string]bool, target string, rt *RoutingTable) []proto.NodeInfo {
	merged := mergeCandidateNodes(existing, incoming, map[string]bool{}, target, rt)
	out := make([]proto.NodeInfo, 0, len(merged))
	for _, node := range merged {
		if visited[node.ID] {
			continue
		}
		out = append(out, node)
	}
	return out
}

func mergeCandidateNodes(existing, incoming []proto.NodeInfo, visited map[string]bool, target string, rt *RoutingTable) []proto.NodeInfo {
	all := append([]proto.NodeInfo(nil), existing...)
	all = append(all, incoming...)

	seen := make(map[string]struct{}, len(all))
	dedup := make([]proto.NodeInfo, 0, len(all))
	for _, node := range all {
		if node.ID == "" || node.Addr == "" {
			continue
		}
		if visited[node.ID] {
			continue
		}
		if _, ok := seen[node.ID]; ok {
			continue
		}
		seen[node.ID] = struct{}{}
		dedup = append(dedup, node)
	}

	sort.Slice(dedup, func(i, j int) bool {
		distI := rt.distance(dedup[i].ID, target)
		distJ := rt.distance(dedup[j].ID, target)
		return distI.Cmp(distJ) < 0
	})
	return dedup
}

func selectReplicationNodes(nodes []proto.NodeInfo, limit int) []proto.NodeInfo {
	if len(nodes) <= limit {
		return append([]proto.NodeInfo(nil), nodes...)
	}

	out := make([]proto.NodeInfo, 0, limit)
	sameNetwork := make([]proto.NodeInfo, 0, len(nodes))
	networks := map[string]struct{}{}

	for _, node := range nodes {
		network := sourceObservation{addr: node.Addr}.networkKey()
		if network == "" {
			out = append(out, node)
		} else if _, ok := networks[network]; !ok {
			networks[network] = struct{}{}
			out = append(out, node)
		} else {
			sameNetwork = append(sameNetwork, node)
		}
		if len(out) == limit {
			return out
		}
	}

	for _, node := range sameNetwork {
		out = append(out, node)
		if len(out) == limit {
			return out
		}
	}
	return out
}

func storedVersionFromValidated(value validatedValue) StoredVersion {
	return StoredVersion{
		Family:          value.family,
		Fingerprint:     value.fingerprint,
		PublisherNodeID: value.publisherNodeID,
		Version:         value.version,
		IssuedAt:        value.issuedAt.UTC().Format(time.RFC3339),
		ExpiresAt:       value.expiresAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) recordVersionLocked(key string, previous, incoming validatedValue, changed bool) {
	if incoming.raw == "" {
		return
	}
	state := s.versions[key]
	if changed {
		if previous.raw != "" {
			state.Previous = appendBoundedVersion(state.Previous, storedVersionFromValidated(previous))
		}
		state.Current = storedVersionFromValidated(incoming)
		s.versions[key] = state
		return
	}
	if state.Current.Fingerprint == "" {
		state.Current = storedVersionFromValidated(incoming)
		s.versions[key] = state
	}
}

func (s *Server) recordConflictLocked(key string, incoming validatedValue) {
	if incoming.raw == "" {
		return
	}
	state := s.versions[key]
	state.Conflicts = appendBoundedVersion(state.Conflicts, storedVersionFromValidated(incoming))
	s.versions[key] = state
}

func appendBoundedVersion(values []StoredVersion, incoming StoredVersion) []StoredVersion {
	for _, existing := range values {
		if existing.Fingerprint == incoming.Fingerprint && existing.Version == incoming.Version && existing.PublisherNodeID == incoming.PublisherNodeID {
			return values
		}
	}
	values = append(values, incoming)
	const maxTrackedVersions = 6
	if len(values) > maxTrackedVersions {
		values = values[len(values)-maxTrackedVersions:]
	}
	return values
}
