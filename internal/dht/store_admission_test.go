package dht

import (
	"testing"
	"time"

	"github.com/vx6/vx6/internal/identity"
)

func TestStoreAdmissionRejectsStaleTrustedUpdate(t *testing.T) {
	now := time.Now()
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	server := NewServerWithIdentity(id)

	current := mustServiceRecordForIdentity(t, id, "owner", "api", "[2001:db8::61]:4242", false, "", now.Add(time.Minute))
	stale := mustServiceRecordForIdentity(t, id, "owner", "api", "[2001:db8::60]:4242", false, "", now)

	server.mu.Lock()
	server.Values[ServiceKey("owner.api")] = mustSignedValue(t, id, ServiceKey("owner.api"), mustJSON(t, current), now.Add(time.Minute))
	server.mu.Unlock()

	if err := server.admitStoreValue("[2001:db8::10]:4242", ServiceKey("owner.api"), mustSignedValue(t, id, ServiceKey("owner.api"), mustJSON(t, stale), now), now); err == nil {
		t.Fatal("expected stale trusted update to be rejected")
	}
}

func TestStoreAdmissionRateLimitsRepeatedSource(t *testing.T) {
	server := NewServer("self")
	now := time.Now()
	key := "service/owner.api"

	allowed := hiddenDescriptorStoreRateLimit
	for i := 0; i < allowed; i++ {
		if err := server.allowStoreRequest("[2001:db8::10]:4242", key, true, now); err != nil {
			t.Fatalf("expected request %d to pass, got %v", i+1, err)
		}
	}
	if err := server.allowStoreRequest("[2001:db8::10]:4242", key, true, now); err == nil {
		t.Fatal("expected repeated trusted store to be rate limited")
	}
}

func TestStoreAdmissionTracksFamiliesIndependently(t *testing.T) {
	server := NewServer("self")
	now := time.Now()

	if err := server.allowStoreRequest("[2001:db8::10]:4242", ServiceKey("owner.api"), true, now); err != nil {
		t.Fatalf("trusted service store should pass: %v", err)
	}
	if err := server.allowStoreRequest("[2001:db8::10]:4242", PrivateCatalogKey("owner"), true, now); err != nil {
		t.Fatalf("trusted private catalog store should pass: %v", err)
	}
	if err := server.allowStoreRequest("[2001:db8::10]:4242", HiddenServiceKeyAt("ghost", now), true, now); err != nil {
		t.Fatalf("trusted hidden descriptor store should pass: %v", err)
	}
}
