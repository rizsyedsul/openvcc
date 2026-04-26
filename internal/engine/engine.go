package engine

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/syedsumx/openvcc/internal/admin"
	"github.com/syedsumx/openvcc/internal/config"
	"github.com/syedsumx/openvcc/internal/cost"
	"github.com/syedsumx/openvcc/internal/health"
	"github.com/syedsumx/openvcc/internal/metrics"
	"github.com/syedsumx/openvcc/internal/pool"
	"github.com/syedsumx/openvcc/internal/proxy"
	"github.com/syedsumx/openvcc/internal/tracing"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const shutdownGrace = 30 * time.Second

type Engine struct {
	cfg     *config.Config
	cfgPath string
	pool    *pool.Pool
	proxy   *proxy.Handler
	admin   *admin.Server
	health  *health.Checker
	metrics *metrics.Collectors
	log     *slog.Logger
	reg     *prometheus.Registry

	mu              sync.Mutex
	adminToken      string
	accountant      *cost.Accountant
	tracingShutdown tracing.Shutdown
	proxySrv        *http.Server
	metricsSrv      *http.Server
	adminSrv        *http.Server
	servers         []*http.Server
	stopHealth      context.CancelFunc
}

func New(cfg *config.Config, cfgPath string, log *slog.Logger) (*Engine, error) {
	if log == nil {
		log = slog.Default()
	}
	traceShutdown, err := tracing.Init(context.Background(), cfg.Tracing, "openvcc")
	if err != nil {
		return nil, fmt.Errorf("tracing: %w", err)
	}
	backends, err := buildBackends(cfg.Backends)
	if err != nil {
		_ = traceShutdown(context.Background())
		return nil, err
	}
	accountant := buildAccountant(cfg.Cost)
	strategy, stickyCookie, err := buildStrategy(cfg, accountant)
	if err != nil {
		return nil, err
	}
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	p := pool.New(backends, strategy)

	ph := proxy.New(p, m, log, cfg.Proxy)
	if err := applyBackendTLS(ph, cfg.Backends); err != nil {
		return nil, err
	}
	ph.SetAccountant(accountant)
	if cfg.Sticky != nil {
		ph.SetStickyCookie(stickyCookie, cfg.Sticky.CookieTTL)
	}
	hc := health.New(p, cfg.Health, m, log)

	token := ""
	if cfg.Admin.BearerTokenEnv != "" {
		token = os.Getenv(cfg.Admin.BearerTokenEnv)
	}
	e := &Engine{
		cfg:             cfg,
		cfgPath:         cfgPath,
		pool:            p,
		proxy:           ph,
		health:          hc,
		metrics:         m,
		log:             log.With("component", "engine"),
		reg:             reg,
		adminToken:      token,
		accountant:      accountant,
		tracingShutdown: traceShutdown,
	}
	e.admin = admin.New(p, ph, log, admin.Options{Token: token, Reload: e.Reload})
	return e, nil
}

