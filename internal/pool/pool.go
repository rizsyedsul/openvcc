package pool

import (
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type Backend struct {
	Name   string
	URL    *url.URL
	Cloud  string
	Region string
	Weight int
	Kind   string // optional: writeable | read_only | cache | <user-defined>

	healthy   atomic.Bool
	inflight  atomic.Int64
	latencyNS atomic.Uint64
}

func NewBackend(name string, u *url.URL, cloud, region string, weight int) *Backend {
	b := &Backend{Name: name, URL: u, Cloud: cloud, Region: region, Weight: weight}
	b.healthy.Store(true)
	return b
}

// WithKind sets the backend's workload kind label. Returns the receiver to
// allow chaining at construction time.
func (b *Backend) WithKind(kind string) *Backend {
	b.Kind = kind
	return b
}

func (b *Backend) Healthy() bool     { return b.healthy.Load() }
func (b *Backend) SetHealthy(v bool) { b.healthy.Store(v) }
func (b *Backend) Inflight() int64   { return b.inflight.Load() }
func (b *Backend) IncInflight()      { b.inflight.Add(1) }
func (b *Backend) DecInflight()      { b.inflight.Add(-1) }

// Latency returns the current EWMA-smoothed request duration.
// Zero before any samples have been recorded.
func (b *Backend) Latency() time.Duration {
	return time.Duration(b.latencyNS.Load())
}

// RecordLatency feeds a new sample into the backend's EWMA. The smoothing
// factor (alpha) is 0.1 — recent samples are weighted but old samples decay
// gradually, so a single outlier does not yank the routing decision.
func (b *Backend) RecordLatency(d time.Duration) {
	if d <= 0 {
		return
	}
	const alpha = 0.1
	sample := uint64(d.Nanoseconds())
	for {
		cur := b.latencyNS.Load()
		var next uint64
		if cur == 0 {
			next = sample
		} else {
			next = uint64(float64(sample)*alpha + float64(cur)*(1-alpha))
		}
		if b.latencyNS.CompareAndSwap(cur, next) {
			return
		}
	}
}

type Pool struct {
	mu       sync.RWMutex
	all      []*Backend
	strategy Strategy
}

func New(backends []*Backend, s Strategy) *Pool {
	return &Pool{all: append([]*Backend(nil), backends...), strategy: s}
}

func (p *Pool) Pick(req *http.Request) (*Backend, bool) {
	p.mu.RLock()
	healthy := make([]*Backend, 0, len(p.all))
	for _, b := range p.all {
		if b.Healthy() {
			healthy = append(healthy, b)
		}
	}
	s := p.strategy
	p.mu.RUnlock()
	return s.Pick(req, healthy)
}

func (p *Pool) Backends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Backend, len(p.all))
	copy(out, p.all)
	return out
}

func (p *Pool) Find(name string) (*Backend, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, b := range p.all {
		if b.Name == name {
			return b, true
		}
	}
	return nil, false
}

func (p *Pool) Replace(backends []*Backend, s Strategy) {
	p.mu.Lock()
	p.all = append([]*Backend(nil), backends...)
	if s != nil {
		p.strategy = s
	}
	p.mu.Unlock()
}

func (p *Pool) Add(b *Backend) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, existing := range p.all {
		if existing.Name == b.Name {
			return false
		}
	}
	p.all = append(p.all, b)
	return true
}

func (p *Pool) Remove(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, b := range p.all {
		if b.Name == name {
			p.all = append(p.all[:i], p.all[i+1:]...)
			return true
		}
	}
	return false
}

func (p *Pool) Strategy() Strategy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.strategy
}
