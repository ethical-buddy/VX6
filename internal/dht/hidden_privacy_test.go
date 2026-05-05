package dht

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/vx6/vx6/internal/record"
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

func TestScheduleBucketCoverDeterministic(t *testing.T) {
	cfg := HiddenDescriptorPrivacyConfig{
		BucketPeriod:    6 * time.Minute,
		BucketBaseCover: []int{1, 3, 2},
	}
	ts := time.Unix(0, 0).UTC()
	if got := scheduleBucketCover(cfg, ts); got != 1 {
		t.Fatalf("unexpected bucket 0 cover %d", got)
	}
	if got := scheduleBucketCover(cfg, ts.Add(2*time.Minute)); got != 3 {
		t.Fatalf("unexpected bucket 1 cover %d", got)
	}
	if got := scheduleBucketCover(cfg, ts.Add(4*time.Minute)); got != 2 {
		t.Fatalf("unexpected bucket 2 cover %d", got)
	}
}

func TestEffectiveCoverLookupCountEscalatesOnAnomaly(t *testing.T) {
	cfg := HiddenDescriptorPrivacyConfig{
		CoverLookups:           1,
		BucketPeriod:           10 * time.Minute,
		BucketBaseCover:        []int{1},
		AnomalyEscalationSteps: []float64{0.2, 0.4, 0.6},
	}
	base := effectiveCoverLookupCount(cfg, time.Unix(0, 0).UTC(), 0.0)
	if base != 2 {
		t.Fatalf("unexpected base cover count %d", base)
	}
	escalated := effectiveCoverLookupCount(cfg, time.Unix(0, 0).UTC(), 0.61)
	if escalated != 8 {
		t.Fatalf("unexpected escalated cover count %d", escalated)
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

func TestCircuitRelayDiversityCountsGroups(t *testing.T) {
	SetASNResolver(ASNResolverFunc(func(ip net.IP) (string, bool) {
		if ip == nil {
			return "", false
		}
		if ip.String() == "2001:db8:1::10" {
			return "AS100", true
		}
		return "AS200", true
	}), ASNResolverStatus{Loaded: true, Source: "test"})
	t.Cleanup(func() { SetASNResolver(noASNResolver{}, ASNResolverStatus{}) })

	nodes := []record.EndpointRecord{
		{Address: "[2001:db8:1::10]:4242"},
		{Address: "[2001:db8:2::10]:4242"},
		{Address: "[2001:db8:2::20]:4242"},
	}
	nets, providers, asns := circuitRelayDiversity(nodes)
	if nets < 2 {
		t.Fatalf("expected at least 2 network groups, got %d", nets)
	}
	if providers < 1 {
		t.Fatalf("expected at least 1 provider group, got %d", providers)
	}
	if asns < 2 {
		t.Fatalf("expected at least 2 ASN groups in test map fallback, got %d", asns)
	}
}

func TestHiddenDescriptorConsensusFingerprintStable(t *testing.T) {
	rec := record.ServiceRecord{
		NodeID:      "vx6_abc",
		NodeName:    "alice",
		ServiceName: "web",
		Alias:       "ghost",
		PublicKey:   "pub",
		IssuedAt:    "2026-05-05T10:00:00Z",
		ExpiresAt:   "2026-05-05T10:30:00Z",
		Signature:   "sig",
	}
	a := hiddenDescriptorConsensusFingerprint(rec)
	b := hiddenDescriptorConsensusFingerprint(rec)
	if a != b {
		t.Fatalf("fingerprint must be stable: %q != %q", a, b)
	}
	rec2 := rec
	rec2.Signature = "sig2"
	if hiddenDescriptorConsensusFingerprint(rec2) == a {
		t.Fatal("fingerprint should change when signed descriptor changes")
	}
}

type ASNResolverFunc func(ip net.IP) (string, bool)

func (f ASNResolverFunc) Resolve(ip net.IP) (string, bool) {
	return f(ip)
}
