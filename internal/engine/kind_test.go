package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syedsumx/openvcc/internal/config"
)

func TestEngine_KindRouting_WritesGoToWriteable(t *testing.T) {
	var awsHits, azureHits atomic.Int64
	awsApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		awsHits.Add(1)
		_, _ = w.Write([]byte("aws"))
	}))
	defer awsApp.Close()
	azureApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		azureHits.Add(1)
		_, _ = w.Write([]byte("azure"))
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
			{Name: "aws", URL: awsApp.URL, Cloud: "aws", Region: "us-east-1", Weight: 1, Kind: "writeable"},
			{Name: "azure", URL: azureApp.URL, Cloud: "azure", Region: "eastus", Weight: 1, Kind: "read_only"},
		},
		Strategy: "round_robin",
		KindRouting: &config.KindRouting{
			WriteMethods: []string{"POST", "PUT", "DELETE", "PATCH"},
			WriteKinds:   []string{"writeable"},
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

	// Drive 10 writes — every one should hit aws (the only writeable backend).
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest("POST", "http://"+addr+"/", strings.NewReader("x"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		got := resp.Header.Get("X-Cloud")
		resp.Body.Close()
		if got != "aws" {
			t.Errorf("write %d hit %q, want aws", i, got)
		}
	}

	// Drive 20 reads — should split across both backends.
	for i := 0; i < 20; i++ {
		req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		resp.Body.Close()
	}
	if azureHits.Load() == 0 {
		t.Error("reads should have reached azure too")
	}
}
