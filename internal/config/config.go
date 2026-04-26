package config

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen       Listen        `yaml:"listen"`
	Admin        Admin         `yaml:"admin"`
	Backends     []Backend     `yaml:"backends"`
	Health       Health        `yaml:"health"`
	Strategy     string        `yaml:"strategy"`
	Sticky       *Sticky       `yaml:"sticky,omitempty"`
	KindRouting  *KindRouting  `yaml:"kind_routing,omitempty"`
	Proxy        Proxy         `yaml:"proxy"`
	ProxyTLS     *FrontTLS     `yaml:"proxy_tls,omitempty"`
	Cost         *Cost         `yaml:"cost,omitempty"`
	Tracing      *Tracing      `yaml:"tracing,omitempty"`
	ACME         *ACME         `yaml:"acme,omitempty"`
	Log          Log           `yaml:"log"`
}

// ACME enables automatic certificate provisioning via Let's Encrypt (or any
// ACME-compatible CA). When set, the front TLS listener (listen.proxy_tls)
// uses certificates obtained on demand for the listed Domains, overriding
// proxy_tls.cert_file/key_file. HTTP-01 challenges are served from the
// listen.proxy listener.
type ACME struct {
	Domains      []string `yaml:"domains"`
	Email        string   `yaml:"email"`
	CacheDir     string   `yaml:"cache_dir"`
	DirectoryURL string   `yaml:"directory_url"`
	Staging      bool     `yaml:"staging"`
}

// Tracing configures OpenTelemetry tracing. Endpoint is the OTLP/HTTP collector
// (e.g. "http://localhost:4318"); empty = no tracing. ServiceName labels every
// emitted span; defaults to "openvcc". SamplerRatio is in [0.0, 1.0].
type Tracing struct {
	Endpoint     string  `yaml:"endpoint"`
	ServiceName  string  `yaml:"service_name"`
	SamplerRatio float64 `yaml:"sampler_ratio"`
	Insecure     bool    `yaml:"insecure"`
	Headers      map[string]string `yaml:"headers"`
}

// KindRouting filters healthy backends by kind based on the request method.
// If unset, kind is just a label (surfaced in metrics + admin) and routing is
// unaffected.
type KindRouting struct {
	WriteMethods []string `yaml:"write_methods"` // default: POST, PUT, DELETE, PATCH
	WriteKinds   []string `yaml:"write_kinds"`   // backends with these kinds receive writes; default ["writeable"]
	ReadKinds    []string `yaml:"read_kinds"`    // backends with these kinds receive reads; empty = any
}

// Sticky configures session affinity on top of the chosen base strategy.
// Hash names what to identify the client by:
//   "cookie:NAME"  — read cookie NAME; if present, route to its cloud.
//                    On every response, emit Set-Cookie: NAME=<served-cloud>.
//   "header:NAME"  — hash request header NAME (e.g. X-Forwarded-For) to
//                    one of the present clouds (deterministic per client).
//   "remote_addr"  — hash r.RemoteAddr.
type Sticky struct {
	Hash             string        `yaml:"hash"`
	FallbackStrategy string        `yaml:"fallback_strategy"`
	CookieTTL        time.Duration `yaml:"cookie_ttl"`
}

type Cost struct {
	EgressPricesPerGB map[string]float64 `yaml:"egress_prices_per_gb"`
	CloudEgressBudget map[string]Budget  `yaml:"cloud_egress_budget"`
}

type Budget struct {
	Window time.Duration `yaml:"window"`
	MaxGB  float64       `yaml:"max_gb"`
}

type Listen struct {
	Proxy    string `yaml:"proxy"`
	ProxyTLS string `yaml:"proxy_tls"`
	Metrics  string `yaml:"metrics"`
	Admin    string `yaml:"admin"`
}

type FrontTLS struct {
	CertFile     string `yaml:"cert_file"`
	KeyFile      string `yaml:"key_file"`
	ClientCAFile string `yaml:"client_ca_file"`
}

type Admin struct {
	BearerTokenEnv string `yaml:"bearer_token_env"`
}

type Backend struct {
	Name   string      `yaml:"name"`
	URL    string      `yaml:"url"`
	Cloud  string      `yaml:"cloud"`
	Region string      `yaml:"region"`
	Weight int         `yaml:"weight"`
	Kind   string      `yaml:"kind,omitempty"` // writeable | read_only | cache | <user-defined>
	TLS    *BackendTLS `yaml:"tls,omitempty"`
}

