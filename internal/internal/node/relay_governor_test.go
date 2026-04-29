package node

import "testing"

func TestRelayGovernorHonorsOffMode(t *testing.T) {
	t.Parallel()

	g := newRelayGovernor(relayModeOff, 33)
	if _, err := g.Acquire(6); err == nil {
		t.Fatal("expected relay admission to fail when relay mode is off")
	}
}

func TestRelayGovernorReleasesCapacity(t *testing.T) {
	t.Parallel()

	g := newRelayGovernor(relayModeOn, 5)
	snap := g.Snapshot()
	releases := make([]func(), 0, snap.Budget)
	for i := 0; i < snap.Budget; i++ {
		release, err := g.Acquire(6)
		if err != nil {
			t.Fatalf("unexpected relay acquire error at %d: %v", i, err)
		}
		releases = append(releases, release)
	}
	if _, err := g.Acquire(6); err == nil {
		t.Fatal("expected relay acquire to fail at capacity")
	}
	for _, release := range releases {
		release()
	}
	if _, err := g.Acquire(6); err != nil {
		t.Fatalf("expected relay acquire after release to succeed, got %v", err)
	}
}
