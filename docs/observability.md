# Observability

The engine exposes Prometheus metrics on `:9090/metrics` and emits structured JSON logs (slog) to stderr.

## Metrics

All metrics are namespaced `openvcc_`.

| Metric | Type | Labels | Notes |
|---|---|---|---|
| `openvcc_requests_total` | counter | backend, cloud, code | One increment per proxied request. |
| `openvcc_request_duration_seconds` | histogram | backend, cloud | Default Prom buckets. |
| `openvcc_backend_up` | gauge | backend, cloud | 1 healthy / 0 unhealthy. |
| `openvcc_active_backends` | gauge | — | Set after each health-check round. |
| `openvcc_health_check_duration_seconds` | histogram | backend | Each probe. |
| `openvcc_proxy_errors_total` | counter | backend, cloud, kind | Kinds: `timeout`, `canceled`, `transport`, `no_backend`. |
| `openvcc_backend_inflight` | gauge | backend, cloud | In-flight per backend. |

Standard `process_*` and `go_*` collectors are not registered; the registry is isolated so the surface stays predictable.

## Grafana dashboard

`deploy/grafana/dashboards/openvcc.json` is provisioned automatically by the compose stack. Panels:

- **Requests per second by cloud**
- **Active backends** (single stat)
- **Backend up** (timeseries, one line per backend)
- **p95 request duration by cloud**
- **Proxy errors per second by kind**

Import the JSON into your own Grafana via *Dashboards → Import → Upload JSON file*.

## Logs

```
{"time":"...","level":"INFO","msg":"listening","addr":":8080","component":"engine"}
{"time":"...","level":"WARN","msg":"backend unhealthy","backend":"aws","cloud":"aws","err":"status 500","component":"health"}
```

Level and format are set via `log.level` (`debug|info|warn|error`) and `log.format` (`json|text`). For development:

```yaml
log:
  level: debug
  format: text
```

## What to alert on

Reasonable starting alerts:

- `openvcc_active_backends < 1` for 5 minutes — total outage of the proxy path.
- `sum by (cloud) (rate(openvcc_proxy_errors_total[5m])) > 0.05` — sustained error rate.
- `histogram_quantile(0.99, sum by (cloud, le) (rate(openvcc_request_duration_seconds_bucket[5m]))) > 1` — latency spike.
