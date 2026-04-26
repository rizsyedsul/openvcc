package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/syedsumx/openvcc/internal/config"
)

func TestEngine_Sticky_CookiePinsCloud(t *testing.T) {
	awsApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("aws-served"))
	}))
	defer awsApp.Close()
	azureApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("azure-served"))
	}))
	defer azureApp.Close()

	pp := freePort(t)
	mp := freePort(t)
	cfg := &config.Config{
		Listen: config.Listen{
			Proxy:   fmt.Sprintf("127.0.0.1:%d", pp),
			Metrics: fmt.Sprintf("127.0.0.1:%d", mp),
		},
		Backends: []config.Backend{
			{Name: "aws", URL: awsApp.URL, Cloud: "aws", Region: "us-east-1", Weight: 1},
			{Name: "azure", URL: azureApp.URL, Cloud: "azure", Region: "eastus", Weight: 1},
		},
		Strategy: "sticky_by_cloud",
		Sticky: &config.Sticky{
			Hash:             "cookie:openvcc_cloud",
			FallbackStrategy: "round_robin",
			CookieTTL:        time.Hour,
		},
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

	addr := fmt.Sprintf("127.0.0.1:%d", pp)
	waitListening(t, addr)

	// First request with no cookie: gets pinned via fallback + Set-Cookie.
	req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("first GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	pinned := resp.Header.Get("X-Cloud")
	if pinned != "aws" && pinned != "azure" {
		t.Fatalf("X-Cloud=%q", pinned)
	}
	setCookie := resp.Header.Get("Set-Cookie")
	if !strings.Contains(setCookie, "openvcc_cloud="+pinned) {
		t.Fatalf("Set-Cookie missing openvcc_cloud=%s: %q", pinned, setCookie)
	}
	expectedBody := pinned + "-served"
	if string(body) != expectedBody {
		t.Errorf("body=%q want %q", string(body), expectedBody)
	}

	// Subsequent requests carrying the cookie should keep going to that cloud.
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
		req.AddCookie(&http.Cookie{Name: "openvcc_cloud", Value: pinned})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		got := resp.Header.Get("X-Cloud")
		resp.Body.Close()
		if got != pinned {
			t.Errorf("iter %d: X-Cloud=%q want %q", i, got, pinned)
		}
	}
}
