# `infra/examples/ha`

Full HA topology in one `tofu apply`:

```
                      DNS (round-robin + health checks)
                           │              │
                ┌──────────▼──┐      ┌────▼─────────┐
                │ engine-aws  │      │ engine-azure │     :80 / :8081 / :9090
                │ openvcc     │      │ openvcc      │
                └────┬───┬────┘      └──┬───┬───────┘
                     │   │              │   │
                     ▼   ▼              ▼   ▼
                ┌──────────┐      ┌──────────┐
                │ app-aws  │      │ app-azure│             :8000
                └──────────┘      └──────────┘
```

Four VMs total: app + engine in each cloud. Each engine runs the openvcc
container in front of *both* app VMs, so a single-engine outage doesn't
take traffic offline as long as DNS still routes to a live engine.

## Quickstart

```sh
cp terraform.tfvars.example terraform.tfvars
# edit ssh_public_key, ingress_cidrs, admin_token

tofu init
tofu apply
```

Outputs include the four public IPs and a `next_steps` block walking
through DNS configuration and a chaos verification run.

## What this *doesn't* do

- **DNS provisioning.** Terraform can do this (`aws_route53_record`,
  `azurerm_dns_a_record`, etc.) but it ties the example to one DNS
  provider; better to leave it to the operator and document the
  intent.
- **Private cross-cloud networking.** Engine VMs target the app VMs
  over public IPs. Pair with the (future) `infra/modules/network-peering`
  module for VPC peering / VPN.
- **Single-tenant TLS certs.** The engine container exposes plain HTTP
  on `:80`; for HTTPS, add `proxy_tls` to the engine's openvcc.yaml
  and mount the cert/key files into `/etc/openvcc`.
