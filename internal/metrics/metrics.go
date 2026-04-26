package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "openvcc"

type Collectors struct {
	Requests          *prometheus.CounterVec
	RequestDuration   *prometheus.HistogramVec
	BackendUp         *prometheus.GaugeVec
	HealthCheckDur    *prometheus.HistogramVec
	ActiveBackends    prometheus.Gauge
	ProxyErrors       *prometheus.CounterVec
	BackendInflight   *prometheus.GaugeVec
	EgressBytes       *prometheus.CounterVec
	BackendLatencyEMA *prometheus.GaugeVec
}

func New(reg prometheus.Registerer) *Collectors {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	c := &Collectors{
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "requests_total",
			Help:      "Total proxied requests by backend, cloud, and HTTP status code.",
		}, []string{"backend", "cloud", "code"}),

		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "request_duration_seconds",
			Help:      "Proxied request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"backend", "cloud"}),

		BackendUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "backend_up",
			Help:      "1 if the backend is healthy, 0 otherwise.",
		}, []string{"backend", "cloud"}),

		HealthCheckDur: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "health_check_duration_seconds",
			Help:      "Active health check duration in seconds.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}, []string{"backend"}),

		ActiveBackends: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_backends",
			Help:      "Number of currently healthy backends.",
		}),

		ProxyErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "proxy_errors_total",
			Help:      "Total proxy-side errors (backend unreachable, transport error).",
		}, []string{"backend", "cloud", "kind"}),

		BackendInflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "backend_inflight",
			Help:      "In-flight requests per backend.",
		}, []string{"backend", "cloud"}),

		EgressBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "egress_bytes_total",
			Help:      "Bytes proxied outbound to backends, partitioned by cloud.",
		}, []string{"cloud"}),

		BackendLatencyEMA: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "backend_latency_ema_seconds",
			Help:      "Exponentially weighted moving average of request duration per backend.",
		}, []string{"backend", "cloud"}),
	}

	reg.MustRegister(
		c.Requests,
		c.RequestDuration,
		c.BackendUp,
		c.HealthCheckDur,
		c.ActiveBackends,
		c.ProxyErrors,
		c.BackendInflight,
		c.EgressBytes,
		c.BackendLatencyEMA,
	)
	return c
}
