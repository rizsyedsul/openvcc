package engine

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syedsumx/openvcc/internal/config"
)

// genSelfSigned writes a fresh self-signed cert + key to dir and returns the paths.
func genSelfSigned(t *testing.T, dir, host string) (certPath, keyPath string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{host},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPath = filepath.Join(dir, "front.crt")
	keyPath = filepath.Join(dir, "front.key")
	keyDER, _ := x509.MarshalECPrivateKey(priv)
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath
}

func TestEngine_FrontTLSListener_Serves(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("via-tls"))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	certPath, keyPath := genSelfSigned(t, dir, "localhost")

	pp := freePort(t)
	mp := freePort(t)
	tlsPort := freePort(t)
	cfg := &config.Config{
		Listen: config.Listen{
			Proxy:    fmt.Sprintf("127.0.0.1:%d", pp),
			ProxyTLS: fmt.Sprintf("127.0.0.1:%d", tlsPort),
			Metrics:  fmt.Sprintf("127.0.0.1:%d", mp),
		},
		ProxyTLS: &config.FrontTLS{CertFile: certPath, KeyFile: keyPath},
		Backends: []config.Backend{{Name: "a", URL: upstream.URL, Cloud: "aws", Region: "r", Weight: 1}},
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

	tlsAddr := fmt.Sprintf("127.0.0.1:%d", tlsPort)
	waitListening(t, tlsAddr)

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} //nolint:gosec
	resp, err := client.Get("https://" + tlsAddr + "/")
	if err != nil {
		t.Fatalf("https GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "via-tls" {
		t.Errorf("status=%d body=%q", resp.StatusCode, string(body))
	}
}