func (e *Engine) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	hctx, hcancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.stopHealth = hcancel
	e.mu.Unlock()
	go func() { _ = e.health.Run(hctx) }()

	errCh := make(chan error, 4)
	e.mu.Lock()

	var acmeManager *autocert.Manager
	if e.cfg.ACME != nil {
		acmeManager = newAutocertManager(e.cfg.ACME)
	}

	proxyHandler := http.Handler(e.proxy)
	if acmeManager != nil {
		// HTTP-01 challenges arrive on the plain HTTP listener; the manager's
		// HTTPHandler answers them and falls through to the proxy for everything else.
		proxyHandler = acmeManager.HTTPHandler(e.proxy)
	}
	e.proxySrv = &http.Server{Addr: e.cfg.Listen.Proxy, Handler: proxyHandler, ReadHeaderTimeout: 10 * time.Second}
	e.metricsSrv = &http.Server{Addr: e.cfg.Listen.Metrics, Handler: metricsHandler(e.reg), ReadHeaderTimeout: 10 * time.Second}
	e.servers = []*http.Server{e.proxySrv, e.metricsSrv}
	if e.cfg.Listen.Admin != "" {
		e.adminSrv = &http.Server{Addr: e.cfg.Listen.Admin, Handler: e.admin.Handler(), ReadHeaderTimeout: 10 * time.Second}
		e.servers = append(e.servers, e.adminSrv)
	}

	var proxyTLSSrv *http.Server
	if e.cfg.Listen.ProxyTLS != "" {
		var tc *tls.Config
		if acmeManager != nil {
			tc = acmeManager.TLSConfig()
		} else {
			built, err := buildFrontTLS(e.cfg.ProxyTLS)
			if err != nil {
				e.mu.Unlock()
				return fmt.Errorf("front tls: %w", err)
			}
			tc = built
		}
		proxyTLSSrv = &http.Server{
			Addr:              e.cfg.Listen.ProxyTLS,
			Handler:           e.proxy,
			TLSConfig:         tc,
			ReadHeaderTimeout: 10 * time.Second,
		}
		e.servers = append(e.servers, proxyTLSSrv)
	}
	e.mu.Unlock()

	for _, s := range e.servers {
		s := s
		isTLS := s == proxyTLSSrv
		go func() {
			e.log.Info("listening", "addr", s.Addr, "tls", isTLS)
			var err error
			if isTLS {
				err = s.ListenAndServeTLS("", "")
			} else {
				err = s.ListenAndServe()
			}
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("listener %s: %w", s.Addr, err)
			}
		}()
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errCh:
	}
	e.shutdown()
	return runErr
}

func (e *Engine) Reload(ctx context.Context) error {
	if e.cfgPath == "" {
		return errors.New("no config path: reload only works when started with a file")
	}
	cfg, err := config.Load(e.cfgPath)
	if err != nil {
		return fmt.Errorf("reload: %w", err)
	}
	backends, err := buildBackends(cfg.Backends)
	if err != nil {
		return fmt.Errorf("reload: %w", err)
	}
	accountant := buildAccountant(cfg.Cost)
	strategy, stickyCookie, err := buildStrategy(cfg, accountant)
	if err != nil {
		return fmt.Errorf("reload: %w", err)
	}
	e.mu.Lock()
	e.accountant = accountant
	e.mu.Unlock()
	e.proxy.SetAccountant(accountant)
	if cfg.Sticky != nil {
		e.proxy.SetStickyCookie(stickyCookie, cfg.Sticky.CookieTTL)
	} else {
		e.proxy.SetStickyCookie("", 0)
	}
	old := e.pool.Backends()
	e.pool.Replace(backends, strategy)
	keep := make(map[string]struct{}, len(backends))
	for _, b := range backends {
		keep[b.Name] = struct{}{}
	}
	for _, b := range old {
		if _, ok := keep[b.Name]; !ok {
			e.proxy.Forget(b.Name)
		}
	}
	if err := applyBackendTLS(e.proxy, cfg.Backends); err != nil {
		return fmt.Errorf("reload tls: %w", err)
	}
	e.log.Info("config reloaded", "backends", len(backends), "strategy", cfg.Strategy)
	return nil
}

func (e *Engine) Pool() *pool.Pool        { return e.pool }
func (e *Engine) Registry() *prometheus.Registry { return e.reg }

func (e *Engine) shutdown() {
	e.mu.Lock()
	servers := e.servers
	stopHealth := e.stopHealth
	traceShutdown := e.tracingShutdown
	e.mu.Unlock()
	if stopHealth != nil {
		stopHealth()
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	var wg sync.WaitGroup
	for _, s := range servers {
		s := s
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Shutdown(ctx)
		}()
	}
	wg.Wait()
	if traceShutdown != nil {
		_ = traceShutdown(ctx)
	}
}

