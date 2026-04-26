# Roadmap

Loose, ordered. Things at the top are the next things we'd build.

## Near term

- **GCP module** under `infra/modules/gcp` matching the same input/output contract.
- **Latency-aware strategy** — passive RTT measurement, prefer the lower-latency backend with hysteresis.
- **Sticky-by-cloud routing** — once a client lands on a cloud, keep them there. Reduces cross-cloud egress.
- **Hot-reload of health-checker parameters** — recreate the checker when `health.*` changes, not just backends.
- **`dockertest` integration test** for `examples/notes` data layer (skips cleanly when Docker is absent).
- **Replace cached ReverseProxy when a backend's URL changes** under the same name.

## Medium term

- **Multi-instance engine** with gossip for shared health state, removing the SPOF and letting the admin API operate cluster-wide.
- **mTLS to backends** + **ACME** (Let's Encrypt) for the engine front.
- **Egress-cost-weighted routing** — per-cloud egress tables driving weights.
- **Private cross-cloud networking module** (AWS Transit Gateway + Azure VPN Gateway).
- **Web UI for `/admin`** — read-only first, then live edits.
- **OpenTelemetry traces** in addition to Prometheus metrics.

## Longer term

- **Hetzner**, **on-prem (Proxmox)**, and **bare-metal** provider modules.
- **Kubernetes deployment** as a Helm chart (after the VM story is solid; we're not chasing the k8s LB problem first).
- **Request hedging / retries / circuit breakers** behind a feature flag.
- **WAF integration** as a documented "what to put in front of Open VCC" pattern with examples.

## Things we're explicitly *not* doing

- Becoming a service mesh.
- Replacing CDN/WAF/DNS-level GSLB.
- Building a control plane.

If your use case needs these, a different tool is the right answer.

## How to influence the roadmap

Open an issue with the `enhancement` label and a description of the problem you're trying to solve. We don't track every "nice to have" here; the list above is a reading order, not a commitment.
