package dht

import (
	"sort"
	"time"
)

type ReplicaKind string

const (
	ReplicaKindNodeName         ReplicaKind = "node_name"
	ReplicaKindNodeID           ReplicaKind = "node_id"
	ReplicaKindPublicService    ReplicaKind = "public_service"
	ReplicaKindPrivateCatalog   ReplicaKind = "private_catalog"
	ReplicaKindHiddenDescriptor ReplicaKind = "hidden_descriptor"
)

type ReplicaHealth string

const (
	ReplicaHealthHealthy  ReplicaHealth = "healthy"
	ReplicaHealthDegraded ReplicaHealth = "degraded"
	ReplicaHealthStale    ReplicaHealth = "stale"
)

type ReplicaObservation struct {
	Key            string
	Kind           ReplicaKind
	Subject        string
	Epoch          int64
	Desired        int
	Attempted      int
	StoredRemotely int
	LocalStored    bool
	PublishedAt    time.Time
	RefreshBy      time.Time
	ExpiresAt      time.Time
	LastError      string
}

type ReplicaSummary struct {
	Tracked                 int
	Healthy                 int
	Degraded                int
	Stale                   int
	HiddenDescriptors       int
	HiddenHealthy           int
	HiddenDegraded          int
	HiddenStale             int
	NextRefreshBy           time.Time
	LastPublishedAt         time.Time
	RefreshInterval         time.Duration
	HiddenRotation          time.Duration
	HiddenPublishOverlapKey int
}

func HiddenDescriptorRotationInterval() time.Duration {
	return hiddenDescriptorRotation
}

func HiddenDescriptorPublishOverlap() int {
	return 2
}

func HiddenDescriptorEpochFromKey(key string) (int64, bool) {
	epoch, err := parseHiddenDescriptorEpoch(key)
	if err != nil {
		return 0, false
	}
	return epoch, true
}

func (o ReplicaObservation) HealthAt(now time.Time) ReplicaHealth {
	if now.IsZero() {
		now = time.Now()
	}
	if !o.ExpiresAt.IsZero() && now.After(o.ExpiresAt) {
		return ReplicaHealthStale
	}
	if !o.RefreshBy.IsZero() && now.After(o.RefreshBy) {
		return ReplicaHealthStale
	}
	if o.LastError != "" {
		return ReplicaHealthDegraded
	}
	if !o.LocalStored {
		return ReplicaHealthDegraded
	}
	if o.Desired == 0 {
		return ReplicaHealthDegraded
	}
	if o.StoredRemotely < o.Desired {
		return ReplicaHealthDegraded
	}
	return ReplicaHealthHealthy
}

func (s *Server) RecordReplicaObservation(observation ReplicaObservation) {
	if observation.Key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replicas[observation.Key] = observation
}

func (s *Server) ReplicaObservationsSnapshot() []ReplicaObservation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]ReplicaObservation, 0, len(s.replicas))
	for _, observation := range s.replicas {
		out = append(out, observation)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func (s *Server) ReplicaSummary(now time.Time, refreshInterval time.Duration) ReplicaSummary {
	if now.IsZero() {
		now = time.Now()
	}
	if refreshInterval <= 0 {
		refreshInterval = time.Second
	}

	observations := s.ReplicaObservationsSnapshot()
	summary := ReplicaSummary{
		Tracked:                 len(observations),
		RefreshInterval:         refreshInterval,
		HiddenRotation:          hiddenDescriptorRotation,
		HiddenPublishOverlapKey: HiddenDescriptorPublishOverlap(),
	}

	for _, observation := range observations {
		health := observation.HealthAt(now)
		switch health {
		case ReplicaHealthHealthy:
			summary.Healthy++
		case ReplicaHealthDegraded:
			summary.Degraded++
		case ReplicaHealthStale:
			summary.Stale++
		}

		if observation.Kind == ReplicaKindHiddenDescriptor {
			summary.HiddenDescriptors++
			switch health {
			case ReplicaHealthHealthy:
				summary.HiddenHealthy++
			case ReplicaHealthDegraded:
				summary.HiddenDegraded++
			case ReplicaHealthStale:
				summary.HiddenStale++
			}
		}

		if !observation.PublishedAt.IsZero() && observation.PublishedAt.After(summary.LastPublishedAt) {
			summary.LastPublishedAt = observation.PublishedAt
		}
		if !observation.RefreshBy.IsZero() && (summary.NextRefreshBy.IsZero() || observation.RefreshBy.Before(summary.NextRefreshBy)) {
			summary.NextRefreshBy = observation.RefreshBy
		}
	}

	return summary
}
