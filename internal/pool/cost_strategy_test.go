package pool

import (
	"net/url"
	"testing"
	"time"

	"github.com/syedsumx/openvcc/internal/cost"
)

func mkB(name, cloud string, latencyNS uint64) *Backend {
	u, _ := url.Parse("http://" + name)
	b := NewBackend(name, u, cloud, "r", 1)
	if latencyNS > 0 {
		b.latencyNS.Store(latencyNS)
	}
	return b
}

func TestCostBoundedLeastLatency_PrefersUnsampled(t *testing.T) {
	a := mkB("a", "aws", 100_000_000)
	b := mkB("b", "azure", 0) // no sample yet
	s := NewCostBoundedLeastLatency(nil)
	got, ok := s.Pick(nil, []*Backend{a, b})
	if !ok || got != b {
		t.Fatalf("expected to prefer unsampled backend; got %v", got)
	}
}

func TestCostBoundedLeastLatency_PicksLowestLatency(t *testing.T) {
	a := mkB("a", "aws", 100_000_000)
	b := mkB("b", "azure", 50_000_000)
	s := NewCostBoundedLeastLatency(nil)
	got, _ := s.Pick(nil, []*Backend{a, b})
	if got != b {
		t.Fatalf("expected b (lower latency); got %v", got)
	}
}

func TestCostBoundedLeastLatency_FiltersByBudget(t *testing.T) {
	now := time.Now()
	acc := cost.New(map[string]cost.Budget{
		"aws": {Window: time.Minute, MaxGB: 0.001}, // ~1 MB
	}, nil)
	// Burn the aws budget
	acc.AddEgress("aws", 10*1024*1024)

	a := mkB("a", "aws", 50_000_000)   // would win on latency, but blocked
	b := mkB("b", "azure", 100_000_000)
	s := NewCostBoundedLeastLatency(acc)
	got, _ := s.Pick(nil, []*Backend{a, b})
	if got != b {
		t.Fatalf("expected azure (aws over budget); got %v", got)
	}
	_ = now
}

func TestCostBoundedLeastLatency_FallbackWhenAllOverBudget(t *testing.T) {
	acc := cost.New(map[string]cost.Budget{
		"aws":   {Window: time.Minute, MaxGB: 0.0001},
		"azure": {Window: time.Minute, MaxGB: 0.0001},
	}, nil)
	acc.AddEgress("aws", 1024*1024)
	acc.AddEgress("azure", 1024*1024)

	a := mkB("a", "aws", 100_000_000)
	b := mkB("b", "azure", 50_000_000) // lower
	s := NewCostBoundedLeastLatency(acc)
	got, ok := s.Pick(nil, []*Backend{a, b})
	if !ok {
		t.Fatal("expected a pick when both over budget (fallback path)")
	}
	if got != b {
		t.Errorf("fallback should still pick lowest latency; got %v", got)
	}
}

func TestCostBoundedLeastLatency_EmptyHealthy(t *testing.T) {
	s := NewCostBoundedLeastLatency(nil)
	if _, ok := s.Pick(nil, nil); ok {
		t.Error("empty pool should return ok=false")
	}
}

func TestParseStrategy_KnowsCostBounded(t *testing.T) {
	// ParseStrategy returns a default-constructed instance; engine is responsible
	// for swapping in a properly-configured one via NewCostBoundedLeastLatency.
	s, err := ParseStrategy(StrategyCostBoundedLeastLatency)
	if err != nil {
		t.Fatalf("ParseStrategy: %v", err)
	}
	if s.Name() != StrategyCostBoundedLeastLatency {
		t.Errorf("Name=%q", s.Name())
	}
}