func metricsHandler(reg *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// buildAccountant constructs a cost.Accountant from cfg.Cost. Returns nil if
// no Cost block is configured (proxy and strategy both treat nil as "off").
func buildAccountant(c *config.Cost) *cost.Accountant {
	if c == nil {
		return nil
	}
	budgets := make(map[string]cost.Budget, len(c.CloudEgressBudget))
	for cloud, b := range c.CloudEgressBudget {
		budgets[cloud] = cost.Budget{Window: b.Window, MaxGB: b.MaxGB}
	}
	return cost.New(budgets, c.EgressPricesPerGB)
}

// buildStrategy returns the strategy named by `name`, then wraps it (in
// composable order) with sticky_by_cloud and kind_aware when the matching
// config blocks are set. The outer-to-inner order is:
//
//	KindAware ⊃ StickyByCloud ⊃ Base
//
// so kind filtering happens first (writes only see writeable backends),
// then sticky pins within the kind-allowed set, then the base picks.
func buildStrategy(cfg *config.Config, a *cost.Accountant) (pool.Strategy, string, error) {
	baseName := cfg.Strategy
	if cfg.Sticky != nil {
		baseName = cfg.Sticky.FallbackStrategy
	}
	base, err := buildBaseStrategy(baseName, a)
	if err != nil {
		return nil, "", err
	}

	current := base
	cookieName := ""
	if cfg.Sticky != nil {
		hash, cn, err := pool.HashFromRequest(cfg.Sticky.Hash)
		if err != nil {
			return nil, "", fmt.Errorf("sticky.hash: %w", err)
		}
		cookieName = cn
		current = pool.NewStickyByCloud(hash, cn, current)
	}
	if cfg.KindRouting != nil {
		current = pool.NewKindAware(
			cfg.KindRouting.WriteMethods,
			cfg.KindRouting.WriteKinds,
			cfg.KindRouting.ReadKinds,
			current,
		)
	}
	return current, cookieName, nil
}

func buildBaseStrategy(name string, a *cost.Accountant) (pool.Strategy, error) {
	s, err := pool.ParseStrategy(name)
	if err != nil {
		return nil, err
	}
	switch s.Name() {
	case pool.StrategyCostBoundedLeastLatency:
		return pool.NewCostBoundedLeastLatency(a), nil
	case pool.StrategyStickyByCloud:
		return nil, errors.New("sticky_by_cloud cannot be used as a fallback strategy")
	}
	return s, nil
}

// applyBackendTLS loads each backend's TLS spec, builds a *tls.Config, and
// installs a lookup function on the proxy. Returns an error if any spec
// fails to load (so a misconfigured cert path fails fast at startup).
func applyBackendTLS(ph *proxy.Handler, bs []config.Backend) error {
	configs := make(map[string]*tls.Config)
	for _, b := range bs {
		if b.TLS == nil {
			continue
		}
		tc, err := proxy.BuildBackendTLSConfig(b.TLS)
		if err != nil {
			return fmt.Errorf("backend %q tls: %w", b.Name, err)
		}
		configs[b.Name] = tc
	}
	ph.SetTLSByBackend(func(name string) *tls.Config { return configs[name] })
	return nil
}

func newAutocertManager(cfg *config.ACME) *autocert.Manager {
	dir := cfg.DirectoryURL
	if dir == "" && cfg.Staging {
		dir = "https://acme-staging-v02.api.letsencrypt.org/directory"
	}
	m := &autocert.Manager{
		Cache:      autocert.DirCache(cfg.CacheDir),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.Domains...),
		Email:      cfg.Email,
	}
	if dir != "" {
		m.Client = &acme.Client{DirectoryURL: dir}
	}
	return m
}

func buildFrontTLS(cfg *config.FrontTLS) (*tls.Config, error) {
	if cfg == nil || cfg.CertFile == "" || cfg.KeyFile == "" {
		return nil, errors.New("proxy_tls cert_file/key_file are required")
	}
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load front cert: %w", err)
	}
	tc := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if cfg.ClientCAFile != "" {
		pem, err := os.ReadFile(cfg.ClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("read client_ca_file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("client_ca_file contained no parseable certificates")
		}
		tc.ClientCAs = pool
		tc.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return tc, nil
}

func buildBackends(in []config.Backend) ([]*pool.Backend, error) {
	out := make([]*pool.Backend, 0, len(in))
	for _, b := range in {
		u, err := url.Parse(b.URL)
		if err != nil {
			return nil, fmt.Errorf("backend %q: %w", b.Name, err)
		}
		out = append(out, pool.NewBackend(b.Name, u, b.Cloud, b.Region, b.Weight).WithKind(b.Kind))
	}
	return out, nil
}