type BackendTLS struct {
	CertFile           string `yaml:"cert_file"`
	KeyFile            string `yaml:"key_file"`
	CAFile             string `yaml:"ca_file"`
	ServerName         string `yaml:"server_name"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type Health struct {
	Interval            time.Duration `yaml:"interval"`
	Timeout             time.Duration `yaml:"timeout"`
	Path                string        `yaml:"path"`
	UnhealthyThreshold  int           `yaml:"unhealthy_threshold"`
	HealthyThreshold    int           `yaml:"healthy_threshold"`
	ExpectedStatusFloor int           `yaml:"expected_status_floor"`
	ExpectedStatusCeil  int           `yaml:"expected_status_ceil"`
}

type Proxy struct {
	RequestTimeout      time.Duration `yaml:"request_timeout"`
	MaxIdleConnsPerHost int           `yaml:"max_idle_conns_per_host"`
	PassHeaders         []string      `yaml:"pass_headers"`
}

type Log struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Listen.Proxy == "" {
		c.Listen.Proxy = ":8080"
	}
	if c.Listen.Metrics == "" {
		c.Listen.Metrics = ":9090"
	}
	if c.Strategy == "" {
		c.Strategy = "weighted_round_robin"
	}
	if c.Health.Interval == 0 {
		c.Health.Interval = 5 * time.Second
	}
	if c.Health.Timeout == 0 {
		c.Health.Timeout = 2 * time.Second
	}
	if c.Health.Path == "" {
		c.Health.Path = "/healthz"
	}
	if c.Health.UnhealthyThreshold == 0 {
		c.Health.UnhealthyThreshold = 2
	}
	if c.Health.HealthyThreshold == 0 {
		c.Health.HealthyThreshold = 2
	}
	if c.Health.ExpectedStatusFloor == 0 {
		c.Health.ExpectedStatusFloor = 200
	}
	if c.Health.ExpectedStatusCeil == 0 {
		c.Health.ExpectedStatusCeil = 399
	}
	if c.Proxy.RequestTimeout == 0 {
		c.Proxy.RequestTimeout = 30 * time.Second
	}
	if c.Proxy.MaxIdleConnsPerHost == 0 {
		c.Proxy.MaxIdleConnsPerHost = 64
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "json"
	}
	for i := range c.Backends {
		if c.Backends[i].Weight == 0 {
			c.Backends[i].Weight = 1
		}
	}
	if c.Sticky != nil {
		if c.Sticky.FallbackStrategy == "" {
			c.Sticky.FallbackStrategy = "weighted_round_robin"
		}
		if c.Sticky.CookieTTL == 0 {
			c.Sticky.CookieTTL = 24 * time.Hour
		}
	}
	if c.KindRouting != nil {
		if len(c.KindRouting.WriteMethods) == 0 {
			c.KindRouting.WriteMethods = []string{"POST", "PUT", "DELETE", "PATCH"}
		}
		if len(c.KindRouting.WriteKinds) == 0 {
			c.KindRouting.WriteKinds = []string{"writeable"}
		}
	}
	if c.Tracing != nil {
		if c.Tracing.ServiceName == "" {
			c.Tracing.ServiceName = "openvcc"
		}
		if c.Tracing.SamplerRatio == 0 {
			c.Tracing.SamplerRatio = 1.0
		}
	}
	if c.ACME != nil {
		if c.ACME.CacheDir == "" {
			c.ACME.CacheDir = "/var/lib/openvcc/acme"
		}
	}
}

func (c *Config) Validate() error {
	var errs []error
	if len(c.Backends) == 0 {
		errs = append(errs, errors.New("backends: must declare at least one backend"))
	}
	seen := make(map[string]bool, len(c.Backends))
	for i, b := range c.Backends {
		if b.Name == "" {
			errs = append(errs, fmt.Errorf("backends[%d]: name is required", i))
		} else if seen[b.Name] {
			errs = append(errs, fmt.Errorf("backends[%d]: duplicate name %q", i, b.Name))
		} else {
			seen[b.Name] = true
		}
		if b.URL == "" {
			errs = append(errs, fmt.Errorf("backends[%d] (%s): url is required", i, b.Name))
		} else {
			u, err := url.Parse(b.URL)
			if err != nil {
				errs = append(errs, fmt.Errorf("backends[%d] (%s): invalid url: %w", i, b.Name, err))
			} else if u.Scheme != "http" && u.Scheme != "https" {
				errs = append(errs, fmt.Errorf("backends[%d] (%s): url scheme must be http or https", i, b.Name))
			} else if u.Host == "" {
				errs = append(errs, fmt.Errorf("backends[%d] (%s): url must include a host", i, b.Name))
			}
		}
		if b.Weight < 0 {
			errs = append(errs, fmt.Errorf("backends[%d] (%s): weight must be >= 0", i, b.Name))
		}
	}
	if c.Health.Interval <= 0 {
		errs = append(errs, errors.New("health.interval must be > 0"))
	}
	if c.Health.Timeout <= 0 {
		errs = append(errs, errors.New("health.timeout must be > 0"))
	}
	if c.Health.Timeout > c.Health.Interval {
		errs = append(errs, errors.New("health.timeout must be <= health.interval"))
	}
	if c.Health.UnhealthyThreshold < 1 {
		errs = append(errs, errors.New("health.unhealthy_threshold must be >= 1"))
	}
	if c.Health.HealthyThreshold < 1 {
		errs = append(errs, errors.New("health.healthy_threshold must be >= 1"))
	}
	if c.Listen.Proxy == "" {
		errs = append(errs, errors.New("listen.proxy is required"))
	}
	if c.Listen.ProxyTLS != "" && c.ACME == nil {
		if c.ProxyTLS == nil || c.ProxyTLS.CertFile == "" || c.ProxyTLS.KeyFile == "" {
			errs = append(errs, errors.New("listen.proxy_tls is set but proxy_tls.cert_file/key_file are missing (or configure acme:)"))
		}
	}
	for i, b := range c.Backends {
		if b.TLS == nil {
			continue
		}
		if (b.TLS.CertFile == "") != (b.TLS.KeyFile == "") {
			errs = append(errs, fmt.Errorf("backends[%d] (%s): tls.cert_file and tls.key_file must be set together", i, b.Name))
		}
	}
	if c.Cost != nil {
		for cloud, b := range c.Cost.CloudEgressBudget {
			if b.Window <= 0 {
				errs = append(errs, fmt.Errorf("cost.cloud_egress_budget[%q].window must be > 0", cloud))
			}
			if b.MaxGB <= 0 {
				errs = append(errs, fmt.Errorf("cost.cloud_egress_budget[%q].max_gb must be > 0", cloud))
			}
		}
	}
	if c.Sticky != nil {
		if c.Sticky.Hash == "" {
			errs = append(errs, errors.New("sticky.hash is required (cookie:NAME, header:NAME, or remote_addr)"))
		} else if !validStickyHash(c.Sticky.Hash) {
			errs = append(errs, fmt.Errorf("sticky.hash %q must be 'remote_addr' or start with 'cookie:' or 'header:'", c.Sticky.Hash))
		}
		if c.Sticky.FallbackStrategy == "sticky_by_cloud" {
			errs = append(errs, errors.New("sticky.fallback_strategy must not itself be sticky_by_cloud"))
		}
	}
	if c.Tracing != nil {
		if c.Tracing.Endpoint == "" {
			errs = append(errs, errors.New("tracing.endpoint is required when the tracing block is present"))
		}
		if c.Tracing.SamplerRatio < 0 || c.Tracing.SamplerRatio > 1 {
			errs = append(errs, fmt.Errorf("tracing.sampler_ratio must be in [0,1], got %v", c.Tracing.SamplerRatio))
		}
	}
	if c.ACME != nil {
		if len(c.ACME.Domains) == 0 {
			errs = append(errs, errors.New("acme.domains must list at least one domain"))
		}
		if c.Listen.ProxyTLS == "" {
			errs = append(errs, errors.New("acme requires listen.proxy_tls to be set (the HTTPS port)"))
		}
		if c.Listen.Proxy == "" {
			errs = append(errs, errors.New("acme requires listen.proxy to also be set (HTTP-01 challenge handler)"))
		}
	}
	if c.KindRouting != nil && len(c.KindRouting.WriteKinds) > 0 {
		// At least one declared backend should match write_kinds; otherwise
		// every write would 502.
		writeable := make(map[string]bool, len(c.KindRouting.WriteKinds))
		for _, k := range c.KindRouting.WriteKinds {
			writeable[k] = true
		}
		hasWriteable := false
		for _, b := range c.Backends {
			if writeable[b.Kind] {
				hasWriteable = true
				break
			}
		}
		if !hasWriteable {
			errs = append(errs, fmt.Errorf("kind_routing.write_kinds=%v but no backend has a matching kind",
				c.KindRouting.WriteKinds))
		}
	}
	return errors.Join(errs...)
}

func validStickyHash(s string) bool {
	switch {
	case s == "remote_addr":
		return true
	case len(s) > len("cookie:") && s[:len("cookie:")] == "cookie:":
		return true
	case len(s) > len("header:") && s[:len("header:")] == "header:":
		return true
	}
	return false
}
