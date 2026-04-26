package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/syedsumx/openvcc/internal/config"
	"github.com/syedsumx/openvcc/internal/metrics"
	"github.com/syedsumx/openvcc/internal/pool"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestProxy_EmitsSpansWithBackendAttributes(t *testing.T) {
	// Install in-memory recorder + W3C propagator.
	rec := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	// Backend echoes the W3C traceparent header it received so we can verify
	// propagation worked.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("traceparent")))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	b := pool.NewBackend("a", u, "aws", "us-east-1", 1)
	p := pool.New([]*pool.Backend{b}, &pool.RoundRobin{})
	m := metrics.New(prometheus.NewRegistry())
	h := New(p, m, slog.New(slog.NewTextHandler(io.Discard, nil)),
		config.Proxy{RequestTimeout: time.Second, MaxIdleConnsPerHost: 4})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/foo", nil))

	body := w.Body.String()
	if body == "" || len(body) < 20 {
		t.Errorf("backend did not see a traceparent header: %q", body)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name() != "openvcc.proxy" {
		t.Errorf("span name=%q", s.Name())
	}
	attrs := map[string]string{}
	for _, a := range s.Attributes() {
		attrs[string(a.Key)] = a.Value.Emit()
	}
	if attrs["openvcc.backend.name"] != "a" {
		t.Errorf("backend.name=%q", attrs["openvcc.backend.name"])
	}
	if attrs["openvcc.backend.cloud"] != "aws" {
		t.Errorf("backend.cloud=%q", attrs["openvcc.backend.cloud"])
	}
	if attrs["http.method"] != "GET" {
		t.Errorf("http.method=%q", attrs["http.method"])
	}
	if attrs["http.target"] != "/foo" {
		t.Errorf("http.target=%q", attrs["http.target"])
	}
	if attrs["http.status_code"] != "200" {
		t.Errorf("http.status_code=%q", attrs["http.status_code"])
	}
}
