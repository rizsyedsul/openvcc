package pool

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/syedsumx/openvcc/internal/cost"
)

type Strategy interface {
	Name() string
	Pick(req *http.Request, healthy []*Backend) (*Backend, bool)
}

const (
	StrategyRoundRobin              = "round_robin"
	StrategyWeightedRoundRobin      = "weighted_round_robin"
	StrategyLeastConnections        = "least_connections"
	StrategyRandom                  = "random"
	StrategyCostBoundedLeastLatency = "cost_bounded_least_latency"
	StrategyStickyByCloud           = "sticky_by_cloud"
)

func ParseStrategy(name string) (Strategy, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", StrategyWeightedRoundRobin:
		return &WeightedRoundRobin{}, nil
	case StrategyRoundRobin:
		return &RoundRobin{}, nil
	case StrategyLeastConnections:
		return &LeastConnections{}, nil
	case StrategyRandom:
		return &Random{}, nil
	case StrategyCostBoundedLeastLatency:
		// Default-constructed instance with a nil accountant. The engine is
		// expected to replace this with NewCostBoundedLeastLatency(accountant)
		// when a config.Cost block is present.
		return NewCostBoundedLeastLatency(nil), nil
	case StrategyStickyByCloud:
		// Default sticky has no hash and no fallback; engine replaces it via
		// NewStickyByCloud once the Sticky config block is read.
		return NewStickyByCloud(nil, "", nil), nil
	default:
		return nil, fmt.Errorf("unknown strategy %q (want %s|%s|%s|%s|%s|%s)",
			name, StrategyWeightedRoundRobin, StrategyRoundRobin,
			StrategyLeastConnections, StrategyRandom,
			StrategyCostBoundedLeastLatency, StrategyStickyByCloud)
	}
}

type RoundRobin struct{ counter atomic.Uint64 }

func (*RoundRobin) Name() string { return StrategyRoundRobin }

func (r *RoundRobin) Pick(_ *http.Request, healthy []*Backend) (*Backend, bool) {
	if len(healthy) == 0 {
		return nil, false
	}
	i := r.counter.Add(1) - 1
	return healthy[int(i%uint64(len(healthy)))], true
}

type WeightedRoundRobin struct{ counter atomic.Uint64 }

func (*WeightedRoundRobin) Name() string { return StrategyWeightedRoundRobin }

func (w *WeightedRoundRobin) Pick(_ *http.Request, healthy []*Backend) (*Backend, bool) {
	if len(healthy) == 0 {
		return nil, false
	}
	total := 0
	for _, b := range healthy {
		if b.Weight > 0 {
			total += b.Weight
		}
	}
	if total <= 0 {
		i := w.counter.Add(1) - 1
		return healthy[int(i%uint64(len(healthy)))], true
	}
	i := int(w.counter.Add(1) - 1)
	pos := i % total
	cum := 0
	for _, b := range healthy {
		if b.Weight <= 0 {
			continue
		}
		cum += b.Weight
		if pos < cum {
			return b, true
		}
	}
	return healthy[len(healthy)-1], true
}

type LeastConnections struct{}

func (LeastConnections) Name() string { return StrategyLeastConnections }

func (LeastConnections) Pick(_ *http.Request, healthy []*Backend) (*Backend, bool) {
	if len(healthy) == 0 {
		return nil, false
	}
	pick := healthy[0]
	min := pick.Inflight()
	for _, b := range healthy[1:] {
		if l := b.Inflight(); l < min {
			min, pick = l, b
		}
	}
	return pick, true
}

type Random struct{}

func (Random) Name() string { return StrategyRandom }

func (Random) Pick(_ *http.Request, healthy []*Backend) (*Backend, bool) {
	if len(healthy) == 0 {
		return nil, false
	}
	return healthy[rand.IntN(len(healthy))], true
}

// CostBoundedLeastLatency picks the lowest-EWMA-latency backend whose cloud
// is currently within its egress budget. Backends with no latency sample
// yet are preferred (so traffic flows to fresh backends and starts measuring).
// If every cloud is over budget, falls back to the global-lowest-latency pick
// so we keep serving rather than hard-fail.
type CostBoundedLeastLatency struct {
	accountant *cost.Accountant
	rr         atomic.Uint64
}

func NewCostBoundedLeastLatency(a *cost.Accountant) *CostBoundedLeastLatency {
	return &CostBoundedLeastLatency{accountant: a}
}

func (*CostBoundedLeastLatency) Name() string { return StrategyCostBoundedLeastLatency }

func (s *CostBoundedLeastLatency) Pick(_ *http.Request, healthy []*Backend) (*Backend, bool) {
	if len(healthy) == 0 {
		return nil, false
	}
	allowed := make([]*Backend, 0, len(healthy))
	for _, b := range healthy {
		if s.accountant == nil || s.accountant.Allowed(b.Cloud) {
			allowed = append(allowed, b)
		}
	}
	if len(allowed) == 0 {
		// every cloud is over budget — keep serving instead of failing closed
		allowed = healthy
	}
	// Prefer any backend with no latency sample yet, RR among them
	var unsampled []*Backend
	for _, b := range allowed {
		if b.Latency() == 0 {
			unsampled = append(unsampled, b)
		}
	}
	if len(unsampled) > 0 {
		i := s.rr.Add(1) - 1
		return unsampled[int(i%uint64(len(unsampled)))], true
	}
	best := allowed[0]
	bestLat := best.Latency()
	for _, b := range allowed[1:] {
		if l := b.Latency(); l < bestLat {
			bestLat, best = l, b
		}
	}
	return best, true
}
