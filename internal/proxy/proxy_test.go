package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/syedsumx/openvcc/internal/config"
	"github.com/syedsumx/openvcc/internal/cost"
	"github.com/syedsumx/openvcc/internal/metrics"
	"github.com/syedsumx/openvcc/internal/pool"

	"github.com/prometheus/client_golang/prometheus"
)

func newBackend(t *testing.T, name, cloud string, h http.Handler) (*pool.Backend, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	u, _ := url.Parse(srv.URL)
	return pool.NewBackend(name, u, cloud, "r", 1), srv
}

func mkHandler(t *testing.T, p *pool.Pool) *Handler {
	t.Helper()
	m := metrics.New(prometheus.NewRegistry())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(p, m, log, config.Proxy{RequestTimeout: 2 * time.Second, MaxIdleConnsPerHost: 8})
}

func TestProxy_RoutesAndStampsHeaders(t *testing.T) {
	a, srvA := newBackend(t, "a", "aws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("from-a"))
	}))
	defer srvA.Close()
	b, srvB := newBackend(t, "b", "azure", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("from-b"))
	}))
	defer srvB.Close()

	p := pool.New([]*pool.Backend{a, b}, &pool.RoundRobin{})
	h := mkHandler(t, p)

	hits := map[string]int{}
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		h.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("status=%d", w.Code)
		}
		body := w.Body.String()
		hits[body]++
		if got := w.Header().Get("X-Backend"); got == "" {
			t.Errorf("missing X-Backend header")
		}
		if got := w.Header().Get("X-Cloud"); got == "" {
			t.Errorf("missing X-Cloud header")
		}
	}
	if hits["from-a"] != 5 || hits["from-b"] != 5 {
		t.Errorf("uneven distribution: %+v", hits)
	}
}

func TestProxy_NoBackend_502(t *testing.T) {
	p := pool.New(nil, &pool.RoundRobin{})
	h := mkHandler(t, p)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("status=%d want 502", w.Code)
	}
}

func TestProxy_BackendUnreachable_502(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:1") // unlikely to be open
	dead := pool.NewBackend("dead", u, "aws", "r", 1)
	p := pool.New([]*pool.Backend{dead}, &pool.RoundRobin{})
	h := mkHandler(t, p)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("status=%d want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "upstream error") {
		t.Errorf("body=%q", w.Body.String())
	}
}

func TestProxy_FeedsAccountant_AndLatency(t *testing.T) {
	a, srvA := newBackend(t, "a", "aws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("body-12345"))
	}))
	defer srvA.Close()
	p := pool.New([]*pool.Backend{a}, &pool.RoundRobin{})
	h := mkHandler(t, p)
	acc := cost.New(map[string]cost.Budget{"aws": {Window: time.Minute, MaxGB: 1}}, nil)
	h.SetAccountant(acc)

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	}

	if got := acc.UsageBytes("aws"); got < 30 {
		t.Errorf("accountant did not record bytes: %d", got)
	}
	if a.Latency() == 0 {
		t.Error("backend latency not recorded")
	}
}

func TestProxy_Forget_RemovesCachedProxy(t *testing.T) {
	a, srvA := newBackend(t, "a", "aws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srvA.Close()
	p := pool.New([]*pool.Backend{a}, &pool.RoundRobin{})
	h := mkHandler(t, p)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if _, ok := h.proxies["a"]; !ok {
		t.Fatal("expected proxy cached after first request")
	}
	h.Forget("a")
	if _, ok := h.proxies["a"]; ok {
		t.Fatal("Forget should remove cached proxy")
	}
}
