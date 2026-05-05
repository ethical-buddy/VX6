package dht

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/vx6/vx6/internal/record"
)

type cachedHiddenService struct {
	Invite       string
	LookupKey    string
	Record       record.ServiceRecord
	CachedAt     time.Time
	RefreshAfter time.Time
	ExpiresAt    time.Time
}

type rateWindow struct {
	WindowStart time.Time
	Count       int
}

func (s *Server) ResolveHiddenService(ctx context.Context, lookup string, now time.Time) (record.ServiceRecord, error) {
	if now.IsZero() {
		now = time.Now()
	}

	if cached, ok := s.lookupHiddenServiceCache(lookup, now); ok {
		s.startHiddenDescriptorWarmer(lookup)
		return cached.Record, nil
	}

	rec, key, err := s.refreshHiddenServiceNetwork(ctx, lookup, now)
	if err != nil {
		if stale, ok := s.lookupStaleHiddenServiceCache(lookup, now); ok {
			s.startHiddenDescriptorWarmer(lookup)
			return stale.Record, nil
		}
		return record.ServiceRecord{}, err
	}

	s.storeHiddenServiceCache(lookup, key, rec, now)
	s.startHiddenDescriptorWarmer(lookup)
	return rec, nil
}

func (s *Server) refreshHiddenServiceNetwork(ctx context.Context, lookup string, now time.Time) (record.ServiceRecord, string, error) {
	keys := HiddenServiceLookupKeys(lookup, now)
	if len(keys) > 1 && rand.Intn(2) == 1 {
		keys[0], keys[1] = keys[1], keys[0]
	}
	realKeySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		realKeySet[key] = struct{}{}
	}
	s.mu.RLock()
	cfg := s.hidden
	s.mu.RUnlock()
	groups := cfg.ConsensusGroups
	if groups <= 0 {
		groups = 3
	}
	minMatches := cfg.ConsensusMinMatches
	if minMatches <= 0 {
		minMatches = 2
	}
	if minMatches > groups {
		minMatches = groups
	}

	type consensusRecord struct {
		record record.ServiceRecord
		key    string
		groups map[int]struct{}
	}
	seen := map[string]*consensusRecord{}
	var lastErr error
	for group := 0; group < groups; group++ {
		candidates, err := s.fetchHiddenDescriptorGroup(ctx, lookup, now, keys, realKeySet, cfg)
		if err != nil {
			lastErr = err
		}
		for fp, candidate := range candidates {
			entry, ok := seen[fp]
			if !ok {
				entry = &consensusRecord{
					record: candidate.record,
					key:    candidate.key,
					groups: map[int]struct{}{},
				}
				seen[fp] = entry
			}
			entry.groups[group] = struct{}{}
			if len(entry.groups) >= minMatches {
				return entry.record, entry.key, nil
			}
		}
	}
	if len(seen) > 0 {
		best := 0
		for _, entry := range seen {
			if len(entry.groups) > best {
				best = len(entry.groups)
			}
		}
		return record.ServiceRecord{}, "", fmt.Errorf("hidden descriptor consensus not reached: best_match_groups=%d required=%d", best, minMatches)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("hidden descriptor not found")
	}
	return record.ServiceRecord{}, "", lastErr
}

type hiddenDescriptorCandidate struct {
	record record.ServiceRecord
	key    string
}

