package record

import (
	"testing"
	"time"

	"github.com/vx6/vx6/internal/identity"
)

func TestEndpointRecordRoundTrip(t *testing.T) {
	t.Parallel()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	now := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	rec, err := NewEndpointRecord(id, "alpha", "[2001:db8::1]:4242", 10*time.Minute, now)
	if err != nil {
		t.Fatalf("new endpoint record: %v", err)
	}

	if err := VerifyEndpointRecord(rec, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("verify endpoint record: %v", err)
	}
}

func TestEndpointRecordExpiry(t *testing.T) {
	t.Parallel()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	now := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	rec, err := NewEndpointRecord(id, "alpha", "[2001:db8::1]:4242", time.Minute, now)
	if err != nil {
		t.Fatalf("new endpoint record: %v", err)
	}

	if err := VerifyEndpointRecord(rec, now.Add(2*time.Minute)); err == nil {
		t.Fatal("expected expired record to fail verification")
	}
}
