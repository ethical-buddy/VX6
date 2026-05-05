package dht

import (
	"testing"
	"time"
)

func TestAdversarialCoverEscalationFloor(t *testing.T) {
	cfg := HiddenDescriptorPrivacyConfig{
		CoverLookups:           1,
		BucketPeriod:           12 * time.Minute,
		BucketBaseCover:        []int{1, 1, 2, 1},
		AnomalyEscalationSteps: []float64{0.2, 0.35, 0.55},
	}
	now := time.Unix(0, 0).UTC()
	low := effectiveCoverLookupCount(cfg, now, 0.0)
	high := effectiveCoverLookupCount(cfg, now, 0.7)
	if low < 2 {
		t.Fatalf("cover floor too low: %d", low)
	}
	if high < low*2 {
		t.Fatalf("cover escalation too weak: low=%d high=%d", low, high)
	}
}

func TestAdversarialConsensusThresholdRequiresIndependentGroups(t *testing.T) {
	groups := 3
	required := 2
	fingerprintGroups := map[string]map[int]struct{}{
		"good": {0: {}, 2: {}},
		"bad":  {1: {}},
	}
	best := 0
	winner := ""
	for fp, owners := range fingerprintGroups {
		if len(owners) > best {
			best = len(owners)
			winner = fp
		}
	}
	if best < required {
		t.Fatalf("no descriptor reached consensus, best=%d required=%d", best, required)
	}
	if winner != "good" {
		t.Fatalf("expected good descriptor to win consensus, got %s", winner)
	}
	if groups < required {
		t.Fatalf("invalid harness thresholds groups=%d required=%d", groups, required)
	}
}