func (s *Server) fetchHiddenDescriptorGroup(ctx context.Context, lookup string, now time.Time, realKeys []string, realKeySet map[string]struct{}, cfg HiddenDescriptorPrivacyConfig) (map[string]hiddenDescriptorCandidate, error) {
	batch := buildHiddenLookupBatch(realKeys, now, cfg.FetchParallel, cfg.FetchBatchSize)
	rand.Shuffle(len(batch), func(i, j int) { batch[i], batch[j] = batch[j], batch[i] })

	type fetchResult struct {
		key string
		val string
		err error
	}
	results := make(chan fetchResult, len(batch))
	var wg sync.WaitGroup
	for _, key := range batch {
		wg.Add(1)
		go func(lookupKey string) {
			defer wg.Done()
			queryCtx, cancel := context.WithTimeout(ctx, 2200*time.Millisecond)
			defer cancel()
			val, err := s.RecursiveFindValue(queryCtx, lookupKey)
			results <- fetchResult{key: lookupKey, val: val, err: err}
		}(key)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	out := map[string]hiddenDescriptorCandidate{}
	var lastErr error
	for result := range results {
		if _, ok := realKeySet[result.key]; !ok {
			continue
		}
		if result.err != nil || result.val == "" {
			if result.err != nil {
				lastErr = result.err
			}
			continue
		}
		rec, err := DecodeHiddenServiceRecord(result.key, result.val, lookup, now)
		if err != nil {
			lastErr = err
			continue
		}
		fp := hiddenDescriptorConsensusFingerprint(rec)
		if _, ok := out[fp]; !ok {
			out[fp] = hiddenDescriptorCandidate{record: rec, key: result.key}
		}
	}
	return out, lastErr
}

func buildHiddenLookupBatch(realKeys []string, now time.Time, parallel, batchSize int) []string {
	if parallel <= 0 {
		parallel = 2
	}
	if batchSize <= 0 {
		batchSize = 6
	}
	minBatch := len(realKeys) * parallel
	if batchSize < minBatch {
		batchSize = minBatch
	}
	batch := make([]string, 0, batchSize)
	for _, key := range realKeys {
		for i := 0; i < parallel; i++ {
			batch = append(batch, key)
		}
	}
	for len(batch) < batchSize {
		coverKey, err := randomHiddenDescriptorCoverKey(now)
		if err != nil {
			break
		}
		batch = append(batch, coverKey)
	}
	return batch
}

func (s *Server) lookupHiddenServiceCache(lookup string, now time.Time) (cachedHiddenService, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.hiddenCache[lookup]
	if !ok {
		return cachedHiddenService{}, false
	}
	if !entry.ExpiresAt.IsZero() && !entry.ExpiresAt.After(now) {
		return cachedHiddenService{}, false
	}
	if entry.RefreshAfter.After(now) {
		return entry, true
	}
	return cachedHiddenService{}, false
}

func (s *Server) lookupStaleHiddenServiceCache(lookup string, now time.Time) (cachedHiddenService, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.hiddenCache[lookup]
	if !ok {
		return cachedHiddenService{}, false
	}
	if !entry.ExpiresAt.IsZero() && !entry.ExpiresAt.After(now) {
		return cachedHiddenService{}, false
	}
	return entry, true
}

func (s *Server) storeHiddenServiceCache(lookup, key string, rec record.ServiceRecord, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	refreshWindow := hiddenDescriptorCacheWindow
	if s.hidden.CacheWindow > 0 {
		refreshWindow = s.hidden.CacheWindow
	}
	refreshAfter := now.Add(refreshWindow / 2)
	expiresAt := now.Add(refreshWindow)
	if recExpiry, err := time.Parse(time.RFC3339, rec.ExpiresAt); err == nil && recExpiry.Before(expiresAt) {
		expiresAt = recExpiry
	}
	s.hiddenCache[lookup] = cachedHiddenService{
		Invite:       lookup,
		LookupKey:    key,
		Record:       rec,
		CachedAt:     now,
		RefreshAfter: refreshAfter,
		ExpiresAt:    expiresAt,
	}
}

func (s *Server) startHiddenDescriptorWarmer(lookup string) {
	s.mu.Lock()
	if _, ok := s.hiddenWarmers[lookup]; ok {
		s.mu.Unlock()
		return
	}
	cfg := s.hidden
	if cfg.PollInterval <= 0 || cfg.CacheWindow <= 0 {
		s.mu.Unlock()
		return
	}
	s.hiddenWarmers[lookup] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.hiddenWarmers, lookup)
			s.mu.Unlock()
		}()

		timer := time.NewTimer(cfg.CacheWindow)
		defer timer.Stop()
		ticker := time.NewTicker(jitterDuration(cfg.PollInterval, cfg.PollJitter))
		defer ticker.Stop()

		for {
			select {
			case <-timer.C:
				return
			case <-ticker.C:
				refreshCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				rec, key, err := s.refreshHiddenServiceNetwork(refreshCtx, lookup, time.Now())
				cancel()
				if err == nil {
					s.storeHiddenServiceCache(lookup, key, rec, time.Now())
				}
				s.performHiddenDescriptorCoverLookups(cfg, time.Now())
				ticker.Reset(jitterDuration(cfg.PollInterval, cfg.PollJitter))
			}
		}
	}()
}

