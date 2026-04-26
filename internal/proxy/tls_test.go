package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syedsumx/openvcc/internal/config"
	"github.com/syedsumx/openvcc/internal/metrics"
	"github.com/syedsumx/openvcc/internal/pool"

	"github.com/prometheus/client_golang/prometheus"
)

// writePEM writes the test server's auto-generated cert + key to disk so
// BuildBackendTLSConfig can load them as files.
func writePEM(t *testing.T, dir, name string, cert tls.Certificate) (string, string) {
	t.Helper()
	certPath := filepath.Join(dir, name+".crt")
	keyPath := filepath.Join(dir, name+".key")
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	certPEM := encodePEM("CERTIFICATE", x509Cert.Raw)
	keyPEM, err := encodeKey(cert.PrivateKey)
	if err != nil {
		t.Fatalf("encode key: %v", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath
}

func TestBuildBackendTLSConfig_NilSpec(t *testing.T) {
	tc, err := BuildBackendTLSConfig(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if tc != nil {
		t.Fatalf("expected nil, got %+v", tc)
	}
}

func TestBuildBackendTLSConfig_LoadsCAFile(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	certPEM := encodePEM("CERTIFICATE", srv.Certificate().Raw)
	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}
	tc, err := BuildBackendTLSConfig(&config.BackendTLS{CAFile: caPath, ServerName: "example.com"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if tc.RootCAs == nil {
		t.Fatal("expected RootCAs to be populated")
	}
	if tc.ServerName != "example.com" {
		t.Errorf("ServerName=%q", tc.ServerName)
	}
}

func TestProxy_HTTPSBackend_WithCAPinning(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("via tls"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, encodePEM("CERTIFICATE", srv.Certificate().Raw), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	u, _ := url.Parse(srv.URL)
	b := pool.NewBackend("a", u, "aws", "r", 1)
	p := pool.New([]*pool.Backend{b}, &pool.RoundRobin{})
	m := metrics.New(prometheus.NewRegistry())
	h := New(p, m, slog.New(slog.NewTextHandler(io.Discard, nil)), config.Proxy{
		RequestTimeout:      2 * time.Second,
		MaxIdleConnsPerHost: 4,
	})

	tc, err := BuildBackendTLSConfig(&config.BackendTLS{
		CAFile:     caPath,
		ServerName: u.Hostname(),
	})
	if err != nil {
		t.Fatalf("BuildBackendTLSConfig: %v", err)
	}
	h.SetTLSByBackend(func(name string) *tls.Config {
		if name == "a" {
			return tc
		}
		return nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "via tls") {
		t.Errorf("body=%q", w.Body.String())
	}
}

func TestSetTLSByBackend_InvalidatesCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	b := pool.NewBackend("a", u, "aws", "r", 1)
	p := pool.New([]*pool.Backend{b}, &pool.RoundRobin{})
	m := metrics.New(prometheus.NewRegistry())
	h := New(p, m, slog.New(slog.NewTextHandler(io.Discard, nil)), config.Proxy{})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if _, ok := h.proxies["a"]; !ok {
		t.Fatal("proxy should be cached after first request")
	}
	h.SetTLSByBackend(func(name string) *tls.Config { return nil })
	if _, ok := h.proxies["a"]; ok {
		t.Fatal("SetTLSByBackend should invalidate cache")
	}
}
