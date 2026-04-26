// Package cost implements rolling-window egress accounting and per-cloud
// budget enforcement. It is consumed by the cost_bounded_least_latency
// strategy in internal/pool.
package cost

import (
	"sync"
	"time"
)

// Budget caps the bytes a single cloud may emit per Window.
type Budget struct {
	Window time.Duration
	MaxGB  float64
}

// MaxBytes returns the byte cap for the budget window.
func (b Budget) MaxBytes() int64 {
	return int64(b.MaxGB * 1024 * 1024 * 1024)
}

// Accountant tracks egress bytes per cloud over a sliding window and answers
// Allowed(cloud) based on the configured Budget for that cloud.
//
// Goroutine-safe.
type Accountant struct {
	now     func() time.Time
	mu      sync.Mutex
	budgets map[string]Budget
	prices  map[string]float64
	tally   map[string]*ringWindow
}

// New constructs an Accountant. budgets and prices may be nil.
func New(budgets map[string]Budget, prices map[string]float64) *Accountant {
	return &Accountant{
		now:     time.Now,
		budgets: copyBudgets(budgets),
		prices:  copyFloats(prices),
		tally:   make(map[string]*ringWindow),
	}
}

// AddEgress records that `cloud` emitted `bytes` outbound.
func (a *Accountant) AddEgress(cloud string, bytes int64) {
	if a == nil || bytes <= 0 || cloud == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	w, ok := a.tally[cloud]
	if !ok {
		w = newRingWindow()
		a.tally[cloud] = w
	}
	w.add(a.now(), bytes)
}

// Allowed reports whether `cloud` is currently under its budget.
// A cloud with no configured budget is always allowed.
func (a *Accountant) Allowed(cloud string) bool {
	if a == nil {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.budgets[cloud]
	if !ok {
		return true
	}
	w := a.tally[cloud]
	if w == nil {
		return true
	}
	used := w.sum(a.now(), b.Window)
	return used < b.MaxBytes()
}

// UsageBytes returns how many bytes `cloud` emitted in the last Window
// configured for it (or the last minute if none configured).
func (a *Accountant) UsageBytes(cloud string) int64 {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	w := a.tally[cloud]
	if w == nil {
		return 0
	}
	window := time.Minute
	if b, ok := a.budgets[cloud]; ok {
		window = b.Window
	}
	return w.sum(a.now(), window)
}

// PricePerGB returns the configured egress price for cloud in $/GB.
// Zero if not configured.
func (a *Accountant) PricePerGB(cloud string) float64 {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.prices[cloud]
}

func copyBudgets(in map[string]Budget) map[string]Budget {
	out := make(map[string]Budget, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyFloats(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
