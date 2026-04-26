package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syedsumx/openvcc/internal/config"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func waitListening(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("tcp", addr); err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("port %s never opened", addr)
}

func TestEngine_EndToEnd(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer upstream.Close()

	pp := freePort(t)
	mp := freePort(t)
	cfg := &config.Config{
		Listen:   config.Listen{Proxy: fmt.Sprintf("127.0.0.1:%d", pp), Metrics: fmt.Sprintf("127.0.0.1:%d", mp)},
		Backends: []config.Backend{{Name: "a", URL: upstream.URL, Cloud: "aws", Region: "us-east-1", Weight: 1}},
		Strategy: "round_robin",
		Health: config.Health{
			Interval: 100 * time.Millisecond, Timeout: 50 * time.Millisecond,
			Path: "/", UnhealthyThreshold: 1, HealthyThreshold: 1,
			ExpectedStatusFloor: 200, ExpectedStatusCeil: 399,
		},
		Proxy: config.Proxy{RequestTimeout: 2 * time.Second, MaxIdleConnsPerHost: 4},
		Log:   config.Log{Level: "info", Format: "json"},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	e, err := New(cfg, "", log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx) }()
	defer func() { cancel(); <-done }()

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", pp)
	waitListening(t, proxyAddr)
	waitListening(t, fmt.Sprintf("127.0.0.1:%d", mp))

	resp, err := http.Get("http://" + proxyAddr + "/")
	if err != nil {
		t.Fatalf("GET proxy: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "hello" {
		t.Errorf("status=%d body=%q", resp.StatusCode, string(body))
	}
	if resp.Header.Get("X-Backend") != "a" {
		t.Errorf("X-Backend=%q", resp.Header.Get("X-Backend"))
	}

	mresp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", mp))
	if err != nil {
		t.Fatalf("GET metrics: %v", err)
	}
	mbody, _ := io.ReadAll(mresp.Body)
	mresp.Body.Close()
	if !strings.Contains(string(mbody), "openvcc_requests_total") {
		t.Errorf("metrics missing openvcc_requests_total: %s", string(mbody))
	}
}

func TestEngine_Reload_FromFile(t *testing.T) {
	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("A"))
	}))
	defer upstreamA.Close()
	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("B"))
	}))
	defer upstreamB.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "openvcc.yaml")
	v1 := fmt.Sprintf(`
backends:
  - {name: a, url: %s, cloud: aws}
strategy: round_robin
health:
  interval: 100ms
  timeout:  50ms
  unhealthy_threshold: 1
  healthy_threshold:   1
`, upstreamA.URL)
	if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load v1: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e, err := New(cfg, path, log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if got := len(e.Pool().Backends()); got != 1 {
		t.Fatalf("initial backends=%d", got)
	}

	v2 := fmt.Sprintf(`
backends:
  - {name: b, url: %s, cloud: azure}
strategy: round_robin
health:
  interval: 100ms
  timeout:  50ms
  unhealthy_threshold: 1
  healthy_threshold:   1
`, upstreamB.URL)
	if err := os.WriteFile(path, []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	bs := e.Pool().Backends()
	if len(bs) != 1 || bs[0].Name != "b" {
		t.Fatalf("after reload: %+v", bs)
	}
}

func TestEngine_Reload_NoPath_Errors(t *testing.T) {
	cfg := &config.Config{
		Backends: []config.Backend{{Name: "a", URL: "http://x:1"}},
		Strategy: "round_robin",
		Health: config.Health{
			Interval: time.Second, Timeout: 100 * time.Millisecond,
			Path: "/", UnhealthyThreshold: 1, HealthyThreshold: 1,
			ExpectedStatusFloor: 200, ExpectedStatusCeil: 399,
		},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e, _ := New(cfg, "", log)
	if err := e.Reload(context.Background()); err == nil {
		t.Fatal("expected error reloading without a config path")
	}
}
