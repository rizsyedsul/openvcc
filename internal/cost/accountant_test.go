package cost

import (
	"testing"
	"time"
)

func gb(f float64) int64 {
	const oneGB float64 = 1 << 30
	return int64(f * oneGB)
}

func TestAccountant_NilSafe(t *testing.T) {
	var a *Accountant
	a.AddEgress("aws", 100)
	if !a.Allowed("aws") {
		t.Error("nil accountant should allow everything")
	}
	if a.UsageBytes("aws") != 0 {
		t.Error("nil accountant usage should be 0")
	}
}

func TestAccountant_NoBudget_AlwaysAllowed(t *testing.T) {
	a := New(nil, nil)
	a.AddEgress("aws", 1<<40)
	if !a.Allowed("aws") {
		t.Error("uncapped cloud should always be allowed")
	}
}

func TestAccountant_BlocksWhenOverBudget(t *testing.T) {
	now := time.Now()
	a := New(map[string]Budget{
		"aws": {Window: time.Minute, MaxGB: 1},
	}, nil)
	a.now = func() time.Time { return now }

	if !a.Allowed("aws") {
		t.Fatal("fresh accountant should allow")
	}
	a.AddEgress("aws", gb(0.5))
	if !a.Allowed("aws") {
		t.Fatal("under budget should still allow")
	}
	a.AddEgress("aws", gb(0.6))
	if a.Allowed("aws") {
		t.Fatal("over budget should block")
	}
}

func TestAccountant_WindowSlide(t *testing.T) {
	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	a := New(map[string]Budget{
		"aws": {Window: 10 * time.Second, MaxGB: 1},
	}, nil)
	a.now = func() time.Time { return t0 }

	a.AddEgress("aws", gb(2)) // 2 GB at t0
	if a.Allowed("aws") {
		t.Fatal("should be over budget at t0")
	}

	a.now = func() time.Time { return t0.Add(11 * time.Second) }
	if !a.Allowed("aws") {
		t.Fatal("after window slides past, should be allowed again")
	}
}

func TestAccountant_PerCloudIndependent(t *testing.T) {
	a := New(map[string]Budget{
		"aws":   {Window: time.Minute, MaxGB: 0.001}, // ~1 MB
		"azure": {Window: time.Minute, MaxGB: 1},
	}, nil)
	a.AddEgress("aws", 2*1024*1024) // 2 MB
	if a.Allowed("aws") {
		t.Error("aws should be over budget")
	}
	if !a.Allowed("azure") {
		t.Error("azure should be unaffected")
	}
}

func TestAccountant_PricePerGB(t *testing.T) {
	a := New(nil, map[string]float64{"aws": 0.09, "azure": 0.087})
	if a.PricePerGB("aws") != 0.09 {
		t.Errorf("aws price=%v", a.PricePerGB("aws"))
	}
	if a.PricePerGB("missing") != 0 {
		t.Error("unknown cloud should return 0")
	}
}

func TestAccountant_UsageBytes(t *testing.T) {
	t0 := time.Now()
	a := New(map[string]Budget{
		"aws": {Window: 30 * time.Second, MaxGB: 10},
	}, nil)
	a.now = func() time.Time { return t0 }
	a.AddEgress("aws", 100)
	a.AddEgress("aws", 200)
	if got := a.UsageBytes("aws"); got != 300 {
		t.Errorf("usage=%d want 300", got)
	}
}
