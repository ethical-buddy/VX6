package dht

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/vx6/vx6/internal/proto"
)

func TestASNResolverLoadsAndResolves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asn-map.json")
	writeASNMap(t, path, []ASNMapEntry{
		{CIDR: "2001:db8:1::/48", ASN: "AS64500"},
		{CIDR: "2001:db8:2::/48", ASN: "AS64501"},
	})
	t.Cleanup(func() {
		SetASNResolver(noASNResolver{}, ASNResolverStatus{})
	})

	resolver, status, err := LoadASNResolver(path)
	if err != nil {
		t.Fatalf("load ASN resolver: %v", err)
	}
	if !status.Loaded || status.Entries != 2 || status.Source != path {
		t.Fatalf("unexpected resolver status: %+v", status)
	}
	SetASNResolver(resolver, status)

	if got, ok := ResolveASNForAddr("[2001:db8:1::10]:4242"); !ok || got != "AS64500" {
		t.Fatalf("unexpected ASN resolve result: %q ok=%v", got, ok)
	}
	if got, ok := ResolveASNForAddr("[2001:db8:9::10]:4242"); ok || got != "" {
		t.Fatalf("expected fallback miss, got %q ok=%v", got, ok)
	}
}

func TestCandidateConfirmationPrefersASNDiversity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asn-map.json")
	writeASNMap(t, path, []ASNMapEntry{
		{CIDR: "2001:db8:1::/48", ASN: "AS64500"},
		{CIDR: "2001:db8:2::/48", ASN: "AS64501"},
	})
	resolver, status, err := LoadASNResolver(path)
	if err != nil {
		t.Fatalf("load ASN resolver: %v", err)
	}
	SetASNResolver(resolver, status)
	t.Cleanup(func() {
		SetASNResolver(noASNResolver{}, ASNResolverStatus{})
	})

	value := validatedValue{verified: true, raw: "value", family: "service", fingerprint: "fp"}
	candidate := newCandidateObservation(value)
	candidate.addExactSource(sourceObservation{nodeID: "a", addr: "[2001:db8:1::10]:4242", trust: 2, branch: 1}, value)
	candidate.addExactSource(sourceObservation{nodeID: "b", addr: "[2001:db8:2::10]:4242", trust: 2, branch: 2}, value)

	if !candidate.confirmed() {
		t.Fatalf("expected ASN-diverse candidate to confirm: %+v", candidate.lookupResult(2, 0))
	}
	result := candidate.lookupResult(2, 0)
	if result.ASNDiversity != 2 {
		t.Fatalf("expected ASN diversity to be tracked, got %+v", result)
	}
}

func TestCandidateConfirmationFallsBackWithoutASNData(t *testing.T) {
	SetASNResolver(noASNResolver{}, ASNResolverStatus{})

	value := validatedValue{verified: true, raw: "value", family: "service", fingerprint: "fp"}
	candidate := newCandidateObservation(value)
	candidate.addExactSource(sourceObservation{nodeID: "a", addr: "[2001:db8:10::10]:4242", trust: 2, branch: 1}, value)
	candidate.addExactSource(sourceObservation{nodeID: "b", addr: "[2001:db9:11::10]:4242", trust: 2, branch: 2}, value)

	if !candidate.confirmed() {
		t.Fatalf("expected provider-based fallback to confirm without ASN data: %+v", candidate.lookupResult(2, 0))
	}
	t.Cleanup(func() {
		SetASNResolver(noASNResolver{}, ASNResolverStatus{})
	})
}

func TestSelectReplicationNodesPrefersASNSpread(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asn-map.json")
	writeASNMap(t, path, []ASNMapEntry{
		{CIDR: "2001:db8:1::/48", ASN: "AS64500"},
		{CIDR: "2001:db8:2::/48", ASN: "AS64501"},
	})
	resolver, status, err := LoadASNResolver(path)
	if err != nil {
		t.Fatalf("load ASN resolver: %v", err)
	}
	SetASNResolver(resolver, status)
	t.Cleanup(func() {
		SetASNResolver(noASNResolver{}, ASNResolverStatus{})
	})

	nodes := []proto.NodeInfo{
		{ID: "a", Addr: "[2001:db8:1::10]:4242"},
		{ID: "b", Addr: "[2001:db8:1::11]:4242"},
		{ID: "c", Addr: "[2001:db8:2::10]:4242"},
		{ID: "d", Addr: "[2001:db8:2::11]:4242"},
	}
	selected := selectReplicationNodes(nodes, 2)
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected nodes, got %d", len(selected))
	}
	asns := map[string]struct{}{}
	for _, node := range selected {
		asn, ok := ResolveASNForAddr(node.Addr)
		if !ok {
			t.Fatalf("expected ASN mapping for %s", node.Addr)
		}
		asns[asn] = struct{}{}
	}
	if len(asns) != 2 {
		t.Fatalf("expected ASN spread, got %+v", selected)
	}
}

