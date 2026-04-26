package chaos

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const testToken = "secret-token"

// fakeStack stands in for a running engine + admin pair.
type fakeStack struct {
	mu        sync.Mutex
	unhealthy map[string]bool
	backends  []map[string]any
	admin     *httptest.Server
	proxy     *httptest.Server
}

func newFakeStack() *fakeStack {
	f := &fakeStack{
		unhealthy: make(map[string]bool),
		backends: []map[string]any{
			{"name": "aws-1", "url": "http://aws-1", "cloud": "aws"},
			{"name": "azure-1", "url": "http://azure-1", "cloud": "azure"},
		},
	}
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /admin/backends", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			http.Error(w, "no", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(f.backends)
	})
	adminMux.HandleFunc("PUT /admin/backends/{name}/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			http.Error(w, "no", 401)
			return
		}
		var in struct {
			Healthy *bool `json:"healthy"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Healthy == nil {
			http.Error(w, "bad", 400)
			return
		}
		name := r.PathValue("name")
		f.mu.Lock()
		f.unhealthy[name] = !*in.Healthy
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"name": name, "healthy": *in.Healthy})
	})
	f.admin = httptest.NewServer(adminMux)

	f.proxy = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		awsDown := f.unhealthy["aws-1"]
		azureDown := f.unhealthy["azure-1"]
		f.mu.Unlock()
		switch {
		case azureDown && !awsDown:
			w.Header().Set("X-Cloud", "aws")
		case awsDown && !azureDown:
			w.Header().Set("X-Cloud", "azure")
		case awsDown && azureDown:
			http.Error(w, "all down", http.StatusBadGateway)
			return
		default:
			// Round-robin-ish: alternate based on request count
			w.Header().Set("X-Cloud", []string{"aws", "azure"}[time.Now().UnixNano()%2])
		}
		_, _ = w.Write([]byte("ok"))
	}))
	return f
}

func (f *fakeStack) close() {
	f.admin.Close()
	f.proxy.Close()
}

func TestRun_PassesOnCleanFailover(t *testing.T) {
	f := newFakeStack()
	defer f.close()

	var out bytes.Buffer
	r, err := Run(context.Background(), Options{
		AdminURL:    f.admin.URL,
		AdminToken:  testToken,
		ProxyURL:    f.proxy.URL,
		FailCloud:   "aws",
		Duration:    300 * time.Millisecond,
		RPS:         200,
		Concurrency: 4,
		Output:      &out,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Result != "pass" {
		t.Errorf("expected pass, got %q reason=%q", r.Result, r.Reason)
	}
	if r.PerCloudCounts["aws"] != 0 {
		t.Errorf("expected 0 aws hits during failure, got %d", r.PerCloudCounts["aws"])
	}
	if r.PerCloudCounts["azure"] == 0 {
		t.Errorf("expected azure to serve traffic, got 0")
	}
	if !strings.Contains(out.String(), `"result": "pass"`) {
		t.Errorf("report missing pass result: %s", out.String())
	}
}

func TestRun_FailsOnUnknownCloud(t *testing.T) {
	f := newFakeStack()
	defer f.close()
	_, err := Run(context.Background(), Options{
		AdminURL: f.admin.URL, AdminToken: testToken, ProxyURL: f.proxy.URL,
		FailCloud: "gcp", Duration: 100 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "no backends matched") {
		t.Fatalf("expected no-backend error, got %v", err)
	}
}

func TestRun_RequiresOptions(t *testing.T) {
	if _, err := Run(context.Background(), Options{}); err == nil {
		t.Fatal("expected validation error on empty Options")
	}
}
