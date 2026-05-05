package dht

import (
	"strings"
	"testing"
	"time"
)

func TestJitterDurationWithinBounds(t *testing.T) {
	base := 10 * time.Second
	jitter := 2 * time.Second
	for i := 0; i < 100; i++ {
		got := jitterDuration(base, jitter)
		if got < base-jitter || got > base+jitter {
			t.Fatalf("jitter out of bounds: %s", got)
		}
	}
}

func TestSetHiddenDescriptorPrivacyAppliesDefaults(t *testing.T) {
	s := NewServer("self")
	s.SetHiddenDescriptorPrivacy(HiddenDescriptorPrivacyConfig{})
	s.mu.RLock()
	cfg := s.hidden
	s.mu.RUnlock()
	if cfg.CoverLookups <= 0 {
		t.Fatalf("expected default cover lookups, got %d", cfg.CoverLookups)
	}
	if cfg.CoverInterval <= 0 {
		t.Fatalf("expected default cover interval, got %s", cfg.CoverInterval)
	}
	if cfg.PollJitter <= 0 {
		t.Fatalf("expected default poll jitter, got %s", cfg.PollJitter)
	}
}

func TestTrackHiddenLookupInviteRegistersInvite(t *testing.T) {
	s := NewServer("self")
	s.TrackHiddenLookupInvite("ghost#super-secret-hidden-token")
	s.mu.RLock()
	_, ok := s.hiddenTracked["ghost#super-secret-hidden-token"]
	s.mu.RUnlock()
	if !ok {
		t.Fatal("expected hidden invite to be tracked")
	}
}

func TestBuildHiddenLookupBatchPadsAndRepeatsRealKeys(t *testing.T) {
	now := time.Now()
	real := []string{"hidden-desc/v1/1/a", "hidden-desc/v1/1/b"}
	batch := buildHiddenLookupBatch(real, now, 3, 10)
	if len(batch) != 10 {
		t.Fatalf("unexpected batch size %d", len(batch))
	}
	counts := map[string]int{}
	cover := 0
	for _, key := range batch {
		counts[key]++
		if strings.HasPrefix(key, "hidden-desc/v1/") && key != real[0] && key != real[1] {
			cover++
		}
	}
	if counts[real[0]] != 3 || counts[real[1]] != 3 {
		t.Fatalf("unexpected real key repeat counts: %+v", counts)
	}
	if cover != 4 {
		t.Fatalf("expected 4 cover keys, got %d", cover)
	}
}
