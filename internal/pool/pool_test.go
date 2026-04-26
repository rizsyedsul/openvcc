package pool

import (
	"net/url"
	"testing"
)

func mkBackend(t *testing.T, name, raw, cloud, region string, w int) *Backend {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return NewBackend(name, u, cloud, region, w)
}

func TestPool_FindAddRemove(t *testing.T) {
	a := mkBackend(t, "a", "http://a", "aws", "us-east-1", 1)
	b := mkBackend(t, "b", "http://b", "azure", "eastus", 1)
	p := New([]*Backend{a}, &RoundRobin{})

	if got, ok := p.Find("a"); !ok || got != a {
		t.Fatalf("Find(a) failed: got=%v ok=%v", got, ok)
	}
	if _, ok := p.Find("missing"); ok {
		t.Fatal("Find(missing) should fail")
	}

	if !p.Add(b) {
		t.Fatal("Add(b) should succeed")
	}
	if p.Add(b) {
		t.Fatal("Add(b) twice should be rejected")
	}
	if got := p.Backends(); len(got) != 2 {
		t.Fatalf("Backends len=%d want 2", len(got))
	}

	if !p.Remove("a") {
		t.Fatal("Remove(a) should succeed")
	}
	if p.Remove("a") {
		t.Fatal("Remove(a) again should fail")
	}
	if got := p.Backends(); len(got) != 1 || got[0].Name != "b" {
		t.Fatalf("after remove: %+v", got)
	}
}

func TestPool_Replace(t *testing.T) {
	a := mkBackend(t, "a", "http://a", "aws", "us-east-1", 1)
	b := mkBackend(t, "b", "http://b", "azure", "eastus", 1)
	p := New([]*Backend{a}, &RoundRobin{})

	p.Replace([]*Backend{b}, &Random{})
	if got := p.Backends(); len(got) != 1 || got[0].Name != "b" {
		t.Fatalf("Replace failed: %+v", got)
	}
	if p.Strategy().Name() != StrategyRandom {
		t.Fatalf("strategy not swapped: %s", p.Strategy().Name())
	}
}

func TestPool_PickSkipsUnhealthy(t *testing.T) {
	a := mkBackend(t, "a", "http://a", "aws", "us-east-1", 1)
	b := mkBackend(t, "b", "http://b", "azure", "eastus", 1)
	p := New([]*Backend{a, b}, &RoundRobin{})

	a.SetHealthy(false)
	for i := 0; i < 5; i++ {
		got, ok := p.Pick(nil)
		if !ok || got.Name != "b" {
			t.Fatalf("pick %d: ok=%v got=%v want b", i, ok, got)
		}
	}

	b.SetHealthy(false)
	if _, ok := p.Pick(nil); ok {
		t.Fatal("Pick should fail when no backends are healthy")
	}
}

func TestBackend_Latency_FirstSampleAdoptedExactly(t *testing.T) {
	b := mkBackend(t, "a", "http://a", "aws", "us-east-1", 1)
	if b.Latency() != 0 {
		t.Fatal("initial latency should be 0")
	}
	b.RecordLatency(50_000_000) // 50ms
	if got := b.Latency(); got != 50_000_000 {
		t.Errorf("first sample should be exact: got %v", got)
	}
}

func TestBackend_Latency_EWMASmooths(t *testing.T) {
	b := mkBackend(t, "a", "http://a", "aws", "us-east-1", 1)
	b.RecordLatency(100_000_000) // 100ms
	b.RecordLatency(1_000_000_000) // 1000ms outlier
	got := b.Latency().Nanoseconds()
	// 0.1*1000 + 0.9*100 = 100 + 90 = 190ms
	if got < 150_000_000 || got > 220_000_000 {
		t.Errorf("EWMA out of expected range: got %dns", got)
	}
}

func TestBackend_RecordLatency_IgnoresNonPositive(t *testing.T) {
	b := mkBackend(t, "a", "http://a", "aws", "us-east-1", 1)
	b.RecordLatency(0)
	b.RecordLatency(-1)
	if b.Latency() != 0 {
		t.Errorf("non-positive sample should be ignored: got %v", b.Latency())
	}
}

func TestBackend_Inflight(t *testing.T) {
	b := mkBackend(t, "a", "http://a", "aws", "us-east-1", 1)
	if b.Inflight() != 0 {
		t.Fatal("initial inflight should be 0")
	}
	b.IncInflight()
	b.IncInflight()
	if b.Inflight() != 2 {
		t.Fatalf("Inflight=%d want 2", b.Inflight())
	}
	b.DecInflight()
	if b.Inflight() != 1 {
		t.Fatalf("Inflight=%d want 1", b.Inflight())
	}
}
