package dht

import (
	"context"
	crand "crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
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
	var lastErr error
	for _, key := range HiddenServiceLookupKeys(lookup, now) {
		val, err := s.RecursiveFindValue(ctx, key)
		if err != nil || val == "" {
			if err != nil {
				lastErr = err
			}
			continue
		}
		rec, err := DecodeHiddenServiceRecord(key, val, lookup, now)
		if err != nil {
			lastErr = err
			continue
		}
		return rec, key, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("hidden descriptor not found")
	}
	return record.ServiceRecord{}, "", lastErr
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
		ticker := time.NewTicker(cfg.PollInterval)
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
