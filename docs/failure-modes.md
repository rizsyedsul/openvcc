# Failure modes

What breaks under load, partial outages, and operator mistakes — and how Open VCC behaves in each case.

## SPOF: the engine itself

A single engine instance is a single point of failure for every request. The intended production deployment runs **two or more engine instances** behind a health-checked DNS RR or a small L4 LB (e.g. Hetzner load balancer, Cloudflare LB, or one global anycast IP per cloud). Multi-instance shared state (gossip, consensus on health) is roadmap; today each instance keeps its own health view, which is fine because health checks are idempotent.

## Total backend outage

If every backend is unhealthy, the proxy returns `502 upstream error` and increments `openvcc_proxy_errors_total{kind="no_backend"}`. The engine itself stays up so observability and the admin API remain functional.

## Single-cloud outage

If `aws` goes dark:

- Health checks for `aws` fail → after `unhealthy_threshold` consecutive failures, that backend's `openvcc_backend_up` drops to 0.
- The pool stops returning it; the strategy distributes 100% of traffic to remaining healthy backends.
- When `aws` recovers and survives `healthy_threshold` successes, it returns to rotation. No drain, no preflight.

Failover lag is approximately `interval × unhealthy_threshold`, so with the default `5s × 2 = 10s`. Tune down for tighter SLOs at the cost of more health-check load.

## Partial outage (some endpoints fail)

`/healthz` is a coarse signal. If the app returns 200 on `/healthz` but 500s on `/api`, the engine has no way to know. Two mitigations:

1. Make `/healthz` *meaningful* (the `notes` example checks DB reachability before returning 200).
2. Watch `openvcc_requests_total{code=~"5.."}` and alert on sustained 5xx ratios.

## Egress costs

Cross-cloud routing isn't free. With 50/50 traffic between AWS and Azure:

- Half the request bodies traverse the engine→Azure path (egress from wherever the engine runs).
- Stateful workloads with cross-cloud Raft replicate writes both ways.

Minimize this by running the engine close to your write traffic and keeping reads sticky to the local cloud (sticky-by-cloud strategy is roadmap). Document your expected cost before you turn this on at scale.

## Misconfiguration

- Empty bearer token + admin listener enabled → admin returns 403 (fail-closed).
- Reload sees a parse/validate error → returns 500 with the error text and keeps running on the previous config.
- Bad backend URL → caught by `openvcc engine validate` before serve.

## Hot-reload edge cases

- A backend whose name is reused with a different URL: the per-backend ReverseProxy is **not** reused — it's evicted via `proxy.Forget` only when the name disappears from the new config. If you change the URL but keep the name, restart instead. This is a v0.1 limitation worth knowing.
- Strategy swap is atomic via `Pool.Replace`; in-flight requests keep going to whatever backend they were sent to.

## What we don't try to handle

Out of scope for v0.1: brownouts, slow-loris detection, request hedging, retries with backoff, circuit breakers. The engine is a load balancer, not a service mesh. Put one in front of the workload (or a real service mesh inside it) if you need those.
