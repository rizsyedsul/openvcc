# Engine configuration

The engine is configured by a single YAML file (`openvcc.yaml`). Unknown keys are rejected so typos surface as validation errors instead of silent drift.

## Full schema

```yaml
listen:
  proxy: :8080            # required (also serves HTTP-01 ACME challenges)
  proxy_tls: :8443        # optional; HTTPS front
  metrics: :9090          # default :9090
  admin:   :8081          # default :8081; set "" to disable

admin:
  bearer_token_env: OPENVCC_ADMIN_TOKEN

backends:                  # required, at least one
  - name: aws-us-east      # unique
    url: http://10.0.1.10:8000   # http or https with a host
    cloud: aws             # free-form label
    region: us-east-1
    weight: 1
    kind: writeable        # optional: writeable | read_only | cache | <user>
    tls:                   # optional: per-backend mTLS / CA pinning
      cert_file: /etc/openvcc/clients/aws.crt
      key_file:  /etc/openvcc/clients/aws.key
      ca_file:   /etc/openvcc/ca.pem
      server_name: aws-backend.internal

health:
  interval: 5s
  timeout:  2s
  path:     /healthz
  unhealthy_threshold: 2
  healthy_threshold:   2

strategy: weighted_round_robin
# round_robin | weighted_round_robin | least_connections | random
# | cost_bounded_least_latency | sticky_by_cloud

# Wraps the chosen base strategy with session affinity.
sticky:
  hash: cookie:openvcc_cloud   # cookie:NAME | header:NAME | remote_addr
  fallback_strategy: cost_bounded_least_latency
  cookie_ttl: 24h

# Filters backends by kind based on the request method.
kind_routing:
  write_methods: [POST, PUT, DELETE, PATCH]   # defaults shown
  write_kinds:   [writeable]                  # default
  read_kinds:    []                           # empty = any

# Cost-aware routing inputs.
cost:
  egress_prices_per_gb:
    aws: 0.09
    azure: 0.087
  cloud_egress_budget:
    aws:   { window: 1m, max_gb: 5 }
    azure: { window: 1m, max_gb: 5 }

proxy:
  request_timeout: 30s
  max_idle_conns_per_host: 64

# Static cert for the HTTPS front; ignored when `acme:` is set.
proxy_tls:
  cert_file: /etc/openvcc/server.crt
  key_file:  /etc/openvcc/server.key
  client_ca_file: /etc/openvcc/clients/ca.pem    # optional mTLS

# Automatic HTTPS certificates via Let's Encrypt (or any ACME CA).
acme:
  domains: [api.example.com]
  email: ops@example.com
  cache_dir: /var/lib/openvcc/acme   # default
  staging: false                     # use Let's Encrypt staging while testing
  directory_url: ""                  # override for a non-LE CA

# OpenTelemetry traces (OTLP/HTTP).
tracing:
  endpoint: http://otel-collector.observability.svc.cluster.local:4318
  service_name: openvcc        # default
  sampler_ratio: 1.0           # default; in [0,1]
  insecure: true               # set to true for plain HTTP collectors
  headers: { X-Tenant: api }   # optional auth headers

log:
  level:  info             # debug | info | warn | error
  format: json             # json | text
```

## Validation

- `openvcc engine validate --config=openvcc.yaml` exits non-zero with the joined error list on any problem.
- Required: `backends` (≥1, unique names, http(s) URL with a host); `listen.proxy`.
- `health.timeout <= health.interval`.
- Weights must be `>= 0`.

## Hot reload

`POST /admin/reload` re-reads the config file and atomically swaps the backend list and strategy. Restart is required to change listener addresses, log level, or health-checker parameters.
