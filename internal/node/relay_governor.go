package node

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/vx6/vx6/internal/proto"
)

const (
	defaultRelayMode            = "on"
	defaultRelayResourcePercent = 33
	minRelayResourcePercent     = 5
	maxRelayResourcePercent     = 90
	relayStreamsPerCPUUnit      = 8
)

type relayGovernor struct {
	mu         sync.Mutex
	mode       string
	percent    int
	budget     int
	active     int
	lastReason string
}

func newRelayGovernor(mode string, percent int) *relayGovernor {
	g := &relayGovernor{}
	g.Update(mode, percent)
	return g
}

func (g *relayGovernor) Update(mode string, percent int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.mode = normalizeRelayMode(mode)
	g.percent = normalizeRelayResourcePercent(percent)
	g.budget = relayBudget(g.percent)
	if g.active > g.budget {
		g.lastReason = fmt.Sprintf("relay load above refreshed budget (%d/%d active)", g.active, g.budget)
	}
}

func (g *relayGovernor) Acquire(kind byte) (func(), error) {
	if !isRelayKind(kind) {
		return func() {}, nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.mode != relayModeOn {
		g.lastReason = "relay participation disabled"
		return nil, fmt.Errorf("relay participation is disabled")
	}
	if g.active >= g.budget {
		g.lastReason = fmt.Sprintf("relay budget reached (%d/%d active; %d%% reserved for local work)", g.active, g.budget, 100-g.percent)
		return nil, fmt.Errorf("relay budget reached (%d/%d active; %d%% reserved for local work)", g.active, g.budget, 100-g.percent)
	}

	g.active++
	return func() {
		g.mu.Lock()
		if g.active > 0 {
			g.active--
		}
		g.mu.Unlock()
	}, nil
}

type relayGovernorSnapshot struct {
	Mode       string
	Percent    int
	Budget     int
	Active     int
	LastReason string
}

func (g *relayGovernor) Snapshot() relayGovernorSnapshot {
	g.mu.Lock()
	defer g.mu.Unlock()
	return relayGovernorSnapshot{
		Mode:       g.mode,
		Percent:    g.percent,
		Budget:     g.budget,
		Active:     g.active,
		LastReason: g.lastReason,
	}
}

func relayBudget(percent int) int {
	cpuUnits := runtime.GOMAXPROCS(0)
	if cpuUnits <= 0 {
		cpuUnits = 1
	}
	usableUnits := (cpuUnits * percent) / 100
	if usableUnits <= 0 {
		usableUnits = 1
	}
	budget := usableUnits * relayStreamsPerCPUUnit
	if budget < 2 {
		budget = 2
	}
	return budget
}

func isRelayKind(kind byte) bool {
	return kind == proto.KindExtend || kind == proto.KindRendezvous
}

const (
	relayModeOn  = "on"
	relayModeOff = "off"
)

func normalizeRelayMode(mode string) string {
	switch mode {
	case relayModeOff:
		return relayModeOff
	default:
		return relayModeOn
	}
}

func normalizeRelayResourcePercent(percent int) int {
	switch {
	case percent <= 0:
		return defaultRelayResourcePercent
	case percent < minRelayResourcePercent:
		return minRelayResourcePercent
	case percent > maxRelayResourcePercent:
		return maxRelayResourcePercent
	default:
		return percent
	}
}
