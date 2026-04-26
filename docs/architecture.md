# Architecture

Open VCC is one engine binary plus opinionated IaC plus reference apps. Everything else flows from those three pieces.

## Components

```
                client
                  │
                  ▼
     ┌──────────────────────────┐
     │  openvcc engine (Go)     │
     │                          │
     │  ┌────────────────────┐  │       :8080  proxy
     │  │ proxy (httputil)   │  │       :9090  /metrics, /healthz
     │  │  + X-Backend       │  │       :8081  /admin (bearer-auth)
     │  │  + X-Cloud         │  │
     │  └─────────┬──────────┘  │
     │            │             │
     │  ┌─────────▼──────────┐  │
     │  │ pool + strategy    │  │
     │  │  (WRR/RR/LC/Rand)  │  │
     │  └─────────┬──────────┘  │
     │            │             │
     │  ┌─────────▼──────────┐  │
     │  │ health checker     │  │
     │  │  (active GET /hz)  │  │
     │  └────────────────────┘  │
     └────────────┬─────────────┘
                  │
       ┌──────────┴──────────┐
       ▼                     ▼
 ┌────────────┐         ┌────────────┐
 │ AWS VM     │         │ Azure VM   │
 │ app :8000  │◀──CRDB──▶│ app :8000 │
 └────────────┘  Raft    └────────────┘
```

## Process model

The engine is a single Go process. It owns three listeners:

| Port | Purpose | Notes |
|---:|---|---|
| 8080 | Proxy | Where client traffic lands. |
| 9090 | Metrics + `/healthz` | Prometheus scrapes here. |
| 8081 | Admin | Optional. Disabled by setting `listen.admin: ""`. |

A single goroutine owns the active health checker; it polls every backend's health URL on a ticker and updates the pool's healthy set with hysteresis (configurable thresholds prevent flapping).

The proxy path is one `httputil.ReverseProxy` per backend, lazily created on first request and cached. On `Reload`, removed backends have their cached proxies evicted so a new backend with the same name doesn't reuse the old `Transport`.

## Data flow for one request

1. Client request hits `:8080`.
2. Proxy handler asks the pool to `Pick` a healthy backend via the configured strategy.
3. The chosen backend's inflight counter increments; the per-backend `ReverseProxy` forwards the request.
4. Response is wrapped with `X-Backend: <name>` and `X-Cloud: <cloud>` headers.
5. `openvcc_requests_total` and `openvcc_request_duration_seconds` are recorded; the inflight counter decrements.

Errors (no backend healthy, transport failure, timeout) yield 502 with `upstream error` and increment `openvcc_proxy_errors_total{kind=...}`.

## Hot reload

`POST /admin/reload` (bearer-authed) re-reads the config file and atomically swaps the backend list and strategy via `pool.Replace`. Listener addresses, log level, and health-checker parameters are read at startup; changing them needs a restart.

## What lives outside the engine

- **OpenTofu modules** under `infra/modules/{aws,azure}` — symmetric I/O.
- **Reference apps** under `examples/{echo,notes}`.
- **Local demo** under `deploy/compose.yaml` (Prometheus + Grafana included).
