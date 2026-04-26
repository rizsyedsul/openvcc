# Changelog

All notable changes to Open VCC are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project will adhere to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) once it reaches v1.0.

## [Unreleased]

### Added
- Project scaffold: repository layout, build tooling, contributor docs.
- `instructions.md` for AI coding agents working in this repo.
- Engine: pool, strategies (round_robin, weighted_round_robin, least_connections, random), active health checker, reverse proxy with X-Backend / X-Cloud headers, bearer-auth admin API, Prometheus metrics, lifecycle orchestration, Cobra CLI.
- Reference apps: stateless `echo` and CRDB-backed `notes`.
- IaC: `infra/modules/{aws,azure}` with a shared input/output contract, `infra/examples/{stateless,stateful}` driving local engine setups.
- Local demo via `deploy/compose.yaml` plus Prometheus + Grafana dashboards and a smoke test.
- CI: lint + race-tested unit tests + compose smoke; OpenTofu fmt + validate; release pipeline (GoReleaser, multi-arch binaries + Docker images, SBOM, cosign); CodeQL; dependabot; issue/PR templates.
- TLS: per-backend `tls` block (mTLS, CA pinning, SNI), front TLS listener (`listen.proxy_tls`) with optional client-cert verification.
- Cost-aware routing: `internal/cost.Accountant` + `cost_bounded_least_latency` strategy honouring `cloud_egress_budget`, with `openvcc_egress_bytes_total` and `openvcc_backend_latency_ema_seconds` metrics.
- Chaos verification: `internal/chaos` + `openvcc chaos run` subcommand that drives synthetic failover via `PUT /admin/backends/{name}/health` and emits a JSON pass/fail report.
- HA topology IaC: `infra/examples/ha` brings up app + engine VMs in both clouds in a single `tofu apply`.
- Sticky-by-cloud routing: `sticky_by_cloud` strategy + `sticky:` config block (cookie / header / remote_addr identifiers) wraps any base strategy and pins clients to one cloud once they land there.
- Workload-typed backends: `Backend.kind` label + `kind_routing:` config block; writes are restricted to backends whose kind appears in `write_kinds` (default `["writeable"]`), reads default to any kind. Composes with sticky.
- OpenTelemetry traces: `tracing:` config block; `internal/tracing` exports OTLP/HTTP; the proxy emits `openvcc.proxy` spans with backend attributes and propagates W3C trace context to backends.
- Pre-canned Prometheus alerts: `deploy/prometheus/alerts.yaml` covers no-active-backends, single-cloud-only, error rate, 5xx ratio, p99 latency, and egress-budget exhaustion. New `openvcc-slo.json` Grafana dashboard with availability, p99, error budget burn, and per-cloud egress.
- `openvcc chaos schedule` subcommand: cron-style runner that rotates failure across clouds and archives each `Report` to a directory via the new `chaos.Sink` interface (`WriterSink`, `FileSink`).
- ACME / Let's Encrypt for the front TLS listener: `acme:` config block uses `golang.org/x/crypto/acme/autocert` for on-demand cert issuance and rotation; HTTP-01 challenges are served from the plain HTTP listener.
