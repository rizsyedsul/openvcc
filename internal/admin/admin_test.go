package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/syedsumx/openvcc/internal/config"
	"github.com/syedsumx/openvcc/internal/metrics"
	"github.com/syedsumx/openvcc/internal/pool"
	"github.com/syedsumx/openvcc/internal/proxy"

	"github.com/prometheus/client_golang/prometheus"
)

const token = "secret-token"

func mkServer(t *testing.T, p *pool.Pool, reload func(context.Context) error) (*Server, *proxy.Handler) {
	t.Helper()
	m := metrics.New(prometheus.NewRegistry())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ph := proxy.New(p, m, log, config.Proxy{})
	return New(p, ph, log, Options{Token: token, Reload: reload}), ph
}

func mkPool(t *testing.T) *pool.Pool {
	t.Helper()
	u, _ := url.Parse("http://10.0.0.1:8000")
	a := pool.NewBackend("a", u, "aws", "us-east-1", 1)
	return pool.New([]*pool.Backend{a}, &pool.RoundRobin{})
}

func authedReq(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestAdmin_Auth_Rejects(t *testing.T) {
	s, _ := mkServer(t, mkPool(t), nil)
	cases := map[string]string{
		"missing":     "",
		"wrong":       "Bearer nope",
		"unprefixed":  token,
		"empty token": "Bearer ",
	}
	for name, hdr := range cases {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/admin/backends", nil)
			if hdr != "" {
				req.Header.Set("Authorization", hdr)
			}
			s.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d want 401 (got body %q)", w.Code, w.Body.String())
			}
		})
	}
}

func TestAdmin_Auth_DisabledWhenNoToken(t *testing.T) {
	p := mkPool(t)
	m := metrics.New(prometheus.NewRegistry())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ph := proxy.New(p, m, log, config.Proxy{})
	s := New(p, ph, log, Options{Token: "", Reload: nil})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/backends", nil)
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", w.Code)
	}
}

func TestAdmin_ListAddRemove(t *testing.T) {
	p := mkPool(t)
	s, _ := mkServer(t, p, nil)
	h := s.Handler()

	w := httptest.NewRecorder()
	h.ServeHTTP(w, authedReq("GET", "/admin/backends", ""))
	if w.Code != 200 {
		t.Fatalf("list status=%d body=%s", w.Code, w.Body.String())
	}
	var list []backendDTO
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 || list[0].Name != "a" {
		t.Fatalf("unexpected list: %+v", list)
	}

	w = httptest.NewRecorder()
	body, _ := json.Marshal(backendDTO{Name: "b", URL: "http://10.1.0.1:8000", Cloud: "azure", Weight: 2})
	h.ServeHTTP(w, authedReq("POST", "/admin/backends", string(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("add status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, authedReq("POST", "/admin/backends", string(body)))
	if w.Code != http.StatusConflict {
		t.Fatalf("dup add status=%d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, authedReq("DELETE", "/admin/backends/b", ""))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, authedReq("DELETE", "/admin/backends/missing", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete missing status=%d", w.Code)
	}
}

func TestAdmin_AddRejectsBadInput(t *testing.T) {
	s, _ := mkServer(t, mkPool(t), nil)
	h := s.Handler()
	cases := []struct{ name, body string }{
		{"empty body", `{}`},
		{"missing url", `{"name":"x"}`},
		{"bad scheme", `{"name":"x","url":"ftp://x:1"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, authedReq("POST", "/admin/backends", tc.body))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAdmin_Reload_CallsCallback(t *testing.T) {
	called := 0
	s, _ := mkServer(t, mkPool(t), func(ctx context.Context) error { called++; return nil })
	h := s.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, authedReq("POST", "/admin/reload", ""))
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if called != 1 {
		t.Fatalf("reload called %d times", called)
	}
}

func TestAdmin_Reload_NotConfigured(t *testing.T) {
	s, _ := mkServer(t, mkPool(t), nil)
	h := s.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, authedReq("POST", "/admin/reload", ""))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestAdmin_SetBackendHealth(t *testing.T) {
	p := mkPool(t)
	s, _ := mkServer(t, p, nil)
	h := s.Handler()

	w := httptest.NewRecorder()
	h.ServeHTTP(w, authedReq("PUT", "/admin/backends/a/health", `{"healthy": false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	b, _ := p.Find("a")
	if b.Healthy() {
		t.Fatal("backend should be unhealthy after override")
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, authedReq("PUT", "/admin/backends/a/health", `{"healthy": true}`))
	if w.Code != http.StatusOK {
		t.Fatalf("re-enable status=%d", w.Code)
	}
	if !b.Healthy() {
		t.Fatal("backend should be healthy after re-enable")
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, authedReq("PUT", "/admin/backends/missing/health", `{"healthy": false}`))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing backend status=%d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, authedReq("PUT", "/admin/backends/a/health", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing field status=%d", w.Code)
	}
}

func TestAdmin_BadJSONBody(t *testing.T) {
	s, _ := mkServer(t, mkPool(t), nil)
	h := s.Handler()
	w := httptest.NewRecorder()
	req := authedReq("POST", "/admin/backends", "not json")
	req.Body = io.NopCloser(bytes.NewReader([]byte("not json")))
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
}
