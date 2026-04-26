package health

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syedsumx/openvcc/internal/config"
	"github.com/syedsumx/openvcc/internal/metrics"
	"github.com/syedsumx/openvcc/internal/pool"

	"github.com/prometheus/client_golang/prometheus"
)

func newServer(handler http.Handler) (*httptest.Server, *url.URL) {
	srv := httptest.NewServer(handler)
	u, _ := url.Parse(srv.URL)
	return srv, u
}

func mkChecker(t *testing.T, p *pool.Pool, cfg config.Health) *Checker {
	t.Helper()
	cfg.Path = "/healthz"
	cfg.Interval = 10 * time.Millisecond
	cfg.Timeout = 5 * time.Millisecond
	cfg.UnhealthyThreshold = 2
	cfg.HealthyThreshold = 2
	cfg.ExpectedStatusFloor = 200
	cfg.ExpectedStatusCeil = 399
	m := metrics.New(prometheus.NewRegistry())
	return New(p, cfg, m, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestChecker_FlipsToUnhealthyAfterThreshold(t *testing.T) {
	var failing atomic.Bool
	srv, u := newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := pool.NewBackend("a", u, "aws", "us-east-1", 1)
	p := pool.New([]*pool.Backend{b}, &pool.RoundRobin{})
	c := mkChecker(t, p, config.Health{Timeout: 50 * time.Millisecond})
	c.cfg.Timeout = 50 * time.Millisecond
	c.client.Timeout = 50 * time.Millisecond
	ctx := context.Background()

	c.Once(ctx)
	if !b.Healthy() {
		t.Fatal("expected initial healthy=true after first OK")
	}

	failing.Store(true)
	c.Once(ctx)
	if !b.Healthy() {
		t.Fatal("after 1 failure, threshold=2: should still be healthy")
	}
	c.Once(ctx)
	if b.Healthy() {
		t.Fatal("after 2 failures: should be unhealthy")
	}

	failing.Store(false)
	c.Once(ctx)
	if b.Healthy() {
		t.Fatal("after 1 recovery, threshold=2: should still be unhealthy")
	}
	c.Once(ctx)
	if !b.Healthy() {
		t.Fatal("after 2 recoveries: should be healthy again")
	}
}

func TestChecker_HandlesTimeout(t *testing.T) {
	srv, u := newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := pool.NewBackend("a", u, "aws", "us-east-1", 1)
	p := pool.New([]*pool.Backend{b}, &pool.RoundRobin{})
	c := mkChecker(t, p, config.Health{})
	c.client.Timeout = 20 * time.Millisecond
	ctx := context.Background()

	c.Once(ctx)
	c.Once(ctx)
	if b.Healthy() {
		t.Fatal("backend should be marked unhealthy after timeouts")
	}
}

func TestChecker_RunCancels(t *testing.T) {
	srv, u := newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := pool.NewBackend("a", u, "aws", "us-east-1", 1)
	p := pool.New([]*pool.Backend{b}, &pool.RoundRobin{})
	c := mkChecker(t, p, config.Health{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected ctx-canceled error")
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func mustNotPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	f()
}

func TestChecker_BadURLDoesNotPanic(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:1") // unlikely listener
	b := pool.NewBackend("dead", u, "aws", "x", 1)
	p := pool.New([]*pool.Backend{b}, &pool.RoundRobin{})
	c := mkChecker(t, p, config.Health{})
	c.client.Timeout = 20 * time.Millisecond
	mustNotPanic(t, func() { c.Once(context.Background()) })
}
