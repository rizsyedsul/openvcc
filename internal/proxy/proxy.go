package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"sync"
	"time"

	"github.com/syedsumx/openvcc/internal/config"
	"github.com/syedsumx/openvcc/internal/cost"
	"github.com/syedsumx/openvcc/internal/metrics"
	"github.com/syedsumx/openvcc/internal/pool"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/syedsumx/openvcc/internal/proxy"

type Handler struct {
	pool    *pool.Pool
	metrics *metrics.Collectors
	log     *slog.Logger

	transport    http.RoundTripper
	transportCfg config.Proxy
	timeout      time.Duration

	mu               sync.RWMutex
	proxies          map[string]*httputil.ReverseProxy
	tlsByBackend     func(name string) *tls.Config
	accountant       *cost.Accountant
	stickyCookieName string
	stickyCookieTTL  time.Duration
}

// SetAccountant installs the cost accountant the proxy notifies after each
// proxied request. Pass nil to disable. Goroutine-safe.
func (h *Handler) SetAccountant(a *cost.Accountant) {
	h.mu.Lock()
	h.accountant = a
	h.mu.Unlock()
}

// SetStickyCookie installs a sticky-cookie spec. When name is non-empty, the
// proxy emits Set-Cookie: name=<served-cloud>; Max-Age=<ttl> on every
// response, so a fresh client gets pinned by the next request. Pass empty
// name to disable.
func (h *Handler) SetStickyCookie(name string, ttl time.Duration) {
	h.mu.Lock()
	h.stickyCookieName = name
	h.stickyCookieTTL = ttl
	h.mu.Unlock()
}

func New(p *pool.Pool, m *metrics.Collectors, log *slog.Logger, cfg config.Proxy) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{
		pool:         p,
		metrics:      m,
		log:          log.With("component", "proxy"),
		transport:    newBaseTransport(cfg),
		transportCfg: cfg,
		timeout:      cfg.RequestTimeout,
		proxies:      make(map[string]*httputil.ReverseProxy),
	}
}

func newBaseTransport(cfg config.Proxy) *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// SetTLSByBackend installs the function used to look up a backend's TLS
// configuration by name. Pass nil to disable per-backend TLS.
// All cached per-backend ReverseProxy instances are invalidated.
func (h *Handler) SetTLSByBackend(fn func(name string) *tls.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tlsByBackend = fn
	h.proxies = make(map[string]*httputil.ReverseProxy)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tracer := otel.GetTracerProvider().Tracer(tracerName)
	// Continue any incoming trace; otherwise start a fresh root.
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx, span := tracer.Start(ctx, "openvcc.proxy",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.target", r.URL.Path),
		),
	)
	defer span.End()
	r = r.WithContext(ctx)

	backend, ok := h.pool.Pick(r)
	if !ok {
		h.metrics.ProxyErrors.WithLabelValues("none", "none", "no_backend").Inc()
		span.SetStatus(codes.Error, "no backend available")
		span.SetAttributes(attribute.String("openvcc.error_kind", "no_backend"))
		http.Error(w, "no backend available", http.StatusBadGateway)
		return
	}
	span.SetAttributes(
		attribute.String("openvcc.backend.name", backend.Name),
		attribute.String("openvcc.backend.cloud", backend.Cloud),
		attribute.String("openvcc.backend.region", backend.Region),
	)
	backend.IncInflight()
	if h.metrics != nil {
		h.metrics.BackendInflight.WithLabelValues(backend.Name, backend.Cloud).Set(float64(backend.Inflight()))
	}
	defer func() {
		backend.DecInflight()
		if h.metrics != nil {
			h.metrics.BackendInflight.WithLabelValues(backend.Name, backend.Cloud).Set(float64(backend.Inflight()))
		}
	}()

	if h.timeout > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		defer cancel()
		r = r.WithContext(ctx)
	}

	sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}

	h.mu.RLock()
	stickyName := h.stickyCookieName
	stickyTTL := h.stickyCookieTTL
	h.mu.RUnlock()
	if stickyName != "" && backend.Cloud != "" {
		http.SetCookie(sw, &http.Cookie{
			Name:     stickyName,
			Value:    backend.Cloud,
			Path:     "/",
			MaxAge:   int(stickyTTL.Seconds()),
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	rp := h.proxyFor(backend)
	start := time.Now()
	rp.ServeHTTP(sw, r)
	dur := time.Since(start)

	span.SetAttributes(attribute.Int("http.status_code", sw.code))
	if sw.code >= 500 {
		span.SetStatus(codes.Error, http.StatusText(sw.code))
	}
	backend.RecordLatency(dur)
	h.mu.RLock()
	acc := h.accountant
	h.mu.RUnlock()
	if acc != nil && sw.bytes > 0 {
		acc.AddEgress(backend.Cloud, sw.bytes)
	}
	if h.metrics != nil {
		h.metrics.Requests.WithLabelValues(backend.Name, backend.Cloud, strconv.Itoa(sw.code)).Inc()
		h.metrics.RequestDuration.WithLabelValues(backend.Name, backend.Cloud).Observe(dur.Seconds())
		if sw.bytes > 0 {
			h.metrics.EgressBytes.WithLabelValues(backend.Cloud).Add(float64(sw.bytes))
		}
		if lat := backend.Latency(); lat > 0 {
			h.metrics.BackendLatencyEMA.WithLabelValues(backend.Name, backend.Cloud).Set(lat.Seconds())
		}
	}
}

func (h *Handler) proxyFor(b *pool.Backend) *httputil.ReverseProxy {
	h.mu.RLock()
	rp, ok := h.proxies[b.Name]
	h.mu.RUnlock()
	if ok {
		return rp
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if rp, ok := h.proxies[b.Name]; ok {
		return rp
	}

	target := b
	transport := h.transport
	if h.tlsByBackend != nil {
		if tc := h.tlsByBackend(b.Name); tc != nil {
			t := newBaseTransport(h.transportCfg)
			t.TLSClientConfig = tc
			transport = t
		}
	}
	rp = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target.URL)
			pr.SetXForwarded()
			pr.Out.Host = target.URL.Host
			// Inject the in-flight trace context so the backend can join it.
			otel.GetTextMapPropagator().Inject(pr.In.Context(), propagation.HeaderCarrier(pr.Out.Header))
		},
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Set("X-Backend", target.Name)
			resp.Header.Set("X-Cloud", target.Cloud)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			kind := errKind(err)
			h.log.Warn("proxy error",
				"backend", target.Name, "cloud", target.Cloud,
				"err", err, "kind", kind)
			if h.metrics != nil {
				h.metrics.ProxyErrors.WithLabelValues(target.Name, target.Cloud, kind).Inc()
			}
			http.Error(w, "upstream error", http.StatusBadGateway)
		},
	}
	h.proxies[b.Name] = rp
	return rp
}

func (h *Handler) Forget(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.proxies, name)
}

type statusWriter struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
	bytes       int64
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wroteHeader {
		s.code = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.wroteHeader = true
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

func errKind(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "transport"
	}
}
