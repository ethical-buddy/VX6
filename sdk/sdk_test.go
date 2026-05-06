package sdk

import (
	"testing"

	"github.com/vx6/vx6/internal/config"
)

func TestNormalizeHiddenServiceEntryDefaults(t *testing.T) {
	entry := config.ServiceEntry{
		Target:        "127.0.0.1:8080",
		IsHidden:      true,
		Alias:         "admin",
		HiddenProfile: "",
	}
	got, err := NormalizeHiddenServiceEntry(entry)
	if err != nil {
		t.Fatalf("normalize hidden entry: %v", err)
	}
	if got.HiddenLookupSecret == "" {
		t.Fatal("expected hidden lookup secret to be generated")
	}
	if got.HiddenProfile == "" {
		t.Fatal("expected hidden profile default")
	}
	if got.IntroMode == "" {
		t.Fatal("expected intro mode default")
	}
}

func TestNormalizeHiddenServiceEntryRejectsPrivateHidden(t *testing.T) {
	entry := config.ServiceEntry{IsHidden: true, IsPrivate: true, Alias: "x"}
	if _, err := NormalizeHiddenServiceEntry(entry); err == nil {
		t.Fatal("expected error for hidden+private service")
	}
}

func TestRequestedServiceName(t *testing.T) {
	if got := requestedServiceName("alice.api"); got != "api" {
		t.Fatalf("unexpected extracted service name: %q", got)
	}
	if got := requestedServiceName("hidden-alias"); got != "hidden-alias" {
		t.Fatalf("unexpected service name passthrough: %q", got)
	}
}
