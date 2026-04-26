package health

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/syedsumx/openvcc/internal/config"
	"github.com/syedsumx/openvcc/internal/metrics"
	"github.com/syedsumx/openvcc/internal/pool"
)

type Checker struct {
	pool    *pool.Pool
	cfg     config.Health
	client  *http.Client
	metrics *metrics.Collectors
	log     *slog.Logger

	mu    sync.Mutex
	state map[string]*streak
}

type streak struct {
	upHits, downHits int
}

func New(p *pool.Pool, cfg config.Health, m *metrics.Collectors, log *slog.Logger) *Checker {
	if log == nil {
		log = slog.Default()
	}
	return &Checker{
		pool:    p,
		cfg:     cfg,
		client:  &http.Client{Timeout: cfg.Timeout},
		metrics: m,
		log:     log.With("component", "health"),
		state:   make(map[string]*streak),
	}
}

func (c *Checker) Run(ctx context.Context) error {
	t := time.NewTicker(c.cfg.Interval)
	defer t.Stop()
	c.Once(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			c.Once(ctx)
		}
	}
}

func (c *Checker) Once(ctx context.Context) {
	backends := c.pool.Backends()
	var wg sync.WaitGroup
	for _, b := range backends {
		wg.Add(1)
		go func(b *pool.Backend) {
			defer wg.Done()
			c.check(ctx, b)
		}(b)
	}
	wg.Wait()
	if c.metrics != nil {
		var up int
		for _, b := range c.pool.Backends() {
			if b.Healthy() {
				up++
			}
		}
		c.metrics.ActiveBackends.Set(float64(up))
	}
}

func (c *Checker) check(ctx context.Context, b *pool.Backend) {
	url := strings.TrimRight(b.URL.String(), "/") + c.cfg.Path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.record(b, false, fmt.Errorf("build request: %w", err))
		return
	}
	start := time.Now()
	resp, err := c.client.Do(req)
	dur := time.Since(start)
	if c.metrics != nil {
		c.metrics.HealthCheckDur.WithLabelValues(b.Name).Observe(dur.Seconds())
	}
	if err != nil {
		c.record(b, false, err)
		return
	}
	defer resp.Body.Close()
	ok := resp.StatusCode >= c.cfg.ExpectedStatusFloor && resp.StatusCode <= c.cfg.ExpectedStatusCeil
	if !ok {
		c.record(b, false, fmt.Errorf("status %d", resp.StatusCode))
		return
	}
	c.record(b, true, nil)
}

func (c *Checker) record(b *pool.Backend, ok bool, err error) {
	c.mu.Lock()
	s, found := c.state[b.Name]
	if !found {
		s = &streak{}
		c.state[b.Name] = s
	}
	prev := b.Healthy()
	if ok {
		s.upHits++
		s.downHits = 0
		if !prev && s.upHits >= c.cfg.HealthyThreshold {
			b.SetHealthy(true)
			c.log.Info("backend healthy", "backend", b.Name, "cloud", b.Cloud)
		}
	} else {
		s.downHits++
		s.upHits = 0
		if prev && s.downHits >= c.cfg.UnhealthyThreshold {
			b.SetHealthy(false)
			c.log.Warn("backend unhealthy", "backend", b.Name, "cloud", b.Cloud, "err", err)
		}
	}
	c.mu.Unlock()

	if c.metrics != nil {
		gauge := c.metrics.BackendUp.WithLabelValues(b.Name, b.Cloud)
		if b.Healthy() {
			gauge.Set(1)
		} else {
			gauge.Set(0)
		}
	}
}
