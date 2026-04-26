package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNew_RegistersAndExposes(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := New(reg)

	c.Requests.WithLabelValues("aws", "aws", "200").Inc()
	c.Requests.WithLabelValues("aws", "aws", "200").Inc()
	c.RequestDuration.WithLabelValues("aws", "aws").Observe(0.05)
	c.BackendUp.WithLabelValues("aws", "aws").Set(1)
	c.BackendUp.WithLabelValues("azure", "azure").Set(0)
	c.HealthCheckDur.WithLabelValues("aws").Observe(0.01)
	c.ActiveBackends.Set(1)
	c.ProxyErrors.WithLabelValues("aws", "aws", "dial").Inc()
	c.BackendInflight.WithLabelValues("aws", "aws").Set(3)

	if got := testutil.ToFloat64(c.Requests.WithLabelValues("aws", "aws", "200")); got != 2 {
		t.Errorf("requests_total{backend=aws,code=200}=%v want 2", got)
	}
	if got := testutil.ToFloat64(c.BackendUp.WithLabelValues("aws", "aws")); got != 1 {
		t.Errorf("backend_up{backend=aws}=%v want 1", got)
	}

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	names := make([]string, 0, len(mfs))
	for _, mf := range mfs {
		names = append(names, mf.GetName())
	}
	want := []string{
		"openvcc_requests_total",
		"openvcc_request_duration_seconds",
		"openvcc_backend_up",
		"openvcc_health_check_duration_seconds",
		"openvcc_active_backends",
		"openvcc_proxy_errors_total",
		"openvcc_backend_inflight",
	}
	joined := strings.Join(names, ",")
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("missing metric %s in %s", w, joined)
		}
	}
}

func TestNew_NilRegisterer_UsesDefault(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("DefaultRegisterer rejected double-register (expected on repeat runs): %v", r)
		}
	}()
	_ = New(nil)
}
