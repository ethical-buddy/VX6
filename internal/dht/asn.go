package dht

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ASNResolver interface {
	Resolve(ip net.IP) (string, bool)
}

type ASNResolverStatus struct {
	Loaded    bool      `json:"loaded"`
	Source    string    `json:"source,omitempty"`
	Entries   int       `json:"entries,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type ASNMapEntry struct {
	CIDR string `json:"cidr"`
	ASN  string `json:"asn"`
}

type ASNMapFile struct {
	Entries []ASNMapEntry `json:"entries"`
}

type asnCacheEntry struct {
	asn string
	ok  bool
}

type prefixASNResolver struct {
	mu       sync.RWMutex
	entries  []asnRangeEntry
	cache    map[string]asnCacheEntry
	loadedAt time.Time
}

type asnRangeEntry struct {
	network *net.IPNet
	asn     string
}

type noASNResolver struct{}

var asnResolverState struct {
	mu       sync.RWMutex
	resolver ASNResolver
	status   ASNResolverStatus
}

func init() {
	SetASNResolver(noASNResolver{}, ASNResolverStatus{})
}

func (noASNResolver) Resolve(net.IP) (string, bool) {
	return "", false
}

func (r *prefixASNResolver) Resolve(ip net.IP) (string, bool) {
	if ip == nil {
		return "", false
	}
	host := ip.String()

	r.mu.RLock()
	if cached, ok := r.cache[host]; ok {
		r.mu.RUnlock()
		return cached.asn, cached.ok
	}
	entries := append([]asnRangeEntry(nil), r.entries...)
	r.mu.RUnlock()

	for _, entry := range entries {
		if entry.network.Contains(ip) {
			r.mu.Lock()
			if r.cache == nil {
				r.cache = map[string]asnCacheEntry{}
			}
			r.cache[host] = asnCacheEntry{asn: entry.asn, ok: true}
			r.mu.Unlock()
			return entry.asn, true
		}
	}

	r.mu.Lock()
	if r.cache == nil {
		r.cache = map[string]asnCacheEntry{}
	}
	r.cache[host] = asnCacheEntry{ok: false}
	r.mu.Unlock()
	return "", false
}

func SetASNResolver(resolver ASNResolver, status ASNResolverStatus) {
	if resolver == nil {
		resolver = noASNResolver{}
	}
	asnResolverState.mu.Lock()
	defer asnResolverState.mu.Unlock()
	asnResolverState.resolver = resolver
	asnResolverState.status = status
	if status.Loaded && status.UpdatedAt.IsZero() {
		asnResolverState.status.UpdatedAt = time.Now()
	}
}

func ASNResolverStatusSnapshot() ASNResolverStatus {
	asnResolverState.mu.RLock()
	defer asnResolverState.mu.RUnlock()
	return asnResolverState.status
}

func resolveASN(ip net.IP) (string, bool) {
	asnResolverState.mu.RLock()
	resolver := asnResolverState.resolver
	asnResolverState.mu.RUnlock()
	if resolver == nil {
		return "", false
	}
	return resolver.Resolve(ip)
}

func ResolveASNForAddr(addr string) (string, bool) {
	host := addr
	if parsedHost, _, err := net.SplitHostPort(addr); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip == nil {
		return "", false
	}
	return resolveASN(ip)
}

func ConfigureASNResolver(configPath string) (ASNResolverStatus, error) {
	path := resolveASNMapPath(configPath)
	if path == "" {
		status := ASNResolverStatus{}
		SetASNResolver(noASNResolver{}, status)
		return status, nil
	}

	resolver, status, err := LoadASNResolver(path)
	if err != nil {
		SetASNResolver(noASNResolver{}, ASNResolverStatus{Source: path})
		return ASNResolverStatus{}, err
	}
	SetASNResolver(resolver, status)
	return status, nil
}

func resolveASNMapPath(configPath string) string {
	if path := strings.TrimSpace(os.Getenv("VX6_ASN_MAP")); path != "" {
		return path
	}
	if configPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(configPath), "asn-map.json")
}

func LoadASNResolver(path string) (ASNResolver, ASNResolverStatus, error) {
	status := ASNResolverStatus{Source: path}
	if strings.TrimSpace(path) == "" {
		return noASNResolver{}, status, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return noASNResolver{}, status, nil
		}
		return nil, ASNResolverStatus{}, fmt.Errorf("read ASN map: %w", err)
	}

	var file ASNMapFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, ASNResolverStatus{}, fmt.Errorf("decode ASN map: %w", err)
	}
	if len(file.Entries) == 0 {
		return noASNResolver{}, status, nil
	}

	resolver := &prefixASNResolver{
		entries: make([]asnRangeEntry, 0, len(file.Entries)),
		cache:   map[string]asnCacheEntry{},
	}
	for _, entry := range file.Entries {
		cidr := strings.TrimSpace(entry.CIDR)
		asn := strings.TrimSpace(entry.ASN)
		if cidr == "" || asn == "" {
			continue
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, ASNResolverStatus{}, fmt.Errorf("parse ASN map CIDR %q: %w", cidr, err)
		}
		resolver.entries = append(resolver.entries, asnRangeEntry{network: network, asn: asn})
	}
	if len(resolver.entries) == 0 {
		return noASNResolver{}, status, nil
	}
	resolver.loadedAt = time.Now()
	status.Loaded = true
	status.Entries = len(resolver.entries)
	status.UpdatedAt = resolver.loadedAt
	return resolver, status, nil
}
