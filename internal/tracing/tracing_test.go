package tracing

import (
	"context"
	"testing"

	"github.com/syedsumx/openvcc/internal/config"
)

func TestInit_NilCfg_NoOp(t *testing.T) {
	shutdown, err := Init(context.Background(), nil, "test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestParseEndpoint(t *testing.T) {
	cases := []struct {
		in       string
		host     string
		insecure bool
	}{
		{"localhost:4318", "localhost:4318", false},
		{"http://localhost:4318", "localhost:4318", true},
		{"https://otel.example.com:4318", "otel.example.com:4318", false},
	}
	for _, c := range cases {
		host, insec, err := parseEndpoint(c.in)
		if err != nil {
			t.Errorf("parseEndpoint(%q): %v", c.in, err)
			continue
		}
		if host != c.host || insec != c.insecure {
			t.Errorf("parseEndpoint(%q)=%q,%v want %q,%v", c.in, host, insec, c.host, c.insecure)
		}
	}
}

func TestInit_BuildsProvider(t *testing.T) {
	// We point at a deliberately unreachable endpoint: Init should still
	// succeed (the exporter is lazy). Shutdown will time out trying to
	// flush, but we use a short context to keep the test fast.
	shutdown, err := Init(context.Background(), &config.Tracing{
		Endpoint:     "127.0.0.1:1",
		ServiceName:  "openvcc",
		SamplerRatio: 1.0,
		Insecure:     true,
	}, "test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = shutdown(ctx) // Returns ctx.Canceled; that's fine
}
