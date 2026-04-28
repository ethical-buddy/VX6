package dht

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vx6/vx6/internal/record"
)

var ErrConflictingValues = errors.New("dht lookup returned conflicting values")

type LookupResult struct {
	Value           string
	Verified        bool
	SourceCount     int
	ExactMatchCount int
	QueriedNodes    int
	RejectedValues  int
	ConflictCount   int
}

type validatedValue struct {
	raw         string
	verified    bool
	family      string
	fingerprint string
	issuedAt    time.Time
}

type candidateObservation struct {
	value     validatedValue
	sources   map[string]struct{}
	exactHits map[string]struct{}
}

type lookupCollector struct {
	key        string
	now        time.Time
	verified   map[string]*candidateObservation
	raw        map[string]*candidateObservation
	rejected   int
	conflicted int
}

func newLookupCollector(key string, now time.Time) *lookupCollector {
	return &lookupCollector{
		key:      key,
		now:      now,
		verified: map[string]*candidateObservation{},
		raw:      map[string]*candidateObservation{},
	}
}

func (c *lookupCollector) Observe(sourceID, raw string) {
	if strings.TrimSpace(raw) == "" {
		return
	}
	if sourceID == "" {
		sourceID = "unknown"
	}

	value, err := validateLookupValue(c.key, raw, c.now)
	if err != nil {
		c.rejected++
		return
	}

	if value.verified {
		obs, ok := c.verified[value.family]
		if !ok {
			obs = &candidateObservation{
				value:     value,
				sources:   map[string]struct{}{},
				exactHits: map[string]struct{}{},
			}
			c.verified[value.family] = obs
		}
		obs.sources[sourceID] = struct{}{}
		if isNewerValue(value, obs.value) {
			obs.value = value
			obs.exactHits = map[string]struct{}{}
		}
		if value.fingerprint == obs.value.fingerprint {
			obs.exactHits[sourceID] = struct{}{}
		}
		return
	}

	obs, ok := c.raw[value.fingerprint]
	if !ok {
		obs = &candidateObservation{
			value:     value,
			sources:   map[string]struct{}{},
			exactHits: map[string]struct{}{},
		}
		c.raw[value.fingerprint] = obs
	}
	obs.sources[sourceID] = struct{}{}
	obs.exactHits[sourceID] = struct{}{}
}

func (c *lookupCollector) Resolve(queried int) (LookupResult, error) {
	if len(c.verified) > 0 {
		if len(c.verified) > 1 {
			families := make([]string, 0, len(c.verified))
			for family := range c.verified {
				families = append(families, family)
			}
			sort.Strings(families)
			return LookupResult{
				Verified:       true,
				QueriedNodes:   queried,
				RejectedValues: c.rejected,
				ConflictCount:  len(families),
			}, fmt.Errorf("%w: %s", ErrConflictingValues, strings.Join(families, ", "))
		}
		for _, candidate := range c.verified {
			return LookupResult{
				Value:           candidate.value.raw,
				Verified:        true,
				SourceCount:     len(candidate.sources),
				ExactMatchCount: len(candidate.exactHits),
				QueriedNodes:    queried,
				RejectedValues:  c.rejected,
			}, nil
		}
	}

	if len(c.raw) == 0 {
		return LookupResult{
			QueriedNodes:   queried,
			RejectedValues: c.rejected,
		}, fmt.Errorf("value not found in DHT")
	}

	var best *candidateObservation
	conflicts := 0
	for _, candidate := range c.raw {
		if best == nil {
			best = candidate
			conflicts = 1
			continue
		}
		switch compareObservationStrength(candidate, best) {
		case 1:
			best = candidate
			conflicts = 1
		case 0:
			conflicts++
		}
	}
	if best == nil {
		return LookupResult{
			QueriedNodes:   queried,
			RejectedValues: c.rejected,
		}, fmt.Errorf("value not found in DHT")
	}
	if conflicts > 1 {
		return LookupResult{
			QueriedNodes:   queried,
			RejectedValues: c.rejected,
			ConflictCount:  conflicts,
		}, fmt.Errorf("%w: conflicting unverified values for key %q", ErrConflictingValues, c.key)
	}

	return LookupResult{
		Value:           best.value.raw,
		SourceCount:     len(best.sources),
		ExactMatchCount: len(best.exactHits),
		QueriedNodes:    queried,
		RejectedValues:  c.rejected,
	}, nil
}

