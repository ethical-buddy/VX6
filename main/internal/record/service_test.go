package record

import (
	"testing"
	"time"

	"github.com/vx6/vx6/internal/identity"
)

func TestServiceRecordRoundTrip(t *testing.T) {
	t.Parallel()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	rec, err := NewServiceRecord(id, "surya", "ssh", "[2001:db8::1]:4242", 10*time.Minute, now)
	if err != nil {
		t.Fatalf("new service record: %v", err)
	}

	if err := VerifyServiceRecord(rec, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("verify service record: %v", err)
	}
}