func (s *Server) TrackHiddenLookupInvite(invite string) {
	if invite == "" {
		return
	}
	s.mu.Lock()
	s.hiddenTracked[invite] = struct{}{}
	s.mu.Unlock()
	s.startHiddenDescriptorWarmer(invite)
}

func (s *Server) ensureHiddenCoverWorker() {
	s.mu.Lock()
	if s.hiddenCoverOn {
		s.mu.Unlock()
		return
	}
	s.hiddenCoverOn = true
	s.mu.Unlock()

	go func() {
		for {
			s.mu.RLock()
			cfg := s.hidden
			s.mu.RUnlock()
			interval := cfg.CoverInterval
			if interval <= 0 {
				interval = 18 * time.Second
			}
			time.Sleep(jitterDuration(interval, cfg.PollJitter))

			now := time.Now()
			s.performHiddenDescriptorCoverLookups(cfg, now)
			s.mu.RLock()
			invites := make([]string, 0, len(s.hiddenTracked))
			for invite := range s.hiddenTracked {
				invites = append(invites, invite)
			}
			s.mu.RUnlock()
			for _, invite := range invites {
				refreshCtx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
				rec, key, err := s.refreshHiddenServiceNetwork(refreshCtx, invite, now)
				cancel()
				if err == nil {
					s.storeHiddenServiceCache(invite, key, rec, now)
				}
			}
		}
	}()
}

func (s *Server) performHiddenDescriptorCoverLookups(cfg HiddenDescriptorPrivacyConfig, now time.Time) {
	if cfg.CoverLookups <= 0 {
		return
	}
	for i := 0; i < cfg.CoverLookups; i++ {
		key, err := randomHiddenDescriptorCoverKey(now)
		if err != nil {
			return
		}
		coverCtx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		_, _ = s.RecursiveFindValue(coverCtx, key)
		cancel()
	}
}

func randomHiddenDescriptorCoverKey(now time.Time) (string, error) {
	epoch := hiddenDescriptorEpoch(now)
	raw := make([]byte, 20)
	if _, err := crand.Read(raw); err != nil {
		return "", err
	}
	return "hidden-desc/v1/" + fmt.Sprintf("%d", epoch) + "/" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func jitterDuration(base, jitter time.Duration) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if jitter <= 0 {
		return base
	}
	n := rand.Int63n(int64(jitter)*2+1) - int64(jitter)
	out := base + time.Duration(n)
	if out < time.Second {
		return time.Second
	}
	return out
}

func hiddenDescriptorConsensusFingerprint(rec record.ServiceRecord) string {
	sum := sha256.Sum256([]byte(
		rec.NodeID + "\n" +
			rec.NodeName + "\n" +
			rec.ServiceName + "\n" +
			rec.Alias + "\n" +
			rec.PublicKey + "\n" +
			rec.IssuedAt + "\n" +
			rec.ExpiresAt + "\n" +
			rec.Signature + "\n",
	))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

func (s *Server) allowHiddenDescriptorRequest(remoteAddr, action string, now time.Time) bool {
	var (
		window time.Duration
		limit  int
	)
	switch action {
	case "find_value":
		window = hiddenDescriptorLookupRateWindow
		limit = hiddenDescriptorLookupRateLimit
	case "store":
		window = hiddenDescriptorStoreRateWindow
		limit = hiddenDescriptorStoreRateLimit
	default:
		return true
	}

	host := remoteAddr
	if parsedHost, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = parsedHost
	}
	key := action + "\n" + host

	s.mu.Lock()
	defer s.mu.Unlock()
	for existing, counter := range s.hiddenRates {
		if now.Sub(counter.WindowStart) > window*2 {
			delete(s.hiddenRates, existing)
		}
	}
	counter := s.hiddenRates[key]
	if counter.WindowStart.IsZero() || now.Sub(counter.WindowStart) >= window {
		s.hiddenRates[key] = rateWindow{WindowStart: now, Count: 1}
		return true
	}
	if counter.Count >= limit {
		return false
	}
	counter.Count++
	s.hiddenRates[key] = counter
	return true
}