func validateLookupValue(key, raw string, now time.Time) (validatedValue, error) {
	switch {
	case strings.HasPrefix(key, "node/name/"):
		want := strings.TrimPrefix(key, "node/name/")
		var rec record.EndpointRecord
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			return validatedValue{}, fmt.Errorf("decode endpoint record: %w", err)
		}
		if err := record.VerifyEndpointRecord(rec, now); err != nil {
			return validatedValue{}, err
		}
		if rec.NodeName != want {
			return validatedValue{}, fmt.Errorf("endpoint record name %q does not match key %q", rec.NodeName, want)
		}
		issuedAt, err := time.Parse(time.RFC3339, rec.IssuedAt)
		if err != nil {
			return validatedValue{}, fmt.Errorf("parse endpoint issued_at: %w", err)
		}
		return validatedValue{
			raw:         raw,
			verified:    true,
			family:      "endpoint:" + rec.NodeID,
			fingerprint: record.Fingerprint(rec),
			issuedAt:    issuedAt,
		}, nil
	case strings.HasPrefix(key, "node/id/"):
		want := strings.TrimPrefix(key, "node/id/")
		var rec record.EndpointRecord
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			return validatedValue{}, fmt.Errorf("decode endpoint record: %w", err)
		}
		if err := record.VerifyEndpointRecord(rec, now); err != nil {
			return validatedValue{}, err
		}
		if rec.NodeID != want {
			return validatedValue{}, fmt.Errorf("endpoint record node id %q does not match key %q", rec.NodeID, want)
		}
		issuedAt, err := time.Parse(time.RFC3339, rec.IssuedAt)
		if err != nil {
			return validatedValue{}, fmt.Errorf("parse endpoint issued_at: %w", err)
		}
		return validatedValue{
			raw:         raw,
			verified:    true,
			family:      "endpoint:" + rec.NodeID,
			fingerprint: record.Fingerprint(rec),
			issuedAt:    issuedAt,
		}, nil
	case strings.HasPrefix(key, "service/"):
		want := strings.TrimPrefix(key, "service/")
		var rec record.ServiceRecord
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			return validatedValue{}, fmt.Errorf("decode service record: %w", err)
		}
		if err := record.VerifyServiceRecord(rec, now); err != nil {
			return validatedValue{}, err
		}
		if record.FullServiceName(rec.NodeName, rec.ServiceName) != want {
			return validatedValue{}, fmt.Errorf("service record name %q does not match key %q", record.FullServiceName(rec.NodeName, rec.ServiceName), want)
		}
		issuedAt, err := time.Parse(time.RFC3339, rec.IssuedAt)
		if err != nil {
			return validatedValue{}, fmt.Errorf("parse service issued_at: %w", err)
		}
		return validatedValue{
			raw:         raw,
			verified:    true,
			family:      "service:" + rec.NodeID + ":" + want,
			fingerprint: serviceFingerprint(rec),
			issuedAt:    issuedAt,
		}, nil
	case strings.HasPrefix(key, "hidden/"):
		want := strings.TrimPrefix(key, "hidden/")
		var rec record.ServiceRecord
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			return validatedValue{}, fmt.Errorf("decode hidden service record: %w", err)
		}
		if err := record.VerifyServiceRecord(rec, now); err != nil {
			return validatedValue{}, err
		}
		if !rec.IsHidden || rec.Alias != want {
			return validatedValue{}, fmt.Errorf("hidden service alias %q does not match key %q", rec.Alias, want)
		}
		issuedAt, err := time.Parse(time.RFC3339, rec.IssuedAt)
		if err != nil {
			return validatedValue{}, fmt.Errorf("parse hidden service issued_at: %w", err)
		}
		return validatedValue{
			raw:         raw,
			verified:    true,
			family:      "hidden:" + rec.NodeID + ":" + want,
			fingerprint: serviceFingerprint(rec),
			issuedAt:    issuedAt,
		}, nil
	default:
		return validatedValue{
			raw:         raw,
			fingerprint: rawFingerprint(raw),
		}, nil
	}
}

func chooseStoredValue(key, existing, incoming string, now time.Time) (string, bool, error) {
	if existing == "" {
		return incoming, true, nil
	}

	incomingValue, err := validateLookupValue(key, incoming, now)
	if err != nil {
		return "", false, err
	}
	if !incomingValue.verified {
		if existing == incoming {
			return existing, false, nil
		}
		return incoming, true, nil
	}

	existingValue, err := validateLookupValue(key, existing, now)
	if err != nil {
		return incoming, true, nil
	}
	if !existingValue.verified {
		return incoming, true, nil
	}
	if existingValue.family != incomingValue.family {
		return existing, false, fmt.Errorf("%w: existing=%s incoming=%s", ErrConflictingValues, existingValue.family, incomingValue.family)
	}
	if incomingValue.fingerprint == existingValue.fingerprint {
		return existing, false, nil
	}
	if isNewerValue(incomingValue, existingValue) {
		return incoming, true, nil
	}
	return existing, false, nil
}

func compareObservationStrength(left, right *candidateObservation) int {
	if len(left.sources) > len(right.sources) {
		return 1
	}
	if len(left.sources) < len(right.sources) {
		return -1
	}
	if left.value.fingerprint < right.value.fingerprint {
		return 1
	}
	if left.value.fingerprint > right.value.fingerprint {
		return -1
	}
	return 0
}

func isNewerValue(candidate, current validatedValue) bool {
	if candidate.issuedAt.After(current.issuedAt) {
		return true
	}
	if candidate.issuedAt.Before(current.issuedAt) {
		return false
	}
	return candidate.fingerprint > current.fingerprint
}

func rawFingerprint(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}

func serviceFingerprint(rec record.ServiceRecord) string {
	sum := sha256.Sum256([]byte(rec.Signature + "\n" + rec.IssuedAt + "\n" + rec.ExpiresAt))
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}
