# Open VCC

**Open-source Virtual Cloud Connector for VM workloads.**

Open VCC gives you a working AWS + Azure deployment in one command: opinionated OpenTofu modules with a matched I/O contract, a custom Go engine that load-balances across clouds with first-class cloud awareness, and reference apps (stateless and stateful) so you can prove the path end-to-end before adopting it.

## Status

v0.1 in development. Not yet released.

## Why

HTTP load balancing across clouds is solved (Traefik, Envoy, Cloudflare LB). Kubernetes multi-cluster is solved (LoxiLB, Submariner). The painful gap is **non-Kubernetes VM workloads** — every cloud's LB, security, and IaC story is different, so "lift-and-shift to multi-cloud" turns into a months-long project. Open VCC is a batteries-included starter kit for that case.

## Quickstart (local, ~30s)

```sh
make compose-up
curl -i localhost:8080/   # response carries X-Backend / X-Cloud headers
```

## Quickstart (real cloud, ~10min)

See `docs/quickstart-cloud.md`. With AWS + Azure credentials and a `terraform.tfvars`:

```sh
cd infra/examples/stateful
tofu init && tofu apply
openvcc engine serve --config=./openvcc.yaml
```

## Architecture

```
client ──► openvcc engine ──┬──► AWS VM  (app + crdb)
                            └──► Azure VM (app + crdb)
```

The engine runs `:8080` (proxy), `:9090` (Prometheus metrics), `:8081` (admin API, bearer-auth). CockroachDB joins across clouds via Raft for the stateful example.

Full details in `docs/architecture.md`.

## How it compares

| | Open VCC | Traefik / Envoy | Cloudflare LB | Cloud-native LBs |
|---|---|---|---|---|
| Multi-cloud LB | ✅ | ⚠ self-build | ✅ | ❌ |
| IaC included (AWS+Azure) | ✅ | ❌ | ❌ | ❌ |
| Reference VM apps | ✅ | ❌ | ❌ | ❌ |
| Open source / self-host | ✅ | ✅ | ❌ | ❌ |
| Cloud-aware routing primitives | ✅ | plugin | ❌ | ❌ |

Honest scope: Open VCC does **not** replace a CDN, WAF, or DNS-level GSLB. Put one of those in front for production.

## Documentation

- `docs/architecture.md` — components and data flow
- `docs/quickstart-local.md` — docker compose path
- `docs/quickstart-cloud.md` — `tofu apply` path
- `docs/engine-config.md` — `openvcc.yaml` schema
- `docs/observability.md` — metrics + Grafana dashboard
- `docs/failure-modes.md` — SPOF, egress costs, partial outages
- `docs/roadmap.md` — what's next

## Contributing

See `CONTRIBUTING.md`. Bug reports and feature ideas go in GitHub Issues. AI coding agents working in this repo should read `instructions.md` first.

## License

Not yet set. To be added before v0.1.0 release.