func writeASNMap(t *testing.T, path string, entries []ASNMapEntry) {
	t.Helper()
	data, err := json.MarshalIndent(ASNMapFile{Entries: entries}, "", "  ")
	if err != nil {
		t.Fatalf("marshal ASN map: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write ASN map: %v", err)
	}
}

func TestLoadASNResolverMissingFileFallsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	resolver, status, err := LoadASNResolver(path)
	if err != nil {
		t.Fatalf("missing ASN map should not fail: %v", err)
	}
	if status.Loaded {
		t.Fatalf("expected missing ASN map to stay unloaded: %+v", status)
	}
	if got, ok := resolver.Resolve(net.ParseIP("2001:db8::1")); ok || got != "" {
		t.Fatalf("expected fallback resolver to miss, got %q ok=%v", got, ok)
	}
}

func TestConfigureASNResolverUsesEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-asn.json")
	writeASNMap(t, path, []ASNMapEntry{{CIDR: "2001:db8:3::/48", ASN: "AS64599"}})
	t.Setenv("VX6_ASN_MAP", path)
	status, err := ConfigureASNResolver("")
	if err != nil {
		t.Fatalf("configure ASN resolver: %v", err)
	}
	if !status.Loaded || status.Source != path || status.Entries != 1 {
		t.Fatalf("unexpected status after env override: %+v", status)
	}
	t.Cleanup(func() {
		SetASNResolver(noASNResolver{}, ASNResolverStatus{})
	})

	if got, ok := ResolveASNForAddr("[2001:db8:3::42]:4242"); !ok || got != "AS64599" {
		t.Fatalf("unexpected env-based ASN resolve: %q ok=%v", got, ok)
	}
}

func TestConfigureASNResolverWithMissingPathFallsBack(t *testing.T) {
	t.Setenv("VX6_ASN_MAP", "")
	status, err := ConfigureASNResolver(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("missing ASN map should not fail configure: %v", err)
	}
	if status.Loaded {
		t.Fatalf("expected fallback resolver to remain unloaded: %+v", status)
	}
}

func TestASNResolverStatusSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asn-map.json")
	writeASNMap(t, path, []ASNMapEntry{{CIDR: "2001:db8:4::/48", ASN: "AS64555"}})
	resolver, status, err := LoadASNResolver(path)
	if err != nil {
		t.Fatalf("load ASN resolver: %v", err)
	}
	SetASNResolver(resolver, status)
	t.Cleanup(func() {
		SetASNResolver(noASNResolver{}, ASNResolverStatus{})
	})

	snapshot := ASNResolverStatusSnapshot()
	if !snapshot.Loaded || snapshot.Source != path || snapshot.Entries != 1 {
		t.Fatalf("unexpected ASN status snapshot: %+v", snapshot)
	}
	if snapshot.UpdatedAt.IsZero() {
		t.Fatalf("expected updated-at to be set")
	}
}

func TestResolveASNForAddrRejectsInvalidInput(t *testing.T) {
	t.Cleanup(func() {
		SetASNResolver(noASNResolver{}, ASNResolverStatus{})
	})
	if got, ok := ResolveASNForAddr("not-an-ip"); ok || got != "" {
		t.Fatalf("expected invalid input to miss, got %q ok=%v", got, ok)
	}
}

func TestASNResolverIsStableAcrossRepeatedLookups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asn-map.json")
	writeASNMap(t, path, []ASNMapEntry{{CIDR: "2001:db8:5::/48", ASN: "AS64600"}})
	resolver, status, err := LoadASNResolver(path)
	if err != nil {
		t.Fatalf("load ASN resolver: %v", err)
	}
	SetASNResolver(resolver, status)
	t.Cleanup(func() {
		SetASNResolver(noASNResolver{}, ASNResolverStatus{})
	})

	for i := 0; i < 5; i++ {
		if got, ok := ResolveASNForAddr(fmt.Sprintf("[2001:db8:5::%x]:4242", i)); !ok || got != "AS64600" {
			t.Fatalf("unexpected repeated ASN lookup: %q ok=%v", got, ok)
		}
	}
}
