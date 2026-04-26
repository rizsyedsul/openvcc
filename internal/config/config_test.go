package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minimalYAML = `
backends:
  - name: aws
    url: http://10.0.1.10:8000
    cloud: aws
    region: us-east-1
  - name: azure
    url: http://10.1.1.10:8000
    cloud: azure
    region: eastus
`

func TestParse_Minimal_AppliesDefaults(t *testing.T) {
	c, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Listen.Proxy != ":8080" {
		t.Errorf("Listen.Proxy=%q want :8080", c.Listen.Proxy)
	}
	if c.Strategy != "weighted_round_robin" {
		t.Errorf("Strategy=%q want weighted_round_robin", c.Strategy)
	}
	if c.Health.Interval != 5*time.Second {
		t.Errorf("Health.Interval=%v want 5s", c.Health.Interval)
	}
	if c.Health.Path != "/healthz" {
		t.Errorf("Health.Path=%q want /healthz", c.Health.Path)
	}
	if c.Backends[0].Weight != 1 || c.Backends[1].Weight != 1 {
		t.Errorf("default weights not applied: %+v", c.Backends)
	}
	if c.Log.Level != "info" || c.Log.Format != "json" {
		t.Errorf("log defaults wrong: %+v", c.Log)
	}
}

func TestParse_OverrideDefaults(t *testing.T) {
	yml := minimalYAML + `
listen:
  proxy: :7000
strategy: least_connections
health:
  interval: 1s
  timeout: 500ms
log:
  level: debug
  format: text
`
	c, err := Parse([]byte(yml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Listen.Proxy != ":7000" {
		t.Errorf("Listen.Proxy=%q", c.Listen.Proxy)
	}
	if c.Strategy != "least_connections" {
		t.Errorf("Strategy=%q", c.Strategy)
	}
	if c.Health.Interval != time.Second {
		t.Errorf("Interval=%v", c.Health.Interval)
	}
	if c.Log.Format != "text" {
		t.Errorf("log format=%q", c.Log.Format)
	}
}

func TestParse_RejectsUnknownFields(t *testing.T) {
	yml := minimalYAML + "\nunknown_field: 42\n"
	if _, err := Parse([]byte(yml)); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestValidate_Failures(t *testing.T) {
	cases := []struct {
		name string
		yml  string
		want string
	}{
		{
			name: "no backends",
			yml:  "strategy: round_robin\n",
			want: "at least one backend",
		},
		{
			name: "missing name",
			yml: `backends:
  - url: http://x:1`,
			want: "name is required",
		},
		{
			name: "duplicate name",
			yml: `backends:
  - {name: a, url: http://x:1}
  - {name: a, url: http://y:2}`,
			want: "duplicate name",
		},
		{
			name: "bad scheme",
			yml: `backends:
  - {name: a, url: ftp://x:1}`,
			want: "scheme must be http or https",
		},
		{
			name: "negative weight",
			yml: `backends:
  - {name: a, url: http://x:1, weight: -1}`,
			want: "weight must be >= 0",
		},
		{
			name: "timeout > interval",
			yml: minimalYAML + `
health:
  interval: 1s
  timeout:  2s`,
			want: "timeout must be <= health.interval",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yml))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err=%q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParse_FrontTLS(t *testing.T) {
	yml := minimalYAML + `
listen:
  proxy_tls: :8443
proxy_tls:
  cert_file: /etc/openvcc/server.crt
  key_file:  /etc/openvcc/server.key
`
	c, err := Parse([]byte(yml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Listen.ProxyTLS != ":8443" {
		t.Errorf("Listen.ProxyTLS=%q want :8443", c.Listen.ProxyTLS)
	}
	if c.ProxyTLS == nil || c.ProxyTLS.CertFile == "" {
		t.Errorf("ProxyTLS not parsed: %+v", c.ProxyTLS)
	}
}

func TestParse_FrontTLS_MissingCert_Errors(t *testing.T) {
	yml := minimalYAML + `
listen:
  proxy_tls: :8443
`
	_, err := Parse([]byte(yml))
	if err == nil || !strings.Contains(err.Error(), "proxy_tls.cert_file") {
		t.Fatalf("expected cert/key error, got: %v", err)
	}
}

func TestParse_BackendTLS(t *testing.T) {
	yml := `
backends:
  - name: aws
    url: https://10.0.1.10:8443
    cloud: aws
    tls:
      cert_file: /etc/openvcc/clients/aws.crt
      key_file:  /etc/openvcc/clients/aws.key
      ca_file:   /etc/openvcc/ca.pem
      server_name: aws-backend.internal
`
	c, err := Parse([]byte(yml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Backends[0].TLS == nil {
		t.Fatal("expected backend TLS struct")
	}
	if c.Backends[0].TLS.ServerName != "aws-backend.internal" {
		t.Errorf("ServerName=%q", c.Backends[0].TLS.ServerName)
	}
}

func TestParse_BackendTLS_HalfPair_Errors(t *testing.T) {
	yml := `
backends:
  - name: aws
    url: https://10.0.1.10:8443
    cloud: aws
    tls:
      cert_file: /etc/openvcc/clients/aws.crt
`
	_, err := Parse([]byte(yml))
	if err == nil || !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("expected cert/key pair error, got: %v", err)
	}
}

func TestParse_Cost(t *testing.T) {
	yml := minimalYAML + `
strategy: cost_bounded_least_latency
cost:
  egress_prices_per_gb:
    aws: 0.09
    azure: 0.087
  cloud_egress_budget:
    aws:
      window: 1m
      max_gb: 5
    azure:
      window: 1m
      max_gb: 5
`
	c, err := Parse([]byte(yml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Cost == nil {
		t.Fatal("Cost not parsed")
	}
	if c.Cost.EgressPricesPerGB["aws"] != 0.09 {
		t.Errorf("aws price=%v", c.Cost.EgressPricesPerGB["aws"])
	}
	if got := c.Cost.CloudEgressBudget["aws"].Window; got != time.Minute {
		t.Errorf("aws window=%v want 1m", got)
	}
}

func TestParse_Cost_Invalid(t *testing.T) {
	yml := minimalYAML + `
cost:
  cloud_egress_budget:
    aws:
      window: 0s
      max_gb: 0
`
	_, err := Parse([]byte(yml))
	if err == nil ||
		!strings.Contains(err.Error(), "window must be > 0") ||
		!strings.Contains(err.Error(), "max_gb must be > 0") {
		t.Fatalf("expected window+max_gb errors, got: %v", err)
	}
}

func TestParse_Sticky(t *testing.T) {
	yml := minimalYAML + `
strategy: sticky_by_cloud
sticky:
  hash: cookie:openvcc_cloud
  fallback_strategy: cost_bounded_least_latency
  cookie_ttl: 6h
`
	c, err := Parse([]byte(yml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Sticky == nil {
		t.Fatal("Sticky not parsed")
	}
	if c.Sticky.Hash != "cookie:openvcc_cloud" {
		t.Errorf("Hash=%q", c.Sticky.Hash)
	}
	if c.Sticky.CookieTTL != 6*time.Hour {
		t.Errorf("CookieTTL=%v", c.Sticky.CookieTTL)
	}
}

func TestParse_Sticky_Defaults(t *testing.T) {
	yml := minimalYAML + `
sticky:
  hash: remote_addr
`
	c, err := Parse([]byte(yml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Sticky.FallbackStrategy != "weighted_round_robin" {
		t.Errorf("default fallback=%q", c.Sticky.FallbackStrategy)
	}
	if c.Sticky.CookieTTL != 24*time.Hour {
		t.Errorf("default cookie_ttl=%v", c.Sticky.CookieTTL)
	}
}

func TestParse_Sticky_Invalid(t *testing.T) {
	cases := map[string]string{
		"missing hash":       `sticky: {}`,
		"unknown hash":       `sticky: {hash: "magic"}`,
		"recursive fallback": `sticky: {hash: "remote_addr", fallback_strategy: "sticky_by_cloud"}`,
	}
	for name, sticky := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(minimalYAML + "\n" + sticky))
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParse_KindRouting_Defaults(t *testing.T) {
	yml := `
backends:
  - {name: a, url: http://x:1, cloud: aws, kind: writeable}
  - {name: b, url: http://y:1, cloud: azure, kind: read_only}
kind_routing: {}
`
	c, err := Parse([]byte(yml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.KindRouting == nil {
		t.Fatal("KindRouting not parsed")
	}
	if len(c.KindRouting.WriteMethods) != 4 {
		t.Errorf("default write_methods=%v", c.KindRouting.WriteMethods)
	}
	if got := c.KindRouting.WriteKinds; len(got) != 1 || got[0] != "writeable" {
		t.Errorf("default write_kinds=%v", got)
	}
	if c.Backends[0].Kind != "writeable" {
		t.Errorf("Backend.Kind not parsed: %q", c.Backends[0].Kind)
	}
}

func TestParse_ACME(t *testing.T) {
	yml := minimalYAML + `
listen:
  proxy_tls: :8443
acme:
  domains: [api.example.com]
  email: ops@example.com
  staging: true
`
	c, err := Parse([]byte(yml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.ACME == nil {
		t.Fatal("ACME not parsed")
	}
	if c.ACME.CacheDir != "/var/lib/openvcc/acme" {
		t.Errorf("default CacheDir=%q", c.ACME.CacheDir)
	}
	if !c.ACME.Staging {
		t.Errorf("Staging=%v", c.ACME.Staging)
	}
}

func TestParse_ACME_RequiresProxyTLS(t *testing.T) {
	yml := minimalYAML + `
acme:
  domains: [api.example.com]
`
	_, err := Parse([]byte(yml))
	if err == nil || !strings.Contains(err.Error(), "listen.proxy_tls") {
		t.Fatalf("expected proxy_tls requirement error, got %v", err)
	}
}

func TestParse_ACME_RequiresDomains(t *testing.T) {
	yml := minimalYAML + `
listen:
  proxy_tls: :8443
acme: {}
`
	_, err := Parse([]byte(yml))
	if err == nil || !strings.Contains(err.Error(), "domains") {
		t.Fatalf("expected domains requirement error, got %v", err)
	}
}

func TestParse_KindRouting_NoMatchingBackend(t *testing.T) {
	yml := `
backends:
  - {name: a, url: http://x:1, cloud: aws, kind: read_only}
kind_routing:
  write_kinds: [writeable]
`
	_, err := Parse([]byte(yml))
	if err == nil || !strings.Contains(err.Error(), "no backend has a matching kind") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openvcc.yaml")
	if err := os.WriteFile(path, []byte(minimalYAML), 0o644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(c.Backends))
	}
}
